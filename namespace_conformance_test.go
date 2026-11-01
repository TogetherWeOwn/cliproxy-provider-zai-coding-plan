package main

import (
	"context"
	"testing"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// Every id this plugin can emit carries the fixed "zai/" namespace exactly
// once: the static list is served unchanged by the registrar, the static
// provider and the per-auth discovery path (no subscription exists, so this
// stays code and tests only).
func TestModelNamespaceConformance(t *testing.T) {
	ctx := context.Background()
	var all []pluginapi.ModelInfo
	all = append(all, models()...)

	reg, err := catalog{}.RegisterModels(ctx, pluginapi.ModelRegistrationRequest{})
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, reg.Models...)

	static, err := catalog{}.StaticModels(ctx, pluginapi.StaticModelRequest{})
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, static.Models...)

	discovered, err := catalog{}.ModelsForAuth(ctx, pluginapi.AuthModelRequest{})
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, discovered.Models...)

	if len(all) == 0 {
		t.Fatal("no models to check")
	}
	kit.AssertModelNamespace(t, "zai", all)
}
