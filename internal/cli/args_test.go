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
			argv:     []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--feedback", "stderr"},
			stdinTTY: true,
			want: Args{
				Model:      "gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				Feedback:   "stderr",
			},
		},
		{
			name:     "supports equals feedback syntax",
			argv:     []string{"-m=gemini-2.5-flash", "-p=You are concise.", "-t=45", "--feedback=stderr"},
			stdinTTY: true,
			want: Args{
				Model:      "gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 45,
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
			name:     "tutorial with model override",
			argv:     []string{"--tutorial=quickstart", "-m", "kilo/custom/model"},
			stdinTTY: true,
			want: Args{
				Tutorial: "quickstart",
				Model:    "kilo/custom/model",
			},
		},
		{
			name:     "model before tutorial",
			argv:     []string{"-m", "kilo/custom/model", "--tutorial=quickstart"},
			stdinTTY: true,
			want: Args{
				Tutorial: "quickstart",
				Model:    "kilo/custom/model",
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
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "abc"},
			stdinTTY:  true,
			wantError: "invalid timeout",
		},
		{
			name:      "rejects missing timeout for normal run",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise."},
			stdinTTY:  true,
			wantError: "-t SECONDS is required",
		},
		{
			name:      "rejects zero timeout",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "0"},
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
			name:     "--list-agents combined with nothing else",
			argv:     []string{"--list-agents"},
			stdinTTY: false,
			want:     Args{ListAgents: true},
		},
		{
			name:     "--agent-mode equals syntax",
			argv:     []string{"-m", "google/gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent-mode=yolo"},
			stdinTTY: true,
			want: Args{
				Model:      "google/gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				AgentMode:  "yolo",
			},
		},
		{
			name:     "--agent-mode separate syntax",
			argv:     []string{"-m", "google/gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent-mode", "default"},
			stdinTTY: true,
			want: Args{
				Model:      "google/gemini-2.5-flash",
				Persona:    "You are concise.",
				TimeoutSec: 30,
				AgentMode:  "default",
			},
		},
		{
			name:      "--agent-mode missing value",
			argv:      []string{"-m", "google/gemini-2.5-flash", "-p", "You are concise.", "-t", "30", "--agent-mode"},
			stdinTTY:  true,
			wantError: "missing value for --agent-mode",
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
