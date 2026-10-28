package task

import "time"

// Task represents a Proxmox task
type Task struct {
	UPID       string
	Node       string
	PID        int
	PStart     int64
	StartTime  time.Time
	Type       string
	ID         string
	User       string
	Status     string
	ExitStatus string

	// Progress tracking
	Progress float64
	Saved    string

	// Timestamps
	StartedAt time.Time
	EndedAt   time.Time
	Duration  time.Duration
}

// TaskLog represents task log entry
type TaskLog struct {
	LineNumber int
	Text       string
}

// TaskFilter for filtering tasks
type TaskFilter struct {
	Node    string
	Running bool
	Errors  bool
	Source  string // vmid, ctid, etc.
	TypeID  string // qmrestore, vzdump, etc.
	User    string
	Since   time.Time
	Until   time.Time
	Limit   int
}

// TaskStatus represents possible task statuses
const (
	TaskStatusRunning = "running"
	TaskStatusStopped = "stopped"
)

// TaskExitStatus represents possible exit statuses
const (
	ExitStatusOK    = "OK"
	ExitStatusError = "ERROR"
)
