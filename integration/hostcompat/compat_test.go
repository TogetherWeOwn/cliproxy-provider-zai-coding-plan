//go:build cgo && (linux || darwin || freebsd)

package hostcompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	coreexec "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	translator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"gopkg.in/yaml.v3"
)

type executors map[string]coreauth.ProviderExecutor

func (m executors) Executor(p string) (coreauth.ProviderExecutor, bool) { e, ok := m[p]; return e, ok }
func (m executors) RegisterExecutor(e coreauth.ProviderExecutor)        { m[e.Identifier()] = e }
func (m executors) UnregisterExecutor(p string)                         { delete(m, p) }

type models struct{}

func (models) RegisterClient(string, string, []*registry.ModelInfo) {}
func (models) UnregisterClient(string)                              {}
func (models) GetModelProviders(string) []string                    { return nil }
func TestRealHostLoader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer example-key" {
			t.Error("invalid authorization")
		}
		switch r.URL.Path {
		case "/api/monitor/usage/quota/limit":
			io.WriteString(w, `{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":25}]}}`)
		case "/api/anthropic/v1/messages":
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "glm-5.3" {
				t.Errorf("upstream model %v", body["model"])
			}
			if body["stream"] == true {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"glm-5.3\",\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				return
			}
			io.WriteString(w, `{"id":"msg","type":"message","role":"assistant","model":"glm-5.3","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":3,"output_tokens":2}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	arch := filepath.Join(dir, runtime.GOOS, runtime.GOARCH)
	if err = os.MkdirAll(arch, 0755); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(arch, "zai-coding-plan.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(arch, "zai-coding-plan.dylib")
	}
	build := exec.Command("go", "build", "-tags=hostcompat", "-buildmode=c-shared", "-o", lib, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	enabled := true
	var raw yaml.Node
	if err = yaml.Unmarshal([]byte("enabled: true\nhostcompat_base: "+server.URL+"\n"), &raw); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AuthDir: t.TempDir(), Plugins: config.PluginsConfig{Enabled: true, Dir: dir, Configs: map[string]config.PluginInstanceConfig{"zai-coding-plan": {Enabled: &enabled, Raw: *raw.Content[0]}}}}
	host := pluginhost.New()
	defer host.ShutdownAll()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	host.ApplyConfig(ctx, cfg)
	if !host.PluginRegistered("zai-coding-plan") || !host.HasAuthProvider("zai-coding-plan") || !host.HasQuotaProvider("zai-coding-plan") {
		t.Fatalf("registration %+v", host.RegisteredPlugins())
	}
	auth, handled, err := host.ParseAuth(ctx, pluginapi.AuthParseRequest{Provider: "zai-coding-plan", FileName: "example.json", RawJSON: []byte(`{"type":"zai-coding-plan","api_key":"example-key"}`)})
	if err != nil || !handled || auth == nil {
		t.Fatalf("auth %v %v", handled, err)
	}
	file := filepath.Join(cfg.AuthDir, "example.json")
	if err = auth.Storage.SaveTokenToFile(file); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	quota, handled, err := host.FetchQuota(ctx, pluginapi.QuotaFetchRequest{AuthID: auth.ID, Provider: "zai-coding-plan", StorageJSON: saved})
	if err != nil || !handled || len(quota.Groups) != 1 || quota.Groups[0].Buckets[0].RemainingFraction != .75 {
		t.Fatalf("quota %+v %v %v", quota, handled, err)
	}
	m := executors{}
	host.RegisterModels(ctx, models{})
	host.RegisterExecutors(m, models{})
	ex, ok := m["zai-coding-plan"]
	if !ok {
		t.Fatal("executor not registered")
	}
	req := coreexec.Request{Model: "zai/glm-5.3", Payload: []byte(`{"model":"zai/glm-5.3","messages":[{"role":"user","content":"hello"}],"max_tokens":8}`), Format: translator.FormatClaude}
	opts := coreexec.Options{SourceFormat: translator.FormatClaude, ResponseFormat: translator.FormatClaude}
	resp, err := ex.Execute(ctx, auth, req, opts)
	if err != nil || !strings.Contains(string(resp.Payload), `"input_tokens":3`) {
		t.Fatalf("execute %s %v", resp.Payload, err)
	}
	req.Payload = []byte(`{"model":"zai/glm-5.3","stream":true,"messages":[{"role":"user","content":"hello"}],"max_tokens":8}`)
	opts.Stream = true
	stream, err := ex.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		t.Fatal(err)
	}
	var chunks strings.Builder
	for c := range stream.Chunks {
		if c.Err != nil {
			t.Fatal(c.Err)
		}
		chunks.Write(c.Payload)
	}
	if !strings.Contains(chunks.String(), "message_stop") {
		t.Fatalf("stream %s", chunks.String())
	}
}
