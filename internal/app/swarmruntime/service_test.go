package swarmruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swarminator/internal/domain/swarmrun"
	"swarminator/internal/infra/orchestratorconfig"
	"swarminator/internal/infra/swarmcatalog"
)

func TestServiceExecuteCreatesPrivateRunAndUsesConversationTransport(t *testing.T) {
	t.Parallel()

	swarmRoot := tempSwarmRoot(t, `{
		"id": "main",
		"invoke": "kilo/kilo-auto/free",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 2048,
		"runtime_profile": "main",
		"budget_override": {"context": 12000, "output": 2048}
	}`, `{
		"id": "worker",
		"agent": "kilo",
		"invoke": "github-copilot/gpt-5-mini",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 512,
		"budget_override": {"context": 12000, "output": 512}
	}`, `---
id: critic
group: review
intent: identify risks
---
You are a critic.
`)

	runDir := filepath.Join(t.TempDir(), "run-123")
	var captured swarmrun.ConversationRequest
	svc := NewServiceWithDependencies(Dependencies{
		LoadProfile: func(name string) (orchestratorconfig.Config, error) {
			if name != "main" {
				return orchestratorconfig.Config{}, errors.New("unexpected profile")
			}
			return orchestratorconfig.Config{Backend: "openai-compatible", MessageAPIFormat: "openai.chat.completions", BaseURL: "http://127.0.0.1:8080", Auth: orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"}}, nil
		},
		ResolveBudget: func(ctx context.Context, model swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error) {
			return swarmrun.TokenBudget{ContextWindow: 12000, MaxInputTokens: 9952, MaxOutputTokens: 2048}, nil
		},
		NewTransport: func(cfg orchestratorconfig.Config) (swarmrun.ConversationTransport, error) {
			return conversationTransportFunc(func(ctx context.Context, req swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error) {
				captured = req
				return swarmrun.ConversationResponse{Message: swarmrun.Message{Role: swarmrun.RoleAssistant, Content: "final answer"}}, nil
			}), nil
		},
	})

	got, err := svc.Execute(context.Background(), Request{SwarmRoot: swarmRoot, Orchestrator: "main", RunDir: runDir, EventSink: "file:///" + filepath.ToSlash(filepath.Join(runDir, "events.jsonl")), Input: "Investigate the bug"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got != "final answer" {
		t.Fatalf("Execute() = %q, want %q", got, "final answer")
	}
	if captured.Model != "kilo/kilo-auto/free" {
		t.Fatalf("captured model = %q, want %q", captured.Model, "kilo/kilo-auto/free")
	}
	if len(captured.Messages) < 2 {
		t.Fatalf("captured messages length = %d, want >= 2", len(captured.Messages))
	}
	if captured.Messages[0].Role != swarmrun.RoleSystem || strings.TrimSpace(captured.Messages[0].Content) == "" {
		t.Fatalf("first message = %#v, want non-empty system contract", captured.Messages[0])
	}
	if len(captured.Tools) != 1 {
		t.Fatalf("captured tools = %#v, want one tool", captured.Tools)
	}
	if got := captured.Tools[0].Parameters[0].Enum; len(got) != 1 || got[0] != "worker" {
		t.Fatalf("captured model enum = %#v, want worker", got)
	}
	if got := captured.Tools[0].Parameters[1].Enum; len(got) != 1 || got[0] != "critic" {
		t.Fatalf("captured persona enum = %#v, want critic", got)
	}

	assertTrimmedFile(t, filepath.Join(runDir, "final.md"), "final answer")
	assertFileContains(t, filepath.Join(runDir, "status.json"), "completed")
	assertFileContains(t, filepath.Join(runDir, "input.txt"), "Investigate the bug")
	assertFileContains(t, filepath.Join(runDir, "memory", "current.md"), "Investigate the bug")
}

func TestServiceStartWritesPendingReceiptAndStatus(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-start")
	svc := NewServiceWithDependencies(Dependencies{
		ExecutablePath: func() (string, error) { return "/tmp/swarminator", nil },
		StartProcess: func(ctx context.Context, executable string, args []string, stdoutPath, stderrPath string) (int, error) {
			if executable != "/tmp/swarminator" {
				t.Fatalf("executable = %q, want %q", executable, "/tmp/swarminator")
			}
			if !strings.Contains(strings.Join(args, " "), "swarm worker") {
				t.Fatalf("args = %#v, want swarm worker invocation", args)
			}
			return 4242, nil
		},
	})

	receipt, err := svc.Start(context.Background(), Request{SwarmRoot: "/tmp/swarm", Orchestrator: "main", RunDir: runDir, Input: "async task"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if receipt.RunID != "run-start" || receipt.Status != string(swarmrun.RunStatePending) {
		t.Fatalf("receipt = %#v, want pending receipt", receipt)
	}
	assertFileContains(t, filepath.Join(runDir, "input.txt"), "async task")
	assertFileContains(t, filepath.Join(runDir, "status.json"), "4242")
}

func TestServiceExecuteSupportsMultiTurnNodeActionsAndArtifactReferences(t *testing.T) {
	t.Parallel()

	swarmRoot := tempSwarmRoot(t, `{
		"id": "main",
		"invoke": "kilo/kilo-auto/free",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 1024,
		"runtime_profile": "main",
		"budget_override": {"context": 1200, "output": 1024}
	}`, `{
		"id": "worker",
		"agent": "kilo",
		"invoke": "github-copilot/gpt-5-mini",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 512,
		"budget_override": {"context": 1200, "output": 512}
	}`, `---
id: critic
group: review
intent: identify risks
---
You are a critic.
Return concise findings.
`)

	runDir := filepath.Join(t.TempDir(), "run-node")
	requests := make([]swarmrun.ConversationRequest, 0, 2)
	svc := NewServiceWithDependencies(Dependencies{
		LoadProfile: func(name string) (orchestratorconfig.Config, error) {
			return orchestratorconfig.Config{Backend: "openai-compatible", MessageAPIFormat: "openai.chat.completions", BaseURL: "http://127.0.0.1:8080", Auth: orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"}}, nil
		},
		ResolveBudget: func(ctx context.Context, model swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error) {
			return swarmrun.TokenBudget{ContextWindow: 1200, MaxInputTokens: 176, MaxOutputTokens: 1024}, nil
		},
		ExecuteWorker: func(ctx context.Context, req WorkerRequest) (WorkerResult, error) {
			return WorkerResult{Output: strings.Repeat("VERY_LONG_RAW_NODE_OUTPUT ", 16)}, nil
		},
		NewTransport: func(cfg orchestratorconfig.Config) (swarmrun.ConversationTransport, error) {
			call := 0
			return conversationTransportFunc(func(ctx context.Context, req swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error) {
				requests = append(requests, req)
				call++
				if call == 1 {
					return swarmrun.ConversationResponse{ToolCall: &swarmrun.ToolCall{Name: swarmrun.ToolNameRunWorkerNode, Arguments: map[string]string{"model": "worker", "persona": "critic", "input": "inspect the patch"}}}, nil
				}
				return swarmrun.ConversationResponse{Message: swarmrun.Message{Role: swarmrun.RoleAssistant, Content: "# Merged answer\n\nmerged answer"}}, nil
			}), nil
		},
	})

	got, err := svc.Execute(context.Background(), Request{SwarmRoot: swarmRoot, Orchestrator: "main", RunDir: runDir, Input: "Fix the feature"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got != "# Merged answer\n\nmerged answer" {
		t.Fatalf("Execute() = %q, want %q", got, "# Merged answer\n\nmerged answer")
	}
	if len(requests) != 2 {
		t.Fatalf("transport call count = %d, want 2", len(requests))
	}
	if requests[0].Tools == nil || len(requests[0].Tools) != 1 {
		t.Fatalf("first request tools = %#v, want one tool", requests[0].Tools)
	}
	if requests[0].Tools[0].Name != swarmrun.ToolNameRunWorkerNode {
		t.Fatalf("first request tool = %#v, want run worker tool", requests[0].Tools[0])
	}
	if got := requests[0].Tools[0].Parameters[0].Enum; len(got) != 1 || got[0] != "worker" {
		t.Fatalf("first request model enum = %#v, want worker", got)
	}
	if got := requests[0].Tools[0].Parameters[1].Enum; len(got) != 1 || got[0] != "critic" {
		t.Fatalf("first request persona enum = %#v, want critic", got)
	}
	joinedSecond := joinMessageContent(requests[1].Messages)
	if !strings.Contains(joinedSecond, "nodes/") {
		t.Fatalf("second request messages missing artifact reference: %q", joinedSecond)
	}
	if !strings.Contains(joinedSecond, "VERY_LONG_RAW_NODE_OUTPUT") {
		t.Fatalf("second request missing raw node output: %q", joinedSecond)
	}

	entries, err := os.ReadDir(filepath.Join(runDir, "nodes"))
	if err != nil {
		t.Fatalf("ReadDir(nodes) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("node artifact count = %d, want 1", len(entries))
	}
	artifactBytes, err := os.ReadFile(filepath.Join(runDir, "nodes", entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile(node artifact) error = %v", err)
	}
	if !strings.Contains(string(artifactBytes), "VERY_LONG_RAW_NODE_OUTPUT") {
		t.Fatalf("node artifact missing raw output: %q", string(artifactBytes))
	}
	assertFileContains(t, filepath.Join(runDir, "transcript.private.jsonl"), "artifact_ref")
	assertTrimmedFile(t, filepath.Join(runDir, "final.md"), "# Merged answer\n\nmerged answer")
}

func TestBuildConversationMessagesIncludesMemoryBeforeRecentTurns(t *testing.T) {
	t.Parallel()

	messages := buildConversationMessages("system contract", "memory summary", []swarmrun.Message{
		{Role: swarmrun.RoleUser, Content: "user turn", Kind: swarmrun.MessageKindUserInput},
		{Role: swarmrun.RoleAssistant, Content: "assistant turn", Kind: swarmrun.MessageKindAssistantOutput},
		{Role: swarmrun.RoleTool, Content: "tool result", Kind: swarmrun.MessageKindToolResult},
	})
	if len(messages) != 5 {
		t.Fatalf("buildConversationMessages() length = %d, want 5", len(messages))
	}
	if messages[0].Role != swarmrun.RoleSystem || messages[1].Role != swarmrun.RoleSystem {
		t.Fatalf("first two messages = %#v %#v, want system contract and memory summary", messages[0], messages[1])
	}
	if messages[4].Role != swarmrun.RoleUser || !strings.Contains(messages[4].Content, "tool result") {
		t.Fatalf("tool result was not normalized into a user-visible message: %#v", messages[4])
	}
}

func TestCompactTranscriptWritesSnapshotAndShrinksPrompt(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-compact")
	store, err := newRunStore(runDir, "")
	if err != nil {
		t.Fatalf("newRunStore() error = %v", err)
	}
	entries := []swarmrun.Message{
		{Role: swarmrun.RoleUser, Content: strings.Repeat("old context ", 20), Kind: swarmrun.MessageKindUserInput},
		{Role: swarmrun.RoleAssistant, Content: strings.Repeat("assistant context ", 20), Kind: swarmrun.MessageKindAssistantOutput},
		{Role: swarmrun.RoleTool, Content: "summary with artifact", Kind: swarmrun.MessageKindToolResult, ArtifactRef: "nodes/a.md"},
		{Role: swarmrun.RoleUser, Content: "recent request", Kind: swarmrun.MessageKindUserInput},
	}
	before := estimateConversationTokens(buildConversationMessages("system", "", entries))
	memory, kept, err := compactTranscript(store, "task summary", entries, 2)
	if err != nil {
		t.Fatalf("compactTranscript() error = %v", err)
	}
	after := estimateConversationTokens(buildConversationMessages("system", memory, kept))
	if after >= before {
		t.Fatalf("token estimate after compaction = %d, want < %d", after, before)
	}
	if len(kept) != 2 {
		t.Fatalf("kept message count = %d, want 2 recent messages", len(kept))
	}
	if !strings.Contains(memory, "nodes/a.md") {
		t.Fatalf("memory summary missing artifact reference: %q", memory)
	}
	assertFileContains(t, filepath.Join(runDir, "memory", "snapshot-001.md"), "task summary")
}

func TestServiceExecuteFailsClosedWithoutBudgetMetadata(t *testing.T) {
	t.Parallel()

	swarmRoot := tempSwarmRoot(t, `{
		"id": "main",
		"invoke": "kilo/kilo-auto/free",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 2048,
		"runtime_profile": "main"
	}`, `{
		"id": "worker",
		"agent": "kilo",
		"invoke": "github-copilot/gpt-5-mini",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 512,
		"budget_override": {"context": 1200, "output": 512}
	}`, `---
id: critic
group: review
intent: identify risks
---
You are a critic.
`)

	svc := NewServiceWithDependencies(Dependencies{
		LoadProfile: func(name string) (orchestratorconfig.Config, error) {
			return orchestratorconfig.Config{Backend: "openai-compatible", MessageAPIFormat: "openai.chat.completions", BaseURL: "http://127.0.0.1:8080", Auth: orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"}}, nil
		},
		ResolveBudget: func(ctx context.Context, model swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error) {
			return swarmrun.TokenBudget{}, errors.New("missing models.dev budget metadata")
		},
		NewTransport: func(cfg orchestratorconfig.Config) (swarmrun.ConversationTransport, error) {
			return conversationTransportFunc(func(ctx context.Context, req swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error) {
				return swarmrun.ConversationResponse{}, nil
			}), nil
		},
	})

	if _, err := svc.Execute(context.Background(), Request{SwarmRoot: swarmRoot, Orchestrator: "main", RunDir: filepath.Join(t.TempDir(), "run-err"), Input: "Investigate"}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("Execute() error = %v, want budget failure", err)
	}
}

func TestServiceExecuteRejectsToolCallOutsideWorkerCatalog(t *testing.T) {
	t.Parallel()

	swarmRoot := tempSwarmRoot(t, `{
		"id": "main",
		"invoke": "kilo/kilo-auto/free",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 1024,
		"runtime_profile": "main",
		"budget_override": {"context": 1200, "output": 1024}
	}`, `{
		"id": "worker",
		"agent": "kilo",
		"invoke": "github-copilot/gpt-5-mini",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 512,
		"budget_override": {"context": 1200, "output": 512}
	}`, `---
id: critic
group: review
intent: identify risks
---
You are a critic.
`)

	svc := NewServiceWithDependencies(Dependencies{
		LoadProfile: func(name string) (orchestratorconfig.Config, error) {
			return orchestratorconfig.Config{Backend: "openai-compatible", MessageAPIFormat: "openai.chat.completions", BaseURL: "http://127.0.0.1:8080", Auth: orchestratorconfig.AuthConfig{Method: orchestratorconfig.AuthMethodBearerTokenEnv, CredentialRef: "KILO_API_KEY"}}, nil
		},
		ResolveBudget: func(ctx context.Context, model swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error) {
			return swarmrun.TokenBudget{ContextWindow: 1200, MaxInputTokens: 176, MaxOutputTokens: 1024}, nil
		},
		NewTransport: func(cfg orchestratorconfig.Config) (swarmrun.ConversationTransport, error) {
			return conversationTransportFunc(func(ctx context.Context, req swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error) {
				return swarmrun.ConversationResponse{ToolCall: &swarmrun.ToolCall{Name: swarmrun.ToolNameRunWorkerNode, Arguments: map[string]string{"model": "unknown", "persona": "critic", "input": "inspect the patch"}}}, nil
			}), nil
		},
	})

	if _, err := svc.Execute(context.Background(), Request{SwarmRoot: swarmRoot, Orchestrator: "main", RunDir: filepath.Join(t.TempDir(), "run-invalid-tool"), Input: "Fix the feature"}); err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("Execute() error = %v, want enum validation failure", err)
	}
}

func tempSwarmRoot(t *testing.T, modelDocs ...string) string {
	t.Helper()
	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(models) error = %v", err)
	}
	for i, doc := range modelDocs {
		name := filepath.Join(modelsDir, filepath.Base([]string{"main.json", "worker.json", "extra.json"}[min(i, 2)]))
		content := doc
		if strings.HasPrefix(strings.TrimSpace(doc), "---") {
			personasDir := filepath.Join(root, "personas")
			if err := os.MkdirAll(personasDir, 0o700); err != nil {
				t.Fatalf("MkdirAll(personas) error = %v", err)
			}
			name = filepath.Join(personasDir, "critic.md")
		}
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}
	return root
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func joinMessageContent(messages []swarmrun.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n")
}

func assertTrimmedFile(t *testing.T, path, want string) {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if strings.TrimSpace(string(bytes)) != want {
		t.Fatalf("file %q = %q, want %q", path, string(bytes), want)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !strings.Contains(string(bytes), want) {
		t.Fatalf("file %q = %q, want substring %q", path, string(bytes), want)
	}
}

type conversationTransportFunc func(context.Context, swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error)

func (f conversationTransportFunc) Send(ctx context.Context, req swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error) {
	return f(ctx, req)
}
