package orchestratortransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"swarminator/internal/domain/swarmrun"
	"swarminator/internal/infra/orchestratorconfig"
)

func TestSendOpenAICompatibleUsesChatCompletionsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer test-token"; got != want {
			t.Fatalf("Authorization header = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("x-kilocode-mode"), "orchestrator"; got != want {
			t.Fatalf("x-kilocode-mode header = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/api/gateway/chat/completions"; got != want {
			t.Fatalf("request path = %q, want %q", got, want)
		}

		var payload captureOpenAIChatPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.Model != "kilo-auto/free" {
			t.Fatalf("model = %q, want kilo-auto/free", payload.Model)
		}
		if len(payload.Messages) != 3 {
			t.Fatalf("messages length = %d, want 3", len(payload.Messages))
		}
		if payload.Messages[0].Role != "system" || payload.Messages[0].Content != "You are concise." {
			t.Fatalf("first message = %#v, want system prompt", payload.Messages[0])
		}
		if len(payload.Tools) != 1 {
			t.Fatalf("tools length = %d, want 1", len(payload.Tools))
		}
		decl := payload.Tools[0].Function
		if decl.Name != string(swarmrun.ToolNameRunWorkerNode) {
			t.Fatalf("function name = %q, want %q", decl.Name, swarmrun.ToolNameRunWorkerNode)
		}
		if got := decl.Parameters.Properties["model"].Enum; len(got) != 2 || got[0] != "reviewer" || got[1] != "worker" {
			t.Fatalf("model enum = %#v, want reviewer/worker", got)
		}
		if got := decl.Parameters.Properties["persona"].Enum; len(got) != 2 || got[0] != "critic" || got[1] != "summarizer" {
			t.Fatalf("persona enum = %#v, want critic/summarizer", got)
		}

		_ = json.NewEncoder(w).Encode(captureOpenAIChatResponse{
			Choices: []captureOpenAIChatChoice{{
				Message: captureOpenAIChatMessage{Role: "assistant", Content: "final answer"},
			}},
		})
	}))
	defer server.Close()

	t.Setenv("KILO_API_KEY", "test-token")
	transport, err := New(orchestratorconfig.Config{
		Backend:          BackendOpenAICompatible,
		MessageAPIFormat: MessageAPIFormatOpenAIChat,
		BaseURL:          server.URL + "/api/gateway",
		Auth:             orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := transport.Send(context.Background(), swarmrun.ConversationRequest{
		Model: "kilo/kilo-auto/free",
		Messages: []swarmrun.Message{
			{Role: swarmrun.RoleSystem, Content: "You are concise."},
			{Role: swarmrun.RoleUser, Content: "hello"},
			{Role: swarmrun.RoleAssistant, Content: "ack"},
		},
		Tools: []swarmrun.ToolDefinition{{
			Name:        swarmrun.ToolNameRunWorkerNode,
			Description: "Run worker",
			Parameters: []swarmrun.ToolParameter{
				{Name: "model", Required: true, Type: swarmrun.ToolParameterTypeString, Enum: []string{"reviewer", "worker"}},
				{Name: "persona", Required: true, Type: swarmrun.ToolParameterTypeString, Enum: []string{"critic", "summarizer"}},
				{Name: "input", Required: true, Type: swarmrun.ToolParameterTypeString},
			},
		}},
		MaxOutputTokens: 512,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if resp.Message.Role != swarmrun.RoleAssistant || resp.Message.Content != "final answer" {
		t.Fatalf("response = %#v, want assistant final answer", resp)
	}
}

func TestSendOpenAICompatibleLeavesNonAutoModelsUntouched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-kilocode-mode"); got != "" {
			t.Fatalf("x-kilocode-mode header = %q, want empty", got)
		}
		var payload captureOpenAIChatPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.Model != "openai/gpt-4.1" {
			t.Fatalf("model = %q, want openai/gpt-4.1", payload.Model)
		}
		_ = json.NewEncoder(w).Encode(captureOpenAIChatResponse{
			Choices: []captureOpenAIChatChoice{{
				Message: captureOpenAIChatMessage{Role: "assistant", Content: "final answer"},
			}},
		})
	}))
	defer server.Close()

	t.Setenv("KILO_API_KEY", "test-token")
	transport, err := New(orchestratorconfig.Config{
		Backend:          BackendOpenAICompatible,
		MessageAPIFormat: MessageAPIFormatOpenAIChat,
		BaseURL:          server.URL,
		Auth:             orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := transport.Send(context.Background(), swarmrun.ConversationRequest{
		Model:    "openai/gpt-4.1",
		Messages: []swarmrun.Message{{Role: swarmrun.RoleUser, Content: "hello"}},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestSendOpenAICompatibleReturnsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(captureOpenAIChatResponse{
			Choices: []captureOpenAIChatChoice{{
				Message: captureOpenAIChatMessage{
					Role: "assistant",
					ToolCalls: []captureOpenAIResponseTool{{
						ID:   "call_1",
						Type: "function",
						Function: captureOpenAIResponseFunction{
							Name:      string(swarmrun.ToolNameRunWorkerNode),
							Arguments: `{"model":"worker","persona":"critic","input":"inspect the patch"}`,
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	t.Setenv("KILO_API_KEY", "test-token")
	transport, err := New(orchestratorconfig.Config{
		Backend:          BackendOpenAICompatible,
		MessageAPIFormat: MessageAPIFormatOpenAIChat,
		BaseURL:          server.URL,
		Auth:             orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := transport.Send(context.Background(), swarmrun.ConversationRequest{
		Model:    "kilo/kilo-auto/free",
		Messages: []swarmrun.Message{{Role: swarmrun.RoleUser, Content: "hello"}},
		Tools: []swarmrun.ToolDefinition{{
			Name: swarmrun.ToolNameRunWorkerNode,
			Parameters: []swarmrun.ToolParameter{{Name: "model", Required: true, Type: swarmrun.ToolParameterTypeString}},
		}},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if resp.ToolCall == nil {
		t.Fatal("Send() tool call = nil, want tool call")
	}
	if resp.ToolCall.Name != swarmrun.ToolNameRunWorkerNode || resp.ToolCall.Arguments["model"] != "worker" {
		t.Fatalf("tool call = %#v, want run_worker_node with worker args", resp.ToolCall)
	}
}

func TestNewRejectsUnsupportedBackend(t *testing.T) {
	t.Setenv("KILO_API_KEY", "test-token")
	if _, err := New(orchestratorconfig.Config{
		Backend:          "gemini",
		MessageAPIFormat: "openai.chat.completions",
		BaseURL:          "http://127.0.0.1:8080",
		Auth:             orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"},
	}); err == nil {
		t.Fatal("New() error = nil, want unsupported backend failure")
	}
}

func TestTransportTimeoutUsesConfigOverride(t *testing.T) {
	t.Parallel()

	if got := transportTimeout(orchestratorconfig.Config{}); got != 30*time.Second {
		t.Fatalf("transportTimeout(default) = %v, want %v", got, 30*time.Second)
	}
	if got := transportTimeout(orchestratorconfig.Config{TimeoutSeconds: 75}); got != 75*time.Second {
		t.Fatalf("transportTimeout(override) = %v, want %v", got, 75*time.Second)
	}
}

type captureOpenAIChatPayload struct {
	Model    string                     `json:"model"`
	Messages []captureOpenAIChatMessage `json:"messages"`
	Tools    []captureOpenAIChatTool    `json:"tools,omitempty"`
}

type captureOpenAIChatTool struct {
	Type     string                         `json:"type"`
	Function captureOpenAIFunctionDefinition `json:"function"`
}

type captureOpenAIFunctionDefinition struct {
	Name       string              `json:"name"`
	Parameters captureOpenAISchema `json:"parameters"`
}

type captureOpenAISchema struct {
	Properties map[string]captureOpenAISchema `json:"properties,omitempty"`
	Enum       []string                       `json:"enum,omitempty"`
}

type captureOpenAIChatResponse struct {
	Choices []captureOpenAIChatChoice `json:"choices"`
}

type captureOpenAIChatChoice struct {
	Message captureOpenAIChatMessage `json:"message"`
}

type captureOpenAIChatMessage struct {
	Role      string                      `json:"role,omitempty"`
	Content   string                      `json:"content,omitempty"`
	ToolCalls []captureOpenAIResponseTool `json:"tool_calls,omitempty"`
}

type captureOpenAIResponseTool struct {
	ID       string                        `json:"id,omitempty"`
	Type     string                        `json:"type,omitempty"`
	Function captureOpenAIResponseFunction `json:"function"`
}

type captureOpenAIResponseFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
