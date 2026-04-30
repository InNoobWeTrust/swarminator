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
	codexHelperEnv   = "SWARMINATOR_CODEX_HELPER"
	codexScenarioEnv = "SWARMINATOR_CODEX_SCENARIO"
)

// TestCodexProviderMockHelper is the subprocess entry point for codex mock tests.
// When the test binary is invoked as:
//
//	exe -test.run=^TestCodexProviderMockHelper$ exec --json --dangerously-bypass-approvals-and-sandbox [-m <model>] <message>
//
// flag.Parse() stops at "exec" (first non-flag arg), so flag.Args() holds everything
// after the test flags.
func TestCodexProviderMockHelper(t *testing.T) {
	if os.Getenv(codexHelperEnv) != "1" {
		return
	}

	// Parse: exec --json --dangerously-bypass-approvals-and-sandbox [-m <model>] <message>
	args := flag.Args()
	message := ""
	model := ""
	i := 0
	if i < len(args) && args[i] == "exec" {
		i++
	}
	for i < len(args) {
		switch args[i] {
		case "--json", "--dangerously-bypass-approvals-and-sandbox", "--full-auto":
			i++
		case "-m":
			if i+1 < len(args) {
				model = args[i+1]
				i += 2
			} else {
				i++
			}
		default:
			message = args[i]
			i++
		}
	}

	if err := runMockCodexAgent(os.Getenv(codexScenarioEnv), model, message); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func newMockCodexProvider(t *testing.T, scenario string) *CodexProvider {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Setenv(codexHelperEnv, "1")
	t.Setenv(codexScenarioEnv, scenario)

	return &CodexProvider{
		binary: exe,
		args:   []string{"-test.run=^TestCodexProviderMockHelper$"},
	}
}

func writeCodexNDJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

func codexAgentMessageEvent(id, text string) map[string]any {
	return map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   id,
			"type": "agent_message",
			"text": text,
		},
	}
}

func runMockCodexAgent(scenario, model, message string) error {
	switch scenario {
	case "complete":
		_ = writeCodexNDJSON(map[string]any{"type": "thread.started", "thread_id": "test-thread"})
		_ = writeCodexNDJSON(map[string]any{"type": "turn.started"})
		_ = writeCodexNDJSON(codexAgentMessageEvent("item_0", "hello"))
		return writeCodexNDJSON(map[string]any{"type": "turn.completed", "usage": map[string]any{"input_tokens": 10, "output_tokens": 1}})

	case "multi-message":
		// Simulates codex with intermediate reasoning then final answer.
		_ = writeCodexNDJSON(map[string]any{"type": "thread.started", "thread_id": "test-thread"})
		_ = writeCodexNDJSON(map[string]any{"type": "turn.started"})
		_ = writeCodexNDJSON(codexAgentMessageEvent("item_0", "I will help with that."))
		_ = writeCodexNDJSON(map[string]any{
			"type": "item.completed",
			"item": map[string]any{
				"id": "item_1", "type": "command_execution",
				"command": "/bin/sh -c echo hello", "aggregated_output": "hello\n", "exit_code": 0,
			},
		})
		_ = writeCodexNDJSON(codexAgentMessageEvent("item_2", "hello world"))
		return writeCodexNDJSON(map[string]any{"type": "turn.completed"})

	case "empty":
		_ = writeCodexNDJSON(map[string]any{"type": "thread.started", "thread_id": "test-thread"})
		_ = writeCodexNDJSON(map[string]any{"type": "turn.started"})
		return writeCodexNDJSON(map[string]any{"type": "turn.completed"})

	case "noise":
		_ = writeCodexNDJSON(map[string]any{"type": "thread.started", "thread_id": "test-thread"})
		_, _ = fmt.Fprintln(os.Stdout, `not json at all{`)
		_ = writeCodexNDJSON(map[string]any{"type": "turn.started"})
		_ = writeCodexNDJSON(map[string]any{"type": "item.completed", "item": map[string]any{
			"id": "item_0", "type": "command_execution", "aggregated_output": "some output",
		}})
		_ = writeCodexNDJSON(codexAgentMessageEvent("item_1", "real content"))
		return writeCodexNDJSON(map[string]any{"type": "turn.completed"})

	case "echo-message":
		return writeCodexNDJSON(codexAgentMessageEvent("item_0", message))

	case "echo-model":
		return writeCodexNDJSON(codexAgentMessageEvent("item_0", model))

	default:
		return fmt.Errorf("unknown codex mock scenario %q", scenario)
	}
}

func TestCodexProviderComplete(t *testing.T) {
	provider := newMockCodexProvider(t, "complete")
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

func TestCodexProviderMultipleAgentMessages(t *testing.T) {
	// Multiple agent_message items should be joined with "\n\n".
	provider := newMockCodexProvider(t, "multi-message")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "Multi"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	const want = "I will help with that.\n\nhello world"
	if resp != want {
		t.Fatalf("Complete returned %q, want %q", resp, want)
	}
}

func TestCodexProviderEmptyResponse(t *testing.T) {
	provider := newMockCodexProvider(t, "empty")
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

func TestCodexProviderIgnoresNoise(t *testing.T) {
	provider := newMockCodexProvider(t, "noise")
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

func TestCodexProviderPersonaFormatting(t *testing.T) {
	provider := newMockCodexProvider(t, "echo-message")
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

func TestCodexProviderNoPersonaFormatting(t *testing.T) {
	provider := newMockCodexProvider(t, "echo-message")
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

func TestCodexProviderBinaryNotFound(t *testing.T) {
	provider := &CodexProvider{binary: "definitely-not-a-real-codex-binary-for-tests"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, CompletionRequest{Input: "test"})
	if err == nil {
		t.Fatal("Complete error = nil, want start failure")
	}
	if !strings.Contains(err.Error(), "failed to start definitely-not-a-real-codex-binary-for-tests") {
		t.Fatalf("Complete error = %q, want start failure message", err)
	}
}

func TestCodexProviderModelPassthrough(t *testing.T) {
	provider := newMockCodexProvider(t, "echo-model")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{
		Model: "openai/o3",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	const want = "openai/o3"
	if resp != want {
		t.Fatalf("Complete returned %q, want model echoed as %q", resp, want)
	}
}

func TestCodexProviderNoModelOmitsMFlag(t *testing.T) {
	// When req.Model is empty, no -m flag should be passed; the mock echoes "" for model.
	provider := newMockCodexProvider(t, "echo-model")
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
