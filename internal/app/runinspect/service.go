package runinspect

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"swarminator/internal/domain/swarmrun"
	"swarminator/internal/infra/runstore"
)

type InspectResult struct {
	RunDir         string          `json:"run_dir"`
	Status         swarmrun.Status `json:"status"`
	StatusPath     string          `json:"status_path"`
	EventsPath     string          `json:"events_path"`
	FinalPath      string          `json:"final_path"`
	RequestPath    string          `json:"request_path"`
	TranscriptPath string          `json:"transcript_path"`
	MemoryPath     string          `json:"memory_path"`
	ArtifactsDir   string          `json:"artifacts_dir"`
	NodesDir       string          `json:"nodes_dir"`
	LogsDir        string          `json:"logs_dir"`
}

type Service struct {
	pollInterval time.Duration
}

func NewService() *Service {
	return NewServiceWithPollInterval(250 * time.Millisecond)
}

func NewServiceWithPollInterval(pollInterval time.Duration) *Service {
	if pollInterval <= 0 {
		pollInterval = 250 * time.Millisecond
	}
	return &Service{pollInterval: pollInterval}
}

func (s *Service) Final(runDir string) (string, error) {
	store, err := runstore.OpenExisting(runDir)
	if err != nil {
		return "", err
	}
	return store.ReadFinal()
}

func (s *Service) Inspect(runDir string) (InspectResult, error) {
	store, err := runstore.OpenExisting(runDir)
	if err != nil {
		return InspectResult{}, err
	}
	status, err := store.ReadStatus()
	if err != nil {
		return InspectResult{}, err
	}
	paths := store.Paths()
	return InspectResult{
		RunDir:         paths.RunDir,
		Status:         status,
		StatusPath:     paths.StatusPath,
		EventsPath:     paths.EventsPath,
		FinalPath:      paths.FinalPath,
		RequestPath:    paths.InputPath,
		TranscriptPath: paths.TranscriptPath,
		MemoryPath:     paths.MemoryPath,
		ArtifactsDir:   paths.ArtifactsDir,
		NodesDir:       paths.NodesDir,
		LogsDir:        paths.LogsDir,
	}, nil
}

func (s *Service) InspectJSON(runDir string) ([]byte, error) {
	result, err := s.Inspect(runDir)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(result, "", "  ")
}

func (s *Service) Tail(runDir string) (string, error) {
	store, err := runstore.OpenExisting(runDir)
	if err != nil {
		return "", err
	}
	return store.ReadEventsRaw()
}

func (s *Service) Wait(ctx context.Context, runDir string) (string, error) {
	for {
		store, err := runstore.OpenExisting(runDir)
		if err != nil {
			return "", err
		}
		status, err := store.ReadStatus()
		if err != nil {
			return "", err
		}
		switch status.State {
		case swarmrun.RunStateCompleted:
			return store.ReadFinal()
		case swarmrun.RunStateFailed:
			return "", fmt.Errorf("run %s failed: %s", status.RunID, status.Error)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(s.pollInterval):
		}
	}
}
