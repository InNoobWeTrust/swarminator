# Swarminator

Swarminator is a focused swarm node runner. Its job: validate a node request, resolve the requested model to an agent, run that one node, and return the agent output. Persona design, output format, orchestration, and run IDs are the caller's responsibility via `-p` and stdin.

## Components

- `cmd/swarminator`: CLI entrypoint
- `internal/cli`: argument parsing
- `internal/rules`: deterministic policy checks
- `internal/tutorial`: embedded tutorial content
- `internal/protocol`: lightweight role/intent envelope
- `pkg/llm`: provider abstraction (UnifiedProvider, AgentRegistry, ACP, Kilo, Codex, ADK)

## Usage

```bash
# Node run
cat input.txt | swarminator -m google/gemini-2.5-flash -p "You are an adversarial reviewer." -t 60
cat input.txt | swarminator -m github-copilot/gpt-5-mini -p "You are a spec writer. Return acceptance criteria." -t 60
cat input.txt | swarminator -m claude/sonnet -p "You are a code reviewer." -t 90 --feedback=stderr

# Preflight checks (recommended before node runs)
swarminator --list-agents
printf 'hello' | swarminator -m google/gemini-2.5-flash -p "You are a researcher." -t 60 --dry-run
printf 'hello' | swarminator -m github-copilot/gpt-5-mini -p "You are a spec writer." -t 60 --dry-run

# Explicit Codex harness
cat input.txt | swarminator --agent=codex -m codex-mini -p "You are a coder." -t 120

# Tutorial / reference
swarminator --tutorial quickstart
swarminator --phases
swarminator --protocol
```

## Model Routing

Automatic routing by model prefix (no configuration needed):

| Prefix(es) | Agent |
|------------|-------|
| `google/`, `gemini/`, `gemini-` | `gemini` |
| `kilo/`, `openai/`, `github-copilot/`, `openrouter/`, `gpt-`, `o1-`, `o3-`, `codex-` | `kilo` |
| `claude/`, `anthropic/`, `sonnet-` | `claude` |
| `codex` (no prefix) | explicit-only: `--agent=codex` |
| unknown provider prefix | **error** with actionable message |
| unqualified name | first available authenticated agent |

- Use `--agent=NAME` to force a specific agent. Fails with an error if the agent is unknown or unavailable (no silent fallback).
- Falls back to ADK on rate-limit errors.

## Notes

- Requires the selected agent to be installed and authenticated.
- Caller-provided `-p` and stdin fully control node behavior and output format.
- Tutorial mode is embedded in the binary; no external skill file required.
