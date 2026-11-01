package main

import (
	"fmt"
	"testing"
)

// Two independent implementations treat unit 6 as weekly; go-z-ai explicitly
// records both number=1 and number=7 in upstream responses.
func TestWeeklyDayCountAndIdleReset(t *testing.T) {
	for _, number := range []int{1, 7} {
		q, err := parseQuota([]byte(fmt.Sprintf(`{"code":200,"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":6,"number":%d,"usage":200,"currentValue":0,"nextResetTime":0}]}}`, number)))
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != "weekly" || q.Groups[0].Buckets[0].RemainingFraction != 1 || q.Groups[0].Buckets[0].ResetTime != "" {
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
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != "weekly" || q.Groups[0].Buckets[0].RemainingFraction != .7 || q.Groups[0].Buckets[0].ResetTime != "2030-01-01T00:00:00Z" || len(q.Summary) != 1 || q.Summary[0].Value != 140 {
			t.Fatalf("partial quota %+v %v", q, err)
		}
	}
}

// Fractional credit balances are approximated conservatively — capacity
// truncated down, consumption rounded up, so neither the remaining fraction
// nor the remaining credits ever exceed the exact values — and marked on the
// bucket, instead of blinding the window with unknown_quota.
func TestQuotaFractionalCreditsApproximated(t *testing.T) {
	q, err := parseQuota([]byte(`{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100.5,"currentValue":1.9},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":200,"currentValue":60.4,"nextResetTime":1893456000000}]}}`))
	if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 2 {
		t.Fatalf("fractional quota %+v %v", q, err)
	}
	five := q.Groups[0].Buckets[0]
	// Exact: (100.5-1.9)/100.5 = 0.9810…; reported 98/100 = 0.98.
	if five.Window != "5h" || five.RemainingFraction != .98 || five.Description == "" {
		t.Fatalf("fractional 5h not conservative: %+v", five)
	}
	weekly := q.Groups[0].Buckets[1]
	// Exact: (200-60.4)/200 = 0.698; reported 139/200 = 0.695.
	if weekly.Window != "weekly" || weekly.RemainingFraction != .695 || weekly.Description == "" {
		t.Fatalf("fractional weekly not conservative: %+v", weekly)
	}
	if len(q.Summary) != 2 || q.Summary[0].Value != 98 || q.Summary[1].Value != 139 {
		t.Fatalf("fractional summary %+v", q.Summary)
	}
	// Near-total consumption truncates to exhausted, never to full.
	empty, err := parseQuota([]byte(`{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":1,"currentValue":0.9}]}}`))
	if err != nil || len(empty.Groups[0].Buckets) != 1 || empty.Groups[0].Buckets[0].RemainingFraction != 0 {
		t.Fatalf("near-exhausted fractional not exhausted: %+v %v", empty, err)
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
