package main

import (
	"context"
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	testResourceBase = "/v0/resource/plugins/zai-coding-plan-id"
	testSubmitPath   = "/v0/management/plugin-login/zai-coding-plan"
	testBaseURL      = "http://127.0.0.1:19000/v0/management/oauth-callback"
)

// TestAPIKeyLoginWired drives the management-UI login flow end to end through
// the provider's own ParseAuth pipeline: page, key+region submit, poll.
func TestAPIKeyLoginWired(t *testing.T) {
	ctx := context.Background()
	p, err := authProvider()
	if err != nil {
		t.Fatal(err)
	}
	login := p.Login
	if login == nil {
		t.Fatal("API-key login not wired")
	}
	reg, err := login.RegisterManagement(ctx, pluginapi.ManagementRegistrationRequest{
		BasePath: "/v0/management", ResourceBasePath: testResourceBase})
	if err != nil {
		t.Fatal(err)
	}
	var sawSubmit, sawPage bool
	for _, r := range reg.Routes {
		if r.Method == "POST" && r.Path == "plugin-login/zai-coding-plan" {
			sawSubmit = true
		}
	}
	for _, r := range reg.Resources {
		if r.Path == "login" {
			sawPage = true
		}
	}
	if !sawSubmit || !sawPage {
		t.Fatalf("login routes not registered: %+v", reg)
	}
	start, err := p.StartLogin(ctx, pluginapi.AuthLoginStartRequest{Provider: "zai-coding-plan",
		BaseURL: testBaseURL, Host: pluginapi.HostConfigSummary{AuthDir: "/tmp/zai-auth"}})
	if err != nil {
		t.Fatal(err)
	}
	// Root-relative on purpose: an absolute loopback URL would hand a remote
	// operator's browser a link it cannot load; the panel opens the relative
	// URL against its own origin (the host, which serves the panel).
	if !strings.HasPrefix(start.URL, testResourceBase+"/login?state=") || strings.Contains(start.URL, "://") {
		t.Fatalf("unexpected login URL %q", start.URL)
	}
	poll, err := p.PollLogin(ctx, pluginapi.AuthLoginPollRequest{State: start.State})
	if err != nil || poll.Status != pluginapi.AuthLoginStatusPending {
		t.Fatalf("fresh login not pending: %+v %v", poll, err)
	}
	page, err := login.HandleManagement(ctx, pluginapi.ManagementRequest{Method: "GET",
		Path: testResourceBase + "/login", Query: map[string][]string{"state": {start.State}}})
	if err != nil || page.StatusCode != 200 {
		t.Fatalf("login page %+v %v", page, err)
	}
	body := html.UnescapeString(string(page.Body))
	for _, want := range []string{"read -r PLUGIN_FIELD_REGION", `"region":"$ESC_FIELD_REGION"`, "Region"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing region wiring %q", want)
		}
	}
	submit, err := login.HandleManagement(ctx, pluginapi.ManagementRequest{Method: "POST",
		Path: testSubmitPath,
		Body: []byte(`{"state":"` + start.State + `","api_key":"example-key","region":"cn"}`)})
	if err != nil || submit.StatusCode != 200 {
		t.Fatalf("submit %+v %v", submit, err)
	}
	done, err := p.PollLogin(ctx, pluginapi.AuthLoginPollRequest{State: start.State})
	if err != nil || done.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("poll %+v %v", done, err)
	}
	var store map[string]any
	if err := json.Unmarshal(done.Auth.StorageJSON, &store); err != nil {
		t.Fatal(err)
	}
	if done.Auth.Provider != "zai-coding-plan" || store["region"] != "cn" {
		t.Fatalf("region lost through login: %+v %s", done.Auth, done.Auth.StorageJSON)
	}
	// A second poll must not redeliver credentials.
	again, err := p.PollLogin(ctx, pluginapi.AuthLoginPollRequest{State: start.State})
	if err != nil || again.Status != pluginapi.AuthLoginStatusPending {
		t.Fatalf("consumed login redelivered: %+v %v", again, err)
	}
}
