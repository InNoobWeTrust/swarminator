package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"swarminator/internal/domain/agent"
)

type KiloProvider struct {
	binary string
	args   []string
}

func NewKiloProvider(binary string) LLMAdapter {
	return &KiloProvider{binary: binary}
}

type kiloRunEvent struct {
	Type string      `json:"type"`
	Part kiloRunPart `json:"part"`
}

type kiloRunPart struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

func (p *KiloProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	allArgs := make([]string, 0, len(p.args)+6)
	allArgs = append(allArgs, p.args...)
	allArgs = append(allArgs, "run", "--format", "json")
	if req.ModelID != "" {
		allArgs = append(allArgs, "-m", req.ModelID)
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

	if ctx.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return responseText.String(), ctx.Err()
	}

	exitErr := cmd.Wait()
	if responseText.Len() == 0 && exitErr != nil {
		return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
	}

	return responseText.String(), nil
}