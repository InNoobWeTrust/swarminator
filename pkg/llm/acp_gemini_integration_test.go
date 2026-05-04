package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

const runGeminiTestEnv = "SWARMINATOR_RUN_GEMINI_TEST"

func TestGeminiProviderHeadlessGreeting(t *testing.T) {
	if os.Getenv(runGeminiTestEnv) != "1" {
		t.Skipf("set %s=1 to run Gemini headless integration test", runGeminiTestEnv)
	}

	provider := NewGeminiProvider("gemini")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := provider.Complete(ctx, CompletionRequest{
		Input: "Hello, just say hi and nothing else",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("gemini headless greeting failed after %s: %v", elapsed, err)
	}

	t.Logf("gemini headless response after %s: %q", elapsed, resp)
}
