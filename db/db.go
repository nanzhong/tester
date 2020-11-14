package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/nanzhong/tester"
)

// ErrNotFound is returned when the requested item could not be found.
var ErrNotFound = errors.New("not found")

//go:generate mockgen -package=db -destination=db_mock.go . DB

// DB is the interface for a persistence store implementation.
type DB interface {
	Init(ctx context.Context) error

	AddTest(ctx context.Context, test *tester.Test) error
	GetTest(ctx context.Context, id uuid.UUID) (*tester.Test, error)
	ListTests(ctx context.Context, limit int) ([]*tester.Test, error)
	ListTestsForPackage(ctx context.Context, pkg string, limit int) ([]*tester.Test, error)
	ListTestsForPackageInRange(ctx context.Context, pkg string, begin, end time.Time) ([]*tester.Test, error)

	EnqueueRun(ctx context.Context, run *tester.Run) error
	StartRun(ctx context.Context, id uuid.UUID, runner string) error
	ResetRun(ctx context.Context, id uuid.UUID) error
	DeleteRun(ctx context.Context, id uuid.UUID) error
	CompleteRun(ctx context.Context, id uuid.UUID) error
	FailRun(ctx context.Context, id uuid.UUID, error string) error
	GetRun(ctx context.Context, id uuid.UUID) (*tester.Run, error)
	ListPendingRuns(ctx context.Context) ([]*tester.Run, error)
	ListFinishedRuns(ctx context.Context, limit int) ([]*tester.Run, error)
	ListRunsForPackage(ctx context.Context, pkg string, limit int) ([]*tester.Run, error)
	ListRunSummariesInRange(ctx context.Context, begin, end time.Time, window time.Duration) ([]*tester.RunSummary, error)
}
