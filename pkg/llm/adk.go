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
)

// ADKProvider implements the Provider interface using Google's Gemini REST API
type ADKProvider struct {
	model     string
	apiKey    string
	projectID string
}

// NewADKProvider creates a new ADK provider with the specified model and project ID
func NewADKProvider(model, projectID string) Provider {
	return &ADKProvider{
		model:     model,
		apiKey:    getGoogleAPIKey(),
		projectID: projectID,
	}
}

// getGoogleAPIKey attempts to get Google API key from environment or gcloud
func getGoogleAPIKey() string {
	// First check environment variable
	if apiKey := os.Getenv("GOOGLE_API_KEY"); apiKey != "" {
		return apiKey
	}

	// Try to get from gcloud auth print-access-token
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(string(output))
	}

	return ""
}

// geminiRequest represents the request structure for Gemini API
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

// geminiResponse represents the response structure from Gemini API
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

// Complete sends a completion request to the Gemini REST API and returns the response
func (p *ADKProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	// Determine which model to use
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gemini-pro" // Default model
	}

	// Prepare the prompt with persona context
	prompt := fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)

	// Build the request payload
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

	// Set generation config
	geminiReq.GenerationConfig.Temperature = 0.7
	geminiReq.GenerationConfig.MaxOutputTokens = 2048

	// Marshal the request
	jsonData, err := json.Marshal(geminiReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build the API URL
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	if p.apiKey != "" {
		url += "?key=" + p.apiKey
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Set timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Execute the request
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Handle API errors
	if resp.StatusCode != http.StatusOK {
		// Handle rate limiting (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			return "", &RateLimitError{Err: fmt.Errorf("Gemini API rate limit exceeded")}
		}
		
		// Handle other API errors
		if geminiResp.Error.Message != "" {
			return "", fmt.Errorf("Gemini API error (%d): %s", resp.StatusCode, geminiResp.Error.Message)
		}
		
		return "", fmt.Errorf("Gemini API error (%d): %s", resp.StatusCode, string(body))
	}

	// Extract response text
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

// RateLimitError represents a rate limit error that can be caught by the unified provider
type RateLimitError struct {
	Err error
}

func (r *RateLimitError) Error() string {
	return r.Err.Error()
}