package llm

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	kiloHelperEnv   = "SWARMINATOR_KILO_HELPER"
	kiloScenarioEnv = "SWARMINATOR_KILO_SCENARIO"
)

// TestKiloProviderMockHelper is the subprocess entry point for kilo mock tests.
// When the test binary is invoked as:
//
//	exe -test.run=^TestKiloProviderMockHelper$ run --format json [-m <model>] <message>
//
// flag.Parse() stops at "run" (first non-flag arg), so flag.Args() holds everything
// after the test flags.
func TestKiloProviderMockHelper(t *testing.T) {
	if os.Getenv(kiloHelperEnv) != "1" {
		return
	}

	// Parse: run --format json [-m <model>] <message>
	args := flag.Args()
	message := ""
	model := ""
	i := 0
	if i < len(args) && args[i] == "run" {
		i++
	}
	if i+1 < len(args) && args[i] == "--format" {
		i += 2 // skip "--format" and "json"
	}
	if i+1 < len(args) && args[i] == "-m" {
		model = args[i+1]
		i += 2
	}
	if i < len(args) {
		message = args[i]
	}

	if err := runMockKiloAgent(os.Getenv(kiloScenarioEnv), model, message); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func newMockKiloProvider(t *testing.T, scenario string) *KiloProvider {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Setenv(kiloHelperEnv, "1")
	t.Setenv(kiloScenarioEnv, scenario)

	return &KiloProvider{
		binary: exe,
		args:   []string{"-test.run=^TestKiloProviderMockHelper$"},
	}
}

func writeKiloNDJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

func kiloTextEvent(text string) map[string]any {
	return map[string]any{
		"type": "text",
		"part": map[string]any{"type": "text", "text": text},
	}
}

func runMockKiloAgent(scenario, model, message string) error {
	switch scenario {
	case "complete":
		_ = writeKiloNDJSON(map[string]any{"type": "step_start"})
		_ = writeKiloNDJSON(kiloTextEvent("hello"))
		return writeKiloNDJSON(map[string]any{"type": "step_finish"})

	case "multi-text":
		_ = writeKiloNDJSON(map[string]any{"type": "step_start"})
		_ = writeKiloNDJSON(kiloTextEvent("hello "))
		_ = writeKiloNDJSON(kiloTextEvent("world"))
		return writeKiloNDJSON(map[string]any{"type": "step_finish"})

	case "empty":
		_ = writeKiloNDJSON(map[string]any{"type": "step_start"})
		return writeKiloNDJSON(map[string]any{"type": "step_finish"})

	case "noise":
		_ = writeKiloNDJSON(map[string]any{"type": "step_start"})
		_, _ = fmt.Fprintln(os.Stdout, `not json at all{`)
		_ = writeKiloNDJSON(map[string]any{"type": "tool_call", "part": map[string]any{"type": "tool-call"}})
		_ = writeKiloNDJSON(kiloTextEvent("real content"))
		return writeKiloNDJSON(map[string]any{"type": "step_finish"})

	case "echo-message":
		// Echo the received command-line message back as the response text,
		// allowing tests to verify how the message was constructed.
		return writeKiloNDJSON(kiloTextEvent(message))

	case "echo-model":
		// Echo the received -m model value back as the response text.
		return writeKiloNDJSON(kiloTextEvent(model))

	default:
		return fmt.Errorf("unknown kilo mock scenario %q", scenario)
	}
}

func TestKiloProviderComplete(t *testing.T) {
	provider := newMockKiloProvider(t, "complete")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "Say hello"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "hello" {
		t.Fatalf("Complete returned %q, want %q", resp, "hello")
	}
}

func TestKiloProviderMultipleTextEvents(t *testing.T) {
	provider := newMockKiloProvider(t, "multi-text")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "Multi text"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "hello world" {
		t.Fatalf("Complete returned %q, want %q", resp, "hello world")
	}
}

func TestKiloProviderEmptyResponse(t *testing.T) {
	provider := newMockKiloProvider(t, "empty")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "Empty response"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "" {
		t.Fatalf("Complete returned %q, want empty string", resp)
	}
}

func TestKiloProviderIgnoresNoise(t *testing.T) {
	provider := newMockKiloProvider(t, "noise")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "With noise"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "real content" {
		t.Fatalf("Complete returned %q, want %q", resp, "real content")
	}
}

func TestKiloProviderPersonaFormatting(t *testing.T) {
	provider := newMockKiloProvider(t, "echo-message")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{
		Persona: "architect",
		Input:   "Design a system",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	const expected = "PERSONA: architect\n\nINPUT: Design a system"
	if resp != expected {
		t.Fatalf("Complete returned %q, want %q", resp, expected)
	}
}

func TestKiloProviderNoPersonaFormatting(t *testing.T) {
	provider := newMockKiloProvider(t, "echo-message")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "just the input"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "just the input" {
		t.Fatalf("Complete returned %q, want %q", resp, "just the input")
	}
}

func TestKiloProviderBinaryNotFound(t *testing.T) {
	provider := &KiloProvider{binary: "definitely-not-a-real-kilo-binary-for-tests"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, CompletionRequest{Input: "test"})
	if err == nil {
		t.Fatal("Complete error = nil, want start failure")
	}
	if !strings.Contains(err.Error(), "failed to start definitely-not-a-real-kilo-binary-for-tests") {
		t.Fatalf("Complete error = %q, want start failure message", err)
	}
}

func TestKiloProviderModelPassthrough(t *testing.T) {
	provider := newMockKiloProvider(t, "echo-model")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{
		Model: "kilo/kilo-auto/free",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	const want = "kilo/kilo-auto/free"
	if resp != want {
		t.Fatalf("Complete returned %q, want model echoed as %q", resp, want)
	}
}

func TestKiloProviderNoModelOmitsMFlag(t *testing.T) {
	// When req.Model is empty, no -m flag should be passed; the mock echoes "" for model.
	provider := newMockKiloProvider(t, "echo-model")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "hello"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "" {
		t.Fatalf("Complete returned %q, want empty string (no -m flag passed)", resp)
	}
}
