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

// RunMeta is additional metadata associated with the run.
type RunMeta struct {
	RunnerHostname string `json:"runner_hostname"`
}

func (r *Run) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}

// Package represents a go package that can be tested or benchmarked.
type Package struct {
	Name      string        `json:"name"`
	Path      string        `json:"path"`
	SHA256Sum string        `json:"sha256sum"`
	RunDelay  time.Duration `json:"run_delay"`
	Options   []Option      `json:"options"`
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

type RunSummary struct {
	Time           time.Time
	Duration       time.Duration
	PackageSummary map[string]*PackageSummary
}

func (s *RunSummary) NumRuns() int {
	var total int
	for _, pkgSummary := range s.PackageSummary {
		total += len(pkgSummary.RunIDs)
	}
	return total
}

func (s *RunSummary) NumErrorRuns() int {
	var total int
	for _, pkgSummary := range s.PackageSummary {
		total += len(pkgSummary.ErrorRunIDs)
	}
	return total
}

func (s *RunSummary) NumPassedTests() int {
	var total int
	for _, pkgSummary := range s.PackageSummary {
		total += pkgSummary.NumPassedTests()
	}
	return total
}

func (s *RunSummary) PercentPassedTests() float64 {
	return float64(s.NumPassedTests()) / float64(s.NumTotalTests())
}

func (s *RunSummary) NumFailedTests() int {
	var total int
	for _, pkgSummary := range s.PackageSummary {
		total += pkgSummary.NumFailedTests()
	}
	return total
}

func (s *RunSummary) PercentFailedTests() float64 {
	return float64(s.NumFailedTests()) / float64(s.NumTotalTests())
}

func (s *RunSummary) NumSkippedTests() int {
	var total int
	for _, pkgSummary := range s.PackageSummary {
		total += pkgSummary.NumSkippedTests()
	}
	return total
}

func (s *RunSummary) PercentSkippedTests() float64 {
	return float64(s.NumSkippedTests()) / float64(s.NumTotalTests())
}

func (s *RunSummary) NumTotalTests() int {
	var (
		passed  int
		failed  int
		skipped int
	)
	for _, pkgSummary := range s.PackageSummary {
		passed += pkgSummary.NumPassedTests()
		failed += pkgSummary.NumFailedTests()
		skipped += pkgSummary.NumSkippedTests()
	}
	return passed + failed + skipped
}

type PackageSummary struct {
	Package      string
	RunIDs       []uuid.UUID
	ErrorRunIDs  []uuid.UUID
	PassedTests  map[string][]uuid.UUID
	FailedTests  map[string][]uuid.UUID
	SkippedTests map[string][]uuid.UUID
}

func (s *PackageSummary) NumPassedTests() int {
	var total int
	for _, tests := range s.PassedTests {
		total += len(tests)
	}
	return total
}

func (s *PackageSummary) PercentPassedTests() float64 {
	return float64(s.NumPassedTests()) / float64(s.NumTotalTests())
}

func (s *PackageSummary) NumFailedTests() int {
	var total int
	for _, tests := range s.FailedTests {
		total += len(tests)
	}
	return total
}

func (s *PackageSummary) PercentFailedTests() float64 {
	return float64(s.NumFailedTests()) / float64(s.NumTotalTests())
}

func (s *PackageSummary) NumSkippedTests() int {
	var total int
	for _, tests := range s.SkippedTests {
		total += len(tests)
	}
	return total
}

func (s *PackageSummary) PercentSkippedTests() float64 {
	return float64(s.NumSkippedTests()) / float64(s.NumTotalTests())
}

func (s *PackageSummary) NumTotalTests() int {
	return s.NumPassedTests() + s.NumFailedTests() + s.NumSkippedTests()
}
