package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swarminator/internal/protocol/acp"
)

const (
	acpHelperEnv     = "SWARMINATOR_ACP_HELPER"
	acpScenarioEnv   = "SWARMINATOR_ACP_SCENARIO"
	acpRequestLogEnv = "SWARMINATOR_ACP_REQUEST_LOG"
)

func TestACPProviderComplete(t *testing.T) {
	provider := newMockACPProvider(t, "complete")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{
		Persona: "architect",
		Input:   "Design a durable workflow",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if resp != "mock prompt response" {
		t.Fatalf("Complete returned %q, want %q", resp, "mock prompt response")
	}
}

func TestACPProviderJSONRPCRequests(t *testing.T) {
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	t.Setenv(acpRequestLogEnv, requestLog)

	provider := newMockACPProvider(t, "capture-requests")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := provider.Complete(ctx, CompletionRequest{
		Persona: "reviewer",
		Input:   "Check JSON-RPC sequencing",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	reqs := readLoggedRequests(t, requestLog)
	if len(reqs) != 3 {
		t.Fatalf("logged %d requests, want 3", len(reqs))
	}

	for i, req := range reqs {
		if req.JSONRPC != "2.0" {
			t.Fatalf("request %d jsonrpc = %q, want %q", i+1, req.JSONRPC, "2.0")
		}
	}

	if reqs[0].Method != "initialize" || reqs[0].ID != 1 {
		t.Fatalf("initialize request = method %q id %d, want method %q id %d", reqs[0].Method, reqs[0].ID, "initialize", 1)
	}
	if reqs[1].Method != "session/new" || reqs[1].ID != 2 {
		t.Fatalf("session/new request = method %q id %d, want method %q id %d", reqs[1].Method, reqs[1].ID, "session/new", 2)
	}
	if reqs[2].Method != "session/prompt" || reqs[2].ID != 3 {
		t.Fatalf("session/prompt request = method %q id %d, want method %q id %d", reqs[2].Method, reqs[2].ID, "session/prompt", 3)
	}

	var initParams acp.InitializeParams
	if err := json.Unmarshal(reqs[0].Params, &initParams); err != nil {
		t.Fatalf("failed to parse initialize params: %v", err)
	}
	if initParams.ClientInfo.Name != "swarminator" {
		t.Fatalf("initialize clientInfo.name = %q, want %q", initParams.ClientInfo.Name, "swarminator")
	}
	if initParams.ClientInfo.Version != "1.0.0" {
		t.Fatalf("initialize clientInfo.version = %q, want %q", initParams.ClientInfo.Version, "1.0.0")
	}
	if initParams.ClientInfo.Title != "Swarminator" {
		t.Fatalf("initialize clientInfo.title = %q, want %q", initParams.ClientInfo.Title, "Swarminator")
	}
	if !initParams.ClientCapabilities.Fs.ReadTextFile {
		t.Fatalf("initialize clientCapabilities.fs.readTextFile = false, want true")
	}
	if !initParams.ClientCapabilities.Fs.WriteTextFile {
		t.Fatalf("initialize clientCapabilities.fs.writeTextFile = false, want true")
	}
	if !initParams.ClientCapabilities.Terminal {
		t.Fatalf("initialize clientCapabilities.terminal = false, want true")
	}

  var sessionParams acp.SessionNewParams
  if err := json.Unmarshal(reqs[1].Params, &sessionParams); err != nil {
  	t.Fatalf("failed to parse session/new params: %v", err)
  }
  if len(sessionParams.McpServers) != 0 {
  	t.Fatalf("session/new params McpServers = %v, want []", sessionParams.McpServers)
  }

  var promptParams acp.SessionPromptParams
  if err := json.Unmarshal(reqs[2].Params, &promptParams); err != nil {
  	t.Fatalf("failed to parse session/prompt params: %v", err)
  }
  if promptParams.SessionId != "mock-session-id" {
  	t.Fatalf("session/prompt sessionId = %q, want %q", promptParams.SessionId, "mock-session-id")
  }
  expectedPrompt := []map[string]string{
  	{"type": "text", "text": "PERSONA: reviewer\n\nINPUT: Check JSON-RPC sequencing"},
  }
  if len(promptParams.Prompt) != 1 || promptParams.Prompt[0]["type"] != "text" || promptParams.Prompt[0]["text"] != expectedPrompt[0]["text"] {
  	t.Fatalf("prompt body = %#v, want %#v", promptParams.Prompt, expectedPrompt)
  }
}

func TestACPProviderResponseParsing(t *testing.T) {
	provider := newMockACPProvider(t, "response-noise")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{
		Persona: "analyst",
		Input:   "Parse responses through noise",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if resp != "parsed through noise" {
		t.Fatalf("Complete returned %q, want %q", resp, "parsed through noise")
	}
}

func TestACPProviderSessionClose(t *testing.T) {
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	t.Setenv(acpRequestLogEnv, requestLog)

	provider := newMockACPProvider(t, "session-close-supported")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, CompletionRequest{
		Persona: "tester",
		Input:   "Test session close",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp != "mock prompt response" {
		t.Fatalf("Complete returned %q, want %q", resp, "mock prompt response")
	}

	reqs := readLoggedRequests(t, requestLog)
	if len(reqs) != 4 {
		t.Fatalf("logged %d requests, want 4 (initialize, session/new, session/prompt, session/close)", len(reqs))
	}
	if reqs[3].Method != "session/close" {
		t.Fatalf("request 4 method = %q, want %q", reqs[3].Method, "session/close")
	}
}

func TestACPProviderSessionCancel(t *testing.T) {
	t.Skip("session/cancel notification testing requires pipe-level instrumentation; covered by integration tests")
}

func TestACPProviderErrorHandling(t *testing.T) {
	t.Run("malformed JSON response eventually times out", func(t *testing.T) {
		provider := newMockACPProvider(t, "malformed-json")
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		_, err := provider.Complete(ctx, CompletionRequest{Persona: "ops", Input: "Trigger malformed JSON"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Complete error = %v, want context deadline exceeded", err)
		}
		if !strings.Contains(err.Error(), "initialize:") {
			t.Fatalf("Complete error = %q, want initialize context", err)
		}
	})

	t.Run("missing newSession result fields", func(t *testing.T) {
		provider := newMockACPProvider(t, "missing-newsession-result")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := provider.Complete(ctx, CompletionRequest{Persona: "ops", Input: "Missing response fields"})
		if err == nil {
			t.Fatal("Complete error = nil, want parse failure")
		}
		if !strings.Contains(err.Error(), "failed to parse session result") {
			t.Fatalf("Complete error = %q, want session parse error", err)
		}
	})

	t.Run("initialize RPC error", func(t *testing.T) {
		provider := newMockACPProvider(t, "rpc-error-initialize")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := provider.Complete(ctx, CompletionRequest{Persona: "ops", Input: "Trigger initialize RPC error"})
		if err == nil {
			t.Fatal("Complete error = nil, want RPC error")
		}
		if !strings.Contains(err.Error(), "initialize: RPC error (-32000): initialize failed") {
			t.Fatalf("Complete error = %q, want initialize RPC error", err)
		}
	})

	t.Run("prompt RPC 429 error", func(t *testing.T) {
		provider := newMockACPProvider(t, "rpc-error-429")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := provider.Complete(ctx, CompletionRequest{Persona: "ops", Input: "Trigger 429"})
		if err == nil {
			t.Fatal("Complete error = nil, want RPC error")
		}
		if !strings.Contains(err.Error(), "session/prompt: RPC error (429): rate limited") {
			t.Fatalf("Complete error = %q, want session/prompt 429 RPC error", err)
		}
	})

	t.Run("agent binary not found", func(t *testing.T) {
		provider := &ACPProvider{binary: "definitely-not-a-real-acp-agent-binary-for-tests"}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := provider.Complete(ctx, CompletionRequest{Persona: "ops", Input: "binary missing"})
		if err == nil {
			t.Fatal("Complete error = nil, want start failure")
		}
		if !strings.Contains(err.Error(), "failed to start agent definitely-not-a-real-acp-agent-binary-for-tests") {
			t.Fatalf("Complete error = %q, want start failure", err)
		}
	})
}

func TestACPProviderMockAgentHelper(t *testing.T) {
	if os.Getenv(acpHelperEnv) != "1" {
		return
	}

	if err := runMockACPAgent(os.Getenv(acpScenarioEnv), os.Getenv(acpRequestLogEnv)); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func newMockACPProvider(t *testing.T, scenario string) *ACPProvider {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Setenv(acpHelperEnv, "1")
	t.Setenv(acpScenarioEnv, scenario)

	return &ACPProvider{
		binary: exe,
		args:   []string{"-test.run=^TestACPProviderMockAgentHelper$"},
	}
}

func runMockACPAgent(scenario, requestLog string) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	requestNum := 0
	for scanner.Scan() {
		requestNum++
		line := append([]byte(nil), scanner.Bytes()...)
		if requestLog != "" {
			if err := appendRequestLog(requestLog, line); err != nil {
				return err
			}
		}

		var req acp.Request
		if err := json.Unmarshal(line, &req); err != nil {
			return fmt.Errorf("failed to parse request %d: %w", requestNum, err)
		}

		switch scenario {
		case "complete", "capture-requests":
			if err := writeScenarioSuccessResponse(requestNum, "mock prompt response"); err != nil {
				return err
			}
		case "session-close-supported":
			switch requestNum {
			case 1:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"close":{}}}}`), ID: 1}); err != nil {
					return err
				}
			case 2:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"sessionId":"mock-session-id"}`), ID: 2}); err != nil {
					return err
				}
			case 3:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Method: "session/update", Params: json.RawMessage(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"mock prompt response"}}}`)}); err != nil {
					return err
				}
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"stopReason":"end_turn"}`), ID: 3}); err != nil {
					return err
				}
			case 4:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{}`), ID: 4}); err != nil {
					return err
				}
			}
		case "response-noise":
			if err := writeRawLine(`["ignore this"]`); err != nil {
				return err
			}
			if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Method: "progress"}); err != nil {
				return err
			}
			if err := writeScenarioSuccessResponse(requestNum, "parsed through noise"); err != nil {
				return err
			}
		case "malformed-json":
			return writeRawLine(`{"jsonrpc":"2.0"`)
		case "missing-newsession-result":
			switch requestNum {
			case 1:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{}`), ID: 1}); err != nil {
					return err
				}
			case 2:
				return writeJSONLine(map[string]any{"jsonrpc": "2.0", "id": 2})
			default:
				return nil
			}
		case "rpc-error-initialize":
			return writeJSONLine(acp.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &acp.Error{
					Code:    -32000,
					Message: "initialize failed",
				},
			})
		case "rpc-error-429":
			switch requestNum {
			case 1:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{}`), ID: 1}); err != nil {
					return err
				}
			case 2:
				if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"sessionId":"mock-session-id"}`), ID: 2}); err != nil {
					return err
				}
			case 3:
				return writeJSONLine(acp.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &acp.Error{
						Code:    429,
						Message: "rate limited",
					},
				})
			default:
				return nil
			}
		default:
			return fmt.Errorf("unknown ACP mock scenario %q", scenario)
		}

		if req.Method == "session/prompt" && scenario != "session-close-supported" {
			return nil
		}
		if req.Method == "session/close" && scenario == "session-close-supported" {
			return nil
		}
	}

	return scanner.Err()
}

func writeScenarioSuccessResponse(requestNum int, promptResponse string) error {
	switch requestNum {
	case 1:
		return writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{}`), ID: 1})
	case 2:
		return writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"sessionId":"mock-session-id"}`), ID: 2})
	case 3:
		if err := writeJSONLine(acp.Response{JSONRPC: "2.0", Method: "session/update", Params: json.RawMessage(fmt.Sprintf(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q}}}`, promptResponse))}); err != nil {
			return err
		}
		return writeJSONLine(acp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"stopReason":"end_turn"}`), ID: 3})
	default:
		return nil
	}
}

func writeJSONLine(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

func writeRawLine(line string) error {
	_, err := fmt.Fprintln(os.Stdout, line)
	return err
}

func appendRequestLog(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(line, '\n'))
	return err
}

type loggedRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      int             `json:"id"`
}

func readLoggedRequests(t *testing.T, path string) []loggedRequest {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open request log: %v", err)
	}
	defer f.Close()

	var reqs []loggedRequest
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req loggedRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			t.Fatalf("parse logged request %q: %v", line, err)
		}
		reqs = append(reqs, req)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan request log: %v", err)
	}

	return reqs
}
