package db

import (
	"context"
	"sync"
	"time"

	"github.com/nanzhong/tester"
)

const (
	maxInMemResults = 200
	maxInMemRuns    = 100
)

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

func (m *MemDB) ListTests(_ context.Context, limit int) ([]*tester.Test, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tests []*tester.Test
	for _, t := range m.TestResults {
		tests = append(tests, t)
		if len(tests) == limit {
			break
		}
	}
	return tests, nil
}

func (m *MemDB) Archive(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.TestResults) > maxInMemResults {
		m.TestResults = m.TestResults[:maxInMemResults]
	}

	if len(m.Runs) > maxInMemRuns {
		m.Runs = m.Runs[:maxInMemRuns]
	}

	return nil
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

	var toDelete *int
	for i, run := range m.Runs {
		if run.ID == id {
			toDelete = &i
			break
		}
	}

	if toDelete == nil {
		return ErrNotFound
	}

	m.Runs = append(m.Runs[:*toDelete], m.Runs[*toDelete+1:]...)

	return nil
}

func (m *MemDB) CompleteRun(ctx context.Context, id string, testIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var run *tester.Run
	for _, r := range m.Runs {
		if r.ID == id {
			run = r
		}
	}
	if run == nil {
		return ErrNotFound
	}

	run.FinishedAt = time.Now()
	testIDMap := make(map[string]struct{})
	for _, id := range testIDs {
		testIDMap[id] = struct{}{}
	}

	for _, test := range m.TestResults {
		if _, ok := testIDMap[test.ID]; ok {
			run.Tests = append(run.Tests, test)
		}
	}

	return nil
}

func (m *MemDB) ListPendingRuns(ctx context.Context) ([]*tester.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var runs []*tester.Run
	for _, run := range m.Runs {
		if !run.FinishedAt.IsZero() {
			continue
		}
		runs = append(runs, run)
	}

	return runs, nil
}

func (m *MemDB) ListFinishedRuns(ctx context.Context, limit int) ([]*tester.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var runs []*tester.Run
	for _, run := range m.Runs {
		if run.FinishedAt.IsZero() {
			continue
		}
		runs = append(runs, run)
		if len(runs) == limit {
			break
		}
	}

	return runs, nil
}

func (m *MemDB) GetRun(ctx context.Context, id string) (*tester.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, run := range m.Runs {
		if run.ID == id {
			return run, nil
		}
	}

	return nil, ErrNotFound
}
