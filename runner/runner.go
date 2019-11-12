package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/nanzhong/tester"
)

var resultSubmissionTimeout = 15 * time.Second

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
	testerAddr string
}

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

// WithTesterAddr allows configuring result submission to a tester server.
func WithTesterAddr(addr string) Option {
	return func(opts *options) {
		opts.testerAddr = addr
	}
}

// Runner is the implementation of the test runner.
type Runner struct {
	testerAddr string
	config     TBRunConfig

	stop     chan struct{}
	finished chan struct{}
	kill     context.CancelFunc
}

func New(config TBRunConfig, opts ...Option) *Runner {
	defOpts := &options{}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &Runner{
		config:     config,
		testerAddr: defOpts.testerAddr,

		stop:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (r *Runner) Run() {
	log.Println("starting runner...")
	ctx, cancel := context.WithCancel(context.Background())
	r.kill = cancel

	for {
		select {
		case <-r.stop:
			return
		default:
			err := r.runOnce(ctx)
			if err != nil {
				log.Printf("error running: %s\n", err)
			}
		}
	}
	close(r.finished)
	log.Println("runner finished")
}

func (r *Runner) Stop(ctx context.Context) error {
	log.Println("stopping runner...")
	close(r.stop)

	select {
	case <-r.finished:
		log.Println("runner stopped")
	case <-ctx.Done():
		log.Println("failed to stop runner gracefully, killing")
		r.kill()
	}
	return nil
}

func (r *Runner) runOnce(ctx context.Context) error {
	log.Printf("starting run for %s\n", r.config.Path)

	var output bytes.Buffer
	runArgs := []string{
		"tool",
		"test2json",
		"-t",
		r.config.Path,
		"-test.v",
	}
	if r.config.Timeout != 0 {
		runArgs = append(runArgs, fmt.Sprintf("-test.timeout=%s", r.config.Timeout.String()))
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

	log.Printf("finished run for %s\n", r.config.Path)
	for _, test := range tests {
		log.Printf("Test: %s - %s - %s\n", test.Name, test.State.String(), test.Duration().String())
		if r.testerAddr != "" {
			err := r.submitTestResult(test)
			if err != nil {
				log.Printf("failed to submit result: %s\n", err)
			}

		}
	}
	return nil
}

func (r *Runner) submitTestResult(test *tester.Test) error {
	jsonTest, err := json.Marshal(test)
	if err != nil {
		return fmt.Errorf("marshaling json test: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), resultSubmissionTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/tests", r.testerAddr),
		bytes.NewBuffer(jsonTest),
	)
	if err != nil {
		return fmt.Errorf("constructing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("submitting test: %w", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("received unexpected status code: %d", resp.StatusCode)
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
