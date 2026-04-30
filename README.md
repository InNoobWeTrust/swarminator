# Swarminator

Swarminator is a Go-based swarm node runner with:

- deterministic safety rules enforced in code
- ADK-Go-backed Gemini execution for a first working provider
- self-documenting tutorial mode instead of an external skill file
- lightweight role/intention envelopes instead of rigid schemas

## Current Shape

This is a first-pass implementation focused on:

- `cmd/swarminator`: CLI entrypoint
- `internal/cli`: argument parsing
- `internal/rules`: deterministic policy checks
- `internal/tutorial`: embedded tutorial content
- `internal/protocol`: lightweight text envelope
- `internal/agents`: ADK-Go node and assistant wrappers
- `pkg/llm`: provider abstraction for future multi-provider support

## Usage

```bash
cat input.txt | swarminator -m gemini-2.5-flash -p "You are an adversarial reviewer" -t 60
cat input.txt | swarminator -m gemini-2.5-flash -p "You are an adversarial reviewer" -t 60 --feedback=stderr
cat input.txt | swarminator -m gemini-2.5-flash -p "You are an adversarial reviewer" -t 60 --dry-run
swarminator --tutorial quickstart
swarminator --phases
swarminator --protocol
```

## Notes

- Delegates execution and authentication to an external ACP-compliant CLI agent:
	- `gemini`
	- `codex`
	- `claude`
	- `kilo`
- Automatically detects an available agent based on the requested model when `--agent` is not provided.
- Use the `--agent` flag to force a specific CLI tool.
- Requires the specified agent to be installed and authenticated (e.g., `gemini auth status`).
- The provider abstraction is intentionally small so OpenAI, Anthropic, OpenRouter, or LiteLLM adapters can be added later.
- Tutorial mode is embedded in the binary; no external `SKILL.md` is required.
