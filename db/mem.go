package db

import (
	"errors"
	"sync"

	"github.com/nanzhong/tester"
)

const maxInMemResults = 200

var ErrNotFound = errors.New("could not find result")

type MemDB struct {
	mu         sync.RWMutex
	Tests      []*tester.Test
	Benchmarks []*tester.Benchmark
}

func (m *MemDB) AddTest(test *tester.Test) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Tests = append([]*tester.Test{test}, m.Tests...)

	if len(m.Tests) > maxInMemResults {
		m.Tests = m.Tests[:100]
	}
}

func (m *MemDB) GetTest(id string) (*tester.Test, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, test := range m.Tests {
		if test.ID == id {
			return test, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemDB) ListTests() []*tester.Test {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tests := make([]*tester.Test, 0, len(m.Tests))
	for _, t := range m.Tests {
		tests = append(tests, t)
	}
	return tests
}

func (m *MemDB) UpdateTest(id string, updateFn func(test *tester.Test)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for idx, test := range m.Tests {
		if id == test.ID {
			updateFn(test)
			m.Tests[idx] = test
			return nil
		}
	}
	return ErrNotFound
}
