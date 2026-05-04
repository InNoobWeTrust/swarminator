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

	"swarminator/internal/rules"
	"swarminator/internal/tutorial"
	"swarminator/pkg/llm"
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
	b.WriteString("cat input.txt | swarminator -m MODEL -p PERSONA -t SECONDS [OPTIONS]\n")
	b.WriteString("swarminator --list-agents\n")
	b.WriteString("swarminator --tutorial TOPIC_OR_QUESTION [-m MODEL]\n")
	b.WriteString("swarminator --phases\n")
	b.WriteString("swarminator --protocol\n")
	b.WriteString("swarminator --help\n")
	b.WriteString("```\n\n")

	// --- Flags ---
	b.WriteString("## Flags\n\n")
	b.WriteString("| Flag | Required | Description |\n")
	b.WriteString("|------|----------|-------------|\n")
	b.WriteString("| `-m MODEL` | Yes (node) | Model identifier, e.g. `google/gemini-2.5-flash`, `github-copilot/gpt-5-mini` |\n")
	b.WriteString("| `-p PERSONA` | Yes (node) | System persona for the node; controls behavior and output format |\n")
	b.WriteString("| `-t SECONDS` | Yes (node) | Timeout in seconds (must be > 0); use larger values for reasoning-heavy nodes |\n")
	b.WriteString("| `--agent=NAME` | No | Force a specific agent binary (`kilo`, `gemini`, `claude`, `codex`). Fails if unknown or unavailable. |\n")
	b.WriteString("| `--feedback=stderr` | No | Emit advisory feedback to stderr |\n")
	b.WriteString("| `--dry-run` | No | Preflight: validate input, resolve agent/model route, print envelope; no LLM call |\n")
	b.WriteString("| `--list-agents` | No | Print all known agents with status and model prefixes; does not require `-m`, `-p`, `-t`, or stdin |\n")
	b.WriteString("| `--tutorial TOPIC` | No | Print tutorial text or ask kilo assistant (see Tutorial Mode); optionally override model with `-m` |\n")
	b.WriteString("| `--phases` | No | Print the intent-to-phase map and exit |\n")
	b.WriteString("| `--protocol` | No | Print the lightweight envelope protocol and exit |\n")
	b.WriteString("| `--help` | No | Print this help and exit |\n\n")

	// --- Tutorial mode ---
	b.WriteString("## Tutorial Mode\n\n")
	b.WriteString("When `--tutorial` is given, swarminator first attempts an LLM-assisted answer:\n\n")
	b.WriteString("- Agent: `kilo`\n")
	b.WriteString("- Model: `kilo/kilo-auto/free` (default; override with `-m MODEL`)\n")
	b.WriteString("- Context: embedded generated CLI reference (this document)\n")
	b.WriteString("- Timeout: 120 seconds\n\n")
	b.WriteString("If `kilo` is not installed, not detected, times out, or returns an error,\n")
	b.WriteString("tutorial mode falls back to the static topic lookup.\n\n")
	b.WriteString("### Static Tutorial Topics\n\n")
	for _, entry := range tutorial.ReferenceTopics() {
		b.WriteString(fmt.Sprintf("#### `%s`\n\n", entry.Key))
		b.WriteString(strings.TrimSpace(entry.Text))
		b.WriteString("\n\n")
	}
	b.WriteString("### Examples\n\n```\n")
	b.WriteString("swarminator --tutorial quickstart\n")
	b.WriteString("swarminator --tutorial rules\n")
	b.WriteString("swarminator --tutorial \"how do I pass a timeout?\"\n")
	b.WriteString("swarminator --tutorial quickstart -m kilo/kilo-auto/free\n")
	b.WriteString("```\n\n")

	// --- Agents and model routing ---
	b.WriteString("## Agents and Model Routing\n\n")
	b.WriteString("swarminator routes each node run to one agent based on the model prefix.\n")
	b.WriteString("Routing is intentional: unrecognised provider-style prefixes fail with an actionable error\n")
	b.WriteString("instead of silently falling back. Known agents:\n\n")
	b.WriteString("| Agent | Binary | Model Prefixes | Notes |\n")
	b.WriteString("|-------|--------|----------------|-------|\n")
	for _, a := range llm.KnownAgents() {
		prefixes := strings.Join(a.ModelPrefixes, ", ")
		notes := ""
		if a.Name == "codex" {
			notes = "explicit-only (`--agent=codex`); no automatic prefix routing"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s |\n", a.Name, a.Binary, prefixes, notes))
	}
	b.WriteString("\n")
	b.WriteString("For unqualified model names (no provider prefix), swarminator selects the first available\n")
	b.WriteString("authenticated agent. Unknown provider-style prefixes (e.g. `badprovider/model`) return\n")
	b.WriteString("an actionable routing error listing known routes.\n\n")
	b.WriteString("GPT/OpenAI-family models (`openai/`, `github-copilot/`, `openrouter/`, `gpt-`, `o1-`, `o3-`, `codex-`) default to the `kilo` agent.\n")
	b.WriteString("To explicitly use the Codex CLI harness instead, pass `--agent=codex`.\n\n")
	b.WriteString("Explicit `--agent=NAME` fails with an error listing known agents if NAME is unknown or unavailable.\n\n")
	b.WriteString("### kilo model routing\n\n")
	b.WriteString("The `kilo` agent is a gateway that internally routes to many model providers beyond\n")
	b.WriteString("the prefixes listed above. Any model identifier accepted by the kilo CLI can be used.\n\n")
	b.WriteString("```\n")
	b.WriteString("swarminator -m kilo/grok-3         -p \"...\" -t 60   # xAI Grok via kilo\n")
	b.WriteString("swarminator -m kilo/kilo-auto/free -p \"...\" -t 60   # kilo default free model\n")
	b.WriteString("```\n\n")
	b.WriteString("Run `kilo models` (if available) to list all model identifiers your kilo installation supports.\n\n")
	b.WriteString("**Tutorial mode** bypasses UnifiedProvider and calls the `kilo` agent directly\n")
	b.WriteString("with model `kilo/kilo-auto/free` by default; override with `-m MODEL`.\n\n")
	b.WriteString("### Preflight and introspection\n\n")
	b.WriteString("```\n")
	b.WriteString("swarminator --list-agents\n")
	b.WriteString("printf 'hello' | swarminator -m google/gemini-2.5-flash -p 'You are a researcher.' -t 60 --dry-run\n")
	b.WriteString("printf 'hello' | swarminator -m github-copilot/gpt-5-mini -p 'You are a spec writer.' -t 60 --dry-run\n")
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
