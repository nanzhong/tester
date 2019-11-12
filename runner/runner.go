package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	Path string
	// Timeout to configure for the test.
	Timeout time.Duration
	// Arguments are additional arguments to pass to the test binary.
	Arguments []string
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
					err := r.runTB(context.Background(), runner)
					if err != nil {
						log.Printf("error running: %s\n", err)
					}
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

func (r *Runner) runTB(ctx context.Context, runner *tbRunner) error {
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

	cmd := exec.CommandContext(ctx, "go", runArgs...)
	cmd.Stdout = &output
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		// non 0 exit statuses are okay.
		// eg. failed tests will result in exit status 1.
		if _, ok := err.(*exec.ExitError); !ok {
			return fmt.Errorf("running test/benchmark: %w", err)
		}
	}

	eventBytes := bytes.Split(bytes.Trim(output.Bytes(), " \n"), []byte("\n"))
	var events []*testEvent
	for _, eventData := range eventBytes {
		var event testEvent
		err := json.Unmarshal(eventData, &event)
		if err != nil {
			return fmt.Errorf("parsing test event: %w", err)
		}
		events = append(events, &event)
	}

	tests, _, err := processEvents(events)
	if err != nil {
		return fmt.Errorf("processing events: %w", err)
	}

	for _, test := range tests {
		r.db.AddTest(test)
		runLabels := prometheus.Labels{
			"name":  test.Name,
			"state": test.State.String(),
		}
		RunDurationMetric.With(runLabels).Observe(test.FinishTime.Sub(test.StartTime).Seconds())
		RunLastMetric.With(runLabels).Set(float64(test.StartTime.Unix()))
	}

	return nil
}

func processEvents(events []*testEvent) ([]*tester.Test, []*tester.Benchmark, error) {
	var (
		tests      []*tester.Test
		testMap    = make(map[string]*tester.Test)
		benchmarks []*tester.Benchmark
	)

	for _, event := range events {
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
			testMap[event.Test] = test

			if !event.TopLevel() {
				parentTest, ok := testMap[event.ParentTest()]
				if !ok {
					return nil, nil, fmt.Errorf("missing parent test %s for sub test %s", event.ParentTest(), event.Test)
				}
				parentTest.SubTests = append(parentTest.SubTests, test)
			}
		case "pass", "fail", "skip":
			test, ok := testMap[event.Test]
			if !ok {
				return nil, nil, fmt.Errorf("missing test: %s", event.Test)
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
				tests = append(tests, test)
			}
		case "output":
			test, ok := testMap[event.Test]
			if !ok {
				return nil, nil, fmt.Errorf("missing test: %s", event.Test)
			}
			test.Output = append(test.Output, event.Output.Bytes()...)

			for _, testName := range event.ParentTests() {
				test, ok := testMap[testName]
				if !ok {
					return nil, nil, fmt.Errorf("missing parent test %s for sub test %s", testName, event.Test)
				}
				test.Output = append(test.Output, event.Output.Bytes()...)
			}
		}
	}
	return tests, benchmarks, nil
}
