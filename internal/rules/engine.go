package rules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	ExitSuccess       = 0
	ExitRetryable     = 2
	ExitRuleViolation = 3
)

var secretPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*['"]?[a-z0-9_\-]{6,}['"]?`)

type Violation struct {
	Code    int
	Message string
	Hint    string
}

func (v Violation) Error() string {
	if v.Hint == "" {
		return v.Message
	}
	return fmt.Sprintf("%s Hint: %s", v.Message, v.Hint)
}

type FeedbackSink interface {
	Emit(message string)
}

type Engine struct {
	feedback FeedbackSink
}

func NewEngine(feedback FeedbackSink) *Engine {
	return &Engine{feedback: feedback}
}

func (e *Engine) Validate(model, persona, input string, timeoutSec int) error {
	if strings.TrimSpace(model) == "" {
		return Violation{Code: ExitRuleViolation, Message: "model is required"}
	}
	if strings.TrimSpace(persona) == "" {
		return Violation{Code: ExitRuleViolation, Message: "persona is required"}
	}
	if strings.TrimSpace(input) == "" {
		return Violation{Code: ExitRuleViolation, Message: "stdin input is empty", Hint: "pipe a non-empty prompt or document to swarminator"}
	}
	if timeoutSec < 0 {
		return Violation{Code: ExitRuleViolation, Message: "timeout must be >= 0"}
	}
	if secretPattern.MatchString(persona) {
		e.emit("RULE VIOLATION: inline credential detected in persona")
		return Violation{Code: ExitRuleViolation, Message: "inline credential detected in persona", Hint: "use environment variables or credential references instead"}
	}
	if secretPattern.MatchString(input) {
		e.emit("RULE VIOLATION: inline credential detected in input")
		return Violation{Code: ExitRuleViolation, Message: "inline credential detected in input", Hint: "remove secrets before sending the prompt"}
	}
	if strings.Contains(strings.ToLower(persona), "run shell") || strings.Contains(strings.ToLower(persona), "modify files") {
		e.emit("ADVISORY: node personas should stay analysis-only unless explicitly elevated")
	}
	return nil
}

func ExitCode(err error) int {
	var violation Violation
	if errors.As(err, &violation) {
		return violation.Code
	}
	return ExitRetryable
}

func (e *Engine) emit(message string) {
	if e.feedback != nil {
		e.feedback.Emit(message)
	}
	}
