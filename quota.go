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
	request := pluginapi.HTTPRequest{Method: http.MethodGet, URL: strings.TrimRight(base, "/") + "/api/monitor/usage/quota/limit", Headers: http.Header{"Authorization": {"Bearer " + key}, "Accept": {"application/json"}}}
	resp, err := client.Do(ctx, request)
	if err != nil && kit.MapError(err).HTTPStatus == 401 {
		request.Headers.Set("Authorization", key)
		resp, err = client.Do(ctx, request)
	}
	if err != nil {
		return pluginapi.QuotaFetchResponse{}, err
	}
	return parseQuota(resp.Body)
}
func intNumber(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(n.String())
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return 0, false
	}
	return r.Num().Int64(), true
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
			window = "5h"
		} else if unit == 6 && num == 1 {
			window = "weekly"
		}
		if window == "" || seen[window] {
			continue
		}
		usage, ok := intNumber(row.Usage)
		if !ok || usage <= 0 {
			continue
		}
		used, ok := intNumber(row.CurrentValue)
		if !ok || used < 0 || used > usage {
			continue
		}
		bucket := pluginapi.QuotaBucket{Window: window, RemainingFraction: float64(usage-used) / float64(usage)}
		if len(row.NextResetTime) > 0 && string(row.NextResetTime) != "null" {
			ms, ok := intNumber(row.NextResetTime)
			if !ok || ms <= 0 {
				continue
			}
			bucket.ResetTime = time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
		}
		seen[window] = true
		group.Buckets = append(group.Buckets, bucket)
		out.Summary = append(out.Summary, pluginapi.QuotaMetric{Key: "credits_remaining_" + window, Label: "Credits remaining " + window, Value: float64(usage - used), Unit: "credits", Format: "number"})
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
