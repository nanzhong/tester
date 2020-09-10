package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/nanzhong/tester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withPG(tb testing.TB, fn func(tb testing.TB, pg *PG)) {
	if pgDSN == "" {
		tb.Skip("PG_DSN not set, skipping PG tests. Set PG_DSN to run this test.")
	}

	pool, err := pgxpool.Connect(context.Background(), pgDSN)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	pg := NewPG(pool)
	err = pg.Init(context.Background())
	require.NoError(tb, err)

	fn(tb, pg)
}

func TestPG_Test(t *testing.T) {
	testTime := time.Now().Truncate(time.Millisecond)

	withPG(t, func(tb testing.TB, pg *PG) {
		ctx := context.Background()

		test1 := &tester.Test{
			ID:      uuid.New(),
			Package: "pkg-1",
			RunID:   uuid.New(),

			Result: &tester.T{
				TB: tester.TB{
					StartedAt:  testTime,
					FinishedAt: testTime,
					State:      tester.TBStatePassed,
				},
				SubTs: []*tester.T{
					{
						TB: tester.TB{
							StartedAt:  testTime,
							FinishedAt: testTime,
							State:      tester.TBStatePassed,
						},
						SubTs: []*tester.T{
							{
								TB: tester.TB{
									StartedAt:  testTime,
									FinishedAt: testTime,
									State:      tester.TBStatePassed,
								},
							},
						},
					},
					{
						TB: tester.TB{
							StartedAt:  testTime,
							FinishedAt: testTime,
							State:      tester.TBStatePassed,
						},
					},
				},
			},
			Logs: []tester.TBLog{
				{Time: testTime, Name: "name", Output: []byte("output")},
			},
		}
		test2 := &tester.Test{
			ID:      uuid.New(),
			Package: "pkg-2",
			RunID:   uuid.New(),

			Result: &tester.T{
				TB: tester.TB{
					StartedAt:  testTime,
					FinishedAt: testTime,
					State:      tester.TBStatePassed,
				},
			},
			Logs: []tester.TBLog{
				{Time: testTime, Name: "name", Output: []byte("output")},
			},
		}

		t.Run("add", func(t *testing.T) {
			err := pg.AddTest(ctx, test1)
			require.NoError(t, err)

			err = pg.AddTest(ctx, test2)
			require.NoError(t, err)
		})

		t.Run("get", func(t *testing.T) {
			getTest, err := pg.GetTest(ctx, test1.ID)
			require.NoError(t, err)
			assert.Equal(t, test1, getTest)
		})

		t.Run("list", func(t *testing.T) {
			listAllTests, err := pg.ListTests(ctx, 0)
			require.NoError(t, err)
			assert.Equal(t, []*tester.Test{test1, test2}, listAllTests)

			listPkgTests, err := pg.ListTestsForPackage(ctx, "pkg-2", 0)
			require.NoError(t, err)
			assert.Equal(t, []*tester.Test{test2}, listPkgTests)
		})
	})
}

func TestPG_Run(t *testing.T) {
	ctx := context.Background()
	testTime := time.Now().Truncate(time.Millisecond)

	run1 := &tester.Run{
		ID:         uuid.New(),
		Package:    "pkg-1",
		Args:       []string{"one", "two"},
		EnqueuedAt: testTime,
	}

	run2 := &tester.Run{
		ID:         uuid.New(),
		Package:    "pkg-2",
		Args:       []string{"one", "two"},
		EnqueuedAt: testTime,
	}

	run3 := &tester.Run{
		ID:         uuid.New(),
		Package:    "pkg-3",
		Args:       []string{"one", "two"},
		EnqueuedAt: testTime,
	}

	withPG(t, func(tb testing.TB, pg *PG) {
		pg.now = func() time.Time { return testTime }

		t.Run("enqueue", func(t *testing.T) {
			err := pg.EnqueueRun(ctx, run1)
			require.NoError(t, err)

			err = pg.EnqueueRun(ctx, run2)
			require.NoError(t, err)

			err = pg.EnqueueRun(ctx, run3)
			require.NoError(t, err)
		})

		t.Run("get", func(t *testing.T) {
			getRun, err := pg.GetRun(ctx, run1.ID)
			require.NoError(t, err)
			assert.Equal(t, run1, getRun)
		})

		t.Run("start", func(t *testing.T) {
			err := pg.StartRun(ctx, run1.ID)
			require.NoError(t, err)

			err = pg.StartRun(ctx, run2.ID)
			require.NoError(t, err)
		})

		t.Run("complete", func(t *testing.T) {
			err := pg.CompleteRun(ctx, run1.ID)
			require.NoError(t, err)
			run1, err = pg.GetRun(ctx, run1.ID)
			require.NoError(t, err)
		})

		t.Run("fail", func(t *testing.T) {
			err := pg.FailRun(ctx, run2.ID, "error")
			require.NoError(t, err)
			run2, err = pg.GetRun(ctx, run2.ID)
			require.NoError(t, err)
		})

		t.Run("list", func(t *testing.T) {
			runs, err := pg.ListPendingRuns(ctx)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{run3}, runs)

			runs, err = pg.ListFinishedRuns(ctx, 0)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{run1, run2}, runs)

			runs, err = pg.ListRunsForPackage(ctx, "pkg-3", 0)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{run3}, runs)
		})

		t.Run("reset", func(t *testing.T) {
			err := pg.ResetRun(ctx, run3.ID)
			require.NoError(t, err)

			runs, err := pg.ListPendingRuns(ctx)
			require.NoError(t, err)
			assert.Subset(t, runs, []*tester.Run{run3})

			t.Run("finshed run", func(t *testing.T) {
				err := pg.ResetRun(ctx, run1.ID)
				require.Error(t, err)
				assert.Equal(t, ErrNotFound, err)
			})
		})

		t.Run("delete", func(t *testing.T) {
			err := pg.DeleteRun(ctx, run3.ID)
			require.NoError(t, err)
			runs, err := pg.ListPendingRuns(ctx)
			require.NoError(t, err)
			assert.NotSubset(t, runs, []*tester.Run{run3})
		})
	})
}

func TestPG_ListRunSummariesForRange(t *testing.T) {
	ctx := context.Background()

	t.Run("creates empty buckets", func(t *testing.T) {
		withPG(t, func(tb testing.TB, pg *PG) {
			now := time.Now().UTC()
			summaries, err := pg.ListRunSummariesForRange(ctx, now, now.Add(3*time.Minute+15*time.Second), time.Minute)
			require.NoError(t, err)
			assert.Len(t, summaries, 4)
			for i, summary := range summaries {
				assert.Equal(t, now.Add(time.Duration(i)*time.Minute), summary.Time)
				assert.Equal(t, time.Minute, summary.Duration)
			}
		})
	})

	t.Run("places runs in correct buckets", func(t *testing.T) {
		withPG(t, func(tb testing.TB, pg *PG) {
			begin := time.Now().UTC()
			end := begin.Add(3 * time.Minute).UTC()
			window := time.Minute

			pkg1run1 := &tester.Run{
				ID:         uuid.New(),
				Package:    "pkg-1",
				EnqueuedAt: begin,
				StartedAt:  begin,
				FinishedAt: begin,
			}
			err := pg.EnqueueRun(ctx, pkg1run1)
			require.NoError(t, err)

			pkg1run1.Tests = []*tester.Test{
				{
					ID:      uuid.New(),
					RunID:   pkg1run1.ID,
					Package: pkg1run1.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-pass", State: tester.TBStatePassed},
					},
				},
				{
					ID:      uuid.New(),
					RunID:   pkg1run1.ID,
					Package: pkg1run1.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-fail", State: tester.TBStateFailed},
					},
				},
				{
					ID:      uuid.New(),
					RunID:   pkg1run1.ID,
					Package: pkg1run1.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-skip", State: tester.TBStateSkipped},
					},
				},
			}

			pkg1run2 := &tester.Run{
				ID:         uuid.New(),
				Package:    "pkg-1",
				EnqueuedAt: begin,
				StartedAt:  begin.Add(15 * time.Second),
				FinishedAt: begin,
			}
			err = pg.EnqueueRun(ctx, pkg1run2)
			require.NoError(t, err)

			pkg1run2.Tests = []*tester.Test{
				{
					ID:      uuid.New(),
					RunID:   pkg1run2.ID,
					Package: pkg1run2.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-pass", State: tester.TBStatePassed},
					},
				},
				{
					ID:      uuid.New(),
					RunID:   pkg1run2.ID,
					Package: pkg1run2.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-fail", State: tester.TBStateFailed},
					},
				},
				{
					ID:      uuid.New(),
					RunID:   pkg1run2.ID,
					Package: pkg1run2.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-skip", State: tester.TBStateSkipped},
					},
				},
			}

			pkg1run3 := &tester.Run{
				ID:         uuid.New(),
				Package:    "pkg-1",
				EnqueuedAt: begin,
				StartedAt:  begin.Add(2*time.Minute + 15*time.Second),
				FinishedAt: begin.Add(2*time.Minute + 15*time.Second),
			}
			err = pg.EnqueueRun(ctx, pkg1run3)
			require.NoError(t, err)

			pkg1run3.Tests = []*tester.Test{
				{
					ID:      uuid.New(),
					RunID:   pkg1run3.ID,
					Package: pkg1run3.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-pass", State: tester.TBStatePassed},
					},
				},
				{
					ID:      uuid.New(),
					RunID:   pkg1run3.ID,
					Package: pkg1run3.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-fail", State: tester.TBStateFailed},
					},
				},
				{
					ID:      uuid.New(),
					RunID:   pkg1run3.ID,
					Package: pkg1run3.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-skip", State: tester.TBStateSkipped},
					},
				},
			}

			pkg2run1 := &tester.Run{
				ID:         uuid.New(),
				Package:    "pkg-2",
				EnqueuedAt: begin,
				StartedAt:  begin.Add(2*time.Minute + 15*time.Second),
				FinishedAt: begin.Add(2*time.Minute + 15*time.Second),
			}
			err = pg.EnqueueRun(ctx, pkg2run1)
			require.NoError(t, err)

			pkg2run1.Tests = []*tester.Test{
				{
					ID:      uuid.New(),
					RunID:   pkg2run1.ID,
					Package: pkg2run1.Package,
					Result: &tester.T{
						TB: tester.TB{Name: "test-pass", State: tester.TBStatePassed},
					},
				},
			}

			pkg2run2 := &tester.Run{
				ID:         uuid.New(),
				Package:    "pkg-2",
				EnqueuedAt: begin,
				StartedAt:  begin.Add(2*time.Minute + 15*time.Second),
				FinishedAt: begin.Add(2*time.Minute + 15*time.Second),
				Error:      "failed",
			}
			err = pg.EnqueueRun(ctx, pkg2run2)
			require.NoError(t, err)

			allTests := append(pkg1run1.Tests, pkg1run2.Tests...)
			allTests = append(allTests, pkg1run3.Tests...)
			allTests = append(allTests, pkg2run1.Tests...)
			for _, test := range allTests {
				err := pg.AddTest(ctx, test)
				require.NoError(t, err)
			}

			summaries, err := pg.ListRunSummariesForRange(ctx, begin, end, window)
			require.NoError(t, err)
			assert.Len(t, summaries, 3)
			assert.Equal(t, &tester.RunSummary{
				Time:     begin,
				Duration: window,
				PackageSummary: map[string]*tester.PackageSummary{
					"pkg-1": {
						Package:      "pkg-1",
						RunIDs:       []uuid.UUID{pkg1run1.ID, pkg1run2.ID},
						ErrorRunIDs:  nil,
						PassedTests:  map[string][]uuid.UUID{"test-pass": {pkg1run1.Tests[0].ID, pkg1run2.Tests[0].ID}},
						FailedTests:  map[string][]uuid.UUID{"test-fail": {pkg1run1.Tests[1].ID, pkg1run2.Tests[1].ID}},
						SkippedTests: map[string][]uuid.UUID{"test-skip": {pkg1run1.Tests[2].ID, pkg1run2.Tests[2].ID}},
					},
				},
			}, summaries[0])
		})
	})
}
