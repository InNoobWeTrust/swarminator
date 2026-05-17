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

type GeminiProvider struct {
	binary string
	args   []string
}

func NewGeminiProvider(binary string) LLMAdapter {
	return &GeminiProvider{binary: binary, args: []string{}}
}

type geminiHeadlessResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

func normalizeGeminiModel(model string) string {
	if strings.HasPrefix(model, "google/") {
		return model[len("google/"):]
	}
	if strings.HasPrefix(model, "gemini/") {
		return model[len("gemini/"):]
	}
	return model
}

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

func (p *GeminiProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	message := req.Input
	if req.Persona != "" {
		message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
	}

	allArgs := []string{"--prompt", message, "--output-format", "json"}

	if req.ModelID != "" {
		allArgs = append(allArgs, "--model", normalizeGeminiModel(req.ModelID))
	}

	if req.AgentMode != "" {
		mappedMode, err := mapAgentMode(string(req.AgentMode))
		if err != nil {
			return "", err
		}
		allArgs = append(allArgs, "--approval-mode", mappedMode)
	}

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

	if ctx.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return responseText.String(), ctx.Err()
	}

	exitErr := cmd.Wait()
	output := strings.TrimSpace(responseText.String())

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
			return gemResp.Response, nil
		}
	}

	if exitErr != nil {
		return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
	}

	return output, nil
}