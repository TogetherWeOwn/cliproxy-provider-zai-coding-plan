//go:build cgo && (linux || darwin || freebsd)

package hostcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	coreexec "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	translator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"gopkg.in/yaml.v3"
)

type usageCapture struct{ records chan coreusage.Record }

func (c usageCapture) HandleUsage(_ context.Context, r coreusage.Record) {
	if c.records != nil && r.Provider == "zai-coding-plan" {
		select {
		case c.records <- r:
		default:
		}
	}
}

// ApplyConfig invokes the pinned host's real dlopen/RPC loader and adapters.
// No loader, executor transport, translator, or native selector is reimplemented.
func TestRealHostLoader(t *testing.T) {
	var quotaCalls, accountACalls atomic.Int32
	var streamFinished atomic.Bool
	continueStream := make(chan struct{})
	var releaseOnce sync.Once
	releaseStream := func() { releaseOnce.Do(func() { close(continueStream) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/monitor/usage/quota/limit":
			quotaCalls.Add(1)
			if r.Method != "GET" || r.Header.Get("Authorization") != "example-key" {
				t.Error("invalid quota method or raw-key authorization")
			}
			io.WriteString(w, `{"code":200,"success":true,"data":{"level":"pro","planName":"Pro","limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":25,"nextResetTime":null},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":200,"currentValue":60,"nextResetTime":1893456000000}]}}`)
		case "/api/anthropic/v1/messages":
			if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer example-key") || r.Header.Get("Anthropic-Version") != "2023-06-01" || r.Header.Get("X-Api-Key") != "" {
				t.Error("invalid inference method or headers")
			}
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || (body["model"] != "glm-5.3" && body["model"] != "glm-5.3-flash") {
				t.Errorf("unstripped or invalid upstream model %v", body["model"])
				w.WriteHeader(400)
				return
			}
			code, _ := body["error_code"].(float64)
			if body["failover"] == true && r.Header.Get("Authorization") == "Bearer example-key-a" {
				accountACalls.Add(1)
				code = 1317
			}
			if code != 0 {
				w.WriteHeader(429)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": int(code), "message": "Resets at " + time.Now().UTC().Add(120*time.Second).Format(time.RFC3339)}})
				return
			}
			if body["stream"] == true {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"glm-5.3\",\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-continueStream:
				case <-r.Context().Done():
					return
				}
				io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				streamFinished.Store(true)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"msg","type":"message","role":"assistant","model":"glm-5.3","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)
		default:
			t.Errorf("unexpected upstream path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	defer releaseStream()

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
	if !host.PluginLoaded("zai-coding-plan") || !host.PluginRegistered("zai-coding-plan") || !host.HasAuthProvider("zai-coding-plan") || !host.HasQuotaProvider("zai-coding-plan") || host.HasScheduler() {
		t.Fatalf("registration %+v", host.RegisteredPlugins())
	}
	infos := host.RegisteredPlugins()
	if len(infos) != 1 || !infos[0].SupportsQuota || infos[0].Metadata.Version != "0.1.3" {
		t.Fatalf("quota capability/metadata %+v", infos)
	}
	parse := func(id, key string) *coreauth.Auth {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"type": "zai-coding-plan", "api_key": key, "id": id, "label": "Example"})
		auth, handled, err := host.ParseAuth(ctx, pluginapi.AuthParseRequest{Provider: "zai-coding-plan", FileName: id + ".json", RawJSON: raw})
		if err != nil || !handled || auth == nil || auth.Provider != "zai-coding-plan" {
			t.Fatalf("auth %v %v", handled, err)
		}
		return auth
	}
	auth := parse("example", "example-key")
	file := filepath.Join(cfg.AuthDir, "example.json")
	if err = auth.Storage.SaveTokenToFile(file); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if json.Unmarshal(saved, &fields) != nil || fields["api_key"] != "example-key" || fields["type"] != "zai-coding-plan" || fields["label"] != "Example" {
		t.Fatal("host auth-storage merge failed")
	}
	quotaRequest := pluginapi.QuotaFetchRequest{AuthID: auth.ID, Provider: "zai-coding-plan", StorageJSON: saved}
	quota, handled, err := host.FetchQuota(ctx, quotaRequest)
	if err != nil || !handled || len(quota.Groups) != 1 || len(quota.Groups[0].Buckets) != 2 || quota.Groups[0].Buckets[0].RemainingFraction != .75 || quota.Groups[0].Buckets[1].RemainingFraction != .7 || quota.Subscription == nil || quota.Subscription.TierName != "Pro" {
		t.Fatalf("quota %+v %v %v", quota, handled, err)
	}

	reg := registry.GetGlobalRegistry()
	manager := coreauth.NewManager(nil, &coreauth.FillFirstSelector{}, nil)
	manager.SetRetryConfig(0, 0, 0)
	manager.SetConfig(cfg)
	host.RegisterModels(ctx, reg)
	host.RegisterExecutors(manager, reg)
	ex, ok := manager.Executor("zai-coding-plan")
	if !ok {
		t.Fatal("executor not registered from static model catalog")
	}
	modelResult := host.ModelsForAuth(ctx, auth)
	if !modelResult.Handled || modelResult.Err != nil || len(modelResult.Models) != 2 || modelResult.Models[0].ID != "zai/glm-5.3" || modelResult.Models[1].ID != "zai/glm-5.3-flash" {
		t.Fatalf("per-auth model catalog %+v", modelResult)
	}
	capture := usageCapture{records: make(chan coreusage.Record, 50)}
	coreusage.RegisterNamedPlugin("zai-hostcompat-capture", capture)
	defer coreusage.RegisterNamedPlugin("zai-hostcompat-capture", usageCapture{})
	req := coreexec.Request{Model: "zai/glm-5.3", Payload: []byte(`{"model":"zai/glm-5.3","messages":[{"role":"user","content":"hello"}],"max_tokens":8}`), Format: translator.FormatClaude}
	opts := coreexec.Options{SourceFormat: translator.FormatClaude, ResponseFormat: translator.FormatClaude}
	resp, err := ex.Execute(ctx, auth, req, opts)
	if err != nil || !strings.Contains(string(resp.Payload), `"input_tokens":3`) {
		t.Fatalf("execute %s %v", resp.Payload, err)
	}
	select {
	case record := <-capture.records:
		if record.Detail.InputTokens != 3 || record.Detail.OutputTokens != 2 || record.Detail.TotalTokens != 5 {
			t.Fatalf("host token accounting %+v", record.Detail)
		}
	case <-ctx.Done():
		t.Fatal("host did not publish usage")
	}
	req.Payload = []byte(`{"model":"zai/glm-5.3","stream":true,"messages":[{"role":"user","content":"hello"}],"max_tokens":8}`)
	streamOpts := opts
	streamOpts.Stream = true
	stream, err := ex.ExecuteStream(ctx, auth, req, streamOpts)
	if err != nil {
		t.Fatal(err)
	}
	var chunks strings.Builder
	select {
	case first, ok := <-stream.Chunks:
		if !ok || first.Err != nil || len(first.Payload) == 0 || streamFinished.Load() {
			t.Fatalf("stream buffered or missing first chunk: %+v", first)
		}
		chunks.Write(first.Payload)
		releaseStream()
	case <-ctx.Done():
		t.Fatal("no streaming chunk before upstream completion")
	}
	for c := range stream.Chunks {
		if c.Err != nil {
			t.Fatal(c.Err)
		}
		chunks.Write(c.Payload)
	}
	if !strings.Contains(chunks.String(), "message_stop") || !strings.Contains(chunks.String(), "hello") || !streamFinished.Load() {
		t.Fatalf("stream %s", chunks.String())
	}

	// Error hints must survive the C ABI and host RPC adapter for both paths.
	for _, code := range []int{1001, 1302, 1305, 1308, 1311, 1317} {
		r := req
		r.Payload = []byte(fmt.Sprintf(`{"model":"zai/glm-5.3-flash","error_code":%d}`, code))
		for _, streaming := range []bool{false, true} {
			if streaming {
				_, err = ex.ExecuteStream(ctx, auth, r, streamOpts)
			} else {
				_, err = ex.Execute(ctx, auth, r, opts)
			}
			status, ok := err.(interface{ StatusCode() int })
			wantStatus, minWait, maxWait := 429, 10*time.Second, 10*time.Second
			switch code {
			case 1001:
				wantStatus = 401
			case 1308, 1317:
				minWait, maxWait = 119*time.Second, 120*time.Second
			case 1311:
				minWait, maxWait = time.Hour, time.Hour
			}
			if !ok || status.StatusCode() != wantStatus {
				t.Fatalf("code %d stream %v: status %T %v", code, streaming, err, err)
			}
			if code != 1001 {
				retry, ok := err.(interface{ RetryAfter() *time.Duration })
				if !ok || retry.RetryAfter() == nil || *retry.RetryAfter() < minWait || *retry.RetryAfter() > maxWait {
					t.Fatalf("code %d stream %v: retry hint %T %v", code, streaming, err, err)
				}
			}
		}
	}

	// Native selection, not a plugin scheduler: exhaustion on A rotates to B.
	a := parse("example-a", "example-key-a")
	b := parse("example-b", "example-key-b")
	for _, credential := range []*coreauth.Auth{a, b} {
		reg.RegisterClient(credential.ID, credential.Provider, modelResult.Models)
		defer reg.UnregisterClient(credential.ID)
		if _, err = manager.Register(coreauth.WithSkipPersist(ctx), credential); err != nil {
			t.Fatal(err)
		}
	}
	req.Payload = []byte(`{"model":"zai/glm-5.3","failover":true,"messages":[{"role":"user","content":"hello"}],"max_tokens":8}`)
	for i := 0; i < 2; i++ {
		resp, err = manager.Execute(ctx, []string{"zai-coding-plan"}, req, opts)
		if err != nil || !strings.Contains(string(resp.Payload), "hello") {
			t.Fatalf("native failover %s: %v", resp.Payload, err)
		}
	}
	if accountACalls.Load() != 1 {
		t.Fatalf("exhausted account retried %d times", accountACalls.Load())
	}
	cooled, _ := manager.GetByID(a.ID)
	if state := cooled.ModelStates["zai/glm-5.3"]; state == nil || !state.Quota.Exceeded || time.Until(state.NextRetryAfter) < 115*time.Second {
		t.Fatalf("native exhaustion cooldown %+v", cooled.ModelStates)
	}

	// The v8.0.48/v8.0.49 RPC adapter retains status/hints, but not the newer native
	// TransientRateLimitError interface. Its generic 429 floor is 30 seconds.
	transientManager := coreauth.NewManager(nil, nil, nil)
	transientManager.SetConfig(cfg)
	host.RegisterExecutors(transientManager, reg)
	transient := parse("example-transient", "example-key-t")
	reg.RegisterClient(transient.ID, transient.Provider, modelResult.Models)
	defer reg.UnregisterClient(transient.ID)
	if _, err = transientManager.Register(coreauth.WithSkipPersist(ctx), transient); err != nil {
		t.Fatal(err)
	}
	req.Payload = []byte(`{"model":"zai/glm-5.3","error_code":1305}`)
	if _, err = transientManager.Execute(ctx, []string{"zai-coding-plan"}, req, opts); err == nil {
		t.Fatal("transient error hidden")
	}
	cooled, _ = transientManager.GetByID(transient.ID)
	if state := cooled.ModelStates["zai/glm-5.3"]; state == nil || state.NextRetryAfter.IsZero() || time.Until(state.NextRetryAfter) > 31*time.Second {
		t.Fatalf("transient caused long cooldown %+v", cooled.ModelStates)
	}
	if _, _, err = host.FetchQuota(ctx, quotaRequest); err != nil || quotaCalls.Load() != 1 {
		t.Fatalf("quota display cache or inference independence: calls=%d err=%v", quotaCalls.Load(), err)
	}
}
