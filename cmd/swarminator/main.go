package main

import (
	"context"
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
	if args.Tutorial != "" {
		fmt.Println(answerTutorial(ctx, args.Tutorial, args.Model))
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
		fmt.Printf("%-10s  %-10s  %-12s  %-45s  %s\n", a.Name, a.Binary, status, prefixes, notes)
	}
	if detectErr != nil {
		fmt.Fprintf(os.Stderr, "\nswarminator: detection warning: %v\n", detectErr)
	}
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

	if args.Agent != "" {
		// Explicit agent override: validate reachability.
		a := registry.GetByName(args.Agent)
		if a == nil {
			all := registry.GetAll()
			names := make([]string, 0, len(all))
			for _, info := range all {
				names = append(names, info.Name)
			}
			fmt.Fprintf(os.Stderr, "swarminator: explicit --agent=%q is unknown or unavailable; known agents: %s\n",
				args.Agent, strings.Join(names, ", "))
			os.Exit(rules.ExitRuleViolation)
		}
		fmt.Printf("agent:    %s (explicit --agent=%s)\n", a.Name, args.Agent)
		fmt.Printf("route:    explicit override\n")
	} else {
		route, err := registry.ResolveRoute(args.Model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "swarminator: routing error: %v\n", err)
			os.Exit(rules.ExitRuleViolation)
		}
		fmt.Printf("agent:    %s\n", route.AgentName)
		fmt.Printf("route:    %s\n", route.RouteReason)
	}

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
  cat input.txt | swarminator -m MODEL -p PERSONA -t SECONDS [OPTIONS]
  swarminator --list-agents
  swarminator --tutorial TOPIC_OR_QUESTION [-m MODEL]
  swarminator --phases
  swarminator --protocol

Node run flags (all required for a node run):
  -m MODEL       Model identifier, e.g. gemini-2.5-flash, google/gemini-2.5-pro,
                 github-copilot/gpt-5-mini, openrouter/meta-llama-3, claude/sonnet
  -p PERSONA     System persona (controls node behavior and output format)
  -t SECONDS     Timeout in seconds (must be > 0)

Optional flags:
  --agent=NAME         Force a specific agent binary (kilo, gemini, claude, codex)
                       Use --agent=codex to explicitly route through the Codex harness.
                       If the agent is unknown or unavailable, swarminator exits with an error.
  --agent-mode=MODE    Set the underlying agent session mode (ACP agents only: gemini, claude).
                        Gemini values: default, autoEdit, yolo, plan.
                        Gemini headless maps autoEdit to auto_edit approval mode.
                        yolo auto-approves all Gemini tools; use only for non-interactive network/tool-capable runs.
                        Not supported for kilo or codex (returns an error).
  --feedback=stderr    Emit advisory feedback to stderr
  --dry-run            Preflight: validate input, resolve agent/route, print envelope; no LLM call
  --list-agents        Print all known agents with status and model prefixes; exits without running a node

Model routing (automatic, by prefix):
  google/, gemini/, gemini-        -> gemini
  kilo/, openai/, github-copilot/,
  openrouter/, gpt-, o1-, o3-,
  codex-                           -> kilo
  claude/, anthropic/, sonnet-     -> claude
  codex (explicit-only)            -> use --agent=codex
  unknown provider prefix          -> error with actionable message
  unqualified model name           -> first available authenticated agent

Tutorial mode:
  --tutorial asks kilo assistant (default model: kilo/kilo-auto/free); falls back to static topics.
  --phases and --protocol always print static output.`)
}

func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
