package db

import (
	"context"
	"sync"
	"time"

	"github.com/nanzhong/tester"
)

const maxInMemResults = 200

type MemDB struct {
	mu          sync.RWMutex
	TestResults []*tester.Test
	Runs        []*tester.Run
}

var _ DB = (*MemDB)(nil)

func (m *MemDB) AddTest(_ context.Context, test *tester.Test) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TestResults = append([]*tester.Test{test}, m.TestResults...)

	if len(m.TestResults) > maxInMemResults {
		m.TestResults = m.TestResults[:maxInMemResults]
	}

	return nil
}

func (m *MemDB) GetTest(_ context.Context, id string) (*tester.Test, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, test := range m.TestResults {
		if test.ID == id {
			return test, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemDB) ListTests(_ context.Context) ([]*tester.Test, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tests := make([]*tester.Test, 0, len(m.TestResults))
	for _, t := range m.TestResults {
		tests = append(tests, t)
	}
	return tests, nil
}

func (m *MemDB) EnqueueRun(ctx context.Context, run *tester.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Runs = append(m.Runs, run)

	return nil
}

func (m *MemDB) StartRun(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, run := range m.Runs {
		if run.ID == id {
			run.StartedAt = time.Now()
			return nil
		}
	}

	return ErrNotFound
}

func (m *MemDB) ResetRun(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, run := range m.Runs {
		if run.ID == id {
			run.StartedAt = time.Time{}
			return nil
		}
	}

	return ErrNotFound
}

func (m *MemDB) DeleteRun(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var runIdx *int
	for i, run := range m.Runs {
		if run.ID == id {
			runIdx = &i
		}
	}
	if runIdx == nil {
		return ErrNotFound
	}

	m.Runs = append(m.Runs[:*runIdx], m.Runs[*runIdx+1:]...)

	return nil
}

func (m *MemDB) ListRuns(ctx context.Context) ([]*tester.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.Runs, nil
}
