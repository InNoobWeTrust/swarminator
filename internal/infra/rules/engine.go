package rules

import (
	"errors"
	"strings"

	"swarminator/internal/domain/rules"
)

type Engine struct {
	feedback rules.FeedbackSink
}

func NewEngine(feedback rules.FeedbackSink) *Engine {
	return &Engine{feedback: feedback}
}

func (e *Engine) Validate(model, persona, input string, timeoutSec int) error {
	if strings.TrimSpace(model) == "" {
		return rules.Violation{Code: rules.ExitRuleViolation, Message: "model is required"}
	}
	if strings.TrimSpace(persona) == "" {
		return rules.Violation{Code: rules.ExitRuleViolation, Message: "persona is required"}
	}
	if strings.TrimSpace(input) == "" {
		return rules.Violation{Code: rules.ExitRuleViolation, Message: "stdin input is empty", Hint: "pipe a non-empty prompt or document to swarminator"}
	}
	if timeoutSec <= 0 {
		return rules.Violation{Code: rules.ExitRuleViolation, Message: "timeout must be > 0"}
	}
	if rules.SecretPatternMatch(persona) {
		e.emit("RULE VIOLATION: inline credential detected in persona")
		return rules.Violation{Code: rules.ExitRuleViolation, Message: "inline credential detected in persona", Hint: "use environment variables or credential references instead"}
	}
	if rules.SecretPatternMatch(input) {
		e.emit("RULE VIOLATION: inline credential detected in input")
		return rules.Violation{Code: rules.ExitRuleViolation, Message: "inline credential detected in input", Hint: "remove secrets before sending the prompt"}
	}
	if strings.Contains(strings.ToLower(persona), "run shell") || strings.Contains(strings.ToLower(persona), "modify files") {
		e.emit("ADVISORY: node personas should stay analysis-only unless explicitly elevated")
	}
	return nil
}

func ExitCode(err error) int {
	var violation rules.Violation
	if errors.As(err, &violation) {
		return violation.Code
	}
	return rules.ExitRetryable
}

func (e *Engine) emit(message string) {
	if e.feedback != nil {
		e.feedback.Emit(message)
	}
}