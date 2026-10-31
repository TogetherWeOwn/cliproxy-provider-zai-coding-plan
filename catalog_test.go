package main

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestRegistrarAndStaticCatalogNamespaces(t *testing.T) {
	ctx := context.Background()
	registered, err := (catalog{}).RegisterModels(ctx, pluginapi.ModelRegistrationRequest{})
	if err != nil || registered.Provider != "zai-coding-plan" {
		t.Fatalf("registrar %+v %v", registered, err)
	}
	static, err := (catalog{}).StaticModels(ctx, pluginapi.StaticModelRequest{})
	if err != nil || static.Provider != "zai-coding-plan" {
		t.Fatalf("static catalog %+v %v", static, err)
	}
	for _, list := range [][]pluginapi.ModelInfo{registered.Models, static.Models} {
		if len(list) != 2 || list[0].ID != "zai/glm-5.3" || list[1].ID != "zai/glm-5.3-flash" {
			t.Fatalf("unprefixed or missing registered models: %+v", list)
		}
	}
}
