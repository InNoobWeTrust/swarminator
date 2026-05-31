package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"swarminator/internal/app/runinspect"
	"swarminator/internal/app/swarmruntime"
	"swarminator/internal/cli"
	"swarminator/internal/domain/swarmrun"
)

func TestRunSwarmExecWritesOnlyFinalAnswerToStdout(t *testing.T) {
	t.Parallel()

	args := cliTestArgsForSwarmExec()
	stdin := strings.NewReader("debug this")
	stdout := &bytes.Buffer{}
	runner := swarmExecRunnerFunc(func(ctx context.Context, req swarmruntime.Request) (string, error) {
		return "final answer", nil
	})

	if err := runSwarmExec(context.Background(), args, stdin, stdout, runner); err != nil {
		t.Fatalf("runSwarmExec() error = %v", err)
	}
	if got, want := stdout.String(), "final answer\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunSwarmStartWritesReceiptOnly(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	starter := swarmStartRunnerFunc(func(ctx context.Context, req swarmruntime.Request) (swarmrun.Receipt, error) {
		return swarmrun.Receipt{RunID: "run-123", RunDir: "/tmp/run-123", Status: string(swarmrun.RunStatePending)}, nil
	})

	if err := runSwarmStart(context.Background(), cli.Args{Command: cli.CommandSwarmStart, SwarmRoot: "/tmp/swarm", Orchestrator: "main", RunDir: "/tmp/run-123"}, strings.NewReader("async job"), stdout, starter); err != nil {
		t.Fatalf("runSwarmStart() error = %v", err)
	}
	var receipt swarmrun.Receipt
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if receipt.RunID != "run-123" || receipt.Status != string(swarmrun.RunStatePending) {
		t.Fatalf("receipt = %#v, want pending receipt", receipt)
	}
}

func TestRunRunsCommandsUseInspectorOutputs(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	inspector := runInspectorStub{
		final:   "final answer",
		inspect: runinspect.InspectResult{Status: swarmrun.Status{RunID: "run-123", State: swarmrun.RunStateCompleted}, RunDir: "/tmp/run-123"},
		tail:    "{\"type\":\"run_started\"}\n",
		wait:    "final answer",
	}

	if err := runRunsFinal(cli.Args{Command: cli.CommandRunsFinal, RunDir: "/tmp/run-123"}, stdout, inspector); err != nil {
		t.Fatalf("runRunsFinal() error = %v", err)
	}
	if got := stdout.String(); got != "final answer\n" {
		t.Fatalf("runRunsFinal stdout = %q, want final answer", got)
	}

	stdout.Reset()
	if err := runRunsInspect(cli.Args{Command: cli.CommandRunsInspect, RunDir: "/tmp/run-123"}, stdout, inspector); err != nil {
		t.Fatalf("runRunsInspect() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "run-123") {
		t.Fatalf("runRunsInspect stdout = %q, want run id", stdout.String())
	}

	stdout.Reset()
	if err := runRunsTail(cli.Args{Command: cli.CommandRunsTail, RunDir: "/tmp/run-123"}, stdout, inspector); err != nil {
		t.Fatalf("runRunsTail() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "run_started") {
		t.Fatalf("runRunsTail stdout = %q, want run_started", stdout.String())
	}

	stdout.Reset()
	if err := runRunsWait(context.Background(), cli.Args{Command: cli.CommandRunsWait, RunDir: "/tmp/run-123"}, stdout, inspector); err != nil {
		t.Fatalf("runRunsWait() error = %v", err)
	}
	if got := stdout.String(); got != "final answer\n" {
		t.Fatalf("runRunsWait stdout = %q, want final answer", got)
	}
}

func cliTestArgsForSwarmExec() cli.Args {
	return cli.Args{
		Command:      cli.CommandSwarmExec,
		SwarmRoot:    "/tmp/swarm",
		Orchestrator: "main",
		RunDir:       "/tmp/run-123",
		EventSink:    "file:///tmp/run-123/events.jsonl",
	}
}

type swarmExecRunnerFunc func(context.Context, swarmruntime.Request) (string, error)

func (f swarmExecRunnerFunc) Execute(ctx context.Context, req swarmruntime.Request) (string, error) {
	return f(ctx, req)
}

type swarmStartRunnerFunc func(context.Context, swarmruntime.Request) (swarmrun.Receipt, error)

func (f swarmStartRunnerFunc) Start(ctx context.Context, req swarmruntime.Request) (swarmrun.Receipt, error) {
	return f(ctx, req)
}

type runInspectorStub struct {
	final   string
	inspect runinspect.InspectResult
	tail    string
	wait    string
}

func (s runInspectorStub) Final(runDir string) (string, error) {
	return s.final, nil
}

func (s runInspectorStub) Inspect(runDir string) (runinspect.InspectResult, error) {
	return s.inspect, nil
}

func (s runInspectorStub) Tail(runDir string) (string, error) {
	return s.tail, nil
}

func (s runInspectorStub) Wait(ctx context.Context, runDir string) (string, error) {
	return s.wait, nil
}
