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
	for _, want := range []string{"Usage", "Flags", "Rules and Exit Codes", "kilo/kilo-auto/free"} {
		if !strings.Contains(ref, want) {
			t.Errorf("EmbeddedReference() missing expected section/content: %q", want)
		}
	}
}
