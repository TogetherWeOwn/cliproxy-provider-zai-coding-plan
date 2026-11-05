package main

import (
	"fmt"
	"testing"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
)

// Two independent implementations treat unit 6 as weekly; go-z-ai explicitly
// records both number=1 and number=7 in upstream responses.
func TestWeeklyDayCountAndIdleReset(t *testing.T) {
	for _, number := range []int{1, 7} {
		q, err := parseQuota([]byte(fmt.Sprintf(`{"code":200,"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":6,"number":%d,"usage":200,"currentValue":0,"nextResetTime":0}]}}`, number)))
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != kit.CardWindowWeekly || q.Groups[0].Buckets[0].RemainingFraction != 1 || q.Groups[0].Buckets[0].ResetTime != "" {
			t.Fatalf("weekly number=%d idle reset lost: %+v %v", number, q, err)
		}
	}
}

func TestQuotaInvalidRowsDoNotHideValidWindow(t *testing.T) {
	for _, invalid := range []string{
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":-1,"currentValue":0}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":101}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":-1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":"lots","currentValue":1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":null,"currentValue":1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":1,"nextResetTime":-1}`,
		`{"type":"CREDIT_LIMIT","unit":6,"number":2,"usage":100,"currentValue":1}`,
		`{"type":"TIME_LIMIT","unit":3,"number":5,"usage":100,"currentValue":1}`,
	} {
		q, err := parseQuota([]byte(`{"success":true,"data":{"limits":[` + invalid + `,{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":200,"currentValue":60,"nextResetTime":1893456000000}]}}`))
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != kit.CardWindowWeekly || q.Groups[0].Buckets[0].RemainingFraction != .7 || q.Groups[0].Buckets[0].ResetTime != "2030-01-01T00:00:00Z" || len(q.Summary) != 0 {
			t.Fatalf("partial quota %+v %v", q, err)
		}
		if violations := kit.CheckCard(q); len(violations) != 0 {
			t.Fatalf("CheckCard violations: %v", violations)
		}
	}
}

// Fractional credit balances are approximated conservatively — capacity
// truncated down, consumption rounded up, so neither the remaining fraction
// nor the remaining credits exceed the exact values (apart from float display
// precision), instead of blinding the window with unknown_quota.
func TestQuotaFractionalCreditsApproximated(t *testing.T) {
	q, err := parseQuota([]byte(`{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100.5,"currentValue":1.9},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":200,"currentValue":60.4,"nextResetTime":1893456000000}]}}`))
	if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 2 {
		t.Fatalf("fractional quota %+v %v", q, err)
	}
	five := q.Groups[0].Buckets[0]
	// Exact: (100.5-1.9)/100.5 = 0.9810…; reported 98/100 = 0.98.
	if five.Window != kit.CardWindowFiveHour || five.RemainingFraction != .98 || five.Description != "" {
		t.Fatalf("fractional 5h not conservative: %+v", five)
	}
	weekly := q.Groups[0].Buckets[1]
	// Exact: (200-60.4)/200 = 0.698; reported 139/200 = 0.695.
	if weekly.Window != kit.CardWindowWeekly || weekly.RemainingFraction != .695 || weekly.Description != "" {
		t.Fatalf("fractional weekly not conservative: %+v", weekly)
	}
	if len(q.Summary) != 0 {
		t.Fatalf("fractional summary %+v", q.Summary)
	}
	// Near-total consumption truncates to exhausted, never to full.
	empty, err := parseQuota([]byte(`{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":1,"currentValue":0.9}]}}`))
	if err != nil || len(empty.Groups[0].Buckets) != 1 || empty.Groups[0].Buckets[0].RemainingFraction != 0 {
		t.Fatalf("near-exhausted fractional not exhausted: %+v %v", empty, err)
	}
}

func TestQuotaFractionalRoundingEdges(t *testing.T) {
	for _, tc := range []struct {
		name, capacity, consumed string
		remaining                float64
	}{
		{"near exhaustion", "100.5", "100.2", 0},
		{"exact exhaustion", "100.5", "100.5", 0},
		{"decimal consumption", "2", "1.0000000000000001", 0},
		{"decimal capacity", "1.9999999999999999", "1", 0},
		{"exponent form", "1.005e2", "1.9e0", .98},
		{"tiny consumption", "2", "1e-100", .5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":%s,"currentValue":%s}]}}`, tc.capacity, tc.consumed)
			q, err := parseQuota([]byte(raw))
			if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 {
				t.Fatalf("valid fractional window lost: %+v %v", q, err)
			}
			b := q.Groups[0].Buckets[0]
			if b.RemainingFraction != tc.remaining || b.Description != "" {
				t.Fatalf("fractional rounding: %+v, want %v", b, tc.remaining)
			}
			if violations := kit.CheckCard(q); len(violations) != 0 {
				t.Fatalf("CheckCard violations: %v", violations)
			}
		})
	}
}

func TestQuotaFractionalInvalidValues(t *testing.T) {
	for _, tc := range []struct{ capacity, consumed string }{
		{"100.1", "100.2"},
		{"2", "2.0000000000000001"},
		{"2", "-0.0000000000000001"},
		{"9223372036854775808", "0"},
		{"1e1000000000", "0"},
		{"2", "1e-1000000000"},
		{`"100"`, "1"},
		{"100", `"1"`},
	} {
		raw := fmt.Sprintf(`{"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":%s,"currentValue":%s},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":10,"currentValue":2}]}}`, tc.capacity, tc.consumed)
		q, err := parseQuota([]byte(raw))
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != kit.CardWindowWeekly {
			t.Fatalf("invalid fractional row hid valid window: %+v %v", q, err)
		}
	}
}

func TestQuotaUnparseableNumbersStillRejected(t *testing.T) {
	for _, row := range []string{
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":true,"currentValue":1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":1e999,"currentValue":1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":0.5,"currentValue":0}`,
	} {
		if q, err := parseQuota([]byte(`{"success":true,"data":{"limits":[` + row + `]}}`)); err == nil {
			t.Fatalf("unparseable row accepted: %s -> %+v", row, q)
		}
	}
}
