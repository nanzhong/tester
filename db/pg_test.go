package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/nanzhong/tester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withPG(tb testing.TB, fn func(tb testing.TB, pg *PG)) {
	pgDSN := os.Getenv("PG_DSN")
	if pgDSN == "" {
		tb.Skip("PG_DSN not set, skipping PG tests. Set PG_DSN to run this test.")
	}
	conn, err := pgx.Connect(context.Background(), pgDSN)
	require.NoError(tb, err)

	defer conn.Close(context.Background())

	cfg := conn.Config()
	testDB := fmt.Sprintf("tester_%d", time.Now().UnixNano())

	_, err = conn.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %s WITH OWNER = %s", pgx.Identifier{testDB}.Sanitize(), pgx.Identifier{cfg.User}.Sanitize()))
	require.NoError(tb, err)
	defer func() {
		_, err := conn.Exec(context.Background(), fmt.Sprintf("DROP DATABASE %s", pgx.Identifier{testDB}.Sanitize()))
		require.NoError(tb, err)
	}()

	pgDSN = fmt.Sprintf("postgres://%s:%s@%s:%d/%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, testDB)
	pool, err := pgxpool.Connect(context.Background(), pgDSN)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	pg := NewPG(pool)
	err = pg.Init(context.Background())
	require.NoError(tb, err)

	tb.Log("test")
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

		t.Run("AddTest", func(t *testing.T) {
			err := pg.AddTest(ctx, test1)
			require.NoError(t, err)

			err = pg.AddTest(ctx, test2)
			require.NoError(t, err)
		})

		t.Run("GetTest", func(t *testing.T) {
			getTest, err := pg.GetTest(ctx, test1.ID)
			require.NoError(t, err)
			assert.True(
				t,
				cmp.Equal(test1, getTest),
				"expected to be equal", cmp.Diff(test1, getTest),
			)
		})

		t.Run("list", func(t *testing.T) {
			listAllTests, err := pg.ListTests(ctx, 0)
			require.NoError(t, err)
			assert.True(
				t,
				cmp.Equal([]*tester.Test{test1, test2}, listAllTests),
				"expected to be equal", cmp.Diff([]*tester.Test{test1, test2}, listAllTests),
			)

			t.Run("ListTestsForPackage", func(t *testing.T) {
				listPkgTests, err := pg.ListTestsForPackage(ctx, "pkg-2", 0)
				require.NoError(t, err)
				assert.True(
					t,
					cmp.Equal([]*tester.Test{test2}, listPkgTests),
					"expected to be equal", cmp.Diff([]*tester.Test{test2}, listPkgTests),
				)
			})

			t.Run("ListTestsForPackageInRange", func(t *testing.T) {
				listPkgTestsInRange, err := pg.ListTestsForPackageInRange(ctx, "pkg-2", testTime, testTime)
				require.NoError(t, err)
				assert.True(
					t,
					cmp.Equal([]*tester.Test{test2}, listPkgTestsInRange),
					"expected to be equal", cmp.Diff([]*tester.Test{test2}, listPkgTestsInRange),
				)
			})
		})
	})
}

func TestPG_EnqueueRun_GetRun(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		run := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
			Args:    []string{"one", "two"},
		}

		err := pg.EnqueueRun(ctx, run)
		require.NoError(t, err)

		run, err = pg.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.EnqueuedAt)
	})
}

func TestPG_StartRun(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		run := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
			Args:    []string{"one", "two"},
		}

		err := pg.EnqueueRun(ctx, run)
		require.NoError(t, err)

		err = pg.StartRun(ctx, run.ID, "runner")
		require.NoError(t, err)

		getRun, err := pg.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, getRun.StartedAt)
		assert.Equal(t, "runner", getRun.Meta.Runner)
	})
}

func TestPG_ResetRun(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		run := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
			Args:    []string{"one", "two"},
		}

		err := pg.EnqueueRun(ctx, run)
		require.NoError(t, err)

		err = pg.StartRun(ctx, run.ID, "")
		require.NoError(t, err)

		err = pg.ResetRun(ctx, run.ID)
		require.NoError(t, err)

		getRun, err := pg.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.Empty(t, getRun.StartedAt)
	})
}

func TestPG_DeleteRun(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		run := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
			Args:    []string{"one", "two"},
		}

		err := pg.EnqueueRun(ctx, run)
		require.NoError(t, err)

		err = pg.DeleteRun(ctx, run.ID)
		require.NoError(t, err)

		_, err = pg.GetRun(ctx, run.ID)
		require.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
	})
}

func TestPG_CompleteRun(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		run := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
			Args:    []string{"one", "two"},
		}

		err := pg.EnqueueRun(ctx, run)
		require.NoError(t, err)

		err = pg.StartRun(ctx, run.ID, "")
		require.NoError(t, err)

		err = pg.CompleteRun(ctx, run.ID)
		require.NoError(t, err)

		getRun, err := pg.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, getRun.FinishedAt)
	})
}

func TestPG_FailRun(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		run := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
			Args:    []string{"one", "two"},
		}

		err := pg.EnqueueRun(ctx, run)
		require.NoError(t, err)

		err = pg.StartRun(ctx, run.ID, "")
		require.NoError(t, err)

		err = pg.FailRun(ctx, run.ID, "error")
		require.NoError(t, err)

		getRun, err := pg.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, getRun.FinishedAt)
		assert.NotEmpty(t, getRun.Error)
	})
}

func TestPG_ListRuns(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		runPending := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
		}

		runComplete := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
		}

		runFail := &tester.Run{
			ID:      uuid.New(),
			Package: "pkg",
		}

		for _, r := range []*tester.Run{runPending, runComplete, runFail} {
			err := pg.EnqueueRun(ctx, r)
			require.NoError(t, err)

			err = pg.StartRun(ctx, r.ID, "")
			require.NoError(t, err)
		}

		runPending, err := pg.GetRun(ctx, runPending.ID)
		require.NoError(t, err)

		err = pg.CompleteRun(ctx, runComplete.ID)
		require.NoError(t, err)
		runComplete, err = pg.GetRun(ctx, runComplete.ID)
		require.NoError(t, err)

		err = pg.FailRun(ctx, runFail.ID, "error")
		require.NoError(t, err)
		runFail, err = pg.GetRun(ctx, runFail.ID)
		require.NoError(t, err)

		t.Run("ListPendingRuns", func(t *testing.T) {
			runs, err := pg.ListPendingRuns(ctx)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{runPending}, runs)
		})

		t.Run("ListPendingRuns", func(t *testing.T) {
			runs, err := pg.ListFinishedRuns(ctx, 0)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{runComplete, runFail}, runs)
		})
	})
}

func TestPG_ListRunsForPackage(t *testing.T) {
	ctx := context.Background()

	withPG(t, func(tb testing.TB, pg *PG) {
		runs := []*tester.Run{
			{
				ID:      uuid.New(),
				Package: "pkg-1",
			},
			{
				ID:      uuid.New(),
				Package: "pkg-2",
			},
		}

		for _, r := range runs {
			err := pg.EnqueueRun(ctx, r)
			require.NoError(t, err)
			r, err = pg.GetRun(ctx, r.ID)
			require.NoError(t, err)
		}

		runs, err := pg.ListRunsForPackage(ctx, "pkg-1", 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []*tester.Run{runs[0]}, runs)
	})
}

func TestPG_ListRunSummariesInRange(t *testing.T) {
	ctx := context.Background()

	t.Run("creates empty buckets", func(t *testing.T) {
		withPG(t, func(tb testing.TB, pg *PG) {
			now := time.Now().UTC()
			summaries, err := pg.ListRunSummariesInRange(ctx, now, now.Add(3*time.Minute+15*time.Second), time.Minute)
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

			summaries, err := pg.ListRunSummariesInRange(ctx, begin, end, window)
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
