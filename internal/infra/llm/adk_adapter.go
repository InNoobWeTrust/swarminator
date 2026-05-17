package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"swarminator/internal/domain/agent"
)

type ADKProvider struct {
	model     string
	apiKey    string
	projectID string
}

func NewADKProvider(model, projectID string) LLMAdapter {
	return &ADKProvider{
		model:     model,
		apiKey:    getGoogleAPIKey(),
		projectID: projectID,
	}
}

func getGoogleAPIKey() string {
	if apiKey := os.Getenv("GOOGLE_API_KEY"); apiKey != "" {
		return apiKey
	}

	cmd := exec.Command("gcloud", "auth", "print-access-token")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(string(output))
	}

	return ""
}

type geminiRequest struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
	GenerationConfig struct {
		Temperature      float64 `json:"temperature,omitempty"`
		MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
		TopP             float64 `json:"topP,omitempty"`
		TopK             int     `json:"topK,omitempty"`
	} `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings"`
	} `json:"promptFeedback"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (p *ADKProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	model := req.ModelID
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gemini-pro"
	}

	prompt := fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)

	geminiReq := geminiRequest{}
	geminiReq.Contents = append(geminiReq.Contents, struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}{
		Role: "user",
		Parts: []struct {
			Text string `json:"text"`
		}{
			{Text: prompt},
		},
	})

	geminiReq.GenerationConfig.Temperature = 0.7
	geminiReq.GenerationConfig.MaxOutputTokens = 2048

	jsonData, err := json.Marshal(geminiReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	if p.apiKey != "" {
		url += "?key=" + p.apiKey
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return "", &RateLimitError{Err: fmt.Errorf("Gemini API rate limit exceeded")}
		}

		if geminiResp.Error.Message != "" {
			return "", fmt.Errorf("Gemini API error (%d): %s", resp.StatusCode, geminiResp.Error.Message)
		}

		return "", fmt.Errorf("Gemini API error (%d): %s", resp.StatusCode, string(body))
	}

	if len(geminiResp.Candidates) == 0 {
		return "", fmt.Errorf("Gemini returned empty response")
	}

	if len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned empty response parts")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	if responseText == "" {
		return "", fmt.Errorf("Gemini returned empty response text")
	}

	return responseText, nil
}

type RateLimitError struct {
	Err error
}

func (r *RateLimitError) Error() string {
	return r.Err.Error()
}