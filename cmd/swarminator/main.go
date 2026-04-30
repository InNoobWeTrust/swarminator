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
	if args.Tutorial != "" {
		fmt.Println(tutorial.Lookup(args.Tutorial))
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
		fmt.Println(env.String())
		return
	}

	if args.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(args.TimeoutSec)*time.Second)
		defer cancel()
	}

	provider := llm.NewUnifiedProvider("", args.Agent) // Empty projectID - ADK will use default
	output, err := provider.Complete(ctx, llm.CompletionRequest{
		Model:   args.Model,
		Persona: args.Persona,
		Input:   env.String(),
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
	fmt.Println(`swarminator - intelligent swarm node runner

Usage:
  cat input.txt | swarminator -m MODEL -p PERSONA [--agent=AGENT] [--feedback=stderr] [-t SECONDS]
  swarminator --tutorial TOPIC
  swarminator --phases
  swarminator --protocol

Uses UnifiedProvider for automatic agent detection and fallback:
  - Automatically detects available agents (kilo, codex, claude, gemini)
  - Supports dynamic model selection based on prefix
  - Falls back to ADK on rate limits
  - Model selection is automatic

Make sure your agents are properly configured and authenticated.`)
}

func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
