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
	runConfigs []TBRunConfig
	db         *db.MemDB

	stop    chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	runCmds []*exec.Cmd
}

func New(configs []TBRunConfig, opts ...Option) *Runner {
	defOpts := &options{
		db: &db.MemDB{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	runner := &Runner{
		db:         defOpts.db,
		runConfigs: configs,
	}

	return runner
}

func (r *Runner) Run() {
	log.Println("starting runner...")
	log.Printf("runner configured with %d packages\n", len(r.runConfigs))

	for _, config := range r.runConfigs {
		config := config
		r.wg.Add(1)
		go func() {
			for {
				select {
				case <-r.stop:
					r.wg.Done()
					return
				default:
					r.runTB(config)
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

	c := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(c)
	}()

	select {
	case <-c:
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

func (r *Runner) runTB(config TBRunConfig) error {
	log.Printf("starting test run for %s\n", config.Path)

	var output bytes.Buffer

	runArgs := []string{
		"tool",
		"test2json",
		"-t",
		config.Path,
		"-test.v",
	}
	if config.Timeout != 0 {
		runArgs = append(runArgs, fmt.Sprintf("-test.timeout=%s", config.Timeout.String()))
	}

	cmd := exec.Command("go", runArgs...)
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
	time.Sleep(5 * time.Second)
	return nil
}
