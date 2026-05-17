package rules

import (
	"fmt"
	"regexp"
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

func SecretPatternMatch(s string) bool {
	return secretPattern.MatchString(s)
}