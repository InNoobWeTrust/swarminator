# TRD: Private Swarm Runtime Release Audit

> **Status**: draft
> **Owner**: user-requested
> **Created**: 2026-05-31
> **Scope**: current uncommitted private swarm runtime change

## Parent PRD

Direct user request - document and audit the current private swarm runtime architecture before deciding release readiness.

## Technical Overview

The current change adds a second execution layer to `swarminator`. The legacy path still runs one explicit worker node from piped stdin. The new path adds a private swarm runtime that keeps orchestrator state, worker artifacts, events, memory, and final output inside a caller-supplied run directory. The CLI surface for that slice lives in `cmd/swarminator/main.go` and `internal/cli/args.go`, while the runtime core lives in `internal/app/swarmruntime/service.go`.

The design is local-first and file-backed. A run is created under `--run-dir`, the orchestrator model is loaded from `--swarm-root/models`, personas are loaded from `--swarm-root/personas`, runtime transport settings are loaded from XDG-scoped orchestrator config, and worker-node execution is delegated back into the existing `nodeexecution` and `llm` provider stack. Read-only inspection commands then operate entirely from files already written into the run directory.

The architecture is release-meaningful because it introduces new durability, privacy, process-lifecycle, and correctness boundaries that did not exist in the one-shot node runner. The key question is not whether the code works on the happy path - the test suite shows that it does - but whether the persistence model, background-worker behavior, trust boundaries, and failure handling are sufficiently controlled for release.

## Architecture Decisions

### ADR-1: Separate private swarm runtime from legacy one-shot node execution

- **Context**: The existing binary already had a stable one-shot node path with explicit `--agent`, `-m`, `-p`, and `-t` execution. The new behavior adds multi-turn orchestration, worker delegation, persistence, and later inspection.
- **Decision**: Introduce dedicated `swarm` and `runs` CLI namespaces instead of extending the legacy node path in place.
- **Rationale**: This preserves the old execution contract while allowing the private swarm runtime to own a different state model, different prerequisites, and different operational expectations.
- **Alternatives Considered**:
  - Extend the legacy node command with swarm flags - rejected because it would mix stateless and stateful execution semantics.
  - Build a separate binary - rejected because self-exec worker mode depends on reusing the existing binary and provider stack.

### ADR-2: Make the run directory the system of record

- **Context**: The runtime needs durable status, transcript, events, memory, node outputs, and detached-worker logs.
- **Decision**: Persist all run state inside a private `--run-dir` managed by `internal/infra/runstore/store.go`.
- **Rationale**: A file-backed run directory gives a clear inspection surface, simple local durability, and a natural privacy boundary through file permissions.
- **Alternatives Considered**:
  - Keep state in memory only - rejected because `swarm start` and `runs inspect` require post-execution visibility.
  - Add a database - rejected because the current release is a local CLI, not a long-lived service.

### ADR-3: Expose worker-node execution as a transport-native orchestrator tool

- **Context**: The orchestrator must be able to dispatch external work without direct file or shell access.
- **Decision**: Publish a single `run_worker_node` tool whose allowed `model` and `persona` values are generated from the current worker catalog.
- **Rationale**: The tool contract keeps orchestration explicit, constrains model/persona choices to configured entries, and fits the current OpenAI-compatible chat-completions transport.
- **Alternatives Considered**:
  - Let the orchestrator invent model and persona IDs - rejected because invalid worker routing would become normal runtime behavior.
  - Use shell commands as tool payloads - rejected because the trust boundary would become too wide.

### ADR-4: Fail closed on configuration, catalog, and budget prerequisites

- **Context**: The orchestrator cannot safely run if its transport profile, token budget, worker catalog, or tool contract are unknown.
- **Decision**: Stop the run immediately when profile loading, worker catalog loading, budget resolution, tool validation, or final-answer validation fails.
- **Rationale**: This prevents silent degradation into undefined orchestration behavior.
- **Alternatives Considered**:
  - Best-effort fallback with guessed limits - rejected because the runtime would stop being auditable.
  - Continue without worker-tool validation - rejected because invalid tool calls would become a normal part of control flow.

### ADR-5: Implement async execution as self-exec worker mode

- **Context**: `swarm start` needs to return quickly while still executing the same runtime behavior as `swarm exec`.
- **Decision**: Spawn the current binary as `swarminator swarm worker ...` and persist the worker PID plus stdout/stderr log paths into the run directory.
- **Rationale**: This reuses the same code path and avoids a separate daemon or queue subsystem.
- **Alternatives Considered**:
  - Thread or goroutine backgrounding inside the parent process - rejected because the CLI would lose the run when the parent exits.
  - Dedicated queue/worker service - rejected because it is larger than the current release scope.

### ADR-6: Bound orchestration context with in-band compaction

- **Context**: Multi-turn orchestration can exceed model context windows if all history is retained.
- **Decision**: Keep the system contract, current memory summary, and only a bounded number of recent transcript entries; compact older history into `memory/current.md` snapshots when estimated usage crosses 85 percent of effective input budget.
- **Rationale**: This keeps the runtime local and deterministic without introducing a separate summarization service.
- **Alternatives Considered**:
  - Keep the full transcript forever - rejected because context overflow becomes inevitable.
  - Truncate history without summarization - rejected because the orchestrator would lose working context too aggressively.

## System Components

- **CLI entrypoint**: `cmd/swarminator/main.go` dispatches `swarm exec`, `swarm start`, `swarm worker`, and `runs *` commands.
- **CLI parser**: `internal/cli/args.go` defines the swarm/runs argument contract and keeps those subcommands separate from legacy node execution.
- **Swarm runtime service**: `internal/app/swarmruntime/service.go` owns run lifecycle, compaction, tool execution, status transitions, and final answer persistence.
- **Run inspection service**: `internal/app/runinspect/service.go` provides read-only final/inspect/tail/wait behavior over an existing run directory.
- **Run domain model**: `internal/domain/swarmrun/*.go` defines run state, events, messages, tool contracts, token budgets, transport interfaces, and validation rules.
- **Run store**: `internal/infra/runstore/store.go` materializes the run directory, enforces local file permissions, appends events/transcript entries, and provides writer locking.
- **Swarm catalog**: `internal/infra/swarmcatalog/catalog.go` loads orchestrator models, worker models, and personas from `--swarm-root`.
- **Budget resolver**: `internal/infra/modelsdev/resolver.go` resolves `budget_ref` values from cached `models.dev` metadata when no local override is present.
- **Orchestrator config**: `internal/infra/orchestratorconfig/config.go` loads XDG-scoped transport config, optional env files, and env references.
- **Orchestrator transport**: `internal/infra/orchestratortransport/transport.go` translates `swarmrun.ConversationRequest` into OpenAI-compatible chat-completions HTTP requests.
- **Worker execution boundary**: `internal/app/nodeexecution/service.go` plus `internal/infra/llm/unified.go` route worker requests into the existing agent/provider stack.

## API Contracts / Interfaces

### CLI Contract

```text
cat input.txt | swarminator swarm exec --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]
cat input.txt | swarminator swarm start --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]
swarminator swarm worker --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]
swarminator runs final --run-dir PATH
swarminator runs inspect --run-dir PATH
swarminator runs tail --run-dir PATH
swarminator runs wait --run-dir PATH
```

Input contract:

- `swarm exec` and `swarm start` require non-empty piped stdin.
- `swarm worker` reads input from `input.txt` inside the run directory.
- `--swarm-root`, `--orchestrator`, and `--run-dir` are mandatory for swarm commands.
- `--event-sink` must be a `file://` URI that resolves inside `--run-dir`.

Output contract:

- `swarm exec` prints only the final answer.
- `swarm start` prints only a JSON receipt.
- `runs inspect` prints JSON metadata.
- `runs final`, `runs tail`, and `runs wait` print existing persisted data only.

### Runtime Request Contract

```go
type Request struct {
    SwarmRoot    string
    Orchestrator string
    RunDir       string
    EventSink    string
    Input        string
}
```

Rules:

- `SwarmRoot`, `Orchestrator`, and `RunDir` must be non-empty.
- `Input` may be supplied directly for synchronous execution or read from `input.txt` when empty.
- `RunDir` is both the persistence root and the concurrency boundary.

### Conversation Transport Contract

```go
type ConversationRequest struct {
    Model           string
    Messages        []Message
    Tools           []ToolDefinition
    MaxOutputTokens int
    Temperature     float64
    RuntimeProfile  string
}

type ConversationTransport interface {
    Send(ctx context.Context, req ConversationRequest) (ConversationResponse, error)
}
```

Rules:

- The current transport supports only `backend=openai-compatible` and `message_api_format=openai.chat.completions`.
- The orchestrator response may be either plain assistant text or a single parsed tool call.
- Multiple tool calls in one backend response are not processed; only the first parsed tool call is used.

### Worker Tool Contract

```text
Tool: run_worker_node

Arguments:
  - model   string, required, enum from worker catalog
  - persona string, required, enum from persona catalog
  - input   string, required, plain-text worker task
```

Runtime behavior:

- Tool calls are validated against the generated tool definition before execution.
- Worker output is persisted to `nodes/*.md` and fed back into the next orchestrator turn as readable Markdown plus an artifact reference.

### Run Inspection Contract

```go
type InspectResult struct {
    RunDir         string
    Status         swarmrun.Status
    StatusPath     string
    EventsPath     string
    FinalPath      string
    RequestPath    string
    TranscriptPath string
    MemoryPath     string
    ArtifactsDir   string
    NodesDir       string
    LogsDir        string
}
```

Rules:

- Inspection commands must not mutate run artifacts.
- `Wait()` completes only when `status.json` transitions to `completed` or `failed`, or when the caller context is cancelled.

## Data Models

### Run Status

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `run_id` | string | derived from `filepath.Base(runDir)` | Stable identifier for the run directory |
| `state` | enum | `pending`, `running`, `completed`, `failed` | Current lifecycle state |
| `orchestrator` | string | optional but expected for swarm runs | Orchestrator model ID |
| `worker_pid` | int | async only | Detached worker PID from `swarm start` |
| `started_at` | timestamp | UTC | First persisted lifecycle timestamp |
| `updated_at` | timestamp | UTC | Latest status update |
| `completed_at` | timestamp | optional | Terminal completion time |
| `error` | string | optional | Terminal failure text |
| `events_path`, `final_path`, `request_path`, `memory_path`, `nodes_dir`, `artifacts_dir`, `logs_dir` | string | absolute paths | Persisted pointers to run artifacts |

### Run Event

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `type` | enum | fixed set in `internal/domain/swarmrun/run.go` | Lifecycle or orchestration event |
| `at` | timestamp | UTC | Event time |
| `detail` | string | optional | Human-readable event detail |

### Transcript Message

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `role` | enum | `system`, `user`, `assistant`, `tool` | Message source role |
| `kind` | enum | contract, memory, input, output, tool result | Runtime-specific message classification |
| `content` | string | required | Persisted message body |
| `artifact_ref` | string | optional | Relative path to a persisted artifact |
| `model_id` | string | optional | Worker model used for tool result |
| `persona_id` | string | optional | Worker persona used for tool result |

### Run Directory Layout

| Path | Purpose | Notes |
|------|---------|-------|
| `status.json` | current run status | rewritten atomically |
| `events.jsonl` or custom sink | append-only lifecycle journal | may be relocated within run dir |
| `input.txt` | original request text | rewritten on start/execute |
| `final.md` | final orchestrator answer | written only on completion |
| `transcript.private.jsonl` | private internal transcript | append-only |
| `memory/current.md` | active compacted memory | overwritten during compaction |
| `memory/snapshot-NNN.md` | prior memory states | written before memory replacement |
| `nodes/*.md` | raw worker outputs | artifact references point here |
| `logs/*.log` | detached worker stdout/stderr | async only |
| `artifacts/` | reserved directory | currently created but not used by this slice |
| `.writer.lock` | writer exclusivity marker | removed only on clean unlock |

### Catalog Models

| Entity | Key Fields | Constraints | Description |
|--------|------------|-------------|-------------|
| `ModelDefinition` | `id`, `invoke`, `budget_ref` or `budget_override`, `max_output_tokens` | orchestrator models require `runtime_profile`; worker models require `agent` | Configured model entry under `swarm-root/models` |
| `PersonaDefinition` | `id`, `group`, `intent`, `prompt` | Markdown frontmatter plus body required | Configured persona entry under `swarm-root/personas` |

## Security Assessment

> Security-lens review applied during drafting. This section documents current controls and current release risks.

### Authentication & Authorization

- **Auth model**: The private runtime authenticates only to the orchestrator backend. Auth is configured through `orchestratorconfig.Config.Auth` and resolved from environment variables or env-file-populated environment variables.
- **Local access model**: There is no application-level user auth. The trust boundary is the local OS user who can invoke `swarminator` and access `--run-dir`.
- **Privilege boundary**: Run privacy is enforced primarily through filesystem permissions (`0700` directories, `0600` files) and through the event-sink containment rule.
- **Release risk**: Anyone with the same local account can inspect or reuse the run directory path. This is acceptable only if the release is scoped as a single-user local CLI, not a shared service or multi-tenant runtime.

### Data Protection

- **At-rest model**: Transcript, memory, final output, worker outputs, and worker logs are stored in plain text on disk but with restrictive permissions.
- **Secrets handling**: Orchestrator credentials are not hardcoded in the repo, but `LoadEnvFile()` promotes env-file values into the ambient process environment.
- **Retention**: There is currently no automatic cleanup, expiry, or redaction of run directories.
- **Release risk**: Sensitive prompts, secrets echoed by models, or backend data reflected in worker outputs will persist unredacted in `input.txt`, `transcript.private.jsonl`, `memory/current.md`, `nodes/*.md`, and log files.

### Input Validation & Injection Prevention

- **CLI boundaries**: Swarm subcommands validate required flags and reject unsupported options.
- **Path boundaries**: `--event-sink` is limited to `file://` URIs and must remain inside the run directory.
- **Tool-call boundaries**: Tool calls are validated against generated enum-constrained `model` and `persona` values plus required non-empty input.
- **Execution boundary**: This slice does not shell out with user-provided command strings; worker execution is delegated via typed provider calls.
- **Release risk**: The runtime still passes raw natural-language task text straight to external model backends and local agent binaries. The runtime assumes those lower layers are safe to receive arbitrary prompt content.

### Infrastructure & Configuration

- **Config source**: Orchestrator config is loaded from XDG-scoped JSON files, optionally plus a dotenv-style env file.
- **Network boundary**: The orchestrator backend is currently a caller-configured absolute base URL plus `/chat/completions`.
- **Default exposure**: The runtime is local-only, but it depends on external HTTP transport and local executable availability.
- **Release risk**: Detached async execution has no supervisor beyond the run directory. A worker that exits before updating `status.json` can leave the run stuck in `pending` or `running` without a native recovery mechanism.

### Supply Chain & Dependencies

- **External dependencies**: The runtime depends on the configured chat-completions backend, local agent binaries, and `models.dev` metadata unless local budget overrides are present.
- **Parsing surface**: Swarm model YAML parsing is custom and intentionally narrow.
- **Caching**: `models.dev` metadata is cached locally with a 24-hour TTL and can be used when refresh fails after a cache already exists.
- **Release risk**: There is no pinned schema validation against upstream `models.dev`; compatibility relies on the current JSON shape remaining stable.

### Failure Modes

- **Fail-closed controls**: Missing transport config, invalid tool calls, absent catalog entries, missing budget metadata, and empty final answers all fail the run explicitly.
- **Best-effort persistence on failure**: `failRun()` attempts to persist failed status and a terminal event, but those writes are intentionally ignored if they themselves fail.
- **Detached worker mode**: `swarm start` persists a PID, but completion still depends on the worker updating `status.json`.
- **Release risk**:
  - Reusing an existing `--run-dir` is currently unsafe because prior memory, transcript, events, nodes, and logs are not cleared and `initializeMemory()` preserves existing `memory/current.md`.
  - A stale `.writer.lock` file blocks future writers until manual cleanup.
  - There is no explicit `cancelled` state, no stale-run detector, and no retry policy for failed worker calls.

## Non-Functional Requirements

- **Performance**:
  - The release is currently scoped to local CLI usage, not server-grade throughput.
  - The orchestrator transport uses a default 30-second HTTP timeout unless overridden by profile config.
  - `runs wait` polls every 250 milliseconds.
  - Orchestration is capped at 24 turns.
- **Scalability**:
  - Context is bounded by token budget plus compaction (`85%` trigger, `4` recent messages retained).
  - Worker execution is strictly sequential; no parallel worker fan-out exists in the current runtime.
  - The run store is designed for one active writer per run directory, not shared concurrent writers.
- **Observability**:
  - The primary inspection surface is the run directory itself: `status.json`, events, transcript, memory snapshots, node outputs, and worker logs.
  - There is no external telemetry exporter, metric sink, or structured log aggregation built into this slice.
- **Reliability**:
  - `go test ./...` passes for the current change set, including swarm runtime, run inspection, transport, config, run store, catalog, and budget resolver tests.
  - The runtime intentionally fails closed on several invalid states, but async worker recovery and stale-run cleanup remain manual.

## Child BDD Specs

- No dedicated BDD specs exist yet for this slice.
- Current executable verification coverage is provided by:
  - `internal/app/swarmruntime/service_test.go` - run creation, transport flow, worker tool execution, compaction, invalid-tool rejection, budget failure.
  - `internal/app/runinspect/service_test.go` - final/inspect/tail/wait behavior and read-only inspection.
  - `internal/infra/runstore/store_test.go` - permissions, lock enforcement, and event-sink containment.
  - `internal/infra/orchestratorconfig/config_test.go` - profile resolution, env-file loading, and validation.
  - `internal/infra/orchestratortransport/transport_test.go` - request shaping, auth headers, tool-call parsing, timeout selection.
  - `internal/infra/swarmcatalog/catalog_test.go` - model/persona loading and duplicate-ID rejection.
  - `internal/infra/modelsdev/resolver_test.go` - explicit and derived input limits, stale-cache fallback, missing-model failure.

## ⚔ Challenge Gate

> **Status**: revisions-needed
> **Challenger**: architecture trace + security lens + release audit pass
> **Date**: 2026-05-31

### Debate Record

| # | Vector | Challenge | Response | Verdict |
|---|--------|-----------|----------|---------|
| 1 | correctness | Can the runtime safely treat `--run-dir` as reusable storage when `runstore.Create()` does not clear old artifacts and `initializeMemory()` preserves existing memory? | No. The current implementation assumes a fresh unique run directory even though that requirement is not enforced by code. Reuse contaminates state across runs. | challenger-won |
| 2 | runtime safety | Is detached async execution release-safe when the parent stores only a PID and relies on the worker to update `status.json` itself? | Partially. The happy path works, but there is no supervisor, cancel flow, or stale-pending reconciliation if the worker dies before writing terminal state. | challenger-won |
| 3 | security | Is the privacy model sufficient when prompts, transcript, memory, node outputs, and worker logs are all persisted unredacted on disk? | Only for a narrowly scoped single-user local CLI release where operators understand that run directories are sensitive artifacts. It is not sufficient for shared or multi-tenant environments. | challenger-won |
| 4 | fail-closed behavior | Does the runtime stop safely when transport config, worker catalog, or budget metadata is invalid? | Yes. The code explicitly fails the run instead of guessing defaults, and tests cover those paths. | author-won |
| 5 | operability | Is the run directory an adequate first-release observability surface even without external telemetry? | Yes for a local CLI. `status.json`, `events.jsonl`, transcript, memory, node outputs, and worker logs provide a practical debugging trail. | author-won |
| 6 | scalability | Is the current sequential orchestration model enough for first release? | Yes if the release is explicitly scoped to bounded local orchestration rather than high-throughput or parallel swarm execution. | author-won |

### Challenge Summary

- **Challenges raised**: 6
- **Author victories**: 3
- **Challenger victories**: 3
- **Escalated**: 0
- **Overall verdict**: REVISE

### Revisions Made (if any)

- Added explicit release scoping: single-user local CLI, not multi-tenant service.
- Recorded `--run-dir` freshness as an operational requirement and current design gap.
- Recorded detached-worker supervision and unredacted persistence as release-sensitive concerns.

## Notes

- No `GLOSSARY.md` exists at repo root today. Terminology in this document follows the current code and test suite: `orchestrator`, `worker`, `run dir`, `memory`, `transcript`, `node artifact`, and `runtime profile`.
- The strongest release blockers from the current audit are:
  - enforce or document fresh unique `--run-dir` usage,
  - decide whether detached async execution is acceptable without stronger supervision,
  - decide whether unredacted on-disk persistence is acceptable for the intended release environment.
