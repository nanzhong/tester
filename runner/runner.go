package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
	"github.com/prometheus/client_golang/prometheus"
)

// TBRunConfig is the configuration for a test/benchmark that the Runner should
// schedule.
type TBRunConfig struct {
	// Path to the test binary.
	Path string `json:"path"`
	// Timeout to configure for the test.
	Timeout time.Duration `json:"timeout"`
}

type tbRunner struct {
	config TBRunConfig
	cancel context.CancelFunc
}

type options struct {
	db *db.MemDB
}

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

// WithDB allows configuring a TESTER.
func WithDB(db *db.MemDB) Option {
	return func(opts *options) {
		opts.db = db
	}
}

// Runner is the implementation of the test runner.
type Runner struct {
	runners []*tbRunner
	db      *db.MemDB
	stop    chan struct{}
	wg      sync.WaitGroup
}

func New(configs []TBRunConfig, opts ...Option) *Runner {
	defOpts := &options{
		db: &db.MemDB{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	runner := &Runner{
		db:   defOpts.db,
		stop: make(chan struct{}),
	}

	for _, config := range configs {
		runner.runners = append(runner.runners, &tbRunner{
			config: config,
		})
	}

	return runner
}

func (r *Runner) Run() {
	log.Printf("runner configured with %d packages\n", len(r.runners))
	log.Println("starting runner...")

	for _, runner := range r.runners {
		runner := runner
		r.wg.Add(1)
		go func() {
			for {
				select {
				case <-r.stop:
					r.wg.Done()
					return
				default:
					r.runTB(runner)
				}
			}
		}()
	}
	r.wg.Wait()
	log.Println("runner finished")
}

func (r *Runner) Stop(ctx context.Context) error {
	log.Println("stopping runner...")
	close(r.stop)
	for _, runner := range r.runners {
		runner.cancel()
	}

	c := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		log.Println("runner stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stopping runner: %w", ctx.Err())
	}
}

type testEvent struct {
	Time   time.Time  `json:"time"`
	Action string     `json:"Action"`
	Test   string     `json:"Test"`
	Output *textBytes `json:"Output"`
}

func (e *testEvent) TopLevel() bool {
	return !strings.Contains(e.Test, "/")
}

func (e *testEvent) ParentTest() string {
	parts := strings.Split(e.Test, "/")
	return strings.Join(parts[:len(parts)-1], "/")
}

func (e *testEvent) ParentTests() []string {
	var (
		parents []string
		name    string
	)
	parts := strings.Split(e.Test, "/")
	for _, part := range parts {
		name = name + part
		parents = append(parents, name)
		name = name + "/"
	}
	return parents
}

// https://github.com/golang/go/blob/master/src/cmd/internal/test2json/test2json.go#L44
type textBytes []byte

func (b *textBytes) UnmarshalText(text []byte) error {
	*b = text
	return nil
}

func (b textBytes) Bytes() []byte {
	return []byte(b)
}

func (r *Runner) runTB(runner *tbRunner) error {
	log.Printf("starting test run for %s\n", runner.config.Path)

	var output bytes.Buffer

	runArgs := []string{
		"tool",
		"test2json",
		"-t",
		runner.config.Path,
		"-test.v",
	}
	if runner.config.Timeout != 0 {
		runArgs = append(runArgs, fmt.Sprintf("-test.timeout=%s", runner.config.Timeout.String()))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.cancel = cancel

	cmd := exec.CommandContext(ctx, "go", runArgs...)
	cmd.Stdout = &output
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("running test/benchmark: %w", err)
	}

	testResults := make(map[string]*tester.Test)
	events := bytes.Split(bytes.Trim(output.Bytes(), " \n"), []byte("\n"))
	for _, eventData := range events {
		var event testEvent
		err := json.Unmarshal(eventData, &event)
		if err != nil {
			return fmt.Errorf("parsing test event: %w", err)
		}

		// TODO revisit when adding support for benchmarks
		if event.Test == "" {
			continue
		}

		switch event.Action {
		case "run":
			test := &tester.Test{
				TBCommon: tester.TBCommon{
					ID:        uuid.New().String(),
					Name:      event.Test,
					StartTime: event.Time,
				},
			}
			testResults[event.Test] = test

			if !event.TopLevel() {
				parentTest, ok := testResults[event.ParentTest()]
				if !ok {
					return fmt.Errorf("missing parent test %s for sub test %s", event.ParentTest(), event.Test)
				}
				parentTest.SubTests = append(parentTest.SubTests, test)
			}
		case "pass", "fail", "skipped":
			test, ok := testResults[event.Test]
			if !ok {
				return fmt.Errorf("missing test: %s", event.Test)
			}
			test.FinishTime = event.Time
			switch event.Action {
			case "pass":
				test.State = tester.TBPassed
			case "fail":
				test.State = tester.TBFailed
			case "skip":
				test.State = tester.TBSkipped
			}

			if event.TopLevel() {
				r.db.AddTest(test)
				runLabels := prometheus.Labels{
					"name":  test.Name,
					"state": test.State.String(),
				}
				RunDurationMetric.With(runLabels).Observe(test.FinishTime.Sub(test.StartTime).Seconds())
				RunLastMetric.With(runLabels).Set(float64(test.StartTime.Unix()))
			}
		case "output":
			test, ok := testResults[event.Test]
			if !ok {
				return fmt.Errorf("missing test: %s", event.Test)
			}
			test.Output = append(test.Output, event.Output.Bytes()...)

			for _, testName := range event.ParentTests() {
				test, ok := testResults[testName]
				if !ok {
					return fmt.Errorf("missing parent test %s for sub test %s", testName, event.Test)
				}
				test.Output = append(test.Output, event.Output.Bytes()...)
			}
		}
	}
	return nil
}
