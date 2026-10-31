package main

import (
	"encoding/json"
	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func classifyResponse(r pluginapi.HTTPResponse) error { return classifyError(r, time.Now()) }
func classifyError(r pluginapi.HTTPResponse, now time.Time) *pluginabi.Error {
	var payload struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Next    json.RawMessage `json:"next_flush_time"`
			Message string          `json:"message"`
		} `json:"error"`
		Next json.RawMessage `json:"next_flush_time"`
	}
	_ = json.Unmarshal(r.Body, &payload)
	s := strings.Trim(string(payload.Error.Code), `"`)
	code, _ := strconv.Atoi(s)
	e := &pluginabi.Error{Code: "upstream_error", Message: "upstream request failed", HTTPStatus: r.StatusCode, Retryable: r.StatusCode == 429 || r.StatusCode >= 500, RetryAfterSeconds: kit.RetryAfterSeconds(r.Headers.Get("Retry-After"), now)}
	switch code {
	case 1001, 1003:
		e.Code = "invalid_credentials"
		e.HTTPStatus = 401
		e.Retryable = false
		e.RetryAfterSeconds = 0
	case 1302, 1305:
		e.Code = "rate_limited"
		e.HTTPStatus = 429
		e.Retryable = true
		e.RetryAfterSeconds = 10
	case 1308, 1310, 1316, 1317, 1318, 1319, 1320, 1321:
		e.Code = "quota_exhausted"
		e.HTTPStatus = 429
		e.Retryable = true
		raw := payload.Error.Next
		if len(raw) == 0 {
			raw = payload.Next
		}
		if len(raw) == 0 {
			if match := resetInMessage.FindStringSubmatch(payload.Error.Message); len(match) == 2 {
				raw, _ = json.Marshal(match[1])
			}
		}
		if wait := resetDelay(raw, now); wait > 0 {
			e.RetryAfterSeconds = wait
		}
		if e.RetryAfterSeconds == 0 {
			e.RetryAfterSeconds = 60
		}
	case 1113, 1309, 1313:
		e.Code = "quota_unavailable"
		e.HTTPStatus = 429
		e.Retryable = false
		e.RetryAfterSeconds = 3600
	}
	if e.HTTPStatus < 400 || e.HTTPStatus > 599 {
		e.HTTPStatus = 502
	}
	return e
}

var resetInMessage = regexp.MustCompile(`(?i)next_flush_time["\s:=]+(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2})?|\d{10,13})`)

func resetDelay(raw json.RawMessage, now time.Time) int {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0
	}
	var t time.Time
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 100000000000 {
			t = time.UnixMilli(n)
		} else {
			t = time.Unix(n, 0)
		}
	} else {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
			var err error
			t, err = time.Parse(layout, s)
			if err == nil {
				break
			}
		}
	}
	if !t.After(now) {
		return 0
	}
	secs := math.Ceil(t.Sub(now).Seconds())
	if secs > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(secs)
}
