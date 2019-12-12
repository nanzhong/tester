package scheduler

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
	"golang.org/x/sync/errgroup"
)

type options struct {
	db       db.DB
	runDelay time.Duration
}

// Option is used to inject dependencies into a Scheduler on creation.
type Option func(*options)

// WithDB allows configuring a DB.
func WithDB(db db.DB) Option {
	return func(opts *options) {
		opts.db = db
	}
}

// With RunDelay allows configuring a minimum delay between runs of a package.
func WithRunDelay(d time.Duration) Option {
	return func(opts *options) {
		opts.runDelay = d
	}
}

// Scheduler schedules runs.
type Scheduler struct {
	stop            chan struct{}
	packages        []tester.Package
	lastScheduledAt map[string]time.Time
	runDelay        time.Duration
	db              db.DB
}

// NewScheduler constructs a new scheduler.
func NewScheduler(packages []tester.Package, opts ...Option) *Scheduler {
	defOpts := &options{
		runDelay: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &Scheduler{
		stop:            make(chan struct{}),
		db:              defOpts.db,
		runDelay:        defOpts.runDelay,
		lastScheduledAt: make(map[string]time.Time),
		packages:        packages,
	}
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
	runs, err := s.db.ListRuns(ctx)
	if err != nil {
		return err
	}

	pendingRuns := make(map[string]*tester.Run)
	for _, run := range runs {
		pendingRuns[run.Package.Name] = run
	}

	packagesToRun := make([]tester.Package, len(s.packages))
	copy(packagesToRun, s.packages)
	rand.Shuffle(len(packagesToRun), func(i int, j int) {
		packagesToRun[i], packagesToRun[j] = packagesToRun[j], packagesToRun[i]
	})

	for _, pkg := range packagesToRun {
		if _, exists := pendingRuns[pkg.Name]; !exists {
			last, ran := s.lastScheduledAt[pkg.Name]
			if ran && time.Now().Sub(last) < s.runDelay {
				continue
			}

			err = s.db.EnqueueRun(ctx, &tester.Run{
				ID:         uuid.New().String(),
				Package:    pkg,
				EnqueuedAt: time.Now(),
			})
			s.lastScheduledAt[pkg.Name] = time.Now()
			log.Printf("scheduled run %s", pkg.Name)
		}
	}

	return nil
}

func (s *Scheduler) resetStaleRuns(ctx context.Context) error {
	runs, err := s.db.ListRuns(ctx)
	if err != nil {
		return err
	}

	for _, run := range runs {
		if run.StartedAt.IsZero() {
			continue
		}

		timeout := time.Duration(run.Package.DefaultTimeout) * time.Second
		if timeout == 0 {
			timeout = 15 * time.Minute
		}

		if time.Now().Sub(run.StartedAt) > timeout {
			err = s.db.ResetRun(ctx, run.ID)
			if err != nil {
				return err
			}
			log.Printf("reset run %s", run.Package.Name)
		}
	}

	return nil
}
