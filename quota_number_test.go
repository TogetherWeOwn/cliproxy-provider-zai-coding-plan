package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactNumberBounds(t *testing.T) {
	for _, raw := range []string{
		"1e129", "1e-129", "1e99999999999999999999999999",
		strings.Repeat("1", 129), `"1"`, "null", "true", "[]", "{}", "NaN", "01", "1/2",
	} {
		if _, ok := exactNumber(json.RawMessage(raw)); ok {
			t.Fatalf("invalid or unbounded number accepted: %q", raw)
		}
	}
	for _, raw := range []string{"1e128", "1e-128", strings.Repeat("1", 128), " 1.0 ", "-0"} {
		if _, ok := exactNumber(json.RawMessage(raw)); !ok {
			t.Fatalf("bounded number rejected: %q", raw)
		}
	}
}

func TestCreditCountsInt64Boundary(t *testing.T) {
	limit, used, ok := creditCounts(json.RawMessage("9223372036854775807"), json.RawMessage("9223372036854775806.9"))
	if !ok || limit != 1<<63-1 || used != limit {
		t.Fatalf("rounding overflowed: %d %d %v", limit, used, ok)
	}
	if _, _, ok := creditCounts(json.RawMessage("9223372036854775807.1"), json.RawMessage("0")); ok {
		t.Fatal("capacity beyond int64 accepted")
	}
}

func TestQuotaIntegerFieldsRemainStrict(t *testing.T) {
	for _, field := range []string{
		`"unit":3.1,"number":5`,
		`"unit":3,"number":5.1`,
		`"unit":3,"number":5,"nextResetTime":1.1`,
		`"unit":3,"number":5,"nextResetTime":1e1000000000`,
	} {
		if q, err := parseQuota([]byte(`{"data":{"limits":[{"type":"CREDIT_LIMIT","usage":100.5,"currentValue":1.9,` + field + `}]}}`)); err == nil {
			t.Fatalf("nonintegral or unbounded metadata accepted: %+v", q)
		}
	}
}
