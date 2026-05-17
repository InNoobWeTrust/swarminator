package llm

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommandCodeProvider runs the command-code CLI in one-shot mode and returns plain text output.
type CommandCodeProvider struct {
	binary string
	args   []string
}

// NewCommandCodeProvider creates a Provider backed by the command-code binary.
func NewCommandCodeProvider(binary string) Provider {
	return &CommandCodeProvider{binary: binary}
}

func (p *CommandCodeProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	// CommandCode headless mode: cmd --print "message"
	// The -m model flag is accepted by swarminator but cmd uses its session model;
	// we ignore it here (no --model flag supported in headless mode).
	allArgs := make([]string, 0, len(p.args)+2)
	allArgs = append(allArgs, p.args...)
	allArgs = append(allArgs, "--print", message)

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
			waitForProcessExit(cmd, acpProcessWaitTimeout)
			return responseText.String(), ctx.Err()
		default:
		}
		responseText.WriteString(scanner.Text())
		responseText.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return "", fmt.Errorf("reading %s output: %w", p.binary, err)
	}

	if ctx.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return responseText.String(), ctx.Err()
	}

	exitErr := cmd.Wait()
	result := strings.TrimSpace(responseText.String())
	if result == "" && exitErr != nil {
		return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
	}

	return result, nil
}
