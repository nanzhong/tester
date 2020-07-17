package tester

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TBState represents the completion state of a `testing.TB`.
type TBState string

const (
	// TBStatePassed represents a passed test.
	TBStatePassed TBState = "passed"
	// TBFailed represents a failed test.
	TBStateFailed TBState = "failed"
	// TBSkipped represents a skipped test.
	TBStateSkipped TBState = "skipped"
)

// TB is the representation of the common fields of a testing.TB.
type TB struct {
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	State      TBState   `json:"state"`
}

// Duration returns the run duration the Test.
func (c *TB) Duration() time.Duration {
	return c.FinishedAt.Sub(c.StartedAt)
}

type TBLog struct {
	Time   time.Time `json:"time"`
	Name   string    `json:"name"`
	Output []byte    `json:"output"`
}

// T represents the results of a `testing.T`.
type T struct {
	TB

	SubTs []*T `json:"sub_ts"`
}

// Test is a run of a `testing.T`.
type Test struct {
	ID      uuid.UUID `json:"id"`
	Package string    `json:"package"`
	RunID   uuid.UUID `json:"run_id"`

	Result *T      `json:"result"`
	Logs   []TBLog `json:"logs"`
}

// Run is the representation of a pending test or benchmark that has not
// completed.
type Run struct {
	ID         uuid.UUID `json:"id"`
	Package    string    `json:"package"`
	Args       []string  `json:"args"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Tests      []*Test   `json:"tests"`
	Error      string    `json:"error"`
}

func (r *Run) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}

// Package represents a go package that can be tested or benchmarked.
type Package struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	RunDelay time.Duration `json:"run_delay"`
	Options  []Option      `json:"options"`
}

// Option represents an option for how a package can be run.
type Option struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Default     string `json:"default"`
}

// String returns a string representation of the option.
func (o *Option) String() string {
	return fmt.Sprintf("-%s=%s", o.Name, o.Value)
}
