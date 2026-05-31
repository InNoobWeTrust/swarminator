# Private Swarm Runtime Architecture

This document explains the current private swarm runtime from system context down to file-level behavior. It is descriptive: it documents what the code currently does so the release decision can be made against the actual implementation.

## 1. System Context

`swarminator` now has two execution layers:

| Layer | Entry Surface | Responsibility | Persistence Model |
|-------|---------------|----------------|-------------------|
| Legacy node runner | `swarminator --agent ... -m ... -p ... -t ...` | Execute one explicit worker node and print its output | Stateless, aside from provider behavior |
| Private swarm runtime | `swarminator swarm ...` and `swarminator runs ...` | Run a multi-turn orchestrator, delegate work to configured worker nodes, and persist a private run record | File-backed run directory |

The current change is primarily about the second layer. Supporting README, CLI reference, parser, and tests were updated to expose and verify that new runtime slice.

## 2. High-Level Design

At a high level, the private runtime is a local orchestrator loop with a file-backed control plane:

```text
stdin
  |
  v
cmd/swarminator
  |
  +--> internal/cli.Parse
  |
  +--> internal/app/swarmruntime.Service
         |
         +--> internal/infra/runstore.Store
         +--> internal/infra/swarmcatalog
         +--> internal/infra/orchestratorconfig
         +--> internal/infra/modelsdev
         +--> internal/infra/orchestratortransport
         +--> internal/app/nodeexecution
                 |
                 +--> internal/infra/llm.UnifiedProvider
                         |
                         +--> local agent binary (kilo / gemini / claude / codex / command-code)
```

Two design choices define the runtime:

1. The orchestrator itself is just another model call, but it gets a private system contract plus one transport-native tool, `run_worker_node`.
2. The run directory is the durable source of truth for status, transcript, events, memory, and worker artifacts.

## 3. Top-Level Runtime Flow

### 3.1 `swarm exec`

`cmd/swarminator/main.go` routes `swarm exec` into `runSwarmExec()`, which reads required stdin and calls `swarmruntime.Service.Execute()`.

Inside `Execute()` the runtime does this, in order:

1. Validate `SwarmRoot`, `Orchestrator`, and `RunDir`.
2. Create or reopen the run directory via `runstore.Create()`.
3. Acquire `.writer.lock` so only one writer can mutate that run at a time.
4. Persist `input.txt` and initialize `memory/current.md`.
5. Write `status.json` with `state=running` and append `run_started` to the event log.
6. Load the orchestrator model from `swarm-root/models`.
7. Load the worker catalog from `swarm-root/models` and `swarm-root/personas`.
8. Load the named XDG orchestrator profile.
9. Resolve token budget from local override or `models.dev`.
10. Create the OpenAI-compatible transport.
11. Build the orchestrator system contract and initial transcript.
12. Enter the bounded orchestrator turn loop.

If the loop ends with assistant text, the runtime writes `final.md`, marks the run `completed`, appends `run_completed`, and prints only the final answer to stdout.

### 3.2 `swarm start` and `swarm worker`

`swarm start` calls `swarmruntime.Service.Start()`.

That path does not execute the orchestrator loop inline. Instead it:

1. Creates the run directory.
2. Writes `input.txt` and initial `memory/current.md`.
3. Writes `status.json` with `state=pending`.
4. Spawns the current binary as `swarminator swarm worker ...`.
5. Redirects worker stdout/stderr into `logs/worker.stdout.log` and `logs/worker.stderr.log`.
6. Stores the worker PID in `status.json`.
7. Returns a small JSON receipt.

`swarm worker` then re-enters the same `Execute()` path after reading `input.txt` from the run directory.

This means the async path is not a different runtime. It is the same runtime invoked through a detached self-exec worker process.

### 3.3 `runs final`, `runs inspect`, `runs tail`, `runs wait`

These commands route into `internal/app/runinspect/service.go` and are intentionally read-only.

- `Final()` reads `final.md`.
- `Inspect()` reads `status.json` and emits important paths.
- `Tail()` reads the raw event log.
- `Wait()` polls `status.json` until the run is `completed` or `failed`.

The tests explicitly verify that inspection does not mutate `status.json`.

## 4. Orchestrator Loop Mechanics

The orchestrator loop in `internal/app/swarmruntime/service.go` is the core of the new behavior.

### 4.1 System Contract

The runtime synthesizes a system prompt with `swarmSystemContract()`.

That contract tells the orchestrator to:

- keep the internal transcript private,
- use available tools for external actions,
- choose only configured worker model IDs and persona IDs,
- synthesize from returned worker Markdown,
- emit the final answer as plain-text Markdown.

This is the runtime's behavioral contract with the orchestrator model.

### 4.2 Tool Publication

The runtime publishes exactly one tool today:

```text
run_worker_node(model, persona, input)
```

`availableTools()` derives the `model` and `persona` enum values from the live worker catalog. This matters because the orchestrator is not allowed to invent arbitrary worker IDs; it must choose from configured options.

### 4.3 Tool Execution

When the backend returns a tool call:

1. `ValidateToolCallAgainstDefinitions()` checks the name and enum-constrained arguments.
2. `executeNodeAction()` loads the chosen worker model and persona from disk.
3. The runtime logs `node_started`.
4. `defaultExecuteWorker()` routes the work through `nodeexecution.NewServiceWithProvider(llm.NewUnifiedProvider(...))`.
5. The worker result is written to `nodes/<model>-<persona>.md`.
6. The runtime logs `node_completed` and appends a tool-result transcript entry.

The important low-level detail is what happens next: tool results are stored as `RoleTool` in the private transcript, but before the next orchestrator turn they are normalized back into a user-visible message that looks like:

```text
Tool result (nodes/worker-critic.md):
Worker output from model `...` with persona `...`.
Artifact: `nodes/...`

<raw worker markdown>
```

So the current runtime does not implement a strict tool-result round-trip protocol. It translates worker output back into readable prompt text for the next turn.

### 4.4 Completion

When the backend returns assistant text instead of a tool call, the runtime treats that as the final answer.

The final answer must be non-empty. Otherwise the run fails.

## 5. Context and Budget Management

### 5.1 Budget Resolution

The runtime resolves token limits in this order:

1. `budget_override` on the model definition, if set.
2. `models.dev` metadata using `budget_ref`.

Resolved values are normalized into:

- `ContextWindow`
- `MaxInputTokens`
- `MaxOutputTokens`

The runtime then computes an effective input budget by reserving room for output tokens.

### 5.2 Compaction

The runtime uses simple word-count estimation, not model-native tokenization.

When estimated prompt size crosses 85 percent of effective input budget:

1. Older transcript entries are compacted.
2. Previous memory is snapshotted to `memory/snapshot-NNN.md`.
3. `memory/current.md` is rewritten with:
   - prior memory,
   - a summarized history block,
   - recent artifact references.
4. Only the most recent four transcript messages remain in the live prompt.

This keeps the prompt bounded, but it also means context management is approximate and heavily dependent on the quality of text summarization.

## 6. Persistence Model

The private runtime is easiest to understand by looking at the run directory layout.

```text
run-123/
  status.json
  events.jsonl
  input.txt
  final.md
  transcript.private.jsonl
  .writer.lock
  memory/
    current.md
    snapshot-001.md
  nodes/
    worker-critic.md
  logs/
    worker.stdout.log
    worker.stderr.log
  artifacts/
```

### 6.1 What each file means

- `status.json`: terminal and non-terminal run state, plus important artifact paths.
- `events.jsonl`: append-only lifecycle journal.
- `input.txt`: original request text.
- `final.md`: final answer returned to the caller.
- `transcript.private.jsonl`: private internal conversation record.
- `memory/current.md`: active compacted memory summary.
- `memory/snapshot-*.md`: previous memory states saved during compaction.
- `nodes/*.md`: raw worker outputs.
- `logs/*.log`: detached worker stdout/stderr.
- `artifacts/`: created now but unused by the current implementation.

### 6.2 Atomicity and locking

`runstore.Store` writes JSON and text files through temp-file-plus-rename helpers and uses an `O_EXCL` lock file for writer exclusivity.

This gives good single-writer safety, but it also creates an operational constraint: if the process dies and leaves `.writer.lock` behind, the next writer must clean that up manually.

## 7. State Machine

The lifecycle state machine is intentionally small:

```text
pending  -> running -> completed
   |           |
   |           -> failed
   -> failed
```

Notably absent:

- `cancelled`
- `timed_out`
- `abandoned`
- `stale`

All non-success terminal behavior currently collapses into `failed`.

## 8. Configuration and Dependency Boundaries

The runtime depends on four external sources of truth.

### 8.1 Swarm Root

`--swarm-root` supplies:

- `models/*.json|yaml|yml`
- `personas/*.md`

This is the local contract for orchestrator and worker definitions.

### 8.2 XDG Orchestrator Config

`internal/infra/orchestratorconfig/config.go` loads:

- `~/.config/swarminator/orchestrator.json` for `default`
- `~/.config/swarminator/orchestrators/<profile>.json` for named profiles

The profile controls backend type, API format, base URL, auth method, timeout, and optional env file.

### 8.3 `models.dev`

`internal/infra/modelsdev/resolver.go` is a metadata dependency, not an inference dependency. It only exists to translate `budget_ref` into token-budget numbers.

If no local override exists and no valid cache is available, budget resolution failure stops the run.

### 8.4 Local Agent Binaries

Worker execution depends on the existing `llm.UnifiedProvider`, which still requires an explicit agent and delegates to local binaries such as `kilo`, `gemini`, `claude`, `codex`, or `cmd`.

That means the swarm runtime is not a standalone model-execution engine. It is an orchestrator layer on top of the existing worker-node execution stack.

## 9. Low-Level Package Map

| Package | Key Types / Functions | Role in the runtime |
|---------|-----------------------|---------------------|
| `cmd/swarminator` | `runSwarmExec`, `runSwarmStart`, `runSwarmWorker`, `runRuns*` | CLI dispatch and stdout contract |
| `internal/cli` | `Parse`, `parseSwarmArgs`, `parseRunsArgs` | command grammar and validation |
| `internal/app/swarmruntime` | `Service.Execute`, `Service.Start`, `executeToolCall`, `maybeCompact` | orchestration core |
| `internal/app/runinspect` | `Final`, `Inspect`, `Tail`, `Wait` | read-only inspection facade |
| `internal/domain/swarmrun` | `Status`, `Event`, `Message`, `ToolDefinition`, `ToolCall`, `TokenBudget` | domain vocabulary and validation |
| `internal/infra/runstore` | `Create`, `OpenExisting`, `WriteStatus`, `AppendEvent`, `AppendTranscript` | file persistence and locking |
| `internal/infra/swarmcatalog` | `LoadOrchestratorModel`, `LoadWorkerCatalog`, `LoadPersona` | swarm-root discovery |
| `internal/infra/modelsdev` | `Resolver.Resolve` | budget metadata lookup |
| `internal/infra/orchestratorconfig` | `LoadProfile`, `LoadEnvFile`, `ResolveEnvReferences` | transport profile loading |
| `internal/infra/orchestratortransport` | `Transport.Send`, `buildOpenAIChatRequest` | backend wire protocol |
| `internal/app/nodeexecution` | `Service.Execute` | worker-node delegation |
| `internal/infra/llm` | `UnifiedProvider.Complete` | existing agent/provider execution layer |

## 10. Current Release-Sensitive Constraints

These are the main constraints that matter for release review.

### 10.1 `--run-dir` must currently be treated as fresh and unique

The runtime does not clear old transcript, events, node outputs, or memory. `initializeMemory()` preserves an existing `memory/current.md`.

Implication: reusing an old run directory can blend old state into a new run.

### 10.2 Async worker supervision is intentionally minimal

`swarm start` detaches a worker and records its PID plus logs, but the parent does not supervise it afterward.

Implication: terminal state depends on the worker updating `status.json` itself.

### 10.3 Privacy is permission-based, not encryption-based

The runtime uses `0700` and `0600` permissions, but it does not encrypt transcript, memory, worker outputs, or logs.

Implication: run directories are sensitive local artifacts and should be treated that way operationally.

### 10.4 Token control is approximate

Compaction is driven by word counts, not backend tokenizer counts.

Implication: the runtime reduces context pressure, but it cannot guarantee exact token fit across backends.

### 10.5 Only one worker tool exists and only one tool call is consumed per response

The current design is intentionally narrow.

Implication: the runtime is easy to audit, but it is not yet a general orchestration platform with parallel tool fan-out or arbitrary action surfaces.

## 11. Practical Release Reading

If this change is released as a single-user local CLI feature, the architecture is coherent:

- the CLI surface is explicit,
- the persistence story is understandable,
- the inspection story is strong,
- the worker delegation boundary is narrow,
- the failure path is mostly fail-closed.

If the intended release expects stronger guarantees - fresh-run enforcement, background-worker supervision, secret-safe persistence, or shared-environment isolation - the current implementation should be treated as a solid first slice, but not yet the final release shape.

## 12. Verification Snapshot

The current tree passes `go test ./...`.

The strongest coverage for this architecture lives in:

- `internal/app/swarmruntime/service_test.go`
- `internal/app/runinspect/service_test.go`
- `internal/infra/runstore/store_test.go`
- `internal/infra/orchestratorconfig/config_test.go`
- `internal/infra/orchestratortransport/transport_test.go`
- `internal/infra/swarmcatalog/catalog_test.go`
- `internal/infra/modelsdev/resolver_test.go`

No `GLOSSARY.md` exists in the repo today, so the terminology in this document follows the current code directly.
