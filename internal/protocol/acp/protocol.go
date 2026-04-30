package acp

import (
	"encoding/json"
	"fmt"
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method,omitempty"` // For notifications
	Params  json.RawMessage `json:"params,omitempty"` // For notifications
}

// Error represents a JSON-RPC 2.0 error.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("RPC error (%d): %s", e.Code, e.Message)
}

// InitializeParams defines the parameters for the initialize method.
type InitializeParams struct {
	ClientInfo       ClientInfo       `json:"clientInfo"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ProtocolVersion  int              `json:"protocolVersion"`
}

type ClientCapabilities struct {
	Fs       *FsCapabilities `json:"fs,omitempty"`
	Terminal bool            `json:"terminal,omitempty"`
}

type FsCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
}

// SessionNewParams defines the parameters for the session/new method.
type SessionNewParams struct {
	Cwd        string   `json:"cwd"`
	McpServers []string `json:"mcpServers"`
}

// SessionNewResult represents the result of the session/new method.
type SessionNewResult struct {
	SessionId string `json:"sessionId"`
}

// SessionPromptParams defines the parameters for the session/prompt method.
type SessionPromptParams struct {
	SessionId string                   `json:"sessionId"`
	Prompt    []map[string]string      `json:"prompt"`
}

// SessionPromptResult represents the result of the session/prompt method.
type SessionPromptResult struct {
	StopReason string `json:"stopReason"`
}

// SessionCancelParams defines the parameters for the session/cancel notification.
type SessionCancelParams struct {
	SessionId string `json:"sessionId"`
}

// SessionCloseParams defines the parameters for the session/close method.
type SessionCloseParams struct {
	SessionId string `json:"sessionId"`
}

// SessionCapabilities describes optional session capabilities the agent supports.
// The presence of a non-nil field indicates the capability is available.
type SessionCapabilities struct {
	Close  json.RawMessage `json:"close,omitempty"`
	Resume json.RawMessage `json:"resume,omitempty"`
	List   json.RawMessage `json:"list,omitempty"`
}

// AgentCapabilities describes capabilities advertised by the agent in initialize response.
type AgentCapabilities struct {
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities,omitempty"`
	LoadSession         bool                `json:"loadSession,omitempty"`
}

// InitializeResult represents the result returned by the agent on initialize.
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities,omitempty"`
}
