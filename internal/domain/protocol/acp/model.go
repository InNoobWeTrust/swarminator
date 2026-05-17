package acp

import (
	"encoding/json"
	"fmt"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("RPC error (%d): %s", e.Code, e.Message)
}

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

type SessionNewParams struct {
	Cwd        string   `json:"cwd"`
	McpServers []string `json:"mcpServers"`
}

type SessionNewResult struct {
	SessionId string `json:"sessionId"`
}

type SessionPromptParams struct {
	SessionId string                   `json:"sessionId"`
	Prompt    []map[string]string      `json:"prompt"`
}

type SessionPromptResult struct {
	StopReason string `json:"stopReason"`
}

type SessionCancelParams struct {
	SessionId string `json:"sessionId"`
}

type SessionCloseParams struct {
	SessionId string `json:"sessionId"`
}

type SessionSetModeParams struct {
	SessionId string `json:"sessionId"`
	ModeId    string `json:"modeId"`
}

type RequestPermissionOption struct {
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
}

type SessionRequestPermissionParams struct {
	SessionId string                    `json:"sessionId"`
	Options   []RequestPermissionOption `json:"options,omitempty"`
}

type SessionCapabilities struct {
	Close  json.RawMessage `json:"close,omitempty"`
	Resume json.RawMessage `json:"resume,omitempty"`
	List   json.RawMessage `json:"list,omitempty"`
}

type AgentCapabilities struct {
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities,omitempty"`
	LoadSession         bool                `json:"loadSession,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities,omitempty"`
}