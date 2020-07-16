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
	apiKey     string
}

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

// WithTesterAddr allows configuring a custom address for the tester server.
func WithTesterAddr(addr string) Option {
	return func(opts *options) {
		opts.testerAddr = addr
	}
}

// WithAPIKey allows configuring an api key for authentication.
func WithAPIKey(key string) Option {
	return func(opts *options) {
		opts.apiKey = key
	}
}

// Runner is the implementation of the test runner.
type Runner struct {
	testerAddr string
	apiKey     string
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
		apiKey:     defOpts.apiKey,
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
	r.authAPIRequest(req)
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

	var pkg *tester.Package
	for _, p := range r.packages {
		if p.Name == run.Package {
			pkg = &p
			break
		}
	}
	if pkg == nil {
		return fmt.Errorf("unknown package: %s", run.Package)
	}

	log.Printf("starting run for %s (%s) with options: %s", pkg.Name, run.ID, strings.Join(run.Args, " "))
	var (
		stdout       bytes.Buffer
		stderr       bytes.Buffer
		eventStdout  bytes.Buffer
		errorMessage string
	)

	runArgs := []string{
		"-test.v",
	}

	for _, arg := range run.Args {
		runArgs = append(runArgs, arg)
	}

	reader, writer := io.Pipe()
	teeReader := io.TeeReader(reader, &stdout)

	testCmd := exec.CommandContext(ctx, pkg.Path, runArgs...)
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
			return fmt.Errorf("running: %w", err)
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

	tests, err := processEvents(events)
	if err != nil {
		return fmt.Errorf("processing events: %w", err)
	}

	var testIDs []uuid.UUID
	for _, test := range tests {
		test.RunID = run.ID
		test.Package = run.Package
		log.Printf("Test: %s - %s - %s", test.Result.Name, string(test.Result.State), test.Result.Duration().String())
		testIDs = append(testIDs, test.ID)
		if r.testerAddr != "" {
			err := r.submitTestResult(test, run)
			if err != nil {
				log.Printf("failed to submit result: %s", err)
			}

		}
	}
	err = r.completeRun(run.ID)
	if err != nil {
		log.Printf("failed to mark run complete: %s", err)
	}

	log.Printf("finished run for %s", run.Package)
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
	r.authAPIRequest(req)
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

func (r *Runner) failRun(runID uuid.UUID, errorMessage string) error {
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
	r.authAPIRequest(req)
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

func (r *Runner) completeRun(runID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), resultSubmissionTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/runs/%s/complete", r.testerAddr, runID),
		nil,
	)
	if err != nil {
		return fmt.Errorf("constructing request: %w", err)
	}
	r.authAPIRequest(req)
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

func (r *Runner) authAPIRequest(req *http.Request) {
	if r.apiKey == "" {
		return
	}

	// TODO make this configurable
	name, err := os.Hostname()
	// If getting hostname fails, use the generic "runner" name.
	if err != nil {
		name = "runner"
	}
	req.SetBasicAuth(name, r.apiKey)
}

func processEvents(events []*testEvent) ([]*tester.Test, error) {
	var (
		testMap = make(map[*tester.T]*tester.Test)
		tMap    = make(map[string]*tester.T)
	)

	for _, event := range events {
		// TODO revisit when adding support for benchmarks
		if event.Test == "" {
			continue
		}

		switch event.Action {
		case "run":
			t := &tester.T{
				TB: tester.TB{
					Name:      event.Test,
					StartedAt: event.Time,
				},
			}
			tMap[event.Test] = t

			if event.TopLevel() {
				testMap[t] = &tester.Test{
					ID:     uuid.New(),
					Result: t,
				}
			} else {
				parentT, ok := tMap[event.ParentTest()]
				if !ok {
					return nil, fmt.Errorf("missing parent t %s for sub t %s", event.ParentTest(), event.Test)
				}
				parentT.SubTs = append(parentT.SubTs, t)
			}
		case "pass", "fail", "skip":
			t, ok := tMap[event.Test]
			if !ok {
				return nil, fmt.Errorf("missing t: %s", event.Test)
			}
			t.FinishedAt = event.Time
			switch event.Action {
			case "pass":
				t.State = tester.TBStatePassed
			case "fail":
				t.State = tester.TBStateFailed
			case "skip":
				t.State = tester.TBStateSkipped
			}
		case "output":
			t, ok := tMap[event.TopLevelTest()]
			if !ok {
				return nil, fmt.Errorf("missing t: %s", event.Test)
			}

			test, ok := testMap[t]
			if !ok {
				return nil, fmt.Errorf("missing test: %s", t.Name)
			}

			test.Logs = append(test.Logs, tester.TBLog{
				Time:   event.Time,
				Name:   event.Test,
				Output: event.Output.Bytes(),
			})
		}
	}

	var tests []*tester.Test
	for _, test := range testMap {
		tests = append(tests, test)
	}
	return tests, nil
}
