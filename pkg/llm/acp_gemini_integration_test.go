package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

const runGeminiACPTestEnv = "SWARMINATOR_RUN_GEMINI_ACP_TEST"

func TestACPProviderGeminiGreeting(t *testing.T) {
	if os.Getenv(runGeminiACPTestEnv) != "1" {
		t.Skipf("set %s=1 to run Gemini ACP integration test", runGeminiACPTestEnv)
	}

	provider := NewACPProvider("gemini", "--acp")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := provider.Complete(ctx, CompletionRequest{
		Input: "Hello, just say hi and nothing else",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("gemini ACP greeting failed after %s: %v", elapsed, err)
	}

	t.Logf("gemini ACP response after %s: %q", elapsed, resp)
}
