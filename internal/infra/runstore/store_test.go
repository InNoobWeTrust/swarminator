package runstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swarminator/internal/domain/swarmrun"
)

func TestCreateAndWriteRunFiles(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-123")
	store, err := Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.WriteStatus(swarmrun.Status{
		RunID: "run-123",
		State: swarmrun.RunStateCompleted,
	}); err != nil {
		t.Fatalf("WriteStatus() error = %v", err)
	}
	if err := store.AppendEvent(swarmrun.Event{
		Type: swarmrun.EventRunStarted,
		At:   time.Date(2026, time.May, 24, 18, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if err := store.WriteFinal("final answer"); err != nil {
		t.Fatalf("WriteFinal() error = %v", err)
	}
	if err := store.WriteInput("debug this"); err != nil {
		t.Fatalf("WriteInput() error = %v", err)
	}
	if err := store.WriteMemoryCurrent("summary"); err != nil {
		t.Fatalf("WriteMemoryCurrent() error = %v", err)
	}
	if err := store.AppendTranscript(swarmrun.Message{Role: swarmrun.RoleUser, Content: "hello"}); err != nil {
		t.Fatalf("AppendTranscript() error = %v", err)
	}
	if _, err := store.WriteNodeArtifact("reviewer-pass", "raw node output"); err != nil {
		t.Fatalf("WriteNodeArtifact() error = %v", err)
	}

	info, err := os.Stat(runDir)
	if err != nil {
		t.Fatalf("Stat(runDir) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("run dir mode = %o, want %o", got, 0o700)
	}

	for _, path := range []string{
		filepath.Join(runDir, "status.json"),
		filepath.Join(runDir, "events.jsonl"),
		filepath.Join(runDir, "final.md"),
		filepath.Join(runDir, "input.txt"),
		filepath.Join(runDir, "transcript.private.jsonl"),
		filepath.Join(runDir, "memory", "current.md"),
	} {
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if got := fileInfo.Mode().Perm(); got != 0o600 {
			t.Fatalf("mode for %q = %o, want %o", path, got, 0o600)
		}
	}

	finalBytes, err := os.ReadFile(filepath.Join(runDir, "final.md"))
	if err != nil {
		t.Fatalf("ReadFile(final.md) error = %v", err)
	}
	if strings.TrimSpace(string(finalBytes)) != "final answer" {
		t.Fatalf("final.md = %q, want %q", string(finalBytes), "final answer")
	}

	for _, dir := range []string{
		filepath.Join(runDir, "memory"),
		filepath.Join(runDir, "artifacts"),
		filepath.Join(runDir, "nodes"),
		filepath.Join(runDir, "logs"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %q to exist, err=%v info=%v", dir, err, info)
		}
	}
}

func TestCreateRejectsEventSinkOutsideRunDir(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-123")
	if _, err := Create(runDir, "file:///tmp/elsewhere/events.jsonl"); err == nil {
		t.Fatal("Create() error = nil, want event sink validation failure")
	}
}

func TestAcquireWriterLockPreventsConcurrentWriters(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run-lock")
	store, err := Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.AcquireWriterLock(); err != nil {
		t.Fatalf("AcquireWriterLock() error = %v", err)
	}
	defer store.ReleaseWriterLock()

	second, err := Create(runDir, "")
	if err != nil {
		t.Fatalf("Create() second error = %v", err)
	}
	if err := second.AcquireWriterLock(); err == nil {
		t.Fatal("AcquireWriterLock() error = nil, want lock failure")
	}
}
