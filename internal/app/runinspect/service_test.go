package runinspect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swarminator/internal/domain/swarmrun"
	"swarminator/internal/infra/runstore"
)

func TestServiceReadOnlyCommands(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-123")
	store, err := runstore.Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	completedAt := time.Date(2026, time.May, 24, 12, 0, 0, 0, time.UTC)
	if err := store.WriteStatus(swarmrun.Status{RunID: "run-123", State: swarmrun.RunStateCompleted, UpdatedAt: completedAt, CompletedAt: &completedAt}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.WriteInput("Investigate the failure"); err != nil {
		t.Fatalf("WriteInput() error = %v", err)
	}
	if err := store.WriteFinal("final answer"); err != nil {
		t.Fatalf("WriteFinal() error = %v", err)
	}
	if err := store.AppendEvent(swarmrun.Event{Type: swarmrun.EventRunStarted, At: completedAt, Detail: "started"}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	svc := NewServiceWithPollInterval(5 * time.Millisecond)

	final, err := svc.Final(runDir)
	if err != nil {
		t.Fatalf("Final() error = %v", err)
	}
	if final != "final answer" {
		t.Fatalf("Final() = %q, want %q", final, "final answer")
	}

	inspect, err := svc.Inspect(runDir)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspect.Status.State != swarmrun.RunStateCompleted {
		t.Fatalf("Inspect().Status.State = %q, want %q", inspect.Status.State, swarmrun.RunStateCompleted)
	}
	if !strings.HasSuffix(inspect.RequestPath, "input.txt") {
		t.Fatalf("Inspect().RequestPath = %q, want input.txt suffix", inspect.RequestPath)
	}

	tail, err := svc.Tail(runDir)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if !strings.Contains(tail, string(swarmrun.EventRunStarted)) {
		t.Fatalf("Tail() = %q, want run_started event", tail)
	}

	waited, err := svc.Wait(context.Background(), runDir)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if waited != "final answer" {
		t.Fatalf("Wait() = %q, want %q", waited, "final answer")
	}
}

func TestServiceWaitBlocksUntilCompleted(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-456")
	store, err := runstore.Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.WriteStatus(swarmrun.Status{RunID: "run-456", State: swarmrun.RunStateRunning, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	go func() {
		time.Sleep(25 * time.Millisecond)
		completedAt := time.Now().UTC()
		_ = store.WriteFinal("async final")
		_ = store.WriteStatus(swarmrun.Status{RunID: "run-456", State: swarmrun.RunStateCompleted, UpdatedAt: completedAt, CompletedAt: &completedAt})
	}()

	svc := NewServiceWithPollInterval(5 * time.Millisecond)
	got, err := svc.Wait(context.Background(), runDir)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if got != "async final" {
		t.Fatalf("Wait() = %q, want %q", got, "async final")
	}
}

func TestServiceWaitReturnsFailure(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-789")
	store, err := runstore.Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.WriteStatus(swarmrun.Status{RunID: "run-789", State: swarmrun.RunStateFailed, UpdatedAt: time.Now().UTC(), Error: "boom"}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}

	svc := NewServiceWithPollInterval(5 * time.Millisecond)
	if _, err := svc.Wait(context.Background(), runDir); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Wait() error = %v, want boom failure", err)
	}
}

func TestInspectIsReadOnly(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-900")
	store, err := runstore.Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.WriteStatus(swarmrun.Status{RunID: "run-900", State: swarmrun.RunStateCompleted}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	before, err := os.Stat(filepath.Join(runDir, "status.json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	svc := NewServiceWithPollInterval(5 * time.Millisecond)
	if _, err := svc.Inspect(runDir); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	after, err := os.Stat(filepath.Join(runDir, "status.json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("Inspect() mutated status.json modtime: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}
