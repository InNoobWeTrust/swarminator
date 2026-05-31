# Swarminator

Swarminator has two execution layers:

- a legacy one-shot node runner for explicit worker-node calls
- a private swarm runtime that keeps orchestrator transcript, events, memory, and node artifacts inside a run directory

The node-runner path still validates a node request, resolves the requested model to an agent, runs that one node, and returns the agent output. Persona design, output format, orchestration, and run IDs remain the caller's responsibility via `-p` and stdin.

## Components

- `cmd/swarminator`: CLI entrypoint
- `internal/cli`: argument parsing
- `internal/domain`: pure Go ports (agent metadata, protocol envelopes, rules, tutorial content)
- `internal/infra`: adapter implementations (LLM providers, registry, discovery, rules engine, run store, swarm catalog, models.dev lookup)
- `internal/app`: orchestration services (agent discovery, prompt building, tutorial Q&A, node execution, private swarm runtime, run inspection)

## Architecture

- **Private swarm runtime**: dedicated message-list orchestrator transport, file-backed run directory, read-only inspection commands, async self-exec worker mode, and fail-closed budget enforcement using `models.dev` or local overrides.
- **Gemini**: runs in headless mode (`gemini --prompt ... --output-format json`) — no ACP session management, no timeouts from interactive sessions.
- **Claude**: uses ACP (Agent Communication Protocol) with session management and optional `--agent-mode` (yolo/plan/default/autoEdit).
- **Kilo**: uses its own ACP-compatible protocol for multi-model routing (GPT, OpenAI, GitHub Copilot, OpenRouter, etc.).
- **Codex**: explicit-only harness (`--agent=codex`), no automatic prefix routing.
- **ADK**: Google ADK fallback on rate-limit or when no CLI agent is available.

## Usage

```bash
# Private swarm run (blocking final-only stdout)
cat input.txt | swarminator swarm exec --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123

# Private swarm run (async receipt + later inspection)
cat input.txt | swarminator swarm start --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-124
swarminator runs inspect --run-dir /tmp/run-124
swarminator runs wait --run-dir /tmp/run-124
swarminator runs final --run-dir /tmp/run-124

# Node run (Gemini headless) - explicit agent required
cat input.txt | swarminator --agent=gemini -m google/gemini-2.5-flash -p "You are an adversarial reviewer." -t 60
cat input.txt | swarminator --agent=gemini -m gemini-2.5-pro -p "You are a spec writer." -t 60

# Node run (Kilo - GPT, OpenAI, GitHub Copilot, OpenRouter)
cat input.txt | swarminator --agent=kilo -m kilo/kilo-auto/free -p "You are a reviewer." -t 60
cat input.txt | swarminator --agent=kilo -m github-copilot/gpt-5-mini -p "You are a spec writer." -t 60
cat input.txt | swarminator --agent=kilo -m openai/gpt-4.1 -p "You are a reviewer." -t 60

# Node run (Claude with agent-mode)
cat input.txt | swarminator --agent=claude -m claude/sonnet -p "You are a code reviewer." -t 90 --agent-mode=yolo --feedback=stderr

# Preflight checks (recommended before node runs)
swarminator --list-agents
printf 'hello' | swarminator --agent=gemini -m google/gemini-2.5-flash -p "You are a researcher." -t 60 --dry-run
printf 'hello' | swarminator --agent=kilo -m github-copilot/gpt-5-mini -p "You are a spec writer." -t 60 --dry-run

# Explicit Codex harness (no automatic prefix routing)
cat input.txt | swarminator --agent=codex -m codex-mini -p "You are a coder." -t 120

# Tutorial / reference
swarminator --tutorial quickstart
swarminator --tutorial swarm
swarminator --tutorial swarm-intelligence
swarminator --tutorial "how do I pass a timeout?" --agent=gemini -m google/gemini-2.5-flash
swarminator --tutorial "suggest a cheap model for code review" --agent=gemini
swarminator --phases
swarminator --protocol
```

## Model Routing

Automatic routing by model prefix is no longer supported. Explicit engine choice is required via `--agent=NAME`.

| Agent      | Model Format / Notes | Mode |
|------------|----------------------|------|
| `gemini`   | e.g. `gemini-2.5-flash`, `gemini-2.5-pro` (native CLI identifiers) | headless (`gemini --prompt ...`), no ACP |
| `kilo`     | provider-qualified IDs: `kilo/...`, `openai/...`, `github-copilot/...`, `openrouter/...`, `gpt-...`, `o1-...`, `codex-...` | Kilo ACP |
| `claude`   | e.g. `claude/sonnet` (via ACP) | ACP, supports `--agent-mode` |
| `codex`    | explicit-only (`--agent=codex` required) | Codex harness |

- Use `--agent=NAME` to select a specific agent. Fails with an error if the agent is unknown or unavailable (no silent fallback).
- `--agent-mode` is only supported for ACP agents (`gemini`, `claude`). Non-ACP agents (`kilo`, `codex`) reject `--agent-mode` with a clear error.
- Falls back to ADK on rate-limit (429) errors or when no CLI agent is available/authenticated.
- Gemini headless mode avoids ACP session timeouts — each node run is a one-shot CLI invocation.

## Notes

- `swarm exec` prints only the final answer to stdout; internal transcript, events, and artifacts stay in `--run-dir`.
- `swarm start` prints only a small JSON receipt; use `runs wait`, `runs inspect`, `runs tail`, and `runs final` afterward.
- The private runtime reads orchestrator transport config from the XDG-scoped orchestrator config, separate from worker-node agent configuration.
- The orchestrator transport currently targets an OpenAI-compatible chat-completions API. Typical Kilo gateway profiles use `backend: openai-compatible`, `message_api_format: openai.chat.completions`, `base_url_ref: KILO_BASE_URL`, `auth.credential_ref: KILO_API_KEY`, and optional `env_file` plus `timeout_seconds`.
- Swarm-root discovery is convention-based: `models/` for orchestrator and worker model definitions, `personas/` for Markdown frontmatter persona prompts.
- The private orchestrator uses transport-native tools for external actions such as worker-node execution; worker tool schemas are generated from the discovered worker model/persona config and final answers remain plain-text Markdown.
- Orchestrator context budgets fail closed when `models.dev` metadata cannot be resolved and no local model override is configured.
- Full worker-node outputs are stored under `nodes/` and are also fed back into the active orchestrator turn as readable Markdown with artifact references. Older turns are compacted into bounded summaries when needed.
- **Gemini**: headless mode — no ACP session management, no interactive timeouts. Each node run is a one-shot `gemini --prompt ...` invocation.
- **Claude**: uses ACP with full session management; supports `--agent-mode` (yolo/plan/default/autoEdit).
- **Kilo**: ACP-compatible protocol; routes GPT, OpenAI, GitHub Copilot, OpenRouter, and other models through a single binary.
- **Codex**: explicit-only (`--agent=codex`); no automatic prefix routing.
- **ADK**: Google ADK is used as a fallback on rate-limit errors or when no CLI agent is available/authenticated.
- Requires selected agent to be installed and authenticated.
- Caller-provided `-p` and stdin fully control node behavior and output format.
- Tutorial mode is embedded in the binary; no external skill file is required.
- Use `swarminator --tutorial swarm` or `swarminator --tutorial swarm-intelligence` to print the built-in agent guide.
- Freeform tutorial Q&A requires explicit `--agent=NAME` and `-m MODEL`; there is no global default kilo fallback.
- Agent-scoped model-suggestion questions may omit `-m`, letting swarminator infer a cheap default model for the chosen agent.
- The built-in guide mirrors the swarm operational workflow: one node per call, preflight, phase structure, persona groups, and agent-scoped model hints.
