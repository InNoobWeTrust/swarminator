package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// KiloProvider uses kilo's one-shot run mode (kilo run --format json) to process completions.
// This avoids the need for ACP stdio transport, which kilo does not support.
// kilo acp starts an HTTP server instead of speaking JSON-RPC over stdio.
type KiloProvider struct {
	binary string
	args   []string // prepended before "run --format json <message>"; used in tests
}

// NewKiloProvider creates a Provider that runs "kilo run --format json".
func NewKiloProvider(binary string) Provider {
	return &KiloProvider{binary: binary}
}

// kiloRunEvent is one line of kilo's newline-delimited JSON (NDJSON) output.
type kiloRunEvent struct {
	Type string      `json:"type"`
	Part kiloRunPart `json:"part"`
}

// kiloRunPart is the "part" field inside a kiloRunEvent.
type kiloRunPart struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

func (p *KiloProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	allArgs := make([]string, 0, len(p.args)+6)
	allArgs = append(allArgs, p.args...)
	allArgs = append(allArgs, "run", "--format", "json")
	// Pass the model through to kilo when specified. kilo expects the full
	// provider-qualified ID (e.g. "kilo/kilo-auto/free", "kilo/anthropic/claude-sonnet-4").
	if req.Model != "" {
		allArgs = append(allArgs, "-m", req.Model)
	}
	allArgs = append(allArgs, message)

	cmd := exec.CommandContext(ctx, p.binary, allArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to open stdout for %s: %w", p.binary, err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start %s: %w", p.binary, err)
	}

	var responseText strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			// Context cancelled between lines; clean up and surface the error.
			waitForProcessExit(cmd, acpProcessWaitTimeout)
			return responseText.String(), ctx.Err()
		default:
		}

		var event kiloRunEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "text" {
			responseText.WriteString(event.Part.Text)
		}
	}

	if err := scanner.Err(); err != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return "", fmt.Errorf("reading %s output: %w", p.binary, err)
	}

	// When context is cancelled while scanner.Scan() is blocking, CommandContext
	// kills the process which closes stdout, causing Scan() to return false (EOF)
	// without surfacing the context error. Check explicitly here.
	if ctx.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return responseText.String(), ctx.Err()
	}

	// All stdout consumed; the process has exited (EOF implies the write-end closed).
	// Capture the exit status and surface it when no response was collected —
	// this covers auth failures, network errors, and other non-zero exits from kilo.
	exitErr := cmd.Wait()
	if responseText.Len() == 0 && exitErr != nil {
		return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
	}

	return responseText.String(), nil
}
