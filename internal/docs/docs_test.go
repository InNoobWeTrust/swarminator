package docs_test

import (
	"strings"
	"testing"

	"swarminator/internal/docs"
)

func TestEmbeddedReferenceNonEmpty(t *testing.T) {
	ref := docs.EmbeddedReference()
	if ref == "" {
		t.Fatal("EmbeddedReference() returned empty string")
	}
}

func TestEmbeddedReferenceKeyContent(t *testing.T) {
	ref := docs.EmbeddedReference()
	for _, want := range []string{"Usage", "Flags", "Rules and Exit Codes", "--list-models", "swarm-intelligence", "There is no global default kilo fallback", "swarm start", "runs wait", "Private Swarm Protocol", "transport-native tools", "Final answers are plain-text Markdown"} {
		if !strings.Contains(ref, want) {
			t.Errorf("EmbeddedReference() missing expected section/content: %q", want)
		}
	}
}

func TestEmbeddedReferenceAgentMode(t *testing.T) {
	ref := docs.EmbeddedReference()
	for _, want := range []string{"--agent-mode", "explicit-only", "github-copilot/", "headless"} {
		if !strings.Contains(ref, want) {
			t.Errorf("EmbeddedReference() missing expected content: %q", want)
		}
	}
}
