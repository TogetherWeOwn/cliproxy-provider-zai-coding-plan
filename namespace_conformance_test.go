package main

import (
	"context"
	"testing"

	kit "github.com/TogetherWeOwn/cliproxy-provider-kit"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// Every id this plugin can emit carries the fixed "zai/" namespace exactly
// once: the static-only ModelCatalog serves the same list through the
// registrar, the static provider and the per-auth path. Discovery is
// unverified (Fetch nil), so per-auth currently serves static; the
// conformance holds for all three serving paths.
func TestModelNamespaceConformance(t *testing.T) {
	ctx := context.Background()
	var all []pluginapi.ModelInfo

	reg, err := modelCatalog.RegisterModels(ctx, pluginapi.ModelRegistrationRequest{Plugin: pluginapi.Metadata{Name: provider}})
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, reg.Models...)

	static, err := modelCatalog.StaticModels(ctx, pluginapi.StaticModelRequest{Plugin: pluginapi.Metadata{Name: provider}})
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, static.Models...)

	perAuth, err := modelCatalog.ModelsForAuth(ctx, pluginapi.AuthModelRequest{Plugin: pluginapi.Metadata{Name: provider}, AuthProvider: provider, StorageJSON: []byte(`{"api_key":"example-key"}`)})
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, perAuth.Models...)

	if len(all) == 0 {
		t.Fatal("no models to check")
	}
	kit.AssertModelNamespace(t, "zai", all)
}
