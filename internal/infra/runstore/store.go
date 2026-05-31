package runstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"swarminator/internal/domain/swarmrun"
)

type Store struct {
	runDir         string
	statusPath     string
	eventsPath     string
	finalPath      string
	inputPath      string
	transcriptPath string
	memoryDir      string
	memoryPath     string
	artifactsDir   string
	nodesDir       string
	logsDir        string
	lockPath       string
	lockFile       *os.File
}

type Paths struct {
	RunDir         string
	StatusPath     string
	EventsPath     string
	FinalPath      string
	InputPath      string
	TranscriptPath string
	MemoryPath     string
	ArtifactsDir   string
	NodesDir       string
	LogsDir        string
}

func Create(runDir, eventSink string) (*Store, error) {
	store, err := newStore(runDir, eventSink)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(store.runDir, 0o700); err != nil {
		return nil, fmt.Errorf("create run dir %q: %w", store.runDir, err)
	}
	if err := os.Chmod(store.runDir, 0o700); err != nil {
		return nil, fmt.Errorf("chmod run dir %q: %w", store.runDir, err)
	}
	for _, dir := range []string{store.memoryDir, store.artifactsDir, store.nodesDir, store.logsDir, filepath.Dir(store.eventsPath)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create directory %q: %w", dir, err)
		}
	}
	return store, nil
}

func OpenExisting(runDir string) (*Store, error) {
	store, err := newStore(runDir, "")
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(store.runDir)
	if err != nil {
		return nil, fmt.Errorf("open run dir %q: %w", store.runDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("run dir %q is not a directory", store.runDir)
	}
	status, err := store.ReadStatus()
	if err == nil && strings.TrimSpace(status.EventsPath) != "" {
		store.eventsPath = status.EventsPath
	}
	return store, nil
}

func newStore(runDir, eventSink string) (*Store, error) {
	trimmedRunDir := strings.TrimSpace(runDir)
	if trimmedRunDir == "" {
		return nil, errors.New("run dir is required")
	}
	absRunDir, err := filepath.Abs(trimmedRunDir)
	if err != nil {
		return nil, fmt.Errorf("resolve run dir %q: %w", runDir, err)
	}
	eventsPath, err := resolveEventsPath(absRunDir, eventSink)
	if err != nil {
		return nil, err
	}
	memoryDir := filepath.Join(absRunDir, "memory")
	return &Store{
		runDir:         absRunDir,
		statusPath:     filepath.Join(absRunDir, "status.json"),
		eventsPath:     eventsPath,
		finalPath:      filepath.Join(absRunDir, "final.md"),
		inputPath:      filepath.Join(absRunDir, "input.txt"),
		transcriptPath: filepath.Join(absRunDir, "transcript.private.jsonl"),
		memoryDir:      memoryDir,
		memoryPath:     filepath.Join(memoryDir, "current.md"),
		artifactsDir:   filepath.Join(absRunDir, "artifacts"),
		nodesDir:       filepath.Join(absRunDir, "nodes"),
		logsDir:        filepath.Join(absRunDir, "logs"),
		lockPath:       filepath.Join(absRunDir, ".writer.lock"),
	}, nil
}

func (s *Store) Paths() Paths {
	return Paths{
		RunDir:         s.runDir,
		StatusPath:     s.statusPath,
		EventsPath:     s.eventsPath,
		FinalPath:      s.finalPath,
		InputPath:      s.inputPath,
		TranscriptPath: s.transcriptPath,
		MemoryPath:     s.memoryPath,
		ArtifactsDir:   s.artifactsDir,
		NodesDir:       s.nodesDir,
		LogsDir:        s.logsDir,
	}
}

func (s *Store) AcquireWriterLock() error {
	if s.lockFile != nil {
		return nil
	}
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("run dir %q is already locked for writing", s.runDir)
		}
		return fmt.Errorf("acquire writer lock %q: %w", s.lockPath, err)
	}
	if _, err := lockFile.WriteString(fmt.Sprintf("pid=%d\n", os.Getpid())); err != nil {
		lockFile.Close()
		os.Remove(s.lockPath)
		return fmt.Errorf("write writer lock %q: %w", s.lockPath, err)
	}
	s.lockFile = lockFile
	return nil
}

func (s *Store) ReleaseWriterLock() error {
	if s.lockFile == nil {
		return nil
	}
	if err := s.lockFile.Close(); err != nil {
		return fmt.Errorf("close writer lock %q: %w", s.lockPath, err)
	}
	s.lockFile = nil
	if err := os.Remove(s.lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove writer lock %q: %w", s.lockPath, err)
	}
	return nil
}

func (s *Store) WriteStatus(status swarmrun.Status) error {
	status.RunDir = s.runDir
	status.EventsPath = s.eventsPath
	status.FinalPath = s.finalPath
	status.RequestPath = s.inputPath
	status.MemoryPath = s.memoryPath
	status.NodesDir = s.nodesDir
	status.ArtifactsDir = s.artifactsDir
	status.LogsDir = s.logsDir
	return atomicWriteJSON(s.statusPath, status)
}

func (s *Store) ReadStatus() (swarmrun.Status, error) {
	data, err := os.ReadFile(s.statusPath)
	if err != nil {
		return swarmrun.Status{}, fmt.Errorf("read status %q: %w", s.statusPath, err)
	}
	var status swarmrun.Status
	if err := json.Unmarshal(data, &status); err != nil {
		return swarmrun.Status{}, fmt.Errorf("parse status %q: %w", s.statusPath, err)
	}
	return status, nil
}

func (s *Store) AppendEvent(event swarmrun.Event) error {
	return appendJSONL(s.eventsPath, event)
}

func (s *Store) ReadEventsRaw() (string, error) {
	data, err := os.ReadFile(s.eventsPath)
	if err != nil {
		return "", fmt.Errorf("read events %q: %w", s.eventsPath, err)
	}
	return string(data), nil
}

func (s *Store) WriteFinal(final string) error {
	return atomicWriteText(s.finalPath, strings.TrimSpace(final)+"\n")
}

func (s *Store) ReadFinal() (string, error) {
	data, err := os.ReadFile(s.finalPath)
	if err != nil {
		return "", fmt.Errorf("read final %q: %w", s.finalPath, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *Store) WriteInput(input string) error {
	return atomicWriteText(s.inputPath, strings.TrimSpace(input)+"\n")
}

func (s *Store) ReadInput() (string, error) {
	data, err := os.ReadFile(s.inputPath)
	if err != nil {
		return "", fmt.Errorf("read input %q: %w", s.inputPath, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *Store) AppendTranscript(message swarmrun.Message) error {
	entry := struct {
		At          time.Time            `json:"at"`
		Role        swarmrun.Role        `json:"role"`
		Kind        swarmrun.MessageKind `json:"kind,omitempty"`
		Content     string               `json:"content"`
		Name        string               `json:"name,omitempty"`
		ArtifactRef string               `json:"artifact_ref,omitempty"`
		ModelID     string               `json:"model_id,omitempty"`
		PersonaID   string               `json:"persona_id,omitempty"`
	}{
		At:          time.Now().UTC(),
		Role:        message.Role,
		Kind:        message.Kind,
		Content:     strings.TrimSpace(message.Content),
		Name:        strings.TrimSpace(message.Name),
		ArtifactRef: strings.TrimSpace(message.ArtifactRef),
		ModelID:     strings.TrimSpace(message.ModelID),
		PersonaID:   strings.TrimSpace(message.PersonaID),
	}
	return appendJSONL(s.transcriptPath, entry)
}

func (s *Store) WriteMemoryCurrent(summary string) error {
	return atomicWriteText(s.memoryPath, strings.TrimSpace(summary)+"\n")
}

func (s *Store) ReadMemoryCurrent() (string, error) {
	data, err := os.ReadFile(s.memoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read memory %q: %w", s.memoryPath, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *Store) WriteMemorySnapshot(summary string) (string, error) {
	entries, err := os.ReadDir(s.memoryDir)
	if err != nil {
		return "", fmt.Errorf("read memory directory %q: %w", s.memoryDir, err)
	}
	indexes := make([]int, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "snapshot-") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		index := atoi(strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "snapshot-"), ".md"))
		if index > 0 {
			indexes = append(indexes, index)
		}
	}
	sort.Ints(indexes)
	next := 1
	if len(indexes) > 0 {
		next = indexes[len(indexes)-1] + 1
	}
	path := filepath.Join(s.memoryDir, fmt.Sprintf("snapshot-%03d.md", next))
	if err := atomicWriteText(path, strings.TrimSpace(summary)+"\n"); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) WriteNodeArtifact(name, content string) (string, error) {
	base := sanitizeFileName(name)
	if base == "" {
		base = "node-output"
	}
	rel := filepath.Join("nodes", base+".md")
	abs := filepath.Join(s.runDir, rel)
	if _, err := os.Stat(abs); err == nil {
		rel = filepath.Join("nodes", fmt.Sprintf("%s-%d.md", base, time.Now().UTC().UnixNano()))
		abs = filepath.Join(s.runDir, rel)
	}
	if err := atomicWriteText(abs, strings.TrimSpace(content)+"\n"); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (s *Store) LogFilePath(name string) string {
	return filepath.Join(s.logsDir, sanitizeFileName(name)+".log")
}

func resolveEventsPath(runDir, eventSink string) (string, error) {
	if strings.TrimSpace(eventSink) == "" {
		return filepath.Join(runDir, "events.jsonl"), nil
	}
	parsed, err := url.Parse(eventSink)
	if err != nil {
		return "", fmt.Errorf("parse event sink %q: %w", eventSink, err)
	}
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("event sink %q must use file://", eventSink)
	}
	eventPath, err := filepath.Abs(filepath.FromSlash(parsed.Path))
	if err != nil {
		return "", fmt.Errorf("resolve event sink path %q: %w", eventSink, err)
	}
	if !isWithinRoot(runDir, eventPath) {
		return "", fmt.Errorf("event sink %q must stay within run dir %q", eventSink, runDir)
	}
	return eventPath, nil
}

func isWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "")
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "\t", "-")
	clean := replacer.Replace(strings.TrimSpace(name))
	clean = strings.Trim(clean, "-._")
	return clean
}

func atomicWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	data = append(data, '\n')
	return atomicWriteBytes(path, data)
}

func atomicWriteText(path, text string) error {
	return atomicWriteBytes(path, []byte(text))
}

func atomicWriteBytes(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %q: %w", path, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temp file for %q: %w", path, err)
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("chmod temp file for %q: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file for %q: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file for %q: %w", path, err)
	}
	return nil
}

func appendJSONL(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open jsonl file %q: %w", path, err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("append jsonl file %q: %w", path, err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush jsonl file %q: %w", path, err)
	}
	return nil
}

func atoi(value string) int {
	parsed, _ := strconvAtoi(strings.TrimSpace(value))
	return parsed
}

func strconvAtoi(value string) (int, error) {
	var result int
	_, err := fmt.Sscanf(value, "%d", &result)
	return result, err
}
