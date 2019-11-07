package tester

import "time"

// TBState represents the completion state of a `testing.TB`.
type TBState int

const (
	_ = iota
	// TBPassed represents a passed test.
	TBPassed
	// TBFailed represents a failed test.
	TBFailed
	// TBSkipped represents a skipped test.
	TBSkipped
)

func (s TBState) String() string {
	switch s {
	case TBPassed:
		return "passed"
	case TBFailed:
		return "failed"
	case TBSkipped:
		return "skipped"
	}
	return ""
}

// TBCommon is the representation of the common fields of a testing.TB.
type TBCommon struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	FinishTime time.Time `json:"duration"`
	State      TBState   `json:"state"`
	Output     []byte    `json:"output"`
}

// Duration returns the run duration the Test.
func (c *TBCommon) Duration() time.Duration {
	return c.FinishTime.Sub(c.StartTime)
}

func (c *TBCommon) OutputString() string {
	return string(c.Output)
}

// Test is the representation of a `testing.T`.
type Test struct {
	TBCommon

	SubTests []*Test `json:"sub_tests,omitempty"`
}

// Benchmark is the representation of a `testing.B`.
type Benchmark struct {
	TBCommon

	SubBenchmarks []*Benchmark `json:"sub_benchmarks,omitempty"`
}
