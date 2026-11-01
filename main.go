package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/TogetherWeOwn/cliproxy-provider-kit/cabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const provider = "zai-coding-plan"

func main() {}
func init() {
	if cabi.Export(makePlugin) != nil {
		panic("cannot install provider factory")
	}
}
func authProvider() (kit.APIKeyAuth, error) {
	a := kit.APIKeyAuth{Provider: provider, Validate: func(m map[string]any) error {
		if r, ok := m["region"]; ok {
			s, ok := r.(string)
			if !ok || (s != "" && s != "global" && s != "cn") {
				return &pluginabi.Error{Code: "invalid_auth", Message: "region must be global or cn", HTTPStatus: 400}
			}
		}
		return nil
	}}
	// The login page offers the optional region field; Parse is the full
	// ParseAuth pipeline, so a submitted key faces the same validation as an
	// uploaded auth file.
	login, err := kit.NewAPIKeyLogin(kit.APIKeyLoginConfig{Provider: provider, Parse: a.ParseAuth, Fields: []kit.APIKeyLoginField{
		{Name: "region", Label: "Region", Description: "Z.ai region: global or cn. Leave blank for global.", Required: false},
	}})
	if err != nil {
		return kit.APIKeyAuth{}, err
	}
	a.Login = login
	return a, nil
}

// newQuota polls through client; the kit routes each poll through the auth's
// proxy_url, else the host's global proxy-url.
func newQuota(client quotaDoer) *kit.CachedQuota {
	return kit.NewCachedQuota(provider, "Z.ai Coding Plan", func(ctx context.Context, r pluginapi.QuotaFetchRequest) (pluginapi.QuotaFetchResponse, error) {
		key, err := kit.APIKey(r.StorageJSON, r.Metadata)
		if err != nil {
			return pluginapi.QuotaFetchResponse{}, err
		}
		base, err := authBase(r.StorageJSON, r.Metadata)
		if err != nil {
			return pluginapi.QuotaFetchResponse{}, err
		}
		return fetchQuota(ctx, base, key, client)
	})
}
func makePlugin(host kit.Caller) (*kit.Runtime, error) {
	auth, err := authProvider()
	if err != nil {
		return nil, err
	}
	client := kit.NewQuotaHTTPClient(nil)
	quota := newQuota(client)
	ex := kit.Passthrough{Provider: provider, BuildRequest: buildRequest, PrepareHTTP: prepareHTTP, ClassifyError: classifyResponse}
	return kit.New(kit.Definition{Configure: configureBase, Metadata: pluginapi.Metadata{Name: provider, Version: "0.2.1", Author: "TogetherWeOwn", GitHubRepository: "https://github.com/TogetherWeOwn/cliproxy-provider-zai-coding-plan"}, ModelNamespace: "zai", Capabilities: pluginapi.Capabilities{AuthProvider: auth, ManagementAPI: auth.Login, ModelRegistrar: catalog{}, ModelProvider: catalog{}, Executor: ex, ExecutorModelScope: pluginapi.ExecutorModelScopeBoth, ExecutorInputFormats: []string{"claude"}, ExecutorOutputFormats: []string{"claude"}, QuotaProvider: quota}, OnShutdown: func(context.Context) error { client.CloseIdleConnections(); return nil }}, host)
}

type catalog struct{}

func models() []pluginapi.ModelInfo {
	return []pluginapi.ModelInfo{{ID: "zai/glm-5.3", Object: "model", OwnedBy: provider, DisplayName: "GLM-5.3"}, {ID: "zai/glm-5.3-flash", Object: "model", OwnedBy: provider, DisplayName: "GLM-5.3 Flash"}}
}
func (catalog) RegisterModels(context.Context, pluginapi.ModelRegistrationRequest) (pluginapi.ModelRegistrationResponse, error) {
	return pluginapi.ModelRegistrationResponse{Provider: provider, Models: models()}, nil
}
func (catalog) StaticModels(context.Context, pluginapi.StaticModelRequest) (pluginapi.ModelResponse, error) {
	return pluginapi.ModelResponse{Provider: provider, Models: models()}, nil
}
func (catalog) ModelsForAuth(context.Context, pluginapi.AuthModelRequest) (pluginapi.ModelResponse, error) {
	return pluginapi.ModelResponse{Provider: provider, Models: models()}, nil
}
func authBase(storage []byte, metadata map[string]any) (string, error) {
	m, err := kit.AuthFields(storage, metadata)
	if err != nil {
		return "", err
	}
	if m["region"] == nil || m["region"] == "" || m["region"] == "global" {
		return selectedBase("https://api.z.ai"), nil
	}
	if m["region"] == "cn" {
		return selectedBase("https://open.bigmodel.cn"), nil
	}
	return "", &pluginabi.Error{Code: "invalid_auth", Message: "invalid region", HTTPStatus: 400}
}
func buildRequest(_ context.Context, r pluginapi.ExecutorRequest) (pluginapi.HTTPRequest, error) {
	key, err := kit.APIKey(r.StorageJSON, r.AuthMetadata)
	if err != nil {
		return pluginapi.HTTPRequest{}, err
	}
	base, err := authBase(r.StorageJSON, r.AuthMetadata)
	if err != nil {
		return pluginapi.HTTPRequest{}, err
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(r.Payload, &body) != nil || body == nil {
		return pluginapi.HTTPRequest{}, &pluginabi.Error{Code: "invalid_request", Message: "invalid request payload", HTTPStatus: 400}
	}
	var model string
	if json.Unmarshal(body["model"], &model) != nil || !strings.HasPrefix(model, "zai/") || len(model) <= len("zai/") {
		return pluginapi.HTTPRequest{}, &pluginabi.Error{Code: "invalid_model", Message: "expected zai/ model", HTTPStatus: 400}
	}
	body["model"], _ = json.Marshal(strings.TrimPrefix(model, "zai/"))
	payload, err := json.Marshal(body)
	if err != nil {
		return pluginapi.HTTPRequest{}, err
	}
	headers := http.Header{"Authorization": {"Bearer " + key}, "Content-Type": {"application/json"}, "Anthropic-Version": {"2023-06-01"}}
	if v := r.Headers.Get("Anthropic-Version"); v != "" {
		headers.Set("Anthropic-Version", v)
	}
	if v := r.Headers.Get("Anthropic-Beta"); v != "" {
		headers.Set("Anthropic-Beta", v)
	}
	return pluginapi.HTTPRequest{Method: http.MethodPost, URL: base + "/api/anthropic/v1/messages", Headers: headers, Body: payload}, nil
}
func prepareHTTP(ctx context.Context, r pluginapi.ExecutorHTTPRequest) (pluginapi.HTTPRequest, error) {
	// Apply the same prefix stripping and credential/header allowlist as Execute.
	// Never forward caller-supplied URLs, methods or credentials.
	return buildRequest(ctx, pluginapi.ExecutorRequest{Payload: r.Body, StorageJSON: r.StorageJSON, AuthMetadata: r.Metadata, Headers: r.Headers})
}
