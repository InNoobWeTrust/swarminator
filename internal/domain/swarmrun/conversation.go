package swarmrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type MessageKind string

const (
	MessageKindSystemContract  MessageKind = "system_contract"
	MessageKindMemorySummary   MessageKind = "memory_summary"
	MessageKindUserInput       MessageKind = "user_input"
	MessageKindAssistantOutput MessageKind = "assistant_output"
	MessageKindToolResult      MessageKind = "tool_result"
)

type Message struct {
	Role        Role
	Kind        MessageKind
	Content     string
	Name        string
	ArtifactRef string
	ModelID     string
	PersonaID   string
}

type Receipt struct {
	RunID  string `json:"run_id"`
	RunDir string `json:"run_dir"`
	Status string `json:"status"`
}

type NodeAction struct {
	Model   string `json:"model"`
	Persona string `json:"persona"`
	Input   string `json:"input"`
}

func (a NodeAction) Validate() error {
	if strings.TrimSpace(a.Model) == "" {
		return errors.New("node action requires model")
	}
	if strings.TrimSpace(a.Persona) == "" {
		return errors.New("node action requires persona")
	}
	if strings.TrimSpace(a.Input) == "" {
		return errors.New("node action requires input")
	}
	return nil
}

type ToolName string

const ToolNameRunWorkerNode ToolName = "run_worker_node"

type ToolParameterType string

const ToolParameterTypeString ToolParameterType = "string"

type ToolDefinition struct {
	Name        ToolName
	Description string
	Parameters  []ToolParameter
}

type ToolParameter struct {
	Name        string
	Description string
	Required    bool
	Type        ToolParameterType
	Enum        []string
}

type ToolCall struct {
	Name      ToolName
	Arguments map[string]string
}

func (c ToolCall) Validate() error {
	if strings.TrimSpace(string(c.Name)) == "" {
		return errors.New("tool call name is required")
	}
	if c.Arguments == nil {
		return errors.New("tool call arguments are required")
	}
	return nil
}

func (d ToolDefinition) Validate() error {
	if strings.TrimSpace(string(d.Name)) == "" {
		return errors.New("tool definition name is required")
	}
	seen := make(map[string]struct{}, len(d.Parameters))
	for i, param := range d.Parameters {
		if err := param.Validate(); err != nil {
			return fmt.Errorf("tool %q parameter %d: %w", d.Name, i, err)
		}
		if _, exists := seen[param.Name]; exists {
			return fmt.Errorf("tool %q parameter %q is duplicated", d.Name, param.Name)
		}
		seen[param.Name] = struct{}{}
	}
	return nil
}

func (p ToolParameter) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("parameter name is required")
	}
	switch p.Type {
	case "", ToolParameterTypeString:
	default:
		return fmt.Errorf("unsupported parameter type %q", p.Type)
	}
	seen := make(map[string]struct{}, len(p.Enum))
	for _, value := range p.Enum {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("parameter %q enum values must be non-empty", p.Name)
		}
		if _, exists := seen[trimmed]; exists {
			return fmt.Errorf("parameter %q enum value %q is duplicated", p.Name, trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

func ValidateToolCallAgainstDefinitions(call ToolCall, defs []ToolDefinition) error {
	if err := call.Validate(); err != nil {
		return err
	}
	for _, def := range defs {
		if def.Name != call.Name {
			continue
		}
		return def.ValidateCall(call)
	}
	return fmt.Errorf("tool call %q is not defined", call.Name)
}

func (d ToolDefinition) ValidateCall(call ToolCall) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if err := call.Validate(); err != nil {
		return err
	}
	if d.Name != call.Name {
		return fmt.Errorf("tool call name %q does not match tool definition %q", call.Name, d.Name)
	}
	for _, param := range d.Parameters {
		value := strings.TrimSpace(call.Arguments[param.Name])
		if param.Required && value == "" {
			return fmt.Errorf("tool call %q requires argument %q", call.Name, param.Name)
		}
		if value == "" || len(param.Enum) == 0 {
			continue
		}
		if !containsTrimmed(param.Enum, value) {
			return fmt.Errorf("tool call %q argument %q must be one of %s", call.Name, param.Name, strings.Join(param.Enum, ", "))
		}
	}
	return nil
}

type TokenBudget struct {
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
}

func (b TokenBudget) Validate() error {
	if b.ContextWindow <= 0 {
		return errors.New("context window must be > 0")
	}
	if b.MaxInputTokens <= 0 {
		return errors.New("max input tokens must be > 0")
	}
	if b.MaxOutputTokens <= 0 {
		return errors.New("max output tokens must be > 0")
	}
	if b.MaxInputTokens > b.ContextWindow {
		return errors.New("max input tokens must be <= context window")
	}
	return nil
}

type ConversationRequest struct {
	Model           string
	Messages        []Message
	Tools           []ToolDefinition
	MaxOutputTokens int
	Temperature     float64
	RuntimeProfile  string
}

type ConversationResponse struct {
	Message  Message
	ToolCall *ToolCall
}

type ConversationTransport interface {
	Send(ctx context.Context, req ConversationRequest) (ConversationResponse, error)
}

func (r ConversationRequest) Validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return errors.New("model is required")
	}
	if len(r.Messages) == 0 {
		return errors.New("at least one message is required")
	}
	if r.MaxOutputTokens < 0 {
		return fmt.Errorf("max output tokens must be >= 0")
	}

	for i, msg := range r.Messages {
		switch msg.Role {
		case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		default:
			return fmt.Errorf("message %d role %q is not supported", i, msg.Role)
		}
		if strings.TrimSpace(msg.Content) == "" {
			return fmt.Errorf("message %d content is required", i)
		}
	}
	for i, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return fmt.Errorf("tool %d: %w", i, err)
		}
	}

	return nil
}

func containsTrimmed(values []string, target string) bool {
	trimmedTarget := strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == trimmedTarget {
			return true
		}
	}
	return false
}
