package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// TestQuotaPollUsesConfiguredProxy uses a stand-in HTTP proxy that refuses
// every CONNECT, so the test records the route without reaching Z.ai.
func TestQuotaPollUsesConfiguredProxy(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.Host)
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	defer proxy.Close()
	client := kit.NewQuotaHTTPClient(nil)
	defer client.CloseIdleConnections()
	for _, r := range []pluginapi.QuotaFetchRequest{
		{AuthID: "auth-proxy", StorageJSON: []byte(`{"api_key":"one"}`), Metadata: map[string]any{"proxy_url": proxy.URL}},
		{AuthID: "host-proxy", StorageJSON: []byte(`{"api_key":"two","region":"cn"}`), Host: pluginapi.HostConfigSummary{ProxyURL: proxy.URL}},
	} {
		if _, err := newQuota(client).FetchQuota(context.Background(), r); err == nil {
			t.Fatalf("%s: refused proxy should fail the poll", r.AuthID)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "CONNECT api.z.ai:443" || seen[1] != "CONNECT open.bigmodel.cn:443" {
		t.Fatalf("proxy saw %v", seen)
	}
}
