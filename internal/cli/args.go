package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/tutorial"
)

type Args struct {
	Command       Command
	Model         string
	Persona       string
	Agent         string
	AgentMode     string
	Feedback      string
	Tutorial      string
	SwarmRoot     string
	Orchestrator  string
	RunDir        string
	EventSink     string
	TimeoutSec    int
	DryRun        bool
	ListAgents    bool
	ListModels    bool
	ListProviders bool
	JSONOutput    bool
	ShowHelp      bool
	ShowPhases    bool
	ShowProtocol  bool
}

type Command string

const (
	CommandSwarmExec   Command = "swarm-exec"
	CommandSwarmStart  Command = "swarm-start"
	CommandSwarmWorker Command = "swarm-worker"
	CommandRunsFinal   Command = "runs-final"
	CommandRunsInspect Command = "runs-inspect"
	CommandRunsTail    Command = "runs-tail"
	CommandRunsWait    Command = "runs-wait"
)

func Parse(argv []string, stdin io.Reader, stdinIsTTY bool) (Args, error) {
	if len(argv) > 0 {
		switch argv[0] {
		case "swarm":
			return parseSwarmArgs(argv[1:])
		case "runs":
			return parseRunsArgs(argv[1:])
		}
	}

	args := Args{}
	return parseLegacyArgs(args, argv, stdin, stdinIsTTY)
}

func parseLegacyArgs(args Args, argv []string, stdin io.Reader, stdinIsTTY bool) (Args, error) {
	tutorialRequested := false
	timeoutProvided := false
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "-m" || strings.HasPrefix(arg, "-m="):
			value, err := consumeRequiredValue(arg, "-m", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.Model = value
		case arg == "-p" || strings.HasPrefix(arg, "-p="):
			value, err := consumeRequiredValue(arg, "-p", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.Persona = value
		case arg == "--agent" || strings.HasPrefix(arg, "--agent="):
			value, err := consumeRequiredValue(arg, "--agent", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.Agent = value
		case arg == "--agent-mode" || strings.HasPrefix(arg, "--agent-mode="):
			value, err := consumeRequiredValue(arg, "--agent-mode", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.AgentMode = value
		case arg == "-t" || strings.HasPrefix(arg, "-t="):
			timeoutProvided = true
			value, err := consumeRequiredValue(arg, "-t", argv, &i)
			if err != nil {
				return Args{}, err
			}
			var parsed int
			if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
				return Args{}, fmt.Errorf("invalid timeout %q", value)
			}
			args.TimeoutSec = parsed
		case arg == "--feedback" || strings.HasPrefix(arg, "--feedback="):
			value, err := consumeRequiredValue(arg, "--feedback", argv, &i)
			if err != nil {
				return Args{}, err
			}
			if value != "stderr" {
				return Args{}, fmt.Errorf("unsupported feedback sink: %s", value)
			}
			args.Feedback = value
		case arg == "--tutorial" || strings.HasPrefix(arg, "--tutorial="):
			value, consumed, err := consumeOptionalValue(arg, "--tutorial", argv, &i)
			if err != nil {
				return Args{}, err
			}
			if consumed {
				args.Tutorial = value
			} else {
				tutorialRequested = true
			}
		case arg == "--list-agents":
			args.ListAgents = true
		case arg == "--list-models":
			args.ListModels = true
		case arg == "--list-providers":
			args.ListProviders = true
		case arg == "--json":
			args.JSONOutput = true
		case arg == "--protocol":
			args.ShowProtocol = true
		case arg == "--phases":
			args.ShowPhases = true
		case arg == "--dry-run":
			args.DryRun = true
		case arg == "-h" || arg == "--help":
			args.ShowHelp = true
		default:
			return Args{}, fmt.Errorf("unknown option: %s", arg)
		}
	}

	if args.ShowHelp || args.ShowPhases || args.ShowProtocol || args.ListAgents || args.ListModels || args.ListProviders {
		return args, nil
	}

	if args.JSONOutput && !(args.ListModels || args.ListProviders) {
		return Args{}, errors.New("--json is only supported with --list-models or --list-providers")
	}

	if args.Tutorial == "" && tutorialRequested {
		if stdinIsTTY {
			return Args{}, errors.New("--tutorial requires a topic or non-empty piped stdin")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return Args{}, err
		}
		args.Tutorial = strings.TrimSpace(string(data))
		if args.Tutorial == "" {
			return Args{}, errors.New("--tutorial requires a topic or non-empty piped stdin")
		}
	}

	if args.Tutorial != "" {
		if args.Persona != "" {
			return Args{}, fmt.Errorf("--tutorial cannot be combined with -p")
		}
		if tutorial.IsBuiltInQuery(args.Tutorial) {
			return args, nil
		}
		if args.Agent == "" {
			return Args{}, errors.New("tutorial Q&A requires --agent=NAME; built-in topics like quickstart, rules, protocol, quorum, safety, and swarm do not")
		}
		if args.Model == "" && !tutorial.IsModelSuggestionQuery(args.Tutorial) && agent.AgentRequiresModelFlag(args.Agent) {
			return Args{}, errors.New("tutorial Q&A requires -m MODEL for this agent; model-suggestion questions may omit -m and let swarminator infer a cheap default")
		}
		return args, nil
	}

	if args.Agent == "" {
		return Args{}, errors.New("--agent=NAME is required for node execution (kilo, gemini, claude, codex, command-code)")
	}
	if args.Model == "" && agent.AgentRequiresModelFlag(args.Agent) {
		return Args{}, errors.New("-m MODEL is required (omit only for agents that manage the model internally, e.g. claude, command-code)")
	}
	if args.Persona == "" {
		return Args{}, errors.New("-p PERSONA is required")
	}
	if !timeoutProvided {
		return Args{}, errors.New("-t SECONDS is required")
	}
	if args.TimeoutSec <= 0 {
		return Args{}, fmt.Errorf("-t must be > 0")
	}

	return args, nil
}

func parseSwarmArgs(argv []string) (Args, error) {
	if len(argv) == 0 {
		return Args{}, errors.New("missing swarm subcommand")
	}
	var command Command
	switch argv[0] {
	case "exec":
		command = CommandSwarmExec
	case "start":
		command = CommandSwarmStart
	case "worker":
		command = CommandSwarmWorker
	default:
		return Args{}, fmt.Errorf("unknown swarm subcommand: %s", argv[0])
	}

	args := Args{Command: command}
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--swarm-root" || strings.HasPrefix(arg, "--swarm-root="):
			value, err := consumeRequiredValue(arg, "--swarm-root", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.SwarmRoot = value
		case arg == "--orchestrator" || strings.HasPrefix(arg, "--orchestrator="):
			value, err := consumeRequiredValue(arg, "--orchestrator", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.Orchestrator = value
		case arg == "--run-dir" || strings.HasPrefix(arg, "--run-dir="):
			value, err := consumeRequiredValue(arg, "--run-dir", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.RunDir = value
		case arg == "--event-sink" || strings.HasPrefix(arg, "--event-sink="):
			value, err := consumeRequiredValue(arg, "--event-sink", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.EventSink = value
		case arg == "-h" || arg == "--help":
			args.ShowHelp = true
		default:
			return Args{}, fmt.Errorf("unknown option: %s", arg)
		}
	}

	if args.ShowHelp {
		return args, nil
	}
	if strings.TrimSpace(args.SwarmRoot) == "" {
		return Args{}, errors.New("--swarm-root is required")
	}
	if strings.TrimSpace(args.Orchestrator) == "" {
		return Args{}, errors.New("--orchestrator is required")
	}
	if strings.TrimSpace(args.RunDir) == "" {
		return Args{}, errors.New("--run-dir is required")
	}

	return args, nil
}

func parseRunsArgs(argv []string) (Args, error) {
	if len(argv) == 0 {
		return Args{}, errors.New("missing runs subcommand")
	}
	var command Command
	switch argv[0] {
	case "final":
		command = CommandRunsFinal
	case "inspect":
		command = CommandRunsInspect
	case "tail":
		command = CommandRunsTail
	case "wait":
		command = CommandRunsWait
	default:
		return Args{}, fmt.Errorf("unknown runs subcommand: %s", argv[0])
	}

	args := Args{Command: command}
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--run-dir" || strings.HasPrefix(arg, "--run-dir="):
			value, err := consumeRequiredValue(arg, "--run-dir", argv, &i)
			if err != nil {
				return Args{}, err
			}
			args.RunDir = value
		case arg == "-h" || arg == "--help":
			args.ShowHelp = true
		default:
			return Args{}, fmt.Errorf("unknown option: %s", arg)
		}
	}
	if args.ShowHelp {
		return args, nil
	}
	if strings.TrimSpace(args.RunDir) == "" {
		return Args{}, errors.New("--run-dir is required")
	}
	return args, nil
}

func consumeRequiredValue(arg, flag string, argv []string, i *int) (string, error) {
	if strings.HasPrefix(arg, flag+"=") {
		value := strings.TrimPrefix(arg, flag+"=")
		if value == "" {
			return "", fmt.Errorf("missing value for %s", flag)
		}
		return value, nil
	}
	if arg != flag {
		return "", fmt.Errorf("missing value for %s", flag)
	}
	if *i+1 >= len(argv) {
		return "", fmt.Errorf("missing value for %s", flag)
	}
	*i = *i + 1
	value := argv[*i]
	if value == "" {
		return "", fmt.Errorf("missing value for %s", flag)
	}
	return value, nil
}

func consumeOptionalValue(arg, flag string, argv []string, i *int) (string, bool, error) {
	if strings.HasPrefix(arg, flag+"=") {
		value := strings.TrimPrefix(arg, flag+"=")
		if value == "" {
			return "", true, fmt.Errorf("missing value for %s", flag)
		}
		return value, true, nil
	}
	if arg != flag {
		return "", false, nil
	}
	if *i+1 < len(argv) && !strings.HasPrefix(argv[*i+1], "-") {
		*i = *i + 1
		value := argv[*i]
		if value == "" {
			return "", true, fmt.Errorf("missing value for %s", flag)
		}
		return value, true, nil
	}
	return "", false, nil
}
