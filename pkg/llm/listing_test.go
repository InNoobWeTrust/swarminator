package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDiscoverListingsFiltersByAgent(t *testing.T) {
	groups := DiscoverListings(context.Background(), "gemini")
	if len(groups) != 1 {
		t.Fatalf("len(DiscoverListings(filter)) = %d, want 1", len(groups))
	}
	if groups[0].Engine != "gemini" {
		t.Fatalf("DiscoverListings(filter)[0].Engine = %q, want %q", groups[0].Engine, "gemini")
	}
}

func TestDiscoverListingsJSONShape(t *testing.T) {
	payload := struct {
		Groups []EngineListing `json:"groups"`
	}{Groups: DiscoverListings(context.Background(), "command-code")}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{"groups", "engine", "providers", "model_source"} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON payload missing %q in %s", want, text)
		}
	}
}

func TestFormatDiscoveryTextIncludesGrouping(t *testing.T) {
	text := FormatDiscoveryText([]EngineListing{{
		Engine: "gemini",
		Providers: []ProviderListing{{
			Name:        "gemini",
			Binary:      "gemini",
			Available:   true,
			Source:      "embedded",
			ModelSource: "embedded",
			Models:      []ModelListing{{Name: "google/gemini-2.5-flash", Source: "embedded"}},
		}},
	}}, true, true)
	for _, want := range []string{"ENGINE gemini", "PROVIDER gemini", "google/gemini-2.5-flash"} {
		if !strings.Contains(text, want) {
			t.Fatalf("FormatDiscoveryText() missing %q in %q", want, text)
		}
	}
}
