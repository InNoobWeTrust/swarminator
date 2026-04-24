package rules

import (
	"errors"
	"strings"
	"testing"
)

type captureSink struct {
	messages []string
}

func (c *captureSink) Emit(message string) {
	c.messages = append(c.messages, message)
}

func TestValidateRejectsRuleViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		model        string
		persona      string
		input        string
		timeoutSec   int
		wantMessage  string
		wantExitCode int
		wantFeedback string
	}{
		{
			name:         "missing model",
			persona:      "reviewer",
			input:        "prompt",
			timeoutSec:   0,
			wantMessage:  "model is required",
			wantExitCode: ExitRuleViolation,
		},
		{
			name:         "missing persona",
			model:        "gemini-2.5-flash",
			input:        "prompt",
			timeoutSec:   0,
			wantMessage:  "persona is required",
			wantExitCode: ExitRuleViolation,
		},
		{
			name:         "missing input",
			model:        "gemini-2.5-flash",
			persona:      "reviewer",
			timeoutSec:   0,
			wantMessage:  "stdin input is empty",
			wantExitCode: ExitRuleViolation,
		},
		{
			name:         "negative timeout",
			model:        "gemini-2.5-flash",
			persona:      "reviewer",
			input:        "prompt",
			timeoutSec:   -1,
			wantMessage:  "timeout must be >= 0",
			wantExitCode: ExitRuleViolation,
		},
		{
			name:         "secret in persona",
			model:        "gemini-2.5-flash",
			persona:      "api_key: abcdef12",
			input:        "prompt",
			timeoutSec:   0,
			wantMessage:  "inline credential detected in persona",
			wantExitCode: ExitRuleViolation,
			wantFeedback: "RULE VIOLATION",
		},
		{
			name:         "secret in input",
			model:        "gemini-2.5-flash",
			persona:      "reviewer",
			input:        "token = abcdef12",
			timeoutSec:   0,
			wantMessage:  "inline credential detected in input",
			wantExitCode: ExitRuleViolation,
			wantFeedback: "RULE VIOLATION",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sink := &captureSink{}
			err := NewEngine(sink).Validate(tt.model, tt.persona, tt.input, tt.timeoutSec)
			if err == nil {
				t.Fatalf("Validate() error = nil, want %q", tt.wantMessage)
			}
			var violation Violation
			if !errors.As(err, &violation) {
				t.Fatalf("Validate() error = %T, want Violation", err)
			}
			if violation.Code != tt.wantExitCode {
				t.Fatalf("Violation.Code = %d, want %d", violation.Code, tt.wantExitCode)
			}
			if !strings.Contains(violation.Error(), tt.wantMessage) {
				t.Fatalf("Violation.Error() = %q, want substring %q", violation.Error(), tt.wantMessage)
			}
			if tt.wantFeedback != "" {
				if len(sink.messages) == 0 {
					t.Fatal("expected rule violation feedback")
				}
				if !strings.Contains(sink.messages[0], tt.wantFeedback) {
					t.Fatalf("feedback = %q, want %q", sink.messages[0], tt.wantFeedback)
				}
			} else if len(sink.messages) != 0 {
				t.Fatalf("feedback = %#v, want none", sink.messages)
			}
		})
	}
}

func TestValidateEmitsAdvisoryFeedback(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	err := NewEngine(sink).Validate("gemini-2.5-flash", "please run shell commands", "prompt", 0)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(sink.messages) != 1 {
		t.Fatalf("feedback count = %d, want 1", len(sink.messages))
	}
	if !strings.Contains(sink.messages[0], "ADVISORY: node personas should stay analysis-only") {
		t.Fatalf("feedback = %q, want advisory notice", sink.messages[0])
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	if got := ExitCode(errors.New("boom")); got != ExitRetryable {
		t.Fatalf("ExitCode(non-violation) = %d, want %d", got, ExitRetryable)
	}
	if got := ExitCode(Violation{Code: ExitRuleViolation, Message: "bad"}); got != ExitRuleViolation {
		t.Fatalf("ExitCode(violation) = %d, want %d", got, ExitRuleViolation)
	}
}
