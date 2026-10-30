package broadcast

import (
	"context"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

// TargetType defines the recipient category for a broadcast operation.
type TargetType string

const (
	TargetUsers  TargetType = "users"
	TargetGroups TargetType = "groups"
	TargetAll    TargetType = "all"
)

// Mode defines whether the message is copied without an author header or forwarded with an author header.
type Mode string

const (
	ModeCopy    Mode = "copy"    // Forwards with HideAuthor: true (copy message)
	ModeForward Mode = "forward" // Forwards with HideAuthor: false (forward with attribution)
)

// Status represents the current operational state of the broadcast job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCancelled Status = "cancelled"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Options holds parameters controlling a broadcast operation.
type Options struct {
	Target      TargetType
	Mode        Mode
	Skip        int64
	Concurrency int
	AdminID     int64
}

// Stats represents thread-safe real-time and completed broadcast metrics.
type Stats struct {
	TotalRecipients int64
	Processed       int64
	Successful      int64
	Failed          int64
	Blocked         int64
	Deactivated     int64
	StartTime       time.Time
	EndTime         time.Time
	Status          Status
	IsFloodWait     bool
	FloodSec        int
	Speed           float64 // msgs per second
	Target          TargetType
	Mode            Mode
}

// Service defines the contract for high-performance concurrent message broadcasting.
type Service interface {
	StartBroadcast(ctx context.Context, sourceMsg *telegram.NewMessage, progressMsg *telegram.NewMessage, opts Options) error
	CancelBroadcast() bool
	IsBroadcasting() bool
	GetStats() (Stats, bool)
}
