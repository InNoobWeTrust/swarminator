package cli

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		argv      []string
		stdin     string
		stdinTTY  bool
		want      Args
		wantError string
	}{
		{
			name:     "supports separate feedback syntax",
			argv:     []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent=gemini", "--feedback", "stderr"},
			stdinTTY: true,
			want: Args{
				Model:      "gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				Agent:      "gemini",
				Feedback:   "stderr",
			},
		},
		{
			name:     "supports equals feedback syntax",
			argv:     []string{"-m=gemini-2.5-flash", "-p=You are concise.", "-t=45", "--agent=gemini", "--feedback=stderr"},
			stdinTTY: true,
			want: Args{
				Model:      "gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 45,
				Agent:      "gemini",
				Feedback:   "stderr",
			},
		},
		{
			name:     "reads tutorial from stdin",
			argv:     []string{"--tutorial"},
			stdin:    "quickstart\n",
			stdinTTY: false,
			want: Args{
				Tutorial: "quickstart",
			},
		},
		{
			name:     "supports tutorial equals syntax",
			argv:     []string{"--tutorial=rules"},
			stdinTTY: true,
			want: Args{
				Tutorial: "rules",
			},
		},
		{
			name:     "built-in tutorial may include model override",
			argv:     []string{"--tutorial=quickstart", "-m", "kilo/custom/model"},
			stdinTTY: true,
			want: Args{
				Tutorial: "quickstart",
				Model:    "kilo/custom/model",
			},
		},
		{
			name:     "built-in tutorial may include leading model override",
			argv:     []string{"-m", "kilo/custom/model", "--tutorial=quickstart"},
			stdinTTY: true,
			want: Args{
				Tutorial: "quickstart",
				Model:    "kilo/custom/model",
			},
		},
		{
			name:     "tutorial q and a requires explicit agent and model",
			argv:     []string{"--tutorial", "how do I pass a timeout?", "--agent=gemini", "-m", "google/gemini-2.5-flash"},
			stdinTTY: true,
			want: Args{
				Tutorial: "how do I pass a timeout?",
				Agent:    "gemini",
				Model:    "google/gemini-2.5-flash",
			},
		},
		{
			name:      "rejects tutorial q and a without agent",
			argv:      []string{"--tutorial", "how do I pass a timeout?", "-m", "google/gemini-2.5-flash"},
			stdinTTY:  true,
			wantError: "tutorial Q&A requires --agent=NAME",
		},
		{
			name:      "rejects tutorial q and a without model",
			argv:      []string{"--tutorial", "how do I pass a timeout?", "--agent=gemini"},
			stdinTTY:  true,
			wantError: "tutorial Q&A requires -m MODEL",
		},
		{
			name:     "tutorial model suggestion may omit model when agent is explicit",
			argv:     []string{"--tutorial", "which model should I use for code review", "--agent=gemini"},
			stdinTTY: true,
			want: Args{
				Tutorial: "which model should I use for code review",
				Agent:    "gemini",
			},
		},
		{
			name:      "rejects tutorial without topic or stdin",
			argv:      []string{"--tutorial"},
			stdinTTY:  true,
			wantError: "--tutorial requires a topic",
		},
		{
			name:      "rejects conflicting tutorial and persona flag",
			argv:      []string{"--tutorial=quickstart", "-p", "You are concise."},
			stdinTTY:  true,
			wantError: "--tutorial cannot be combined with -p",
		},
		{
			name:      "rejects invalid timeout",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "abc", "--agent=gemini"},
			stdinTTY:  true,
			wantError: "invalid timeout",
		},
		{
			name:      "rejects missing timeout for normal run",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "--agent=gemini"},
			stdinTTY:  true,
			wantError: "-t SECONDS is required",
		},
		{
			name:      "rejects zero timeout",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "0", "--agent=gemini"},
			stdinTTY:  true,
			wantError: "-t must be > 0",
		},
		{
			name:     "--list-agents bypasses required node flags",
			argv:     []string{"--list-agents"},
			stdinTTY: true,
			want:     Args{ListAgents: true},
		},
		{
			name:     "--list-models bypasses required node flags",
			argv:     []string{"--list-models"},
			stdinTTY: true,
			want:     Args{ListModels: true},
		},
		{
			name:     "--list-providers bypasses required node flags",
			argv:     []string{"--list-providers"},
			stdinTTY: true,
			want:     Args{ListProviders: true},
		},
		{
			name:     "list models supports json and agent filter",
			argv:     []string{"--list-models", "--json", "--agent=gemini"},
			stdinTTY: true,
			want: Args{
				ListModels: true,
				JSONOutput: true,
				Agent:      "gemini",
			},
		},
		{
			name:     "--list-agents combined with nothing else",
			argv:     []string{"--list-agents"},
			stdinTTY: false,
			want:     Args{ListAgents: true},
		},
		{
			name:     "--agent-mode equals syntax",
			argv:     []string{"-m", "google/gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent=gemini", "--agent-mode=yolo"},
			stdinTTY: true,
			want: Args{
				Model:      "google/gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				Agent:      "gemini",
				AgentMode:  "yolo",
			},
		},
		{
			name:     "--agent-mode separate syntax",
			argv:     []string{"-m", "google/gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent=gemini", "--agent-mode", "default"},
			stdinTTY: true,
			want: Args{
				Model:      "google/gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				Agent:      "gemini",
				AgentMode:  "default",
			},
		},
		{
			name:      "rejects json without list mode",
			argv:      []string{"--json", "-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent=gemini"},
			stdinTTY:  true,
			wantError: "--json is only supported",
		},
		{
			name:      "rejects missing agent for normal run",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "30"},
			stdinTTY:  true,
			wantError: "--agent=NAME is required for node execution",
		},
		{
			name:     "agent with all flags",
			argv:     []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent=gemini", "--feedback=stderr"},
			stdinTTY: true,
			want: Args{
				Model:      "gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				Agent:      "gemini",
				Feedback:   "stderr",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.argv, strings.NewReader(tt.stdin), tt.stdinTTY)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("Parse() error = nil, want %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Parse() error = %q, want substring %q", err.Error(), tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
