package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// CodexSandboxMode controls how codex sandboxes shell-tool execution.
type CodexSandboxMode string

const (
	// CodexSandboxFullAuto is the default: codex runs with low-friction sandboxed
	// automatic execution (equivalent to --full-auto). Shell commands are isolated,
	// preventing uncontrolled host mutations.
	CodexSandboxFullAuto CodexSandboxMode = "full-auto"

	// CodexSandboxDangerousNoSandbox bypasses all confirmation prompts and executes
	// shell commands without sandboxing. Use only in externally sandboxed environments.
	CodexSandboxDangerousNoSandbox CodexSandboxMode = "danger"
)

// CodexProvider uses codex's one-shot exec mode (codex exec --json) to process completions.
// codex does not support ACP stdio transport; it uses its own JSONL event format.
//
// # Codex JSONL Wire Format (observed with codex-cli 0.125.0)
//
// All events are newline-delimited JSON written to stdout.
//
//	{"type":"thread.started","thread_id":"<id>"}
//	{"type":"turn.started"}
//	{"type":"item.completed","item":{"id":"<id>","type":"agent_message","text":"<full-text>"}}
//	{"type":"item.started","item":{"id":"<id>","type":"command_execution",...}}   // tool calls
//	{"type":"item.completed","item":{"id":"<id>","type":"command_execution",...}} // tool results
//	{"type":"turn.completed","usage":{...}}
//
// Text content arrives in "item.completed" events where item.type == "agent_message".
// There may be multiple agent_message items (e.g. pre-tool reasoning then a final reply).
// All agent_message texts are collected and returned joined by "\n\n".
type CodexProvider struct {
	binary      string
	args        []string         // prepended before "exec --json ..."; used in tests
	sandboxMode CodexSandboxMode // controls shell-tool sandboxing; defaults to CodexSandboxFullAuto
}

// NewCodexProvider creates a Provider that runs "codex exec --json --full-auto".
// The default sandbox mode is CodexSandboxFullAuto (sandboxed shell execution).
// Use WithSandboxMode to override.
func NewCodexProvider(binary string) Provider {
	return &CodexProvider{binary: binary, sandboxMode: CodexSandboxFullAuto}
}

// WithSandboxMode returns a copy of the provider with the given sandbox mode.
func (p *CodexProvider) WithSandboxMode(mode CodexSandboxMode) *CodexProvider {
	cp := *p
	cp.sandboxMode = mode
	return &cp
}

// codexEvent is one line of codex's newline-delimited JSON output.
type codexEvent struct {
	Type string          `json:"type"`
	Item json.RawMessage `json:"item,omitempty"`
}

// codexItem is the "item" field inside a codexEvent.
type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p *CodexProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	allArgs := make([]string, 0, len(p.args)+6)
	allArgs = append(allArgs, p.args...)
	// Run codex non-interactively with JSONL output.
	allArgs = append(allArgs, "exec", "--json")
	switch p.sandboxMode {
	case CodexSandboxDangerousNoSandbox:
		// Skip all confirmation prompts and execute commands without sandboxing.
		// Use only in externally-sandboxed environments.
		allArgs = append(allArgs, "--dangerously-bypass-approvals-and-sandbox")
	default:
		// CodexSandboxFullAuto: low-friction sandboxed automatic execution.
		allArgs = append(allArgs, "--full-auto")
	}
	// Pass the model through to codex when specified.
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

	// When context is cancelled while scanner.Scan() is blocking, CommandContext
	// kills the process which closes stdout, causing Scan() to return false (EOF)
	// without surfacing the context error. Check explicitly here.
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
