package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestAuthAndCatalog(t *testing.T) {
	p := authProvider()
	input := pluginapi.AuthParseRequest{RawJSON: []byte(`{"type":"zai-coding-plan","api_key":"example-key"}`)}
	r, err := p.ParseAuth(context.Background(), input)
	sum := sha256.Sum256([]byte("example-key"))
	if err != nil || !r.Handled || r.Auth.ID != fmt.Sprintf("zai-coding-plan:%x", sum[:6]) {
		t.Fatalf("auth: %+v %v", r, err)
	}
	input.RawJSON = []byte(`{"type":"other"}`)
	r, err = p.ParseAuth(context.Background(), input)
	if err != nil || r.Handled {
		t.Fatal("wrong type handled")
	}
	input.RawJSON = []byte(`{"type":"zai-coding-plan"}`)
	_, err = p.ParseAuth(context.Background(), input)
	if err == nil {
		t.Fatal("missing key accepted")
	}
	models, _ := modelCatalog.ModelsForAuth(context.Background(), pluginapi.AuthModelRequest{Plugin: pluginapi.Metadata{Name: displayName}, AuthProvider: provider, StorageJSON: []byte(`{"api_key":"example-key"}`)})
	if len(models.Models) != 2 || models.Models[0].ID != "zai/glm-5.3" || models.Models[1].ID != "zai/glm-5.3-flash" {
		t.Fatalf("catalog: %+v", models)
	}
}
func TestQuotaParser(t *testing.T) {
	data := []byte(`{"success":true,"data":{"level":"pro","planName":"Pro","limits":[{"type":"CREDIT_LIMIT","unit":3.0,"number":5.0,"usage":100.0,"currentValue":25.0,"nextResetTime":null},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":200,"currentValue":60,"nextResetTime":1893456000000},{"type":"TOKENS_LIMIT","unit":3,"number":5,"usage":1,"currentValue":1}]}}`)
	q, err := parseQuota(data)
	if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 2 {
		t.Fatalf("quota: %+v %v", q, err)
	}
	if q.Groups[0].Buckets[0].Window != kit.CardWindowFiveHour || q.Groups[0].Buckets[0].RemainingFraction != .75 || q.Groups[0].Buckets[0].ResetTime != "" || q.Groups[0].Buckets[1].Window != kit.CardWindowWeekly || q.Groups[0].Buckets[1].RemainingFraction != .7 || len(q.Summary) != 0 {
		t.Fatalf("quota mapping: %+v", q)
	}
	if violations := kit.CheckCard(q); len(violations) != 0 {
		t.Fatalf("CheckCard violations: %v", violations)
	}
	partial := []byte(`{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":0,"currentValue":1},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":100,"currentValue":10}]}}`)
	q, err = parseQuota(partial)
	if err != nil || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != kit.CardWindowWeekly {
		t.Fatalf("partial: %+v %v", q, err)
	}
	if violations := kit.CheckCard(q); len(violations) != 0 {
		t.Fatalf("CheckCard violations: %v", violations)
	}
	_, err = parseQuota([]byte(`{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT"}]}}`))
	if err == nil {
		t.Fatal("unknown shape accepted")
	}
}
func TestQuotaFallback(t *testing.T) {
	var seen []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.URL.Path != "/api/monitor/usage/quota/limit" {
			t.Errorf("path %s", r.URL.Path)
		}
		if len(seen) == 1 {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":10,"currentValue":2}]}}`)
	}))
	defer s.Close()
	q, err := fetchQuota(context.Background(), s.URL, "example-key", nil)
	if err != nil || len(q.Groups) != 1 || strings.Join(seen, ",") != "example-key,Bearer example-key" {
		t.Fatalf("quota %+v %v %v", q, err, seen)
	}
}
func TestErrors(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		code   int
		status int
		retry  bool
		wait   int
		scoped bool
	}{{1001, 401, false, 0, false}, {1003, 401, false, 0, false}, {1113, 429, false, 3600, true}, {1302, 429, true, 10, false}, {1305, 429, true, 10, false}, {1308, 429, true, 120, true}, {1310, 429, true, 120, true}, {1316, 429, true, 120, true}, {1317, 429, true, 120, true}, {1318, 429, true, 120, true}, {1319, 429, true, 120, true}, {1320, 429, true, 120, true}, {1321, 429, true, 120, true}, {1309, 429, false, 3600, true}, {1313, 429, false, 3600, true}, {1311, 429, false, 3600, false}, {1314, 429, false, 3600, false}, {1315, 429, false, 3600, false}} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": tc.code, "next_flush_time": now.Add(120 * time.Second).Format(time.RFC3339)}})
			e := classifyError(pluginapi.HTTPResponse{StatusCode: 400, Body: body}, now)
			if e.HTTPStatus != tc.status || e.Retryable != tc.retry || e.RetryAfterSeconds != tc.wait || e.CredentialScoped != tc.scoped {
				t.Fatalf("%+v", e)
			}
			if e.CredentialScoped != e.IsCredentialScoped() {
				t.Fatalf("CredentialScoped mismatch: %+v", e)
			}
		})
	}
}

// TestQuotaUnscopedBranchesKeepRetryAfter proves the unscoped branches keep
// their existing retry-after behavior (transient hint, 60s fallback) and stay
// model-scoped, so only the account-wide branches cool the whole credential.
func TestQuotaUnscopedBranchesKeepRetryAfter(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	e := classifyError(pluginapi.HTTPResponse{StatusCode: 429, Headers: http.Header{"Retry-After": {"23"}}, Body: []byte(`{"error":{"code":"1305"}}`)}, now)
	if e.CredentialScoped || e.RetryAfterSeconds != 23 {
		t.Fatalf("transient scoped or hint changed: %+v", e)
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": 1317}})
	e = classifyError(pluginapi.HTTPResponse{StatusCode: 429, Body: body}, now)
	if !e.CredentialScoped || e.RetryAfterSeconds != 60 {
		t.Fatalf("exhausted without reset: %+v", e)
	}
}

// streamQuotaClient serves one error body on a 429 stream, mirroring the host
// transport contract for the kit stream pump.
type streamQuotaClient struct{ body []byte }

func (c streamQuotaClient) Do(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	return pluginapi.HTTPResponse{StatusCode: 429, Body: c.body}, nil
}
func (c streamQuotaClient) DoStream(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPStreamResponse, error) {
	ch := make(chan pluginapi.HTTPStreamChunk, 1)
	ch <- pluginapi.HTTPStreamChunk{Payload: c.body}
	close(ch)
	return pluginapi.HTTPStreamResponse{StatusCode: 429, Headers: http.Header{"Content-Type": {"application/json"}}, Chunks: ch}, nil
}

// TestStreamQuotaErrorStaysCredentialScoped proves an upstream stream error
// for the scoped shape surfaces credential-scoped through the kit runtime:
// the stream pump classifies the 429 body, and kit MapError (the error_detail
// carrier) passes the scoped pair through intact.
func TestStreamQuotaErrorStaysCredentialScoped(t *testing.T) {
	now := time.Now()
	reset := now.Add(120 * time.Second).UTC().Format(time.RFC3339)
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": 1317, "message": "Resets at " + reset}})
	p := kit.Passthrough{Provider: provider, BuildRequest: buildRequest, PrepareHTTP: prepareHTTP, ClassifyError: classifyResponse}
	_, err := p.ExecuteStream(context.Background(), pluginapi.ExecutorRequest{Payload: []byte(`{"model":"zai/glm-5.3","messages":[]}`), StorageJSON: []byte(`{"api_key":"example-key"}`), HTTPClient: streamQuotaClient{body: body}})
	if err == nil {
		t.Fatal("stream quota error hidden")
	}
	mapped := kit.MapError(err)
	if mapped == nil || mapped.Code != "quota_exhausted" || mapped.HTTPStatus != 429 || !mapped.CredentialScoped {
		t.Fatalf("stream scoped pair lost: %+v", mapped)
	}
	if mapped.RetryAfterSeconds <= 0 || mapped.RetryAfterSeconds > 120 {
		t.Fatalf("stream retry-after = %d, want reset-derived <=120", mapped.RetryAfterSeconds)
	}
	// The unscoped transient shape must not cool the whole credential on the
	// same path.
	body, _ = json.Marshal(map[string]any{"error": map[string]any{"code": 1305, "message": "slow down"}})
	_, err = p.ExecuteStream(context.Background(), pluginapi.ExecutorRequest{Payload: []byte(`{"model":"zai/glm-5.3","messages":[]}`), StorageJSON: []byte(`{"api_key":"example-key"}`), HTTPClient: streamQuotaClient{body: body}})
	mapped = kit.MapError(err)
	if mapped == nil || mapped.Code != "rate_limited" || mapped.CredentialScoped {
		t.Fatalf("transient stream scoped: %+v", mapped)
	}
}
func TestModelRewrite(t *testing.T) {
	req, err := buildRequest(context.Background(), pluginapi.ExecutorRequest{Model: "zai/glm-5.3", Payload: []byte(`{"model":"zai/glm-5.3","messages":[]}`), StorageJSON: []byte(`{"api_key":"example-key"}`)})
	if err != nil || req.URL != "https://api.z.ai/api/anthropic/v1/messages" || !strings.Contains(string(req.Body), `"model":"glm-5.3"`) || req.Headers.Get("Authorization") != "Bearer example-key" {
		t.Fatalf("request %+v %v", req, err)
	}
}

func TestResetInErrorMessage(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"code":"1317","message":"quota exhausted, next_flush_time: 2030-01-01 00:02:00"}}`)
	e := classifyError(pluginapi.HTTPResponse{StatusCode: 429, Body: body}, now)
	if e.RetryAfterSeconds != 120 || !e.CredentialScoped {
		t.Fatalf("reset in message ignored: %+v", e)
	}
}

func TestMalformedRegionRejected(t *testing.T) {
	_, err := authBase([]byte(`{"api_key":"example-key","region":[]}`), nil)
	if err == nil {
		t.Fatal("malformed region accepted")
	}
}
