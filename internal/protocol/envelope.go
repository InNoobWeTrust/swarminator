package protocol

import (
	"fmt"
	"strings"
)

type Envelope struct {
	Role   string
	Intent string
	Target string
	Body   string
}

func NewEnvelope(role, intent, target, body string) Envelope {
	return Envelope{Role: role, Intent: intent, Target: target, Body: body}
}

func (e Envelope) String() string {
	var b strings.Builder
	if e.Role != "" {
		fmt.Fprintf(&b, "ROLE: %s\n", e.Role)
	}
	if e.Intent != "" {
		fmt.Fprintf(&b, "INTENT: %s\n", e.Intent)
	}
	if e.Target != "" {
		fmt.Fprintf(&b, "TARGET: %s\n", e.Target)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(strings.TrimSpace(e.Body))
	return b.String()
}

func Parse(input string) Envelope {
	var env Envelope
	lines := strings.Split(input, "\n")
	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			bodyStart = i + 1
			break
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			break
		}
		key := strings.TrimSpace(strings.ToUpper(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "ROLE":
			env.Role = value
		case "INTENT":
			env.Intent = value
		case "TARGET":
			env.Target = value
		default:
			bodyStart = i
			goto done
		}
	}
done:
	if bodyStart < len(lines) {
		env.Body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	}
	if env.Body == "" {
		env.Body = strings.TrimSpace(input)
	}
	return env
}
