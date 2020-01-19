package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nanzhong/tester"
)

var resultSubmissionTimeout = 60 * time.Second

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

// WithTesterAddr allows configuring a custom address for the tester server.
func WithTesterAddr(addr string) Option {
	return func(opts *options) {
		opts.testerAddr = addr
	}
}

// Runner is the implementation of the test runner.
type Runner struct {
	testerAddr string
	packages   []tester.Package

	stop     chan struct{}
	finished chan struct{}
	kill     context.CancelFunc
}

func New(packages []tester.Package, opts ...Option) *Runner {
	defOpts := &options{
		testerAddr: "0.0.0.0:8080",
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &Runner{
		testerAddr: defOpts.testerAddr,
		packages:   packages,

		stop:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (r *Runner) Run() {
	wait := 0 * time.Second
	for {
		select {
		case <-r.stop:
			close(r.finished)
			return
		case <-time.After(wait):
		}
		wait = time.Duration((rand.Int() % 10)) * time.Second
		ctx, cancel := context.WithCancel(context.Background())
		r.kill = cancel

		err := r.runOnce(ctx)
		if err != nil {
			log.Printf("error running: %s\n", err)
		}
	}
}

func (r *Runner) Stop(ctx context.Context) {
	close(r.stop)

	select {
	case <-r.finished:
	case <-ctx.Done():
		r.kill()
	}
}

func (r *Runner) claimRun(ctx context.Context) (*tester.Run, error) {
	var packages []string
	for _, pkg := range r.packages {
		packages = append(packages, pkg.Name)
	}
	body, err := json.Marshal(packages)
	if err != nil {
		return nil, fmt.Errorf("marshaling claim request to json: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/runs/claim", r.testerAddr),
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("constructing claim request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claiming run: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var run tester.Run
		err = json.NewDecoder(resp.Body).Decode(&run)
		if err != nil {
			return nil, fmt.Errorf("decoding claimed run: %w", err)
		}
		return &run, nil
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("received unexpected status code for claim request: %d", resp.StatusCode)
	}
}

func (r *Runner) runOnce(ctx context.Context) error {
	run, err := r.claimRun(ctx)
	if err != nil {
		return fmt.Errorf("claiming run: %w", err)
	}
	if run == nil {
		return nil
	}

	var runOptions []string
	for _, option := range run.Package.Options {
		runOptions = append(runOptions, option.String())
	}

	log.Printf("starting run for %s (%s) with options: %s", run.Package.Name, run.ID, strings.Join(runOptions, " "))
	var (
		stdout       bytes.Buffer
		stderr       bytes.Buffer
		eventStdout  bytes.Buffer
		errorMessage string
	)

	runArgs := []string{
		"-test.v",
	}

	for _, option := range run.Package.Options {
		runArgs = append(runArgs, fmt.Sprintf("-%s", option.Name))
		if option.Value != "" {
			runArgs = append(runArgs, fmt.Sprintf("%s", option.Value))
		}
	}

	reader, writer := io.Pipe()
	teeReader := io.TeeReader(reader, &stdout)

	testCmd := exec.CommandContext(ctx, run.Package.Path, runArgs...)
	testCmd.Stdout = writer
	testCmd.Stderr = &stderr

	jsonCmd := exec.CommandContext(ctx, "go", "tool", "test2json", "-t")
	jsonCmd.Stdin = teeReader
	jsonCmd.Stdout = &eventStdout
	jsonCmd.Stderr = os.Stderr

	testCmd.Start()
	jsonCmd.Start()

	err = testCmd.Wait()
	writer.Close()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return fmt.Errorf("running test/benchmark: %w", err)
		}

		switch exitErr.ExitCode() {
		// non 0 exit statuses are okay.
		// eg. failed tests will result in exit status 1.
		case 1:
		default:
			errorMessage = fmt.Sprintf("Test run failed: %s\nExit Code: %d\nstdout:\n%s\nstderr:\n%s", exitErr.String(), exitErr.ExitCode(), stdout.Bytes(), stderr.Bytes())
			if err := r.failRun(run.ID, errorMessage); err != nil {
				log.Printf("failed to mark run failed: %s", err)
			}
			return exitErr
		}
	}

	if err := jsonCmd.Wait(); err != nil {
		return fmt.Errorf("parsing test output: %w", err)
	}

	eventBytes := bytes.Split(bytes.Trim(eventStdout.Bytes(), " \n"), []byte("\n"))
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

	var testIDs []string
	for _, test := range tests {
		test.Package = run.Package
		log.Printf("Test: %s - %s - %s", test.Name, test.State.String(), test.Duration().String())
		testIDs = append(testIDs, test.ID)
		if r.testerAddr != "" {
			err := r.submitTestResult(test, run)
			if err != nil {
				log.Printf("failed to submit result: %s", err)
			}

		}
	}
	err = r.completeRun(run.ID, testIDs)
	if err != nil {
		log.Printf("failed to mark run complete: %s", err)
	}

	log.Printf("finished run for %s", run.Package.Name)
	return nil
}

func (r *Runner) submitTestResult(test *tester.Test, run *tester.Run) error {
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

func (r *Runner) failRun(runID string, errorMessage string) error {
	log.Printf("failing run")
	jsonError, err := json.Marshal(errorMessage)
	if err != nil {
		return fmt.Errorf("marshaling error message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), resultSubmissionTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/runs/%s/fail", r.testerAddr, runID),
		bytes.NewBuffer(jsonError),
	)
	if err != nil {
		return fmt.Errorf("constructing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failing run: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (r *Runner) completeRun(runID string, testIDs []string) error {
	jsonTestIDs, err := json.Marshal(testIDs)
	if err != nil {
		return fmt.Errorf("marshaling test ids: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), resultSubmissionTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/runs/%s/complete", r.testerAddr, runID),
		bytes.NewBuffer(jsonTestIDs),
	)
	if err != nil {
		return fmt.Errorf("constructing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("completing run: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
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
					StartedAt: event.Time,
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
			test.FinishedAt = event.Time
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
