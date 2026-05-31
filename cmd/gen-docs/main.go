// gen-docs generates a deterministic CLI reference Markdown file from source
// metadata. Run via go generate from internal/docs or directly:
//
//	go run ./cmd/gen-docs -o internal/docs/cli_reference.md
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/rules"
	"swarminator/internal/domain/tutorial"
)

func main() {
	out := flag.String("o", "", "output file path (stdout if empty)")
	flag.Parse()

	md := generate()

	if *out == "" {
		fmt.Print(md)
		return
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-docs: %v\n", err)
		os.Exit(1)
	}
}

func generate() string {
	var b strings.Builder

	b.WriteString("# Swarminator CLI Reference\n\n")
	b.WriteString("> Generated from source. Do not edit manually.\n\n")

	// --- Usage ---
	b.WriteString("## Usage\n\n```\n")
	b.WriteString("cat input.txt | swarminator swarm exec --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]\n")
	b.WriteString("cat input.txt | swarminator swarm start --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]\n")
	b.WriteString("swarminator runs final --run-dir PATH\n")
	b.WriteString("swarminator runs inspect --run-dir PATH\n")
	b.WriteString("swarminator runs tail --run-dir PATH\n")
	b.WriteString("swarminator runs wait --run-dir PATH\n")
	b.WriteString("cat input.txt | swarminator --agent NAME -m MODEL -p PERSONA -t SECONDS [OPTIONS]\n")
	b.WriteString("swarminator --list-agents\n")
	b.WriteString("swarminator --list-models [--agent NAME] [--json]\n")
	b.WriteString("swarminator --list-providers [--agent NAME] [--json]\n")
	b.WriteString("swarminator --tutorial TOPIC\n")
	b.WriteString("swarminator --tutorial QUESTION --agent NAME -m MODEL\n")
	b.WriteString("swarminator --tutorial \"suggest a cheap model for TASK\" --agent NAME\n")
	b.WriteString("swarminator --phases\n")
	b.WriteString("swarminator --protocol\n")
	b.WriteString("swarminator --help\n")
	b.WriteString("```\n\n")
	b.WriteString("## Starting Points\n\n")
	b.WriteString("- Private swarm run: `swarminator swarm exec --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123`\n")
	b.WriteString("- Async private run: `swarminator swarm start --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123`\n")
	b.WriteString("- Inspect a run: `swarminator runs inspect --run-dir /tmp/run-123`\n")
	b.WriteString("- Single node execution: `swarminator --tutorial quickstart`\n")
	b.WriteString("- Multi-node swarm guidance: `swarminator --tutorial swarm`\n\n")
	b.WriteString("## Private Swarm Protocol\n\n")
	b.WriteString("- The private orchestrator uses an OpenAI-compatible chat-completions transport configured from the XDG orchestrator profile.\n")
	b.WriteString("- Kilo gateway profiles typically use `backend: openai-compatible`, `message_api_format: openai.chat.completions`, `base_url_ref: KILO_BASE_URL`, `auth.credential_ref: KILO_API_KEY`, and optional `env_file` plus `timeout_seconds`.\n")
	b.WriteString("- The private orchestrator uses transport-native tools for external actions such as worker-node execution, and those tool schemas are generated from the discovered worker model/persona catalog.\n")
	b.WriteString("- Final answers are plain-text Markdown and are printed directly to stdout by `swarm exec`.\n")
	b.WriteString("- Worker results are returned to the orchestrator as readable Markdown tool results with artifact references.\n")
	b.WriteString("- Worker results are written to `nodes/` and also fed back into the next orchestrator turn as readable Markdown plus artifact references.\n\n")

	// --- Flags ---
	b.WriteString("## Flags\n\n")
	b.WriteString("| Flag | Required | Description |\n")
	b.WriteString("|------|----------|-------------|\n")
	b.WriteString("| `--swarm-root PATH` | Yes (swarm exec/start) | Swarm configuration root; expects `models/` and `personas/` directories for private runtime lookup. |\n")
	b.WriteString("| `--orchestrator NAME` | Yes (swarm exec/start) | Orchestrator model ID to load from `--swarm-root/models`. |\n")
	b.WriteString("| `--run-dir PATH` | Yes (swarm exec/start, runs *) | Private run directory; created with `0700` permissions and used for status, events, transcript, memory, nodes, and final output. |\n")
	b.WriteString("| `--event-sink file:///PATH` | No (swarm exec/start) | Optional events JSONL file sink. Must stay within `--run-dir`. |\n")
	b.WriteString("| `-m MODEL` | Yes (node) | Model identifier, e.g. `google/gemini-2.5-flash`, `github-copilot/gpt-5-mini` |\n")
	b.WriteString("| `-p PERSONA` | Yes (node) | Full system persona prompt text for the node; controls behavior and expected response style |\n")
	b.WriteString("| `-t SECONDS` | Yes (node) | Timeout in seconds (must be > 0); use larger values for reasoning-heavy nodes |\n")
	b.WriteString("| `--agent=NAME` | Yes (node) | Required for node execution. Select the agent binary (`kilo`, `gemini`, `claude`, `codex`, `command-code`). Fails if unknown or unavailable. |\n")
	b.WriteString("| `--agent-mode=MODE` | No | Set the underlying agent session mode (ACP agents only: gemini, claude). Gemini values: default, autoEdit, yolo, plan. For gemini, autoEdit selects the gemini CLI auto_edit mode. Not supported for kilo, codex, or command-code. |\n")
	b.WriteString("| `--feedback=stderr` | No | Emit advisory feedback to stderr |\n")
	b.WriteString("| `--dry-run` | No | Preflight: validate input, validate explicit agent, print envelope; no LLM call |\n")
	b.WriteString("| `--list-agents` | No | Print all known agents with status and model prefixes; does not require `-m`, `-p`, `-t`, or stdin |\n")
	b.WriteString("| `--list-models` | No | Print models grouped by engine/provider; optionally filter with `--agent`; supports `--json` |\n")
	b.WriteString("| `--list-providers` | No | Print providers grouped by engine/provider; optionally filter with `--agent`; supports `--json` |\n")
	b.WriteString("| `--json` | No | Emit JSON output for `--list-models` or `--list-providers` |\n")
	b.WriteString("| `--tutorial TOPIC` | No | Print embedded tutorial content or run explicit-agent tutorial Q&A (see Tutorial Mode) |\n")
	b.WriteString("| `--phases` | No | Print the intent-to-phase map and exit |\n")
	b.WriteString("| `--protocol` | No | Print the lightweight envelope protocol and exit |\n")
	b.WriteString("| `--help` | No | Print this help and exit |\n\n")

	// --- Tutorial mode ---
	b.WriteString("## Tutorial Mode\n\n")
	b.WriteString("Tutorial mode has two paths:\n\n")
	b.WriteString("- Built-in topics: `quickstart`, `rules`, `protocol`, `quorum`, `safety`, `swarm`, plus phase/protocol/rules heuristics. These print embedded guidance directly.\n")
	b.WriteString("- Freeform Q&A: requires explicit `--agent=NAME` and `-m MODEL`. There is no global default kilo fallback.\n")
	b.WriteString("- Agent-scoped model suggestion Q&A: requires `--agent=NAME`; this is the only tutorial Q&A case where `-m MODEL` is optional, and when omitted swarminator tries to infer a cheap default for that agent.\n")
	b.WriteString("- Context: embedded generated CLI reference plus agent-scoped discovery data when available.\n")
	b.WriteString("- Timeout: 120 seconds.\n\n")
	b.WriteString("The `swarm` topic is different: `--tutorial swarm` is the canonical built-in agent guide, and `--tutorial swarm-intelligence` is an alias. The swarm guide bypasses tutorial Q&A and prints embedded guidance plus agent-scoped hints when available.\n\n")
	b.WriteString("### Static Tutorial Topics\n\n")
	for _, entry := range tutorial.ReferenceTopics() {
		if entry.Key == "swarm" {
			continue
		}
		b.WriteString(fmt.Sprintf("#### `%s`\n\n", entry.Key))
		b.WriteString(strings.TrimSpace(entry.Text))
		b.WriteString("\n\n")
	}
	b.WriteString("## Embedded Agent Guide\n\n")
	b.WriteString("`--tutorial swarm` and `--tutorial swarm-intelligence` print the built-in agent guide.\n")
	b.WriteString("Model hints are agent-scoped: pass `--agent=NAME` to narrow the guide to one agent before asking for model suggestions.\n\n")
	b.WriteString(strings.TrimSpace(tutorial.Lookup("swarm")))
	b.WriteString("\n\n")
	b.WriteString("### Examples\n\n```\n")
	b.WriteString("printf 'hello' | swarminator swarm exec --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123\n")
	b.WriteString("printf 'hello' | swarminator swarm start --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123\n")
	b.WriteString("swarminator runs wait --run-dir /tmp/run-123\n")
	b.WriteString("swarminator runs inspect --run-dir /tmp/run-123\n")
	b.WriteString("swarminator --tutorial quickstart\n")
	b.WriteString("swarminator --tutorial rules\n")
	b.WriteString("swarminator --tutorial swarm\n")
	b.WriteString("swarminator --tutorial swarm-intelligence\n")
	b.WriteString("swarminator --tutorial \"how do I pass a timeout?\" --agent=gemini -m google/gemini-2.5-flash\n")
	b.WriteString("swarminator --tutorial \"suggest a cheap model for code review\" --agent=gemini\n")
	b.WriteString("printf 'hello' | swarminator --agent=kilo -m kilo/kilo-auto/free -p 'You are a reviewer.' -t 60\n")
	b.WriteString("```\n\n")

	// --- Agents and model routing ---
	b.WriteString("## Agents and Model Routing\n\n")
	b.WriteString("swarminator requires an explicit agent for node execution. Automatic model-prefix routing\n")
	b.WriteString("is not used to select the execution binary. Known agents:\n\n")
	b.WriteString("| Agent | Binary | Model Prefixes | Notes |\n")
	b.WriteString("|-------|--------|----------------|-------|\n")
	for _, a := range agent.KnownAgents() {
		prefixes := strings.Join(a.ModelPrefixes, ", ")
		notes := ""
		switch a.Name {
		case "gemini":
			notes = "headless one-shot execution (no ACP)"
		case "claude":
			notes = "ACP agent"
		case "codex":
			notes = "explicit-only (`--agent=codex`); no automatic prefix routing"
		case "command-code":
			notes = "explicit-only (`--agent=command-code`); one-shot CLI execution"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s |\n", a.Name, a.Binary, prefixes, notes))
	}
	b.WriteString("\n")
	b.WriteString("Execution model: Gemini uses headless one-shot CLI execution. Claude uses ACP.\n")
	b.WriteString("The `kilo` agent is a gateway that handles GPT/OpenAI-family prefixes.\n")
	b.WriteString("The `command-code` agent is explicit-only and does one-shot CLI execution.\n\n")
	b.WriteString("Use `--list-agents` to inspect availability and `--agent=NAME` to choose the execution binary.\n")
	b.WriteString("Explicit `--agent=NAME` fails with an error listing known agents if NAME is unknown or unavailable.\n\n")
	b.WriteString("### kilo model routing\n\n")
	b.WriteString("The `kilo` agent is a gateway that internally routes to many model providers beyond\n")
	b.WriteString("the prefixes listed above. Any model identifier accepted by the kilo CLI can be used.\n\n")
	b.WriteString("```\n")
	b.WriteString("printf 'hello' | swarminator --agent=kilo -m kilo/grok-3         -p \"...\" -t 60   # xAI Grok via kilo\n")
	b.WriteString("printf 'hello' | swarminator --agent=kilo -m kilo/kilo-auto/free -p \"...\" -t 60   # kilo default free model\n")
	b.WriteString("printf 'hello' | swarminator --agent=command-code -m some-model -p \"...\" -t 60\n")
	b.WriteString("```\n\n")
	b.WriteString("Run `kilo models` (if available) to list all model identifiers your kilo installation supports.\n\n")
	b.WriteString("**Tutorial Q&A mode** uses the explicit `--agent` selected by the caller.\n")
	b.WriteString("Freeform Q&A requires `-m MODEL`, except model-suggestion questions where swarminator may infer a cheap default for the chosen agent.\n")
	b.WriteString("There is no global default kilo fallback for tutorial Q&A.\n\n")
	b.WriteString("### Preflight and introspection\n\n")
	b.WriteString("```\n")
	b.WriteString("swarminator --list-agents\n")
	b.WriteString("swarminator --list-models --agent kilo --json\n")
	b.WriteString("swarminator --list-providers\n")
	b.WriteString("printf 'hello' | swarminator --agent=gemini -m google/gemini-2.5-flash -p 'You are a researcher.' -t 60 --dry-run\n")
	b.WriteString("printf 'hello' | swarminator --agent=kilo -m github-copilot/gpt-5-mini -p 'You are a spec writer.' -t 60 --dry-run\n")
	b.WriteString("```\n\n")

	// --- Rules and exit codes ---
	b.WriteString("## Rules and Exit Codes\n\n")
	b.WriteString("Hard rules are enforced before any LLM call:\n\n")
	b.WriteString("- Model, persona, and non-empty stdin are required.\n")
	b.WriteString("- Timeout must be > 0.\n")
	b.WriteString("- Inline credentials (`api_key`, `secret`, `password`, `token`) are rejected in persona and input.\n")
	b.WriteString("- Node personas containing `run shell` or `modify files` trigger an advisory (not a violation).\n\n")
	b.WriteString("| Exit Code | Constant | Meaning |\n")
	b.WriteString("|-----------|----------|---------|\n")
	b.WriteString(fmt.Sprintf("| `%d` | `ExitSuccess` | Success |\n", rules.ExitSuccess))
	b.WriteString(fmt.Sprintf("| `%d` | `ExitRetryable` | Retryable failure (network, timeout, rate limit) |\n", rules.ExitRetryable))
	b.WriteString(fmt.Sprintf("| `%d` | `ExitRuleViolation` | Rule or policy violation; do not retry unchanged |\n", rules.ExitRuleViolation))
	b.WriteString("\n")

	// --- Protocol envelope ---
	b.WriteString("## Protocol Envelope\n\n")
	b.WriteString(strings.TrimSpace(tutorial.Protocol()))
	b.WriteString("\n\n")

	// --- Phase map ---
	b.WriteString("## Phase Map\n\n")
	b.WriteString(strings.TrimSpace(tutorial.Phases()))
	b.WriteString("\n")

	return b.String()
}
