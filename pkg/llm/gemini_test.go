package llm

import (
	"context"
	"strings"
	"testing"
)

func TestGeminiProviderRejectsMissingAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	_, err := NewGeminiProvider().Complete(context.Background(), CompletionRequest{
		Model:   "gemini-2.5-flash",
		Persona: "You are concise.",
		Input:   "ROLE: reviewer\n\nhello",
	})
	if err == nil {
		t.Fatal("Complete() error = nil, want missing API key error")
	}
	if !strings.Contains(err.Error(), "GOOGLE_API_KEY is required") {
		t.Fatalf("Complete() error = %q, want API key error", err.Error())
	}
}
