package llm

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"swarminator/internal/domain/agent"
)

type CommandCodeProvider struct {
	binary string
	args   []string
}

func NewCommandCodeProvider(binary string) LLMAdapter {
	return &CommandCodeProvider{binary: binary}
}

func (p *CommandCodeProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

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