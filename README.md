# Swarminator

Swarminator is a focused swarm node runner. Its job: validate a node request, resolve the requested model to an agent, run that one node, and return the agent output. Persona design, output format, orchestration, and run IDs are the caller's responsibility via `-p` and stdin.

## Components

- `cmd/swarminator`: CLI entrypoint
- `internal/cli`: argument parsing
- `internal/rules`: deterministic policy checks
- `internal/tutorial`: embedded tutorial content
- `internal/protocol`: lightweight role/intent envelope
- `pkg/llm`: provider abstraction (UnifiedProvider, AgentRegistry, Gemini headless, Kilo, Codex, ADK)

## Architecture

- **Gemini**: runs in headless mode (`gemini --prompt ... --output-format json`) — no ACP session management, no timeouts from interactive sessions.
- **Claude**: uses ACP (Agent Communication Protocol) with session management and optional `--agent-mode` (yolo/plan/default/autoEdit).
- **Kilo**: uses its own ACP-compatible protocol for multi-model routing (GPT, OpenAI, GitHub Copilot, OpenRouter, etc.).
- **Codex**: explicit-only harness (`--agent=codex`), no automatic prefix routing.
- **ADK**: Google ADK fallback on rate-limit or when no CLI agent is available.

## Usage

```bash
# Node run (Gemini headless)
cat input.txt | swarminator -m google/gemini-2.5-flash -p "You are an adversarial reviewer." -t 60
cat input.txt | swarminator -m gemini-2.5-pro -p "You are a spec writer." -t 60

# Node run (Kilo - GPT, OpenAI, GitHub Copilot, OpenRouter)
cat input.txt | swarminator -m github-copilot/gpt-5-mini -p "You are a spec writer." -t 60
cat input.txt | swarminator -m openai/gpt-4.1 -p "You are a reviewer." -t 60

# Node run (Claude with agent-mode)
cat input.txt | swarminator -m claude/sonnet -p "You are a code reviewer." -t 90 --agent-mode=yolo --feedback=stderr

# Preflight checks (recommended before node runs)
swarminator --list-agents
printf 'hello' | swarminator -m google/gemini-2.5-flash -p "You are a researcher." -t 60 --dry-run
printf 'hello' | swarminator -m github-copilot/gpt-5-mini -p "You are a spec writer." -t 60 --dry-run

# Explicit Codex harness (no automatic prefix routing)
cat input.txt | swarminator --agent=codex -m codex-mini -p "You are a coder." -t 120

# Tutorial / reference
swarminator --tutorial quickstart
swarminator --phases
swarminator --protocol
```

## Model Routing

Automatic routing by model prefix (no configuration needed):

| Prefix(es) | Agent | Mode |
|------------|-------|------|
| `google/`, `gemini/`, `gemini-` | `gemini` | headless (`gemini --prompt ...`), no ACP |
| `kilo/`, `openai/`, `github-copilot/`, `openrouter/`, `gpt-`, `o1-`, `o3-`, `codex-` | `kilo` | Kilo ACP |
| `claude/`, `anthropic/`, `sonnet-` | `claude` | ACP, supports `--agent-mode` |
| `codex` (no prefix) | explicit-only | `--agent=codex` required |
| unknown provider prefix | **error** | actionable message with known prefixes |
| unqualified name | first available authenticated agent | fallback |

- Use `--agent=NAME` to force a specific agent. Fails with an error if the agent is unknown or unavailable (no silent fallback).
- `--agent-mode` is only supported for ACP agents (`gemini` headless maps `autoEdit` → `auto_edit`; `claude` supports all modes). Non-ACP agents (`kilo`, `codex`) reject `--agent-mode` with a clear error.
- Falls back to ADK on rate-limit (429) errors or when no CLI agent is available.
- Gemini headless mode avoids ACP session timeouts — each node run is a one-shot CLI invocation.

## Notes

- **Gemini**: headless mode — no ACP session management, no interactive timeouts. Each node run is a one-shot `gemini --prompt ...` invocation.
- **Claude**: uses ACP with full session management; supports `--agent-mode` (yolo/plan/default/autoEdit).
- **Kilo**: ACP-compatible protocol; routes GPT, OpenAI, GitHub Copilot, OpenRouter, and other models through a single binary.
- **Codex**: explicit-only (`--agent=codex`); no automatic prefix routing.
- **ADK**: Google ADK is used as a fallback on rate-limit errors or when no CLI agent is available/authenticated.
- Requires selected agent to be installed and authenticated.
- Caller-provided `-p` and stdin fully control node behavior and output format.
- Tutorial mode is embedded in the binary; no external skill file required.
