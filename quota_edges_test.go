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
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != "Weekly limit" || q.Groups[0].Buckets[0].RemainingFraction != 1 || q.Groups[0].Buckets[0].ResetTime != "" {
			t.Fatalf("weekly number=%d idle reset lost: %+v %v", number, q, err)
		}
	}
}

func TestQuotaInvalidRowsDoNotHideValidWindow(t *testing.T) {
	for _, invalid := range []string{
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":-1,"currentValue":0}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":101}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":-1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100.5,"currentValue":1}`,
		`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":100,"currentValue":1,"nextResetTime":-1}`,
		`{"type":"CREDIT_LIMIT","unit":6,"number":2,"usage":100,"currentValue":1}`,
		`{"type":"TIME_LIMIT","unit":3,"number":5,"usage":100,"currentValue":1}`,
	} {
		q, err := parseQuota([]byte(`{"success":true,"data":{"limits":[` + invalid + `,{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":200,"currentValue":60,"nextResetTime":1893456000000}]}}`))
		if err != nil || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 1 || q.Groups[0].Buckets[0].Window != "Weekly limit" || q.Groups[0].Buckets[0].RemainingFraction != .7 || q.Groups[0].Buckets[0].ResetTime != "2030-01-01T00:00:00Z" || len(q.Summary) != 0 {
			t.Fatalf("partial quota %+v %v", q, err)
		}
	}
}
