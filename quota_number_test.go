package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
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

func TestQuotaQuotedMetadataCompatibility(t *testing.T) {
	for _, tc := range []struct{ name, code, unit, number, reset string }{
		{"numeric control", "200", "3", "5", "1893456000000"},
		{"quoted code", `"200"`, "3", "5", "1893456000000"},
		{"quoted unit", "200", `"3"`, "5", "1893456000000"},
		{"quoted number", "200", "3", `"5"`, "1893456000000"},
		{"quoted reset", "200", "3", "5", `"1893456000000"`},
		{"all quoted", `"200"`, `"3"`, `"5"`, `"1893456000000"`},
		{"quoted integral decimals", `"2e2"`, `"3.0"`, `"5e0"`, `"1893456000000.0"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"code":%s,"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":%s,"number":%s,"usage":100.5,"currentValue":1.9,"nextResetTime":%s}]}}`, tc.code, tc.unit, tc.number, tc.reset)
			q, err := parseQuota([]byte(raw))
			if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 {
				t.Fatalf("valid metadata lost quota: %+v %v", q, err)
			}
			b := q.Groups[0].Buckets[0]
			if b.Window != kit.CardWindowFiveHour || b.RemainingFraction != .98 || b.ResetTime != "2030-01-01T00:00:00Z" {
				t.Fatalf("metadata changed quota: %+v", b)
			}
			if violations := kit.CheckCard(q); len(violations) != 0 {
				t.Fatalf("CheckCard violations: %v", violations)
			}
		})
	}
}

func TestIntNumberBounds(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{
		{"9223372036854775807", 1<<63 - 1},
		{`"9223372036854775807"`, 1<<63 - 1},
		{`"-9223372036854775808"`, -1 << 63},
		{`"3.0"`, 3},
		{`"5e0"`, 5},
		{`"0e128"`, 0},
		{`"0e-128"`, 0},
		{`"0.` + strings.Repeat("0", 126) + `"`, 0},
	} {
		if got, ok := intNumber(json.RawMessage(tc.raw)); !ok || got != tc.want {
			t.Fatalf("bounded integer %q: got %d %v, want %d", tc.raw, got, ok, tc.want)
		}
	}
	for _, raw := range []string{
		`"9223372036854775808"`, `"-9223372036854775809"`, "9223372036854775808",
		`"1.1"`, `"1e129"`, `"1e-129"`, `"1e99999999999999999999999999"`,
		`"` + strings.Repeat("1", 129) + `"`, `"0.` + strings.Repeat("0", 127) + `"`,
		`"01"`, `"+1"`, `"NaN"`, `"1/2"`, `" 1 "`, `""`, `"1`, "null", "true", "[]", "{}", "",
	} {
		if _, ok := intNumber(json.RawMessage(raw)); ok {
			t.Fatalf("invalid or unbounded integer accepted: %q", raw)
		}
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
