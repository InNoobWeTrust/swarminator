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
			argv:     []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "--feedback", "stderr"},
			stdinTTY: true,
			want: Args{
				Model:    "gemini-2.5-flash",
				Persona:  "You are concise.",
				Feedback: "stderr",
			},
		},
		{
			name:     "supports equals feedback syntax",
			argv:     []string{"-m=gemini-2.5-flash", "-p=You are concise.", "--feedback=stderr"},
			stdinTTY: true,
			want: Args{
				Model:    "gemini-2.5-flash",
				Persona:  "You are concise.",
				Feedback: "stderr",
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
			name:      "rejects tutorial without topic or stdin",
			argv:      []string{"--tutorial"},
			stdinTTY:  true,
			wantError: "--tutorial requires a topic",
		},
		{
			name:      "rejects conflicting tutorial and node flags",
			argv:      []string{"--tutorial=quickstart", "-m", "gemini-2.5-flash", "-p", "You are concise."},
			stdinTTY:  true,
			wantError: "--tutorial cannot be combined",
		},
		{
			name:      "rejects invalid timeout",
			argv:      []string{"-m", "gemini-2.5-flash", "-p", "You are concise.", "-t", "abc"},
			stdinTTY:  true,
			wantError: "invalid timeout",
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
