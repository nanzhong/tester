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
			err := pg.ResetRun(ctx, run2.ID)
			require.NoError(t, err)
			run2, err = pg.GetRun(ctx, run2.ID)
			require.NoError(t, err)

			runs, err := pg.ListPendingRuns(ctx)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{run2, run3}, runs)
		})

		t.Run("delete", func(t *testing.T) {
			err := pg.DeleteRun(ctx, run3.ID)
			require.NoError(t, err)
			runs, err := pg.ListPendingRuns(ctx)
			require.NoError(t, err)
			assert.ElementsMatch(t, []*tester.Run{run2}, runs)
		})
	})
}
