package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func staticAuthReq(authID string) pluginapi.AuthModelRequest {
	return pluginapi.AuthModelRequest{Plugin: pluginapi.Metadata{Name: displayName}, AuthID: authID, AuthProvider: provider, StorageJSON: []byte(`{"api_key":"example-key"}`)}
}

func modelIDs(models []pluginapi.ModelInfo) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ID
	}
	return out
}

func TestRegistrarAndStaticCatalogNamespaces(t *testing.T) {
	ctx := context.Background()
	registered, err := modelCatalog.RegisterModels(ctx, pluginapi.ModelRegistrationRequest{Plugin: pluginapi.Metadata{Name: displayName}})
	if err != nil || registered.Provider != provider {
		t.Fatalf("registrar %+v %v", registered, err)
	}
	static, err := modelCatalog.StaticModels(ctx, pluginapi.StaticModelRequest{Plugin: pluginapi.Metadata{Name: displayName}})
	if err != nil || static.Provider != provider {
		t.Fatalf("static catalog %+v %v", static, err)
	}
	for _, list := range [][]pluginapi.ModelInfo{registered.Models, static.Models} {
		if len(list) != 2 || list[0].ID != "zai/glm-5.3" || list[1].ID != "zai/glm-5.3-flash" {
			t.Fatalf("unprefixed or missing registered models: %+v", list)
		}
		if list[0].DisplayName != "GLM-5.3" || list[1].DisplayName != "GLM-5.3 Flash" {
			t.Fatalf("display names: %+v", list)
		}
		if list[0].OwnedBy != provider || list[0].Object != "model" {
			t.Fatalf("ownership: %+v", list[0])
		}
	}
}

// Static-only: ModelsForAuth serves the static list without any fetch.
// Fetch is nil until an operator-recorded fixture verifies the discovery
// URL/schema for both key and region; this test must not claim discovery.
func TestStaticOnlyModelsForAuthServesStaticWithoutFetch(t *testing.T) {
	resp, err := modelCatalog.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := modelIDs(resp.Models); len(got) != 2 || got[0] != "zai/glm-5.3" || got[1] != "zai/glm-5.3-flash" {
		t.Fatalf("static-only ids %v", got)
	}
	if resp.Provider != provider {
		t.Fatalf("provider %q", resp.Provider)
	}
	// Static-only yields no last-good state: repeated sequential calls
	// still converge to the same list (Timeline A for the static shape).
	for i := 0; i < 3; i++ {
		again, err := modelCatalog.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
		if err != nil {
			t.Fatal(err)
		}
		if got := modelIDs(again.Models); len(got) != 2 || got[0] != "zai/glm-5.3" {
			t.Fatalf("repeat %d: %v", i, got)
		}
	}
}

type fetchFixture struct {
	models []pluginapi.ModelInfo
	err    error
}

func (f *fetchFixture) fetch(_ context.Context, _ pluginapi.AuthModelRequest) ([]pluginapi.ModelInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.models, nil
}

func namedUpstream(ids ...string) []pluginapi.ModelInfo {
	out := make([]pluginapi.ModelInfo, len(ids))
	for i, id := range ids {
		out[i] = pluginapi.ModelInfo{ID: id, Object: "model", DisplayName: id}
	}
	return out
}

// Overrides key by upstream (pre-namespace) id and apply through the same
// pipeline for fresh and static. Production bakes display names into
// Static (no Overrides); this proves the released kit's pipeline shape a
// future discovery Fetch would inherit.
func TestOverridesApplyToFreshAndStatic(t *testing.T) {
	fix := &fetchFixture{models: namedUpstream("m-one")}
	cat := kit.NewModelCatalog(kit.ModelCatalogConfig{
		Namespace: "zai",
		Static:    namedUpstream("s-one"),
		Fetch:     fix.fetch,
		Overrides: map[string]kit.ModelOverride{
			"m-one": {DisplayName: "One", ContextLength: 100, MaxCompletionTokens: 10},
			"s-one": {DisplayName: "Static One"},
		},
	})
	resp, err := cat.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 1 || resp.Models[0].ID != "zai/m-one" || resp.Models[0].DisplayName != "One" || resp.Models[0].ContextLength != 100 || resp.Models[0].MaxCompletionTokens != 10 {
		t.Fatalf("fresh override %+v", resp.Models)
	}
	sresp, err := cat.StaticModels(context.Background(), pluginapi.StaticModelRequest{Plugin: pluginapi.Metadata{Name: "zai"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sresp.Models) != 1 || sresp.Models[0].ID != "zai/s-one" || sresp.Models[0].DisplayName != "Static One" {
		t.Fatalf("static override %+v", sresp.Models)
	}
}

// Contributor dedup: a "-contributor" sibling wins without any override;
// an id with no sibling keeps its base id. Production static has neither
// sibling, so both survive; this proves the kit pairing rule for future
// upstream ids unknown at migration time.
func TestContributorDedupWithAndWithoutSibling(t *testing.T) {
	withSibling := &fetchFixture{models: namedUpstream("unknown-a", "unknown-a-contributor", "unknown-solo")}
	cat := kit.NewModelCatalog(kit.ModelCatalogConfig{Namespace: "zai", Fetch: withSibling.fetch})
	resp, err := cat.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := modelIDs(resp.Models); len(got) != 2 || got[0] != "zai/unknown-a-contributor" || got[1] != "zai/unknown-solo" {
		t.Fatalf("contributor dedup %v", got)
	}
	// Without a sibling, the base id is kept unchanged.
	noSibling := &fetchFixture{models: namedUpstream("unknown-solo")}
	cat2 := kit.NewModelCatalog(kit.ModelCatalogConfig{Namespace: "zai", Fetch: noSibling.fetch})
	resp, err = cat2.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := modelIDs(resp.Models); len(got) != 1 || got[0] != "zai/unknown-solo" {
		t.Fatalf("no-sibling %v", got)
	}
}

// Executor mapping: the executor strips exactly one leading "zai/" so the
// catalog-emitted id maps to the upstream id buildRequest sends.
func TestExecutorMappingAssertion(t *testing.T) {
	resp, err := modelCatalog.StaticModels(context.Background(), pluginapi.StaticModelRequest{Plugin: pluginapi.Metadata{Name: displayName}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"zai/glm-5.3": "glm-5.3", "zai/glm-5.3-flash": "glm-5.3-flash"}
	for _, m := range resp.Models {
		upstream, ok := strings.CutPrefix(m.ID, "zai/")
		if !ok || upstream == "" || strings.HasPrefix(upstream, "zai/") {
			t.Fatalf("emitted %q does not survive one-namespace strip", m.ID)
		}
		if want[m.ID] != upstream {
			t.Fatalf("executor upstream id %q, want %q", upstream, want[m.ID])
		}
		req, err := buildRequest(context.Background(), pluginapi.ExecutorRequest{Model: m.ID, Payload: []byte(`{"model":"` + m.ID + `","messages":[]}`), StorageJSON: []byte(`{"api_key":"example-key"}`)})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(req.Body), `"model":"`+upstream+`"`) {
			t.Fatalf("request for %s: %s", m.ID, req.Body)
		}
	}
	if err := kit.CheckModelNamespace("zai", resp.Models); err != nil {
		t.Fatalf("namespace: %v", err)
	}
}

// Timeline B is documented, never claimed as a guarantee: with Fetch nil
// there is no fetch to fail, so no recovery question arises. After a
// Fetch lands, the released kit performs a single fetch per
// ModelsForAuth invocation (no bounded retry is included in v0.6.0), so
// a transient failure serves last-good and recovery waits for the next
// host event. This test documents that residual; it does not promise
// recovery.
func TestTimelineBDocumentedNoRecoveryGuarantee(t *testing.T) {
	// Static-only: no failure path exists to exercise.
	resp, err := modelCatalog.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("static timeline B: %+v", resp.Models)
	}
	// Released-kit shape (single fetch, no retry): failure with no
	// last-good falls back to static, then a later success converges.
	fix := &fetchFixture{err: errors.New("boom")}
	cat := kit.NewModelCatalog(kit.ModelCatalogConfig{Namespace: "zai", Static: []pluginapi.ModelInfo{{ID: "glm-5.3", Object: "model"}}, Fetch: fix.fetch})
	if _, err := cat.ModelsForAuth(context.Background(), staticAuthReq("auth-1")); err != nil {
		t.Fatal(err)
	}
	fix.err = nil
	fix.models = namedUpstream("a")
	got, err := cat.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if ids := modelIDs(got.Models); len(ids) != 1 || ids[0] != "zai/a" {
		t.Fatalf("post-failure convergence %v", ids)
	}
}

// TTL/last-good isolation: TTL bounds only the freshness markers, never
// the durable last-good store. Production (Fetch nil) has no last-good;
// the released kit keeps last-good across TTL expiry (proven here with a
// fixture Fetch on a short TTL).
func TestTTLLastGoodIsolation(t *testing.T) {
	fix := &fetchFixture{models: namedUpstream("a")}
	cat := kit.NewModelCatalogWithTTL(kit.ModelCatalogConfig{Namespace: "zai", Static: namedUpstream("static-1"), Fetch: fix.fetch}, time.Nanosecond)
	if _, err := cat.ModelsForAuth(context.Background(), staticAuthReq("auth-1")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	fix.err = errors.New("boom")
	resp, err := cat.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := modelIDs(resp.Models); len(got) != 1 || got[0] != "zai/a" {
		t.Fatalf("past-TTL served %v, want durable last-good", got)
	}
}

// Credential-generation isolation: production static-only serves the same
// list for every generation; the released kit never leaks generation A's
// last-good into rotated-credential generation B.
func TestCredentialGenerationIsolation(t *testing.T) {
	a, err := modelCatalog.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	rotated := staticAuthReq("auth-1")
	rotated.StorageJSON = []byte(`{"api_key":"rotated-key"}`)
	b, err := modelCatalog.ModelsForAuth(context.Background(), rotated)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Models) != 0 && len(b.Models) != 0 && modelIDs(a.Models)[0] != modelIDs(b.Models)[0] {
		t.Fatalf("static changed across generations: %+v vs %+v", a.Models, b.Models)
	}
	// Kit last-good is keyed by credential generation: B falls back to
	// static on fetch error, not to A's last-good.
	fix := &fetchFixture{models: namedUpstream("a")}
	cat := kit.NewModelCatalog(kit.ModelCatalogConfig{Namespace: "zai", Static: namedUpstream("static-1"), Fetch: fix.fetch})
	reqA := staticAuthReq("auth-1")
	if _, err := cat.ModelsForAuth(context.Background(), reqA); err != nil {
		t.Fatal(err)
	}
	reqB := staticAuthReq("auth-1")
	reqB.StorageJSON = []byte(`{"api_key":"rotated-key"}`)
	fix.err = errors.New("boom")
	resp, err := cat.ModelsForAuth(context.Background(), reqB)
	if err != nil {
		t.Fatal(err)
	}
	if got := modelIDs(resp.Models); len(got) != 1 || got[0] != "zai/static-1" {
		t.Fatalf("rotated served %v, want static (not stale last-good)", got)
	}
	resp, err = cat.ModelsForAuth(context.Background(), reqA)
	if err != nil {
		t.Fatal(err)
	}
	if got := modelIDs(resp.Models); len(got) != 1 || got[0] != "zai/a" {
		t.Fatalf("original generation served %v, want its last-good", got)
	}
}

// Cancellation: a cancelled context aborts before serving. Static-only
// still honors it; the kit's deadline/size fallback applies only to the
// Fetch path (15s ModelsFetchTimeout, 1 MiB MaxModelsBody), which the
// static path never invokes.
func TestCancellationAndDeadlineSizeFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := modelCatalog.ModelsForAuth(ctx, staticAuthReq("auth-1")); err == nil {
		t.Fatal("cancelled context served models")
	}
	// An uncancellable static call always succeeds: no fetch deadline or
	// size check can trip it.
	resp, err := modelCatalog.ModelsForAuth(context.Background(), staticAuthReq("auth-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("static fallback %+v", resp.Models)
	}
}

// OpenAI-style parse: the kit parses {"data":[{"id":...}]} for a future
// Fetch. Production has no Fetch (schema unverified), so this proves the
// parse shape the fixture will validate, via a fixture host client — it
// does not claim this plugin uses that endpoint today.
type fakeHostClient struct {
	status int
	body   []byte
}

func (f fakeHostClient) Do(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	return pluginapi.HTTPResponse{StatusCode: f.status, Body: f.body}, nil
}

func (f fakeHostClient) DoStream(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPStreamResponse, error) {
	return pluginapi.HTTPStreamResponse{}, errors.New("streaming not supported in fixture")
}

func TestOpenAIModelsFetcherParseShape(t *testing.T) {
	fetcher := kit.OpenAIModelsFetcher("https://api.example.test/v1/", "/models", func(_ pluginapi.AuthModelRequest) http.Header {
		return http.Header{"Authorization": {"Bearer k"}}
	})
	req := staticAuthReq("auth-1")
	req.HTTPClient = fakeHostClient{status: 200, body: []byte(`{"data":[{"id":"m-one","object":"model","owned_by":"test"},{"id":"m-two"}]}`)}
	got, err := fetcher(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "m-one" || got[0].OwnedBy != "test" || got[1].ID != "m-two" {
		t.Fatalf("parsed %+v", got)
	}
	// Oversized bodies are a failed fetch, never parsed or cached.
	req.HTTPClient = fakeHostClient{status: 200, body: make([]byte, kit.MaxModelsBody+1)}
	if _, err := fetcher(context.Background(), req); err == nil {
		t.Fatal("oversized body parsed")
	}
	// Non-2xx is a failed fetch.
	req.HTTPClient = fakeHostClient{status: 401, body: []byte(`{"error":"unauthorized"}`)}
	if _, err := fetcher(context.Background(), req); err == nil {
		t.Fatal("non-2xx parsed")
	}
}
