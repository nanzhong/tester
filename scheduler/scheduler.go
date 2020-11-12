package scheduler

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
	"golang.org/x/sync/errgroup"
)

type options struct {
	runTimeout time.Duration
	runDelay   time.Duration
}

// Option is used to inject dependencies into a Scheduler on creation.
type Option func(*options)

// WithRunDelay allows configuring a minimum delay between runs of a package.
func WithRunDelay(d time.Duration) Option {
	return func(opts *options) {
		opts.runDelay = d
	}
}

// WithRunTimeout allows configuring a maximum timeout before runs are deemed
// stale and reset.
func WithRunTimeout(d time.Duration) Option {
	return func(opts *options) {
		opts.runTimeout = d
	}
}

// Scheduler schedules runs.
type Scheduler struct {
	Packages []*tester.Package

	stop            chan struct{}
	lastScheduledAt map[string]time.Time
	runDelay        time.Duration
	runTimeout      time.Duration
	db              db.DB
}

// NewScheduler constructs a new scheduler.
func NewScheduler(db db.DB, packages []*tester.Package, opts ...Option) *Scheduler {
	defOpts := &options{
		runDelay:   5 * time.Minute,
		runTimeout: 15 * time.Minute,
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &Scheduler{
		stop:            make(chan struct{}),
		db:              db,
		runDelay:        defOpts.runDelay,
		runTimeout:      defOpts.runTimeout,
		lastScheduledAt: make(map[string]time.Time),
		Packages:        packages,
	}
}

func (s *Scheduler) Schedule(ctx context.Context, packageName string, args ...string) (*tester.Run, error) {
	var pkg *tester.Package
	for _, p := range s.Packages {
		if p.Name == packageName {
			pkg = p
			break
		}
	}
	if pkg == nil {
		return nil, fmt.Errorf("unknown package: %s", packageName)
	}

	fs := flag.NewFlagSet(packageName, flag.ContinueOnError)
	runPkgOptions := map[string]*string{}
	for _, option := range pkg.Options {
		runPkgOptions[option.Name] = fs.String(option.Name, option.Default, option.Description)
	}
	err := fs.Parse(args)
	if err != nil {
		return nil, fmt.Errorf("parsing run options: %w", err)
	}

	var runArgs []string
	for _, opt := range pkg.Options {
		if value, set := runPkgOptions[opt.Name]; set && value != nil && *value != "" {
			runArgs = append(runArgs, fmt.Sprintf("-%s=%s", opt.Name, *value))
		}

	}

	run := &tester.Run{
		ID:         uuid.New(),
		Package:    pkg.Name,
		Args:       runArgs,
		EnqueuedAt: time.Now(),
	}
	err = s.db.EnqueueRun(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("scheduling package: %w", err)
	}

	log.Printf("scheduled run %s with args: %q", pkg.Name, strings.Join(runArgs, ", "))
	return run, nil
}

// Run starts the scheduler.
func (s *Scheduler) Run() {
	wait := 0 * time.Second
	for {
		select {
		case <-s.stop:
			return
		case <-time.After(wait):
		}
		wait = time.Duration((rand.Int() % 10)) * time.Second

		ctx := context.Background()
		var eg errgroup.Group
		eg.Go(func() error {
			return s.scheduleRuns(ctx)
		})
		eg.Go(func() error {
			return s.resetStaleRuns(ctx)
		})
		eg.Go(func() error {
			return s.cleanupUnprocessableRuns(ctx)
		})
		err := eg.Wait()
		if err != nil {
			log.Printf("scheduling error: %s", err)
		}
	}
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) scheduleRuns(ctx context.Context) error {
	runs, err := s.db.ListPendingRuns(ctx)
	if err != nil {
		return err
	}

	pendingRuns := make(map[string]*tester.Run)
	for _, run := range runs {
		if !run.FinishedAt.IsZero() {
			continue
		}
		pendingRuns[run.Package] = run
	}

	packagesToRun := make([]*tester.Package, len(s.Packages))
	copy(packagesToRun, s.Packages)
	rand.Shuffle(len(packagesToRun), func(i int, j int) {
		packagesToRun[i], packagesToRun[j] = packagesToRun[j], packagesToRun[i]
	})

	for _, pkg := range packagesToRun {
		runDelay := s.runDelay
		if pkg.RunDelay > 0 {
			runDelay = pkg.RunDelay
		}
		if _, exists := pendingRuns[pkg.Name]; !exists {
			last, ran := s.lastScheduledAt[pkg.Name]
			if ran && time.Since(last) < runDelay {
				continue
			}

			var args []string
			for _, option := range pkg.Options {
				if option.Default != "" {
					o := tester.Option{
						Name:  option.Name,
						Value: option.Default,
					}
					args = append(args, o.String())
				}
			}
			err = s.db.EnqueueRun(ctx, &tester.Run{
				ID:         uuid.New(),
				Package:    pkg.Name,
				Args:       args,
				EnqueuedAt: time.Now(),
			})
			s.lastScheduledAt[pkg.Name] = time.Now()
			log.Printf("scheduled run %s", pkg.Name)
		}
	}

	return nil
}

func (s *Scheduler) cleanupUnprocessableRuns(ctx context.Context) error {
	runs, err := s.db.ListPendingRuns(ctx)
	if err != nil {
		return err
	}

	for _, run := range runs {
		// Cleanup runs that haven't been picked up for 1 day.
		// This usually indicates an old run/package that is no longer runnable.
		if !run.StartedAt.IsZero() || time.Now().Sub(run.EnqueuedAt) < 24*time.Hour {
			continue
		}

		err := s.db.DeleteRun(ctx, run.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Scheduler) resetStaleRuns(ctx context.Context) error {
	runs, err := s.db.ListPendingRuns(ctx)
	if err != nil {
		return err
	}

	for _, run := range runs {
		if run.StartedAt.IsZero() || !run.FinishedAt.IsZero() {
			continue
		}

		if time.Now().Sub(run.StartedAt) > s.runTimeout {
			err = s.db.ResetRun(ctx, run.ID)
			if err != nil {
				if err == db.ErrNotFound {
					continue
				}
				return err
			}
			log.Printf("reset run %s", run.Package)
		}
	}

	return nil
}
