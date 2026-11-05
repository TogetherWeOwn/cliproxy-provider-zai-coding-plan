package main

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type quotaDoer interface {
	Do(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error)
}

func fetchQuota(ctx context.Context, base, key string, client quotaDoer) (pluginapi.QuotaFetchResponse, error) {
	if client == nil {
		c := kit.NewQuotaHTTPClient(nil)
		defer c.CloseIdleConnections()
		client = c
	}
	// Prefer the raw-key form used by the first-party usage plugin. Some
	// independent clients use Bearer instead; retry that form only on HTTP 401.
	request := pluginapi.HTTPRequest{Method: http.MethodGet, URL: strings.TrimRight(base, "/") + "/api/monitor/usage/quota/limit", Headers: http.Header{"Authorization": {key}, "Accept": {"application/json"}}}
	resp, err := client.Do(ctx, request)
	if err != nil && kit.MapError(err).HTTPStatus == 401 {
		request.Headers.Set("Authorization", "Bearer "+key)
		resp, err = client.Do(ctx, request)
	}
	if err != nil {
		return pluginapi.QuotaFetchResponse{}, err
	}
	return parseQuota(resp.Body)
}

// exactNumber bounds both the mantissa and exponent before allocating a rational.
// Decode numbers only, not quoted strings, and keep decimal rounding exact.
func exactNumber(raw json.RawMessage) (*big.Rat, bool) {
	s := strings.TrimSpace(string(raw))
	if len(s) == 0 || len(s) > 128 || s[0] != '-' && (s[0] < '0' || s[0] > '9') {
		return nil, false
	}
	var n json.Number
	if json.Unmarshal([]byte(s), &n) != nil {
		return nil, false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(s[i+1:])
		if err != nil || exponent < -128 || exponent > 128 {
			return nil, false
		}
	}
	return new(big.Rat).SetString(n.String())
}

func intNumber(raw json.RawMessage) (int64, bool) {
	r, ok := exactNumber(raw)
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return 0, false
	}
	return r.Num().Int64(), true
}

func creditCounts(capacity, consumed json.RawMessage) (int64, int64, bool) {
	limit, ok := exactNumber(capacity)
	if !ok || limit.Sign() <= 0 || limit.Cmp(new(big.Rat).SetInt64(1<<63-1)) > 0 {
		return 0, 0, false
	}
	used, ok := exactNumber(consumed)
	if !ok || used.Sign() < 0 || used.Cmp(limit) > 0 {
		return 0, 0, false
	}
	// Validate original values before rounding, so valid near-exhausted rows
	// survive while genuinely over-consumed rows remain unknown.
	wholeLimit := new(big.Int).Quo(limit.Num(), limit.Denom()).Int64()
	if wholeLimit == 0 {
		return 0, 0, false
	}
	wholeUsed, remainder := new(big.Int), new(big.Int)
	wholeUsed.QuoRem(used.Num(), used.Denom(), remainder)
	if remainder.Sign() != 0 {
		wholeUsed.Add(wholeUsed, big.NewInt(1))
	}
	return wholeLimit, min(wholeUsed.Int64(), wholeLimit), true
}
func parseQuota(raw []byte) (pluginapi.QuotaFetchResponse, error) {
	bad := func() (pluginapi.QuotaFetchResponse, error) {
		return pluginapi.QuotaFetchResponse{}, &pluginabi.Error{Code: "unknown_quota", Message: "unknown or invalid credit quota response", HTTPStatus: 502}
	}
	var v struct {
		Success *bool           `json:"success"`
		Code    json.RawMessage `json:"code"`
		Data    struct {
			Level    json.RawMessage `json:"level"`
			PlanName string          `json:"planName"`
			Limits   []struct {
				Type                                             string `json:"type"`
				Unit, Number, Usage, CurrentValue, NextResetTime json.RawMessage
			} `json:"limits"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Success != nil && !*v.Success {
		return bad()
	}
	if len(v.Code) > 0 {
		code, ok := intNumber(v.Code)
		if !ok || code != 0 && code != 200 {
			return bad()
		}
	}
	var out pluginapi.QuotaFetchResponse
	group := pluginapi.QuotaGroup{DisplayName: "Coding Plan credits"}
	seen := map[string]bool{}
	for _, row := range v.Data.Limits {
		if row.Type != "CREDIT_LIMIT" {
			continue
		}
		unit, ok := intNumber(row.Unit)
		if !ok {
			continue
		}
		num, ok := intNumber(row.Number)
		if !ok {
			continue
		}
		window := ""
		if unit == 3 && num == 5 {
			window = kit.CardWindowFiveHour
		} else if unit == 6 && (num == 1 || num == 7) {
			window = kit.CardWindowWeekly
		}
		if window == "" || seen[window] {
			continue
		}
		usage, used, ok := creditCounts(row.Usage, row.CurrentValue)
		if !ok {
			continue
		}
		bucket := pluginapi.QuotaBucket{Window: window, RemainingFraction: float64(usage-used) / float64(usage)}
		if len(row.NextResetTime) > 0 && string(row.NextResetTime) != "null" {
			ms, ok := intNumber(row.NextResetTime)
			if !ok || ms < 0 {
				continue
			}
			// Zero, like null, denotes an idle window with no known reset.
			if ms > 0 {
				bucket.ResetTime = time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
			}
		}
		seen[window] = true
		group.Buckets = append(group.Buckets, bucket)
	}
	if len(group.Buckets) == 0 {
		return bad()
	}
	out.Groups = []pluginapi.QuotaGroup{group}
	level := ""
	if json.Unmarshal(v.Data.Level, &level) != nil {
		if n, ok := intNumber(v.Data.Level); ok {
			level = strconv.FormatInt(n, 10)
		}
	}
	if v.Data.PlanName != "" || level != "" {
		out.Subscription = &pluginapi.QuotaSubscription{Plan: level, TierName: v.Data.PlanName}
	}
	return out, nil
}
