package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func redirectGeminiRequests(t *testing.T, serverURL string) {
	t.Helper()

	target, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		clone.Host = ""
		return original.RoundTrip(clone)
	})

	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}

func writeFakeGCloud(t *testing.T, output string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "gcloud")
	contents := "#!/bin/sh\nprintf '%s\\n' \"" + output + "\"\n"
	if runtime.GOOS == "windows" {
		path += ".bat"
		contents = "@echo off\r\necho " + output + "\r\n"
	}

	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake gcloud: %v", err)
	}

	return dir
}

func installPathForTest(t *testing.T, dir string) {
	t.Helper()

	pathEnv := dir
	if runtime.GOOS == "windows" {
		pathEnv = dir + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	t.Setenv("PATH", pathEnv)
}

func TestGetGoogleAPIKey(t *testing.T) {
	t.Run("environment variable takes precedence", func(t *testing.T) {
		t.Setenv("GOOGLE_API_KEY", "env-token")
		installPathForTest(t, writeFakeGCloud(t, "gcloud-token"))

		got := getGoogleAPIKey()
		if got != "env-token" {
			t.Fatalf("getGoogleAPIKey() = %q, want %q", got, "env-token")
		}
	})

	t.Run("falls back to gcloud auth print-access-token", func(t *testing.T) {
		t.Setenv("GOOGLE_API_KEY", "")
		installPathForTest(t, writeFakeGCloud(t, "gcloud-token"))

		got := getGoogleAPIKey()
		if got != "gcloud-token" {
			t.Fatalf("getGoogleAPIKey() = %q, want %q", got, "gcloud-token")
		}
	})

	t.Run("returns empty string when no source is available", func(t *testing.T) {
		t.Setenv("GOOGLE_API_KEY", "")
		t.Setenv("PATH", t.TempDir())

		got := getGoogleAPIKey()
		if got != "" {
			t.Fatalf("getGoogleAPIKey() = %q, want empty string", got)
		}
	})
}

func TestADKProviderComplete(t *testing.T) {
	var (
		capturedPath        string
		capturedQuery       string
		capturedMethod      string
		capturedContentType string
		capturedBody        []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedMethod = r.Method
		capturedContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		capturedBody = body

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"generated response"}]}}]}`))
	}))
	defer server.Close()

	redirectGeminiRequests(t, server.URL)

	p := &ADKProvider{model: "provider-model", apiKey: "test-api-key"}
	resp, err := p.Complete(context.Background(), CompletionRequest{
		Model:   "gemini-2.0-flash",
		Persona: "planner",
		Input:   "summarize the situation",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp != "generated response" {
		t.Fatalf("Complete() response = %q, want %q", resp, "generated response")
	}

	if capturedMethod != http.MethodPost {
		t.Fatalf("request method = %q, want %q", capturedMethod, http.MethodPost)
	}

	if capturedPath != "/v1beta/models/gemini-2.0-flash:generateContent" {
		t.Fatalf("request path = %q, want %q", capturedPath, "/v1beta/models/gemini-2.0-flash:generateContent")
	}

	if capturedQuery != "key=test-api-key" {
		t.Fatalf("request query = %q, want %q", capturedQuery, "key=test-api-key")
	}

	if capturedContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", capturedContentType, "application/json")
	}

	var req geminiRequest
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}

	if len(req.Contents) != 1 || len(req.Contents[0].Parts) != 1 {
		t.Fatalf("unexpected request contents: %+v", req.Contents)
	}

	if req.Contents[0].Parts[0].Text != "PERSONA: planner\n\nINPUT: summarize the situation" {
		t.Fatalf("prompt = %q, want %q", req.Contents[0].Parts[0].Text, "PERSONA: planner\n\nINPUT: summarize the situation")
	}
}

func TestADKProviderErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		wantErr       string
		wantRateLimit bool
	}{
		{
			name:          "http 429 returns rate limit error",
			statusCode:    http.StatusTooManyRequests,
			responseBody:  `{"error":{"message":"too many requests"}}`,
			wantErr:       "Gemini API rate limit exceeded",
			wantRateLimit: true,
		},
		{
			name:         "http error with API message",
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"error":{"message":"backend failed"}}`,
			wantErr:      "Gemini API error (500): backend failed",
		},
		{
			name:         "http error without API message",
			statusCode:   http.StatusBadGateway,
			responseBody: `{}`,
			wantErr:      "Gemini API error (502): {}",
		},
		{
			name:         "empty candidates array",
			statusCode:   http.StatusOK,
			responseBody: `{"candidates":[]}`,
			wantErr:      "Gemini returned empty response",
		},
		{
			name:         "empty parts array",
			statusCode:   http.StatusOK,
			responseBody: `{"candidates":[{"content":{"parts":[]}}]}`,
			wantErr:      "Gemini returned empty response parts",
		},
		{
			name:         "empty response text",
			statusCode:   http.StatusOK,
			responseBody: `{"candidates":[{"content":{"parts":[{"text":""}]}}]}`,
			wantErr:      "Gemini returned empty response text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			redirectGeminiRequests(t, server.URL)

			p := &ADKProvider{}
			resp, err := p.Complete(context.Background(), CompletionRequest{Input: "hello"})
			if err == nil {
				t.Fatalf("Complete() error = nil, want %q", tt.wantErr)
			}
			if resp != "" {
				t.Fatalf("Complete() response = %q, want empty string", resp)
			}

			if err.Error() != tt.wantErr {
				t.Fatalf("Complete() error = %q, want %q", err.Error(), tt.wantErr)
			}

			var rateLimitErr *RateLimitError
			gotRateLimit := errors.As(err, &rateLimitErr)
			if gotRateLimit != tt.wantRateLimit {
				t.Fatalf("errors.As(err, *RateLimitError) = %v, want %v", gotRateLimit, tt.wantRateLimit)
			}
		})
	}
}

func TestADKProviderModelSelection(t *testing.T) {
	tests := []struct {
		name         string
		providerModel string
		reqModel     string
		wantPath     string
	}{
		{
			name:          "uses request model when provided",
			providerModel: "provider-model",
			reqModel:      "request-model",
			wantPath:      "/v1beta/models/request-model:generateContent",
		},
		{
			name:          "falls back to provider model",
			providerModel: "provider-model",
			wantPath:      "/v1beta/models/provider-model:generateContent",
		},
		{
			name:     "falls back to gemini-pro by default",
			wantPath: "/v1beta/models/gemini-pro:generateContent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedPath string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
			}))
			defer server.Close()

			redirectGeminiRequests(t, server.URL)

			p := &ADKProvider{model: tt.providerModel}
			resp, err := p.Complete(context.Background(), CompletionRequest{
				Model:   tt.reqModel,
				Persona: "persona",
				Input:   "input",
			})
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if resp != "ok" {
				t.Fatalf("Complete() response = %q, want %q", resp, "ok")
			}

			if capturedPath != tt.wantPath {
				t.Fatalf("request path = %q, want %q", capturedPath, tt.wantPath)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	rateLimitErr := &RateLimitError{Err: errors.New("Gemini API rate limit exceeded")}

	if rateLimitErr.Error() != "Gemini API rate limit exceeded" {
		t.Fatalf("Error() = %q, want %q", rateLimitErr.Error(), "Gemini API rate limit exceeded")
	}

	var err error = rateLimitErr
	typedErr, ok := err.(*RateLimitError)
	if !ok {
		t.Fatal("type assertion to *RateLimitError failed")
	}
	if typedErr != rateLimitErr {
		t.Fatal("type assertion returned unexpected value")
	}
}
