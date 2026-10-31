package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// The first-party usage script and oh-my-pi use raw-key monitor auth. Only a
// 401 warrants trying the alternate form; a 403/429 must not trigger a retry.
func TestQuotaAuthorizationWireContract(t *testing.T) {
	for _, firstStatus := range []int{200, 401, 403, 429} {
		t.Run(fmt.Sprint(firstStatus), func(t *testing.T) {
			var seen []string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, r.Header.Get("Authorization"))
				if r.Method != "GET" || r.URL.RequestURI() != "/api/monitor/usage/quota/limit" {
					t.Errorf("unexpected quota route %s %s", r.Method, r.URL.RequestURI())
				}
				if len(seen) == 1 && firstStatus != 200 {
					w.WriteHeader(firstStatus)
					return
				}
				fmt.Fprint(w, `{"code":200,"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":25}]}}`)
			}))
			defer s.Close()
			q, err := fetchQuota(context.Background(), s.URL, "example-key", nil)
			want := "example-key"
			if firstStatus == 401 {
				want += ",Bearer example-key"
			}
			if strings.Join(seen, ",") != want {
				t.Fatalf("authorization sequence %q, want %q", seen, want)
			}
			if firstStatus == 200 || firstStatus == 401 {
				if err != nil || len(q.Groups) != 1 || q.Groups[0].Buckets[0].RemainingFraction != .75 {
					t.Fatalf("quota %+v: %v", q, err)
				}
			} else if err == nil {
				t.Fatal("quota failure hidden")
			}
		})
	}
}

func TestOfficialErrorMessageResetTemplates(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, message := range []string{
		"Usage limit reached for 5 hours. Your limit will reset at 2030-01-01 00:02:00",
		"Weekly/Monthly Limit Exhausted. Your limit will reset at 2030-01-01T00:02:00Z",
		"Usage limit reached for the past 5 hours. Insufficient balance for extra usage. Resets at 2030-01-01 00:02:00.",
		"Usage limit reached for the past 7 days. Extra usage is not available due to monthly spend limit. Resets at 1893456120000.",
		"next_flush_time: 2030-01-01T08:02:00+08:00",
	} {
		t.Run(message, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": "1317", "message": message}})
			e := classifyError(pluginapi.HTTPResponse{StatusCode: 429, Body: body}, now)
			if e.Code != "quota_exhausted" || e.RetryAfterSeconds != 120 {
				t.Fatalf("reset hint lost: %+v", e)
			}
		})
	}
}

func TestEntitlementErrorsAreNotTransientOrExhaustion(t *testing.T) {
	for _, code := range []int{1311, 1314, 1315} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			e := classifyError(pluginapi.HTTPResponse{StatusCode: 429, Body: []byte(fmt.Sprintf(`{"error":{"code":%d,"message":"private upstream detail"}}`, code))}, time.Now())
			if e.Code != "entitlement_unavailable" || e.HTTPStatus != 429 || e.Retryable || e.RetryAfterSeconds != 3600 || strings.Contains(e.Message, "private upstream") {
				t.Fatalf("entitlement error misclassified or leaked: %+v", e)
			}
		})
	}
}

func TestTransientErrorRespectsRetryAfter(t *testing.T) {
	e := classifyError(pluginapi.HTTPResponse{StatusCode: 429, Headers: http.Header{"Retry-After": {"23"}}, Body: []byte(`{"error":{"code":"1305"}}`)}, time.Now())
	if e.Code != "rate_limited" || !e.Retryable || e.RetryAfterSeconds != 23 {
		t.Fatalf("transient hint overwritten: %+v", e)
	}
}

func TestPrepareHTTPRewritesModelAndRejectsCallerTarget(t *testing.T) {
	r, err := prepareHTTP(context.Background(), pluginapi.ExecutorHTTPRequest{URL: "https://example.invalid/collect", Method: "DELETE", Headers: http.Header{"Authorization": {"caller-key"}, "X-Api-Key": {"caller-key"}, "Anthropic-Version": {"2023-06-01"}}, Body: []byte(`{"model":"zai/glm-5.3-flash","max_tokens":8}`), StorageJSON: []byte(`{"api_key":"example-key","region":"cn"}`)})
	if err != nil || r.URL != "https://open.bigmodel.cn/api/anthropic/v1/messages" || r.Method != "POST" || r.Headers.Get("Authorization") != "Bearer example-key" || r.Headers.Get("X-Api-Key") != "" || !strings.Contains(string(r.Body), `"model":"glm-5.3-flash"`) {
		t.Fatalf("unsafe or unrewritten HTTP request %+v: %v", r, err)
	}
}

func TestQuotaRejectsUnsuccessfulEnvelope(t *testing.T) {
	for _, prefix := range []string{`"code":500,`, `"code":"500",`, `"success":false,`} {
		raw := []byte(`{` + prefix + `"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":25}]}}`)
		if q, err := parseQuota(raw); err == nil {
			t.Fatalf("failed envelope presented as quota: %+v", q)
		}
	}
}
