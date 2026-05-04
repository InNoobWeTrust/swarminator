package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const runCodexTestEnv = "SWARMINATOR_RUN_CODEX_TEST"

func TestCodexProviderGreeting(t *testing.T) {
	if os.Getenv(runCodexTestEnv) != "1" {
		t.Skipf("set %s=1 to run Codex integration test (requires codex binary with valid auth)", runCodexTestEnv)
	}

	provider := NewCodexProvider("codex")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := provider.Complete(ctx, CompletionRequest{
		Input: "Say exactly the word: hello",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("codex greeting failed after %s: %v", elapsed, err)
	}
	if resp == "" {
		t.Fatalf("codex returned empty response after %s", elapsed)
	}

	t.Logf("codex response after %s: %q", elapsed, resp)

	// The response should contain "hello" somewhere (case-insensitive).
	if !strings.Contains(strings.ToLower(resp), "hello") {
		t.Errorf("codex response %q does not contain expected greeting", resp)
	}
}

func TestCodexProviderModelRouting(t *testing.T) {
	if os.Getenv(runCodexTestEnv) != "1" {
		t.Skipf("set %s=1 to run Codex integration test (requires codex binary with valid auth)", runCodexTestEnv)
	}

	// Verify that explicit --agent=codex routes a GPT model through the Codex provider.
	up := NewUnifiedProvider("", "codex")
	if err := up.DetectAgents(); err != nil {
		t.Fatalf("DetectAgents() error = %v", err)
	}

	agents := up.registry.GetAllAvailable()
	var codexFound bool
	for _, a := range agents {
		if a.Name == "codex" {
			codexFound = true
			t.Logf("codex agent: %s", a)
		}
	}
	if !codexFound {
		t.Fatal("codex agent not found in available agents after DetectAgents()")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := up.Complete(ctx, CompletionRequest{
		Model: "gpt-4o",
		Input: "Say exactly the word: hello",
	})
	if err != nil {
		t.Fatalf("UnifiedProvider.Complete() with explicit --agent=codex error = %v", err)
	}
	t.Logf("codex via explicit override response: %q", resp)
}
