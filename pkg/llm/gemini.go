package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GeminiProvider uses Gemini CLI's headless one-shot mode (gemini --prompt "...")
// to process completions. This avoids the ACP session/prompt timeout that occurs
// with the interactive --acp mode.
type GeminiProvider struct {
	binary string
	args   []string
}

// NewGeminiProvider creates a Provider that runs "gemini --prompt ...".
func NewGeminiProvider(binary string) Provider {
	return &GeminiProvider{binary: binary, args: []string{}}
}

// geminiHeadlessResponse is the JSON object returned by Gemini CLI with --output-format json.
type geminiHeadlessResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

// normalizeGeminiModel strips google/ and gemini/ prefixes from model names
// so that the Gemini CLI receives native model identifiers.
func normalizeGeminiModel(model string) string {
	if strings.HasPrefix(model, "google/") {
		return model[len("google/"):]
	}
	if strings.HasPrefix(model, "gemini/") {
		return model[len("gemini/"):]
	}
	return model
}

// mapAgentMode translates swarminator's agent-mode values to Gemini CLI
// --approval-mode values. Returns an error for unknown modes.
func mapAgentMode(mode string) (string, error) {
	switch mode {
	case "":
		return "", nil
	case "default", "plan", "yolo":
		return mode, nil
	case "autoEdit":
		return "auto_edit", nil
	default:
		return "", fmt.Errorf("unknown agent-mode %q for Gemini; supported values: default, plan, yolo, autoEdit", mode)
	}
}

func (p *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	allArgs := []string{"--prompt", message, "--output-format", "json"}

	if req.Model != "" {
		allArgs = append(allArgs, "--model", normalizeGeminiModel(req.Model))
	}

	if req.AgentMode != "" {
		mappedMode, err := mapAgentMode(req.AgentMode)
		if err != nil {
			return "", err
		}
		allArgs = append(allArgs, "--approval-mode", mappedMode)
	}

	// Check context before spawning the process.
	if err := ctx.Err(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, p.binary, append(p.args, allArgs...)...)
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
	}

	if err := scanner.Err(); err != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return "", fmt.Errorf("reading %s output: %w", p.binary, err)
	}

	// Context cancelled while scanner.Scan() is blocking — CommandContext kills
	// the process which closes stdout, causing Scan() to return false (EOF)
	// without surfacing the context error. Check explicitly here.
	if ctx.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return responseText.String(), ctx.Err()
	}

	exitErr := cmd.Wait()
	output := strings.TrimSpace(responseText.String())

	// Try to parse structured JSON response.
	if output != "" {
		var gemResp geminiHeadlessResponse
		if err := json.Unmarshal([]byte(output), &gemResp); err == nil {
			if gemResp.Error != "" || exitErr != nil {
				errMsg := gemResp.Error
				if exitErr != nil && errMsg == "" {
					errMsg = exitErr.Error()
				}
				return "", fmt.Errorf("%s failed: %s", p.binary, errMsg)
			}
			// Successfully parsed valid JSON response (even if empty string).
			return gemResp.Response, nil
		}
	}

	// Non-JSON output with non-zero exit — treat as failure.
	if exitErr != nil {
		return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
	}

	return output, nil
}
