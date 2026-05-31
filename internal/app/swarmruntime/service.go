package swarmruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"swarminator/internal/app/nodeexecution"
	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/swarmrun"
	"swarminator/internal/infra/llm"
	"swarminator/internal/infra/modelsdev"
	"swarminator/internal/infra/orchestratorconfig"
	"swarminator/internal/infra/orchestratortransport"
	"swarminator/internal/infra/runstore"
	"swarminator/internal/infra/swarmcatalog"
)

const (
	maxOrchestratorTurns   = 24
	maxRecentMessages      = 4
	compactionThresholdPct = 85
	nodeSummaryWordLimit   = 48
)

type Request struct {
	SwarmRoot    string
	Orchestrator string
	RunDir       string
	EventSink    string
	Input        string
}

type WorkerRequest struct {
	Model   swarmcatalog.ModelDefinition
	Persona swarmcatalog.PersonaDefinition
	Input   string
}

type WorkerResult struct {
	Output string
}

type Dependencies struct {
	LoadProfile    func(string) (orchestratorconfig.Config, error)
	NewTransport   func(orchestratorconfig.Config) (swarmrun.ConversationTransport, error)
	ResolveBudget  func(context.Context, swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error)
	ExecuteWorker  func(context.Context, WorkerRequest) (WorkerResult, error)
	ExecutablePath func() (string, error)
	StartProcess   func(context.Context, string, []string, string, string) (int, error)
	Now            func() time.Time
}

type Service struct {
	loadProfile    func(string) (orchestratorconfig.Config, error)
	newTransport   func(orchestratorconfig.Config) (swarmrun.ConversationTransport, error)
	resolveBudget  func(context.Context, swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error)
	executeWorker  func(context.Context, WorkerRequest) (WorkerResult, error)
	executablePath func() (string, error)
	startProcess   func(context.Context, string, []string, string, string) (int, error)
	now            func() time.Time
}

func NewService() *Service {
	return NewServiceWithDependencies(Dependencies{})
}

func NewServiceWithDependencies(deps Dependencies) *Service {
	loadProfile := deps.LoadProfile
	if loadProfile == nil {
		loadProfile = orchestratorconfig.LoadProfile
	}
	newTransport := deps.NewTransport
	if newTransport == nil {
		newTransport = func(cfg orchestratorconfig.Config) (swarmrun.ConversationTransport, error) {
			return orchestratortransport.New(cfg)
		}
	}
	resolveBudget := deps.ResolveBudget
	if resolveBudget == nil {
		resolver := modelsdev.NewResolver(modelsdev.Options{})
		resolveBudget = func(ctx context.Context, model swarmcatalog.ModelDefinition) (swarmrun.TokenBudget, error) {
			if model.BudgetOverride.IsSet() {
				input := model.BudgetOverride.Input
				if input <= 0 {
					input = model.BudgetOverride.Context
				}
				return swarmrun.TokenBudget{ContextWindow: model.BudgetOverride.Context, MaxInputTokens: input, MaxOutputTokens: model.BudgetOverride.Output}, nil
			}
			return resolver.Resolve(ctx, model.BudgetRef)
		}
	}
	executeWorker := deps.ExecuteWorker
	if executeWorker == nil {
		executeWorker = defaultExecuteWorker
	}
	executablePath := deps.ExecutablePath
	if executablePath == nil {
		executablePath = os.Executable
	}
	startProcess := deps.StartProcess
	if startProcess == nil {
		startProcess = defaultStartProcess
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		loadProfile:    loadProfile,
		newTransport:   newTransport,
		resolveBudget:  resolveBudget,
		executeWorker:  executeWorker,
		executablePath: executablePath,
		startProcess:   startProcess,
		now:            now,
	}
}

func (s *Service) Execute(ctx context.Context, req Request) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}

	store, err := newRunStore(req.RunDir, req.EventSink)
	if err != nil {
		return "", err
	}
	if err := store.AcquireWriterLock(); err != nil {
		return "", err
	}
	defer store.ReleaseWriterLock()

	input, err := s.prepareInput(store, req.Input)
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	status := swarmrun.Status{
		RunID:        filepath.Base(strings.TrimSpace(req.RunDir)),
		State:        swarmrun.RunStateRunning,
		Orchestrator: req.Orchestrator,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.WriteStatus(status); err != nil {
		return "", err
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventRunStarted, At: now, Detail: "private swarm run started"}); err != nil {
		return "", err
	}

	model, err := swarmcatalog.LoadOrchestratorModel(req.SwarmRoot, req.Orchestrator)
	if err != nil {
		return s.failRun(store, status, err)
	}
	workerCatalog, err := swarmcatalog.LoadWorkerCatalog(req.SwarmRoot)
	if err != nil {
		return s.failRun(store, status, err)
	}
	profile, err := s.loadProfile(model.RuntimeProfile)
	if err != nil {
		return s.failRun(store, status, err)
	}
	budget, err := s.resolveBudget(ctx, model)
	if err != nil {
		return s.failRun(store, status, fmt.Errorf("resolve budget: %w", err))
	}
	if err := budget.Validate(); err != nil {
		return s.failRun(store, status, fmt.Errorf("resolve budget: %w", err))
	}
	transport, err := s.newTransport(profile)
	if err != nil {
		return s.failRun(store, status, err)
	}

	contract := swarmSystemContract(model.ID)
	memory, err := initializeMemory(store, input)
	if err != nil {
		return s.failRun(store, status, err)
	}
	if err := store.AppendTranscript(swarmrun.Message{Role: swarmrun.RoleSystem, Kind: swarmrun.MessageKindSystemContract, Content: contract}); err != nil {
		return s.failRun(store, status, err)
	}
	transcript := []swarmrun.Message{{Role: swarmrun.RoleUser, Kind: swarmrun.MessageKindUserInput, Content: input}}
	if err := store.AppendTranscript(transcript[0]); err != nil {
		return s.failRun(store, status, err)
	}

	effectiveBudget := effectiveInputBudget(budget, model.MaxOutputTokens)
	if err := effectiveBudget.Validate(); err != nil {
		return s.failRun(store, status, fmt.Errorf("effective budget: %w", err))
	}

	for turn := 1; turn <= maxOrchestratorTurns; turn++ {
		memory, transcript, err = s.maybeCompact(store, memory, transcript, contract, effectiveBudget)
		if err != nil {
			return s.failRun(store, status, err)
		}
		messages := buildConversationMessages(contract, memory, transcript)
		if estimateConversationTokens(messages) > effectiveBudget.MaxInputTokens {
			return s.failRun(store, status, errors.New("conversation exceeds effective input budget after compaction"))
		}

		if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventOrchestratorTurnStarted, At: s.now().UTC(), Detail: fmt.Sprintf("turn=%d", turn)}); err != nil {
			return s.failRun(store, status, err)
		}
		resp, err := transport.Send(ctx, swarmrun.ConversationRequest{Model: model.Invoke, RuntimeProfile: model.RuntimeProfile, MaxOutputTokens: effectiveBudget.MaxOutputTokens, Messages: messages, Tools: availableTools(workerCatalog)})
		if err != nil {
			return s.failRun(store, status, err)
		}
		if resp.ToolCall != nil {
			toolMessage, execErr := s.executeToolCall(ctx, req.SwarmRoot, store, workerCatalog, *resp.ToolCall)
			if execErr != nil {
				return s.failRun(store, status, execErr)
			}
			transcript = append(transcript, toolMessage)
			if err := store.AppendTranscript(toolMessage); err != nil {
				return s.failRun(store, status, err)
			}
			continue
		}

		assistant := swarmrun.Message{Role: swarmrun.RoleAssistant, Kind: swarmrun.MessageKindAssistantOutput, Content: strings.TrimSpace(resp.Message.Content)}
		transcript = append(transcript, assistant)
		if err := store.AppendTranscript(assistant); err != nil {
			return s.failRun(store, status, err)
		}
		return s.completeRun(store, status, assistant.Content)
	}

	return s.failRun(store, status, fmt.Errorf("orchestrator exceeded %d turns without final answer", maxOrchestratorTurns))
}

func (s *Service) Start(ctx context.Context, req Request) (swarmrun.Receipt, error) {
	if err := req.Validate(); err != nil {
		return swarmrun.Receipt{}, err
	}
	store, err := newRunStore(req.RunDir, req.EventSink)
	if err != nil {
		return swarmrun.Receipt{}, err
	}
	if err := store.WriteInput(strings.TrimSpace(req.Input)); err != nil {
		return swarmrun.Receipt{}, err
	}
	if _, err := initializeMemory(store, req.Input); err != nil {
		return swarmrun.Receipt{}, err
	}
	now := s.now().UTC()
	status := swarmrun.Status{RunID: filepath.Base(strings.TrimSpace(req.RunDir)), State: swarmrun.RunStatePending, Orchestrator: req.Orchestrator, StartedAt: now, UpdatedAt: now}
	if err := store.WriteStatus(status); err != nil {
		return swarmrun.Receipt{}, err
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventRunPending, At: now, Detail: "private swarm run queued"}); err != nil {
		return swarmrun.Receipt{}, err
	}

	executable, err := s.executablePath()
	if err != nil {
		return swarmrun.Receipt{}, err
	}
	args := []string{"swarm", "worker", "--swarm-root", req.SwarmRoot, "--orchestrator", req.Orchestrator, "--run-dir", req.RunDir}
	if strings.TrimSpace(req.EventSink) != "" {
		args = append(args, "--event-sink", req.EventSink)
	}
	stdoutPath := store.LogFilePath("worker.stdout")
	stderrPath := store.LogFilePath("worker.stderr")
	pid, err := s.startProcess(ctx, executable, args, stdoutPath, stderrPath)
	if err != nil {
		status.State = swarmrun.RunStateFailed
		status.Error = err.Error()
		status.UpdatedAt = s.now().UTC()
		completedAt := status.UpdatedAt
		status.CompletedAt = &completedAt
		_ = store.WriteStatus(status)
		_ = store.AppendEvent(swarmrun.Event{Type: swarmrun.EventRunFailed, At: completedAt, Detail: err.Error()})
		return swarmrun.Receipt{}, err
	}
	status.WorkerPID = pid
	status.UpdatedAt = s.now().UTC()
	if err := store.WriteStatus(status); err != nil {
		return swarmrun.Receipt{}, err
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventAsyncWorkerStarted, At: s.now().UTC(), Detail: fmt.Sprintf("pid=%d", pid)}); err != nil {
		return swarmrun.Receipt{}, err
	}
	return swarmrun.Receipt{RunID: status.RunID, RunDir: req.RunDir, Status: string(status.State)}, nil
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.SwarmRoot) == "" {
		return errors.New("swarm root is required")
	}
	if strings.TrimSpace(r.Orchestrator) == "" {
		return errors.New("orchestrator is required")
	}
	if strings.TrimSpace(r.RunDir) == "" {
		return errors.New("run dir is required")
	}
	return nil
}

func (s *Service) prepareInput(store *runstore.Store, provided string) (string, error) {
	trimmed := strings.TrimSpace(provided)
	if trimmed != "" {
		if err := store.WriteInput(trimmed); err != nil {
			return "", err
		}
		return trimmed, nil
	}
	trimmed, err := store.ReadInput()
	if err != nil {
		return "", err
	}
	if trimmed == "" {
		return "", errors.New("swarm runtime requires non-empty input")
	}
	return trimmed, nil
}

func initializeMemory(store *runstore.Store, input string) (string, error) {
	current, err := store.ReadMemoryCurrent()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	current = strings.TrimSpace("Current task:\n" + strings.TrimSpace(input))
	if err := store.WriteMemoryCurrent(current); err != nil {
		return "", err
	}
	return current, nil
}

func effectiveInputBudget(budget swarmrun.TokenBudget, requestedMaxOutput int) swarmrun.TokenBudget {
	maxOutput := budget.MaxOutputTokens
	if requestedMaxOutput > 0 && requestedMaxOutput < maxOutput {
		maxOutput = requestedMaxOutput
	}
	maxInput := budget.MaxInputTokens
	remainingContext := budget.ContextWindow - maxOutput
	if remainingContext > 0 && remainingContext < maxInput {
		maxInput = remainingContext
	}
	return swarmrun.TokenBudget{ContextWindow: budget.ContextWindow, MaxInputTokens: maxInput, MaxOutputTokens: maxOutput}
}

func (s *Service) maybeCompact(store *runstore.Store, currentMemory string, transcript []swarmrun.Message, contract string, budget swarmrun.TokenBudget) (string, []swarmrun.Message, error) {
	threshold := budget.MaxInputTokens * compactionThresholdPct / 100
	if threshold <= 0 {
		threshold = budget.MaxInputTokens
	}
	for estimateConversationTokens(buildConversationMessages(contract, currentMemory, transcript)) > threshold && len(transcript) > maxRecentMessages {
		if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventCompactionStarted, At: s.now().UTC(), Detail: fmt.Sprintf("messages=%d", len(transcript))}); err != nil {
			return currentMemory, transcript, err
		}
		var err error
		currentMemory, transcript, err = compactTranscript(store, currentMemory, transcript, maxRecentMessages)
		if err != nil {
			return currentMemory, transcript, err
		}
		if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventCompactionCompleted, At: s.now().UTC(), Detail: fmt.Sprintf("messages=%d", len(transcript))}); err != nil {
			return currentMemory, transcript, err
		}
	}
	return currentMemory, transcript, nil
}

func buildConversationMessages(systemContract, memory string, transcript []swarmrun.Message) []swarmrun.Message {
	messages := []swarmrun.Message{{Role: swarmrun.RoleSystem, Kind: swarmrun.MessageKindSystemContract, Content: strings.TrimSpace(systemContract)}}
	if strings.TrimSpace(memory) != "" {
		messages = append(messages, swarmrun.Message{Role: swarmrun.RoleSystem, Kind: swarmrun.MessageKindMemorySummary, Content: strings.TrimSpace(memory)})
	}
	for _, entry := range transcript {
		message := entry
		message.Content = strings.TrimSpace(message.Content)
		if message.Content == "" {
			continue
		}
		if message.Role == swarmrun.RoleTool {
			message.Role = swarmrun.RoleUser
			prefix := "Tool result"
			if strings.TrimSpace(message.ArtifactRef) != "" {
				prefix = fmt.Sprintf("Tool result (%s)", message.ArtifactRef)
			}
			message.Content = prefix + ":\n" + message.Content
		}
		messages = append(messages, message)
	}
	return messages
}

func compactTranscript(store *runstore.Store, currentMemory string, transcript []swarmrun.Message, keepRecent int) (string, []swarmrun.Message, error) {
	if keepRecent <= 0 || len(transcript) <= keepRecent {
		return currentMemory, transcript, nil
	}
	compacted := transcript[:len(transcript)-keepRecent]
	kept := append([]swarmrun.Message(nil), transcript[len(transcript)-keepRecent:]...)
	if strings.TrimSpace(currentMemory) != "" {
		if _, err := store.WriteMemorySnapshot(currentMemory); err != nil {
			return currentMemory, transcript, err
		}
	}
	newMemory := strings.TrimSpace(currentMemory)
	if newMemory != "" {
		newMemory += "\n\n"
	}
	newMemory += "Compacted history:\n" + summarizeMessages(compacted)
	artifactRefs := summarizeArtifactRefs(kept)
	if artifactRefs != "" {
		newMemory += "\n\nRecent artifacts:\n" + artifactRefs
	}
	if err := store.WriteMemoryCurrent(newMemory); err != nil {
		return currentMemory, transcript, err
	}
	return newMemory, kept, nil
}

func summarizeMessages(messages []swarmrun.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		wordCount := len(strings.Fields(strings.TrimSpace(message.Content)))
		summary := fmt.Sprintf("%d words", wordCount)
		wordLimit := 8
		if message.Kind == swarmrun.MessageKindToolResult {
			wordLimit = nodeSummaryWordLimit
		}
		short := summarizeText(message.Content, wordLimit)
		if short != "" {
			summary = short
		}
		if strings.TrimSpace(message.ArtifactRef) != "" {
			summary += fmt.Sprintf(" [artifact: %s]", message.ArtifactRef)
		}
		parts = append(parts, fmt.Sprintf("- [%s/%s] %s", message.Role, message.Kind, summary))
	}
	return strings.Join(parts, "\n")
}

func summarizeArtifactRefs(messages []swarmrun.Message) string {
	refs := make([]string, 0, len(messages))
	seen := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		ref := strings.TrimSpace(message.ArtifactRef)
		if ref == "" {
			continue
		}
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, "- "+ref)
	}
	return strings.Join(refs, "\n")
}

func estimateConversationTokens(messages []swarmrun.Message) int {
	total := 0
	for _, message := range messages {
		total += len(strings.Fields(message.Content))
	}
	if total == 0 {
		return 0
	}
	return total
}

func summarizeText(text string, wordLimit int) string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) <= wordLimit {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:wordLimit], " ") + " ..."
}

func (s *Service) executeNodeAction(ctx context.Context, swarmRoot string, store *runstore.Store, action swarmrun.NodeAction) (swarmrun.Message, error) {
	model, err := swarmcatalog.LoadWorkerModel(swarmRoot, action.Model)
	if err != nil {
		return swarmrun.Message{}, err
	}
	persona, err := swarmcatalog.LoadPersona(swarmRoot, action.Persona)
	if err != nil {
		return swarmrun.Message{}, err
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventNodeStarted, At: s.now().UTC(), Detail: fmt.Sprintf("model=%s persona=%s", model.ID, persona.ID)}); err != nil {
		return swarmrun.Message{}, err
	}
	result, err := s.executeWorker(ctx, WorkerRequest{Model: model, Persona: persona, Input: action.Input})
	if err != nil {
		return swarmrun.Message{}, err
	}
	artifactRef, err := store.WriteNodeArtifact(fmt.Sprintf("%s-%s", action.Model, action.Persona), result.Output)
	if err != nil {
		return swarmrun.Message{}, err
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventNodeCompleted, At: s.now().UTC(), Detail: artifactRef}); err != nil {
		return swarmrun.Message{}, err
	}
	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "Worker produced no text output."
	}
	toolContent := fmt.Sprintf("Worker output from model `%s` with persona `%s`.\nArtifact: `%s`\n\n%s", model.ID, persona.ID, artifactRef, output)
	return swarmrun.Message{Role: swarmrun.RoleTool, Kind: swarmrun.MessageKindToolResult, Content: toolContent, ArtifactRef: artifactRef, ModelID: model.ID, PersonaID: persona.ID, Name: persona.ID}, nil
}

func (s *Service) executeToolCall(ctx context.Context, swarmRoot string, store *runstore.Store, workerCatalog swarmcatalog.WorkerCatalog, call swarmrun.ToolCall) (swarmrun.Message, error) {
	tools := availableTools(workerCatalog)
	if err := swarmrun.ValidateToolCallAgainstDefinitions(call, tools); err != nil {
		return swarmrun.Message{}, err
	}
	switch call.Name {
	case swarmrun.ToolNameRunWorkerNode:
		return s.executeNodeAction(ctx, swarmRoot, store, swarmrun.NodeAction{
			Model:   strings.TrimSpace(call.Arguments["model"]),
			Persona: strings.TrimSpace(call.Arguments["persona"]),
			Input:   strings.TrimSpace(call.Arguments["input"]),
		})
	default:
		return swarmrun.Message{}, fmt.Errorf("unsupported tool call %q", call.Name)
	}
}

func (s *Service) completeRun(store *runstore.Store, status swarmrun.Status, final string) (string, error) {
	trimmed := strings.TrimSpace(final)
	if trimmed == "" {
		return s.failRun(store, status, errors.New("orchestrator returned an empty final answer"))
	}
	if err := store.WriteFinal(trimmed); err != nil {
		return s.failRun(store, status, err)
	}
	completedAt := s.now().UTC()
	status.State = swarmrun.RunStateCompleted
	status.CompletedAt = &completedAt
	status.UpdatedAt = completedAt
	if err := store.WriteStatus(status); err != nil {
		return "", err
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventRunCompleted, At: completedAt, Detail: "private swarm run completed"}); err != nil {
		return "", err
	}
	return trimmed, nil
}

func (s *Service) failRun(store *runstore.Store, status swarmrun.Status, cause error) (string, error) {
	failedAt := s.now().UTC()
	status.State = swarmrun.RunStateFailed
	status.CompletedAt = &failedAt
	status.UpdatedAt = failedAt
	status.Error = cause.Error()
	_ = store.WriteStatus(status)
	_ = store.AppendEvent(swarmrun.Event{Type: swarmrun.EventRunFailed, At: failedAt, Detail: cause.Error()})
	return "", cause
}

func newRunStore(runDir, eventSink string) (*runstore.Store, error) {
	return runstore.Create(runDir, eventSink)
}

func swarmSystemContract(orchestrator string) string {
	return fmt.Sprintf("You are the private swarm orchestrator for %s. Keep your internal transcript private. Use available tools for external actions such as running worker nodes. When calling a worker tool, use only worker model IDs that exist in swarm-root/models and persona IDs that exist in swarm-root/personas. Worker results will come back as readable Markdown with artifact references; synthesize from that text directly. When you have enough information, reply with the final answer only in plain-text Markdown.", orchestrator)
}

func availableTools(catalog swarmcatalog.WorkerCatalog) []swarmrun.ToolDefinition {
	modelIDs := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		modelIDs = append(modelIDs, model.ID)
	}
	personaIDs := make([]string, 0, len(catalog.Personas))
	personaHints := make([]string, 0, len(catalog.Personas))
	for _, persona := range catalog.Personas {
		personaIDs = append(personaIDs, persona.ID)
		hint := persona.ID
		if strings.TrimSpace(persona.Group) != "" || strings.TrimSpace(persona.Intent) != "" {
			hint = fmt.Sprintf("%s (%s: %s)", persona.ID, strings.TrimSpace(persona.Group), strings.TrimSpace(persona.Intent))
		}
		personaHints = append(personaHints, hint)
	}
	return []swarmrun.ToolDefinition{{
		Name:        swarmrun.ToolNameRunWorkerNode,
		Description: "Run one configured worker node and return its Markdown output with an artifact reference.",
		Parameters: []swarmrun.ToolParameter{
			{Name: "model", Description: "Worker model ID from swarm-root/models. Allowed: " + strings.Join(modelIDs, ", "), Required: true, Type: swarmrun.ToolParameterTypeString, Enum: modelIDs},
			{Name: "persona", Description: "Persona ID from swarm-root/personas. Allowed: " + strings.Join(personaHints, ", "), Required: true, Type: swarmrun.ToolParameterTypeString, Enum: personaIDs},
			{Name: "input", Description: "Plain-text worker task to execute.", Required: true, Type: swarmrun.ToolParameterTypeString},
		},
	}}
}

func defaultExecuteWorker(ctx context.Context, req WorkerRequest) (WorkerResult, error) {
	service := nodeexecution.NewServiceWithProvider(llm.NewUnifiedProvider("", req.Model.Agent))
	output, err := service.Execute(ctx, agent.CompletionRequest{ModelID: req.Model.Invoke, Persona: req.Persona.Prompt, Input: req.Input})
	if err != nil {
		return WorkerResult{}, err
	}
	return WorkerResult{Output: output}, nil
}

func defaultStartProcess(ctx context.Context, executable string, args []string, stdoutPath, stderrPath string) (int, error) {
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open worker stdout log %q: %w", stdoutPath, err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open worker stderr log %q: %w", stderrPath, err)
	}
	defer stderrFile.Close()

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start worker process: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, fmt.Errorf("release worker process handle: %w", err)
	}
	return pid, nil
}
