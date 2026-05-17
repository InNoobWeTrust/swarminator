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
	Model         string
	Persona       string
	Agent         string
	AgentMode     string
	Feedback      string
	Tutorial      string
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

func Parse(argv []string, stdin io.Reader, stdinIsTTY bool) (Args, error) {
	args := Args{}
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
