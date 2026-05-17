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

type CodexSandboxMode string

const (
	CodexSandboxFullAuto            CodexSandboxMode = "full-auto"
	CodexSandboxDangerousNoSandbox  CodexSandboxMode = "danger"
)

type CodexProvider struct {
	binary      string
	args        []string
	sandboxMode CodexSandboxMode
}

func NewCodexProvider(binary string) LLMAdapter {
	return &CodexProvider{binary: binary, sandboxMode: CodexSandboxFullAuto}
}

func (p *CodexProvider) WithSandboxMode(mode CodexSandboxMode) *CodexProvider {
	cp := *p
	cp.sandboxMode = mode
	return &cp
}

type codexEvent struct {
	Type string          `json:"type"`
	Item json.RawMessage `json:"item,omitempty"`
}

type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p *CodexProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	allArgs := make([]string, 0, len(p.args)+6)
	allArgs = append(allArgs, p.args...)
	allArgs = append(allArgs, "exec", "--json")
	switch p.sandboxMode {
	case CodexSandboxDangerousNoSandbox:
		allArgs = append(allArgs, "--dangerously-bypass-approvals-and-sandbox")
	default:
		allArgs = append(allArgs, "--full-auto")
	}
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

	var messageParts []string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			waitForProcessExit(cmd, acpProcessWaitTimeout)
			return strings.Join(messageParts, "\n\n"), ctx.Err()
		default:
		}

		var event codexEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "item.completed" && len(event.Item) > 0 {
			var item codexItem
			if err := json.Unmarshal(event.Item, &item); err == nil &&
				item.Type == "agent_message" && item.Text != "" {
				messageParts = append(messageParts, item.Text)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return "", fmt.Errorf("reading %s output: %w", p.binary, err)
	}

	if ctx.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return strings.Join(messageParts, "\n\n"), ctx.Err()
	}

	exitErr := cmd.Wait()
	result := strings.Join(messageParts, "\n\n")
	if result == "" && exitErr != nil {
		return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
	}

	return result, nil
}