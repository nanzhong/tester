package db

import (
	"context"
	"errors"

	"github.com/nanzhong/tester"
)

// ErrNotFound is returned when the requested item could not be found.
var ErrNotFound = errors.New("not found")

// DB is the interface for a persistence store implementation.
type DB interface {
	AddTest(ctx context.Context, test *tester.Test) error
	GetTest(ctx context.Context, id string) (*tester.Test, error)
	ListTests(ctx context.Context, limit int) ([]*tester.Test, error)

	Archive(ctx context.Context) error

	EnqueueRun(ctx context.Context, run *tester.Run) error
	StartRun(ctx context.Context, id string) error
	ResetRun(ctx context.Context, id string) error
	DeleteRun(ctx context.Context, id string) error
	CompleteRun(ctx context.Context, id string, testIDs []string) error
	FailRun(ctx context.Context, id string, error string) error
	ListPendingRuns(ctx context.Context) ([]*tester.Run, error)
	ListFinishedRuns(ctx context.Context, limit int) ([]*tester.Run, error)
	GetRun(ctx context.Context, id string) (*tester.Run, error)
}
