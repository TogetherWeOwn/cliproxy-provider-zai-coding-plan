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

const displayName = "Z.ai Coding Plan"

func main() {}
func init() {
	if cabi.Export(makePlugin) != nil {
		panic("cannot install provider factory")
	}
}

// authProvider documents the auth-file shape (region values) beside the JSON
// shape in the README. The provider is API-key only: it registers no
// AuthProvider, so the management UI shows no login card and auth files load
// through the host's generic file path. Re-register the returned
// value as the plugin AuthProvider to restore upload-time validation.
func authProvider() kit.APIKeyAuth {
	return kit.APIKeyAuth{Provider: provider, Validate: func(m map[string]any) error {
		if r, ok := m["region"]; ok {
			s, ok := r.(string)
			if !ok || (s != "" && s != "global" && s != "cn") {
				return &pluginabi.Error{Code: "invalid_auth", Message: "region must be global or cn", HTTPStatus: 400}
			}
		}
		return nil
	}}
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
	// API-key only: no AuthProvider and no ManagementAPI login
	// routes, so the management UI shows no login card. Auth files load
	// through the host's generic file path.
	client := kit.NewQuotaHTTPClient(nil)
	quota := newQuota(client)
	ex := kit.Passthrough{Provider: provider, BuildRequest: buildRequest, PrepareHTTP: prepareHTTP, ClassifyError: classifyResponse}
	return kit.New(kit.Definition{Configure: configureBase, Metadata: pluginapi.Metadata{Name: displayName, Version: "0.2.4", Author: "TogetherWeOwn", GitHubRepository: "https://github.com/TogetherWeOwn/cliproxy-provider-zai-coding-plan", Logo: "https://www.google.com/s2/favicons?domain=z.ai"}, ModelNamespace: "zai", Capabilities: pluginapi.Capabilities{ModelRegistrar: modelCatalog, ModelProvider: modelCatalog, Executor: ex, ExecutorModelScope: pluginapi.ExecutorModelScopeBoth, ExecutorInputFormats: []string{"claude"}, ExecutorOutputFormats: []string{"claude"}, QuotaProvider: quota}, OnShutdown: func(context.Context) error { client.CloseIdleConnections(); return nil }}, host)
}

// machineProviderCatalog wraps the kit ModelCatalog and stamps the machine
// provider id on every response. The kit derives Provider from
// req.Plugin.Name, which the host sets to the human-readable metadata name
// ("Z.ai Coding Plan"); the host registers the executor under that
// response's Provider, so echoing the display name would orphan the
// executor from the auth-file provider ("zai-coding-plan") and normal
// inference could not find it. The plugin's registered Metadata.Name stays
// the human-readable displayName — only catalog responses pin the machine
// id.
type machineProviderCatalog struct {
	*kit.ModelCatalog
}

func (c machineProviderCatalog) ModelsForAuth(ctx context.Context, req pluginapi.AuthModelRequest) (pluginapi.ModelResponse, error) {
	resp, err := c.ModelCatalog.ModelsForAuth(ctx, req)
	resp.Provider = provider
	return resp, err
}

func (c machineProviderCatalog) RegisterModels(ctx context.Context, req pluginapi.ModelRegistrationRequest) (pluginapi.ModelRegistrationResponse, error) {
	resp, err := c.ModelCatalog.RegisterModels(ctx, req)
	resp.Provider = provider
	return resp, err
}

func (c machineProviderCatalog) StaticModels(ctx context.Context, req pluginapi.StaticModelRequest) (pluginapi.ModelResponse, error) {
	resp, err := c.ModelCatalog.StaticModels(ctx, req)
	resp.Provider = provider
	return resp, err
}

// modelCatalog serves the static GLM list through the kit ModelCatalog
// pipeline (glob -> overrides -> namespace). Fetch is nil — static-only:
// the discovery URL/schema is unverified because verification requires
// a valid Coding Plan key for both key and region held by a
// credential-holder, and no read-only fixture has landed. Do not add a
// Fetch against the unverified schema or read a credential; a follow-up
// may implement discovery only after an operator-recorded fixture for
// both regions lands. Static-only does not fulfill auto-discovery.
// Static holds upstream (pre-namespace) ids; Namespace "zai" matches
// Definition.ModelNamespace and the pipeline prefixes before serving.
var modelCatalog = machineProviderCatalog{kit.NewModelCatalog(kit.ModelCatalogConfig{
	Namespace: "zai",
	Static: []pluginapi.ModelInfo{
		{ID: "glm-5.3", Object: "model", OwnedBy: provider, DisplayName: "GLM-5.3"},
		{ID: "glm-5.3-flash", Object: "model", OwnedBy: provider, DisplayName: "GLM-5.3 Flash"},
	},
	Fetch: nil,
})}

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
