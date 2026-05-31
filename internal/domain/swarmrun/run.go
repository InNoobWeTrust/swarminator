package swarmrun

import "time"

type RunState string

const (
	RunStatePending   RunState = "pending"
	RunStateRunning   RunState = "running"
	RunStateCompleted RunState = "completed"
	RunStateFailed    RunState = "failed"
)

type Status struct {
	RunID        string     `json:"run_id"`
	RunDir       string     `json:"run_dir,omitempty"`
	State        RunState   `json:"state"`
	Orchestrator string     `json:"orchestrator,omitempty"`
	WorkerPID    int        `json:"worker_pid,omitempty"`
	EventsPath   string     `json:"events_path,omitempty"`
	FinalPath    string     `json:"final_path,omitempty"`
	RequestPath  string     `json:"request_path,omitempty"`
	MemoryPath   string     `json:"memory_path,omitempty"`
	NodesDir     string     `json:"nodes_dir,omitempty"`
	ArtifactsDir string     `json:"artifacts_dir,omitempty"`
	LogsDir      string     `json:"logs_dir,omitempty"`
	StartedAt    time.Time  `json:"started_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Error        string     `json:"error,omitempty"`
}

type EventType string

const (
	EventRunStarted              EventType = "run_started"
	EventRunPending              EventType = "run_pending"
	EventAsyncWorkerStarted      EventType = "async_worker_started"
	EventOrchestratorTurnStarted EventType = "orchestrator_turn_started"
	EventNodeStarted             EventType = "node_started"
	EventNodeCompleted           EventType = "node_completed"
	EventCompactionStarted       EventType = "compaction_started"
	EventCompactionCompleted     EventType = "compaction_completed"
	EventRunCompleted            EventType = "run_completed"
	EventRunFailed               EventType = "run_failed"
)

type Event struct {
	Type   EventType `json:"type"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}
