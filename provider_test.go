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
	p, err := authProvider()
	if err != nil {
		t.Fatal(err)
	}
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
	models, _ := (catalog{}).ModelsForAuth(context.Background(), pluginapi.AuthModelRequest{})
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
	}{{1001, 401, false, 0}, {1003, 401, false, 0}, {1113, 429, false, 3600}, {1302, 429, true, 10}, {1305, 429, true, 10}, {1308, 429, true, 120}, {1310, 429, true, 120}, {1316, 429, true, 120}, {1317, 429, true, 120}, {1318, 429, true, 120}, {1319, 429, true, 120}, {1320, 429, true, 120}, {1321, 429, true, 120}, {1309, 429, false, 3600}, {1313, 429, false, 3600}} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": tc.code, "next_flush_time": now.Add(120 * time.Second).Format(time.RFC3339)}})
			e := classifyError(pluginapi.HTTPResponse{StatusCode: 400, Body: body}, now)
			if e.HTTPStatus != tc.status || e.Retryable != tc.retry || e.RetryAfterSeconds != tc.wait {
				t.Fatalf("%+v", e)
			}
		})
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
	if e.RetryAfterSeconds != 120 {
		t.Fatalf("reset in message ignored: %+v", e)
	}
}

func TestMalformedRegionRejected(t *testing.T) {
	_, err := authBase([]byte(`{"api_key":"example-key","region":[]}`), nil)
	if err == nil {
		t.Fatal("malformed region accepted")
	}
}
