package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"swarminator/internal/cli"
	"swarminator/internal/protocol"
	"swarminator/internal/rules"
	"swarminator/internal/tutorial"
	"swarminator/pkg/llm"
)

type stderrFeedback struct{}

func (stderrFeedback) Emit(message string) {
	fmt.Fprintf(os.Stderr, "[swarminator:feedback] %s\n", message)
}

func main() {
	ctx := context.Background()
	args, err := cli.Parse(os.Args[1:], os.Stdin, isTTY(os.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "swarminator: %v\n", err)
		os.Exit(1)
	}

	if args.ShowHelp {
		printHelp()
		return
	}
	if args.ShowPhases {
		fmt.Println(tutorial.Phases())
		return
	}
	if args.ShowProtocol {
		fmt.Println(tutorial.Protocol())
		return
	}
	if args.ListAgents {
		runListAgents()
		return
	}
	if args.ListModels || args.ListProviders {
		if err := runListDiscovery(ctx, args); err != nil {
			fmt.Fprintf(os.Stderr, "swarminator: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if args.Tutorial != "" {
		result, err := answerTutorial(ctx, tutorialRequest{
			Query:     args.Tutorial,
			Agent:     args.Agent,
			Model:     args.Model,
			AgentMode: args.AgentMode,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "swarminator: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(result)
		return
	}

	inputBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swarminator: failed to read stdin: %v\n", err)
		os.Exit(1)
	}
	input := strings.TrimSpace(string(inputBytes))

	var sink rules.FeedbackSink
	if args.Feedback == "stderr" {
		sink = stderrFeedback{}
	}
	engine := rules.NewEngine(sink)
	if err := engine.Validate(args.Model, args.Persona, input, args.TimeoutSec); err != nil {
		fmt.Fprintf(os.Stderr, "swarminator: %v\n", err)
		os.Exit(rules.ExitCode(err))
	}

	env := protocol.NewEnvelope("node", inferIntent(args.Persona), "orchestrator", input)

	if args.DryRun {
		runDryRun(args, env)
		return
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Duration(args.TimeoutSec)*time.Second)
	defer cancel()

	provider := llm.NewUnifiedProvider("", args.Agent) // Empty projectID - ADK will use default
	output, err := provider.Complete(ctx, llm.CompletionRequest{
		Model:     args.Model,
		Persona:   args.Persona,
		Input:     env.String(),
		AgentMode: args.AgentMode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "swarminator: execution failed: %v\n", err)
		var violation rules.Violation
		if errors.As(err, &violation) {
			os.Exit(violation.Code)
		}
		os.Exit(rules.ExitRetryable)
	}

	fmt.Println(strings.TrimSpace(output))
}

// runListAgents detects available agents and prints a plain table.
func runListAgents() {
	registry := llm.NewAgentRegistry()
	detectErr := registry.Detect()

	all := registry.GetAll()
	if len(all) == 0 {
		all = llm.KnownAgents()
	}

	fmt.Printf("%-10s  %-10s  %-12s  %-45s  %s\n", "AGENT", "BINARY", "STATUS", "MODEL PREFIXES", "NOTES")
	fmt.Println(strings.Repeat("-", 100))
	for _, a := range all {
		status := "unavailable"
		if a.Available {
			if a.Authenticated {
				status = "authenticated"
			} else {
				status = "available"
			}
		}
		prefixes := strings.Join(a.ModelPrefixes, ", ")
		notes := ""
		if a.Name == "codex" {
			notes = "explicit-only (--agent=codex)"
		}
		if a.Name == "command-code" {
			notes = "explicit-only (--agent=command-code)"
		}
		fmt.Printf("%-10s  %-10s  %-12s  %-45s  %s\n", a.Name, a.Binary, status, prefixes, notes)
	}
	if detectErr != nil {
		fmt.Fprintf(os.Stderr, "\nswarminator: detection warning: %v\n", detectErr)
	}
}

func runListDiscovery(ctx context.Context, args cli.Args) error {
	groups := llm.DiscoverListings(ctx, args.Agent)
	if args.JSONOutput {
		payload := struct {
			Groups []llm.EngineListing `json:"groups"`
		}{Groups: groups}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Println(llm.FormatDiscoveryText(groups, args.ListModels, args.ListProviders))
	return nil
}

// runDryRun validates routing and prints a preflight report without calling an LLM.
func runDryRun(args cli.Args, env protocol.Envelope) {
	fmt.Println("=== swarminator dry-run preflight ===")
	fmt.Printf("model:    %s\n", args.Model)
	fmt.Printf("timeout:  %ds\n", args.TimeoutSec)
	if args.AgentMode != "" {
		fmt.Printf("agent-mode: %s\n", args.AgentMode)
	}
	if args.Feedback != "" {
		fmt.Printf("feedback: %s\n", args.Feedback)
	} else {
		fmt.Printf("feedback: (none)\n")
	}

	registry := llm.NewAgentRegistry()
	if err := registry.Detect(); err != nil {
		fmt.Fprintf(os.Stderr, "swarminator: agent detection warning: %v\n", err)
	}

	a := registry.GetByName(args.Agent)
	if a == nil {
		all := registry.GetAll()
		names := make([]string, 0, len(all))
		for _, info := range all {
			names = append(names, info.Name)
		}
		fmt.Fprintf(os.Stderr, "swarminator: explicit --agent=%q is unknown or unavailable; known agents: %s\n",
			args.Agent, strings.Join(names, ", "))
		os.Exit(rules.ExitRetryable)
	}
	fmt.Printf("agent:    %s (explicit --agent=%s)\n", a.Name, args.Agent)
	fmt.Printf("route:    explicit override\n")

	fmt.Println("\n=== LLM-visible envelope ===")
	fmt.Println(env.String())
}

func inferIntent(persona string) string {
	text := strings.ToLower(persona)
	switch {
	case strings.Contains(text, "challenge") || strings.Contains(text, "critic") || strings.Contains(text, "adversarial"):
		return "challenge"
	case strings.Contains(text, "review") || strings.Contains(text, "qa") || strings.Contains(text, "approve"):
		return "review"
	case strings.Contains(text, "decompose") || strings.Contains(text, "task"):
		return "decompose"
	case strings.Contains(text, "write") || strings.Contains(text, "make"):
		return "make"
	default:
		return "extract"
	}
}

func printHelp() {
	fmt.Println(`swarminator - focused swarm node runner

Usage:
  cat input.txt | swarminator --agent NAME -m MODEL -p PERSONA -t SECONDS [OPTIONS]
  swarminator --list-agents
  swarminator --list-models [--agent NAME] [--json]
  swarminator --list-providers [--agent NAME] [--json]
  swarminator --tutorial TOPIC
  swarminator --tutorial QUESTION --agent NAME -m MODEL
  swarminator --tutorial "suggest a cheap model for TASK" --agent NAME
  swarminator --phases
  swarminator --protocol

Starting points:
  Single node execution:      swarminator --tutorial quickstart
  Multi-node swarm guidance:  swarminator --tutorial swarm

Required node arguments:
  --agent=NAME  Required node selector: choose the agent binary (kilo, gemini, claude, codex, command-code)
                If the agent is unknown or unavailable, swarminator exits with an error.
  -m MODEL       Model identifier, e.g. gemini-2.5-flash, google/gemini-2.5-pro,
                 github-copilot/gpt-5-mini, openrouter/meta-llama-3, claude/sonnet
  -p PERSONA     Full system persona prompt text (controls node behavior and expected response style)
  -t SECONDS     Timeout in seconds (must be > 0)

Optional flags:
  --agent-mode=MODE       Set the underlying agent session mode (ACP agents only: gemini, claude).
                          Gemini values: default, autoEdit, yolo, plan.
                          For gemini, autoEdit selects the gemini CLI auto_edit mode.
                          Not supported for kilo, codex, or command-code.
  --feedback=stderr       Emit advisory feedback to stderr
  --dry-run               Preflight: validate input, validate explicit agent, print envelope; no LLM call
  --list-agents           Print all known agents with status and model prefixes
  --list-models           Print models grouped by engine/provider
  --list-providers        Print providers grouped by engine/provider
  --json                  Emit JSON for --list-models or --list-providers

Model routing (automatic, by prefix) is no longer supported.
Explicit engine choice is required: use --agent=NAME.

Agent options:
  kilo          Gateway for provider-qualified models; use provider/model IDs or kilo/* IDs
  gemini        Google Gemini CLI in native headless mode
  claude        Anthropic models via ACP
  codex         Explicit-only (--agent=codex); no automatic prefix routing
  command-code  Explicit-only (--agent=command-code); one-shot CLI execution

Examples:
  printf 'hello' | swarminator --agent=gemini -m google/gemini-2.5-flash -p "You are a concise reviewer." -t 60
  printf 'hello' | swarminator --agent=kilo -m kilo/kilo-auto/free -p "You are a concise reviewer." -t 60
  printf 'hello' | swarminator --agent=kilo -m openai/gpt-4.1 -p "You are a concise reviewer." -t 60
  swarminator --list-models --agent kilo --json
  swarminator --tutorial swarm
  swarminator --tutorial swarm-intelligence
  swarminator --tutorial "how do I pass a timeout?" --agent=gemini -m google/gemini-2.5-flash
  swarminator --tutorial "suggest a cheap model for code review" --agent=gemini

Tutorial mode:
  Built-in topics like quickstart, rules, protocol, quorum, safety, and swarm print embedded guidance directly.
  Freeform tutorial Q&A requires explicit --agent and -m MODEL; there is no global default kilo fallback.
  Agent-scoped model-suggestion questions are the only tutorial Q&A case that may omit -m; swarminator then tries to infer a cheap default for that agent.
  --tutorial swarm is the canonical built-in agent guide topic; --tutorial swarm-intelligence is an alias.
  The swarm guide bypasses tutorial Q&A and prints embedded guidance plus agent-scoped hints when available.
  Model suggestions are agent-scoped and prefer free/cheap options first.
  --phases and --protocol always print static output.`)
}

func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
