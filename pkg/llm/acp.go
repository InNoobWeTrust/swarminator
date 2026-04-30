// Package llm/acp implements the Agent Communication Protocol (ACP) provider.
//
// # ACP Wire Format (observed with gemini v0.38.2)
//
// All messages are newline-delimited JSON-RPC 2.0 over stdio.
//
// ## Outbound (swarminator → agent)
//
//	initialize  → {"jsonrpc":"2.0","method":"initialize","params":{...},"id":1}
//	session/new → {"jsonrpc":"2.0","method":"session/new","params":{...},"id":2}
//	session/prompt → {"jsonrpc":"2.0","method":"session/prompt","params":{...},"id":3}
//	session/close → {"jsonrpc":"2.0","method":"session/close","params":{...},"id":4}
//	[notification] session/cancel → {"jsonrpc":"2.0","method":"session/cancel","params":{...}}
//
// ## Inbound (agent → swarminator)
//
// RPC responses carry "result" or "error" and match the outbound "id".
//
// Text chunks arrive as session/update notifications (no "id"):
//
//	{
//	  "jsonrpc": "2.0",
//	  "method": "session/update",
//	  "params": {
//	    "update": {
//	      "sessionUpdate": "agent_message_chunk",
//	      "content": {
//	        "type": "text",
//	        "text": "<chunk>"
//	      }
//	    }
//	  }
//	}
//
// All text chunks must be concatenated in order to reconstruct the full response.
// The final accumulated text is returned from Complete once the session/prompt RPC resolves.
package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"swarminator/internal/protocol/acp"
)

const acpProcessWaitTimeout = 5 * time.Second

type ACPProvider struct {
	binary string
	args   []string
}

func NewACPProvider(binary string, args ...string) Provider {
	return &ACPProvider{
		binary: binary,
		args:   args,
	}
}

func (p *ACPProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	// Wrap ctx with a cancel so we can trigger process termination promptly when
	// Complete returns, rather than waiting for the caller's deadline to expire.
	ctx, cancelCtx := context.WithCancel(ctx)

	cmd := exec.CommandContext(ctx, p.binary, p.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancelCtx()
		return "", fmt.Errorf("failed to open stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelCtx()
		return "", fmt.Errorf("failed to open stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancelCtx()
		return "", fmt.Errorf("failed to start agent %s: %w", p.binary, err)
	}

	// Ensure cleanup. cancelCtx() fires watchCtx immediately so the process is
	// killed without waiting for the caller's deadline. stdout.Close() is a
	// belt-and-suspenders measure: it unblocks the scanner goroutine even if
	// child processes inherited the pipe write-end and are still alive.
	var wg sync.WaitGroup
	defer func() {
		cancelCtx()     // triggers exec.CommandContext's watchCtx to kill the process
		stdin.Close()
		stdout.Close() // unblock scanner regardless of child-process pipe holders
		wg.Wait()
		waitForProcessExit(cmd, acpProcessWaitTimeout)
	}()

	// Read responses in a goroutine
	responseChan := make(chan *acp.Response, 10)
	errChan := make(chan error, 1)
	var responseText strings.Builder
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(responseChan)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)
		for scanner.Scan() {
			var resp acp.Response
			if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
				continue
			}
			// Handle notifications
			if resp.Method == "session/update" && resp.ID == nil {
				// gemini --acp sends agent text via:
				//   params.update.sessionUpdate == "agent_message_chunk"
				//   params.update.content.type  == "text"
				//   params.update.content.text  == "<chunk>"
				var params struct {
					Update struct {
						SessionUpdate string `json:"sessionUpdate"`
						Content       struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"content"`
					} `json:"update"`
				}
				if err := json.Unmarshal(resp.Params, &params); err == nil &&
					params.Update.SessionUpdate == "agent_message_chunk" &&
					params.Update.Content.Type == "text" {
					responseText.WriteString(params.Update.Content.Text)
				}
				continue
			}
			// Ignore other notifications
			if resp.Method != "" && resp.ID == nil {
				continue
			}
			select {
			case responseChan <- &resp:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			// Suppress errors caused by explicit stdout.Close() in the cleanup defer.
			if !errors.Is(err, os.ErrClosed) {
				errChan <- err
			}
		}
	}()

	// 1. Initialize
	initParams := acp.InitializeParams{
		ClientInfo: acp.ClientInfo{
			Name:    "swarminator",
			Version: "1.0.0",
			Title:   "Swarminator",
		},
		ClientCapabilities: acp.ClientCapabilities{
			Fs: &acp.FsCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ProtocolVersion: 1,
	}
	initParamsJSON, _ := json.Marshal(initParams)
	if err := sendRequest(stdin, "initialize", initParamsJSON, 1); err != nil {
		return "", err
	}
	resp, err := waitResponse(ctx, 1, responseChan, errChan)
	if err != nil {
		return "", fmt.Errorf("initialize: %w", err)
	}
	var agentCaps acp.AgentCapabilities
	if len(resp.Result) > 0 && string(resp.Result) != "null" {
		var initResult acp.InitializeResult
		if err := json.Unmarshal(resp.Result, &initResult); err == nil {
			agentCaps = initResult.AgentCapabilities
		}
	}

	// 2. New Session
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get cwd: %w", err)
	}
	sessionParams := acp.SessionNewParams{
		Cwd:        cwd,
		McpServers: []string{},
	}
	sessionParamsJSON, _ := json.Marshal(sessionParams)
	if err := sendRequest(stdin, "session/new", sessionParamsJSON, 2); err != nil {
		return "", err
	}
	resp, err = waitResponse(ctx, 2, responseChan, errChan)
	if err != nil {
		return "", fmt.Errorf("session/new: %w", err)
	}
	var sessionResult acp.SessionNewResult
	if err := json.Unmarshal(resp.Result, &sessionResult); err != nil {
		return "", fmt.Errorf("failed to parse session result: %w", err)
	}

	// 3. Prompt
	promptParams := acp.SessionPromptParams{
		SessionId: sessionResult.SessionId,
		Prompt: []map[string]string{
			{"type": "text", "text": fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)},
		},
	}
	promptParamsJSON, _ := json.Marshal(promptParams)
	if err := sendRequest(stdin, "session/prompt", promptParamsJSON, 3); err != nil {
		return "", err
	}
	resp, err = waitPromptResponse(ctx, 3, sessionResult.SessionId, stdin, responseChan, errChan)
	if err != nil {
		return "", fmt.Errorf("session/prompt: %w", err)
	}
	var promptResult acp.SessionPromptResult
	if err := json.Unmarshal(resp.Result, &promptResult); err != nil {
		return "", fmt.Errorf("failed to parse prompt result: %w", err)
	}

	if promptResult.StopReason != "" && promptResult.StopReason != "cancelled" {
		if agentCaps.SessionCapabilities.Close != nil {
			closeParams := acp.SessionCloseParams{SessionId: sessionResult.SessionId}
			closeParamsJSON, _ := json.Marshal(closeParams)
			if err := sendRequest(stdin, "session/close", closeParamsJSON, 4); err == nil {
				closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, _ = waitResponse(closeCtx, 4, responseChan, errChan)
			}
		}
	}

	return responseText.String(), nil
}

func sendRequest(w io.Writer, method string, params json.RawMessage, id interface{}) error {
	req := acp.Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
	return json.NewEncoder(w).Encode(req)
}

func sendNotification(w io.Writer, method string, params json.RawMessage) error {
	notif := struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return json.NewEncoder(w).Encode(notif)
}

func waitResponse(ctx context.Context, id interface{}, responseChan chan *acp.Response, errChan chan error) (*acp.Response, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errChan:
			return nil, err
		case resp, ok := <-responseChan:
			if !ok {
				responseChan = nil
				continue
			}
			if fmt.Sprintf("%v", resp.ID) == fmt.Sprintf("%v", id) {
				if resp.Error != nil {
					return nil, resp.Error
				}
				return resp, nil
			}
			// If we get a response for a different ID, we just keep waiting
		}
	}
}

func waitPromptResponse(ctx context.Context, id interface{}, sessionId string, stdin io.Writer, responseChan chan *acp.Response, errChan chan error) (*acp.Response, error) {
	for {
		select {
		case <-ctx.Done():
			// Send session/cancel notification (best-effort, context already expired)
			cancelParams, _ := json.Marshal(acp.SessionCancelParams{SessionId: sessionId})
			_ = sendNotification(stdin, "session/cancel", cancelParams)
			return nil, ctx.Err()
		case err := <-errChan:
			return nil, err
		case resp, ok := <-responseChan:
			if !ok {
				responseChan = nil
				continue
			}
			if fmt.Sprintf("%v", resp.ID) == fmt.Sprintf("%v", id) {
				if resp.Error != nil {
					return nil, resp.Error
				}
				return resp, nil
			}
		}
	}
}

func waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
}
