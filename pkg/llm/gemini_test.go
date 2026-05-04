package llm

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	geminiHelperEnv   = "SWARMINATOR_GEMINI_HELPER"
	geminiScenarioEnv = "SWARMINATOR_GEMINI_SCENARIO"
)

// TestGeminiProviderMockHelper is the subprocess entry point for Gemini mock tests.
// When the test binary is invoked as:
//
//	exe -test.run=^TestGeminiProviderMockHelper$ --prompt <message> [--model <model>] [--approval-mode <mode>] --output-format json
//
// flag.Parse() stops at the first non-flag arg, so flag.Args() holds everything after test flags.
func TestGeminiProviderMockHelper(t *testing.T) {
	if os.Getenv(geminiHelperEnv) != "1" {
		return
	}

	scenario := os.Getenv(geminiScenarioEnv)

	// flag.Parse() has already been called by the Go test harness before any
	// test function runs. The "--" sentinel in args causes everything after it
	// to be left in flag.Args() as positional arguments. Parse them here with a
	// fresh FlagSet so we never attempt to re-call flag.Parse().
	fs := flag.NewFlagSet("gemini-mock", flag.ExitOnError)
	var prompt, model, approvalMode, outputFormat string
	fs.StringVar(&prompt, "prompt", "", "")
	fs.StringVar(&model, "model", "", "")
	fs.StringVar(&approvalMode, "approval-mode", "", "")
	fs.StringVar(&outputFormat, "output-format", "", "")
	if err := fs.Parse(flag.Args()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := runMockGeminiAgent(scenario, model, approvalMode, prompt); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func newMockGeminiProvider(t *testing.T, scenario string) *GeminiProvider {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Setenv(geminiHelperEnv, "1")
	t.Setenv(geminiScenarioEnv, scenario)

	return &GeminiProvider{
		binary: exe,
		// "-test.run=..." routes to the mock helper; "--" makes everything after it
		// positional in flag.Args() so flag.Parse() doesn't reject the gemini flags.
		args: []string{"-test.run=^TestGeminiProviderMockHelper$", "--"},
	}
}

func runMockGeminiAgent(scenario, model, approvalMode, message string) error {
	switch scenario {
	case "complete":
		resp := `{"response": "Hello from Gemini!"}`
		_, err := fmt.Fprintln(os.Stdout, resp)
		return err

	case "persona-formatting":
		// Echo the message back so we can verify PERSONA/INPUT formatting.
		_, err := fmt.Fprintln(os.Stdout, fmt.Sprintf(`{"response": %q}`, message))
		return err

	case "model-normalization":
		// Echo the model value back.
		_, err := fmt.Fprintln(os.Stdout, fmt.Sprintf(`{"response": %q}`, model))
		return err

	case "approval-mode":
		// Echo the approval mode back.
		_, err := fmt.Fprintln(os.Stdout, fmt.Sprintf(`{"response": %q}`, approvalMode))
		return err

	case "structured-error":
		// Return a structured error in JSON.
		_, err := fmt.Fprintln(os.Stdout, `{"error": "Authentication failed", "response": ""}`)
		return err

	case "empty-response":
		_, err := fmt.Fprintln(os.Stdout, `{"response": ""}`)
		return err

	case "non-json-output":
		_, err := fmt.Fprintln(os.Stdout, "just some text")
		return err

	case "invalid-agent-mode":
		// Simulate Gemini rejecting an invalid agent-mode.
		_, err := fmt.Fprintln(os.Stdout, `{"error": "Invalid approval mode: badmode"}`)
		return err

	default:
		return fmt.Errorf("unknown gemini mock scenario %q", scenario)
	}
}

func TestGeminiProviderComplete(t *testing.T) {
	provider := newMockGeminiProvider(t, "complete")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "Say hello"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "Hello from Gemini!" {
		t.Fatalf("Complete returned %q, want %q", resp, "Hello from Gemini!")
	}
}

func TestGeminiProviderPersonaFormatting(t *testing.T) {
	provider := newMockGeminiProvider(t, "persona-formatting")
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

func TestGeminiProviderNoPersonaFormatting(t *testing.T) {
	provider := newMockGeminiProvider(t, "persona-formatting")
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

func TestGeminiProviderModelNormalization(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "google/ prefix stripped", model: "google/gemini-2.5-pro", want: "gemini-2.5-pro"},
		{name: "gemini/ prefix stripped", model: "gemini/flash", want: "flash"},
		{name: "gemini- prefix unchanged", model: "gemini-2.5-flash", want: "gemini-2.5-flash"},
		{name: "bare alias unchanged", model: "flash", want: "flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newMockGeminiProvider(t, "model-normalization")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := provider.Complete(ctx, CompletionRequest{
				Model: tt.model,
				Input: "hello",
			})
			if err != nil {
				t.Fatalf("Complete returned error: %v", err)
			}
			if resp != tt.want {
				t.Fatalf("Complete returned model %q, want %q", resp, tt.want)
			}
		})
	}
}

func TestGeminiProviderAgentModeMapping(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantMode string
	}{
		{name: "default passthrough", mode: "default", wantMode: "default"},
		{name: "plan passthrough", mode: "plan", wantMode: "plan"},
		{name: "yolo passthrough", mode: "yolo", wantMode: "yolo"},
		{name: "autoEdit maps to auto_edit", mode: "autoEdit", wantMode: "auto_edit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newMockGeminiProvider(t, "approval-mode")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := provider.Complete(ctx, CompletionRequest{
				Input:     "hello",
				AgentMode: tt.mode,
			})
			if err != nil {
				t.Fatalf("Complete returned error: %v", err)
			}
			if resp != tt.wantMode {
				t.Fatalf("Complete returned mode %q, want %q", resp, tt.wantMode)
			}
		})
	}
}

func TestGeminiProviderInvalidAgentMode(t *testing.T) {
	provider := newMockGeminiProvider(t, "invalid-agent-mode")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, CompletionRequest{
		Input:     "hello",
		AgentMode: "badmode",
	})
	if err == nil {
		t.Fatal("Complete error = nil, want error for invalid agent-mode")
	}
	if !strings.Contains(err.Error(), "badmode") {
		t.Fatalf("Complete error = %q, want it to mention the invalid mode", err.Error())
	}
}

func TestGeminiProviderStructuredError(t *testing.T) {
	provider := newMockGeminiProvider(t, "structured-error")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, CompletionRequest{Input: "hello"})
	if err == nil {
		t.Fatal("Complete error = nil, want structured error")
	}
	if !strings.Contains(err.Error(), "Authentication failed") {
		t.Fatalf("Complete error = %q, want it to mention the structured error", err.Error())
	}
}

func TestGeminiProviderEmptyResponse(t *testing.T) {
	provider := newMockGeminiProvider(t, "empty-response")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "hello"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "" {
		t.Fatalf("Complete returned %q, want empty string", resp)
	}
}

func TestGeminiProviderNonJSONOutput(t *testing.T) {
	provider := newMockGeminiProvider(t, "non-json-output")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{Input: "hello"})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "just some text" {
		t.Fatalf("Complete returned %q, want %q", resp, "just some text")
	}
}

func TestGeminiProviderBinaryNotFound(t *testing.T) {
	provider := &GeminiProvider{binary: "definitely-not-a-real-gemini-binary-for-tests"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, CompletionRequest{Input: "test"})
	if err == nil {
		t.Fatal("Complete error = nil, want start failure")
	}
	if !strings.Contains(err.Error(), "failed to start definitely-not-a-real-gemini-binary-for-tests") {
		t.Fatalf("Complete error = %q, want start failure message", err.Error())
	}
}

func TestGeminiProviderContextCancellation(t *testing.T) {
	provider := newMockGeminiProvider(t, "complete")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Cancel the context immediately.
	cancel()

	_, err := provider.Complete(ctx, CompletionRequest{Input: "hello"})
	if err == nil {
		t.Fatal("Complete error = nil, want context cancelled error")
	}
	if err != context.Canceled {
		t.Fatalf("Complete error = %v, want context.Canceled", err)
	}
}

func TestNormalizeGeminiModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "google/ prefix stripped", input: "google/gemini-2.5-pro", want: "gemini-2.5-pro"},
		{name: "gemini/ prefix stripped", input: "gemini/flash", want: "flash"},
		{name: "gemini- prefix unchanged", input: "gemini-2.5-flash", want: "gemini-2.5-flash"},
		{name: "bare alias unchanged", input: "flash", want: "flash"},
		{name: "pro alias unchanged", input: "pro", want: "pro"},
		{name: "auto alias unchanged", input: "auto", want: "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGeminiModel(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeGeminiModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapAgentMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    string
		wantErr bool
	}{
		{name: "empty", mode: "", want: "", wantErr: false},
		{name: "default", mode: "default", want: "default", wantErr: false},
		{name: "plan", mode: "plan", want: "plan", wantErr: false},
		{name: "yolo", mode: "yolo", want: "yolo", wantErr: false},
		{name: "autoEdit", mode: "autoEdit", want: "auto_edit", wantErr: false},
		{name: "invalid", mode: "badmode", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapAgentMode(tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("mapAgentMode(%q) error = nil, want error", tt.mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("mapAgentMode(%q) error = %v, want nil", tt.mode, err)
			}
			if got != tt.want {
				t.Fatalf("mapAgentMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}
