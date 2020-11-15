package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/tern/migrate"
	"github.com/nanzhong/tester"
)

var psq = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type pger interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type PG struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

var _ DB = (*PG)(nil)

func NewPG(pool *pgxpool.Pool) *PG {
	return &PG{
		pool: pool,
		now:  time.Now,
	}
}

func (p *PG) Init(ctx context.Context) error {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	m, err := migrate.NewMigrator(ctx, conn.Conn(), "versions")
	if err != nil {
		return err
	}

	for _, migration := range pgMigrations {
		m.AppendMigration(migration.name, migration.up, migration.down)
	}

	return m.Migrate(ctx)
}

func (p *PG) tx(ctx context.Context, f func(tx pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	err = f(tx)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (p *PG) AddTest(ctx context.Context, test *tester.Test) error {
	t := (*pgTest)(test)
	q := psq.Insert("tests").
		Columns(t.Columns()...).
		Values(t.Values()...)

	sql, args, err := q.ToSql()
	if err != nil {
		return err
	}

	_, err = p.pool.Exec(ctx, sql, args...)
	return err
}

func (p *PG) GetTest(ctx context.Context, id uuid.UUID) (*tester.Test, error) {
	test := &pgTest{}
	q := psq.Select(test.Columns()...).
		From("tests").
		Where("id = ?", id)

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}

	row := p.pool.QueryRow(ctx, sql, args...)

	err = test.Scan(row)
	if err != nil {
		return nil, err
	}
	return (*tester.Test)(test), nil
}

func (p *PG) listTests(ctx context.Context, pg pger, pred interface{}, limit int) ([]*tester.Test, error) {
	var tests []*tester.Test
	q := psq.Select((&pgTest{}).Columns()...).
		From("tests").
		OrderBy("result->'started_at' ASC")

	if pred != nil {
		q = q.Where(pred)
	}

	if limit > 0 {
		q = q.Limit(uint64(limit))
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := pg.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		t := &pgTest{}
		err := t.Scan(rows)
		if err != nil {
			return nil, err
		}
		tests = append(tests, (*tester.Test)(t))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tests, nil
}

func (p *PG) ListTests(ctx context.Context, limit int) ([]*tester.Test, error) {
	return p.listTests(ctx, p.pool, nil, limit)
}

func (p *PG) ListTestsForPackage(ctx context.Context, pkg string, limit int) ([]*tester.Test, error) {
	return p.listTests(ctx, p.pool, sq.Eq{"package": pkg}, limit)
}

func (p *PG) ListTestsInDateRange(ctx context.Context, from, to time.Time) ([]*tester.Test, error) {
	return p.listTests(ctx, p.pool, nil, 0)
}

func (p *PG) ListTestsForPackageInRange(ctx context.Context, pkg string, from, to time.Time) ([]*tester.Test, error) {
	return p.listTests(ctx, p.pool, sq.And{
		sq.Eq{"package": pkg},
		sq.Expr("result->'started_at' >= ?", from),
		sq.Expr("result->'started_at' <= ?", to),
	}, 0)
}

func (p *PG) EnqueueRun(ctx context.Context, run *tester.Run) error {
	r := (*pgRun)(run)
	q := psq.Insert("runs").
		Columns(r.Columns()...).
		Values(r.Values()...)

	sql, args, err := q.ToSql()
	if err != nil {
		return err
	}

	_, err = p.pool.Exec(ctx, sql, args...)
	return err
}

func (p *PG) StartRun(ctx context.Context, id uuid.UUID, runner string) error {
	return p.tx(ctx, func(tx pgx.Tx) error {
		r := &pgRun{}
		q := psq.Select(r.Columns()...).
			From("runs").
			Where("id = ?", id)

		sql, args, err := q.ToSql()
		if err != nil {
			return err
		}

		row := p.pool.QueryRow(ctx, sql, args...)
		err = r.Scan(row)
		if err != nil {
			return err
		}

		r.Meta.Runner = runner

		uq := psq.Update("runs").
			Set("started_at", p.now()).
			Set("meta", r.Meta).
			Where("id = ?", id)

		sql, args, err = uq.ToSql()
		if err != nil {
			return err
		}

		_, err = p.pool.Exec(ctx, sql, args...)
		return err
	})

}

func (p *PG) ResetRun(ctx context.Context, id uuid.UUID) error {
	q := psq.Update("runs").
		SetMap(map[string]interface{}{
			"started_at":  sql.NullTime{},
			"finished_at": sql.NullTime{},
			"error":       sql.NullString{},
			"meta":        tester.RunMeta{},
		}).
		Where("id = ?", id).
		Where("finished_at IS NULL")

	sql, args, err := q.ToSql()
	if err != nil {
		return err
	}

	res, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PG) DeleteRun(ctx context.Context, id uuid.UUID) error {
	q := psq.Delete("runs").
		Where("id = ?", id)

	sql, args, err := q.ToSql()
	if err != nil {
		return err
	}

	_, err = p.pool.Exec(ctx, sql, args...)
	return err
}

func (p *PG) CompleteRun(ctx context.Context, id uuid.UUID) error {
	q := psq.Update("runs").
		Set("finished_at", sql.NullTime{Valid: true, Time: p.now()}).
		Where("id = ?", id)

	sql, args, err := q.ToSql()
	if err != nil {
		return err
	}

	_, err = p.pool.Exec(ctx, sql, args...)
	return err
}

func (p *PG) FailRun(ctx context.Context, id uuid.UUID, error string) error {
	q := psq.Update("runs").
		SetMap(map[string]interface{}{
			"finished_at": sql.NullTime{Valid: true, Time: p.now()},
			"error":       sql.NullString{Valid: true, String: error},
		}).
		Where("id = ?", id)

	sql, args, err := q.ToSql()
	if err != nil {
		return err
	}

	_, err = p.pool.Exec(ctx, sql, args...)
	return err
}

func (p *PG) GetRun(ctx context.Context, id uuid.UUID) (*tester.Run, error) {
	var run *tester.Run
	err := p.tx(ctx, func(tx pgx.Tx) error {
		r := &pgRun{}
		q := psq.Select(r.Columns()...).
			From("runs").
			Where("id = ?", id)

		sql, args, err := q.ToSql()
		if err != nil {
			return err
		}

		row := p.pool.QueryRow(ctx, sql, args...)
		err = r.Scan(row)
		if err != nil {
			return err
		}
		run = (*tester.Run)(r)
		tests, err := p.listTests(ctx, tx, sq.Eq{"run_id": id}, 0)
		if err != nil {
			return err
		}

		run.Tests = tests
		return nil
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (p *PG) listRuns(ctx context.Context, pg pger, pred interface{}, order string, limit int) ([]*tester.Run, error) {
	var runs []*tester.Run
	q := psq.Select((&pgRun{}).Columns()...).
		From("runs")

	if pred != nil {
		q = q.Where(pred)
	}
	if order != "" {
		q = q.OrderBy(order)
	}
	if limit > 0 {
		q = q.Limit(uint64(limit))
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := pg.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runMap := make(map[uuid.UUID]*tester.Run)
	for rows.Next() {
		r := &pgRun{}
		err := r.Scan(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, (*tester.Run)(r))
		runMap[r.ID] = (*tester.Run)(r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var runIDs []uuid.UUID
	for id := range runMap {
		runIDs = append(runIDs, id)
	}

	tests, err := p.listTests(ctx, pg, sq.Eq{"run_id": runIDs}, 0)
	if err != nil {
		return nil, err
	}

	for _, test := range tests {
		runMap[test.RunID].Tests = append(runMap[test.RunID].Tests, test)
	}

	return runs, nil
}

func (p *PG) ListPendingRuns(ctx context.Context) ([]*tester.Run, error) {
	var runs []*tester.Run
	err := p.tx(ctx, func(tx pgx.Tx) error {
		var err error
		runs, err = p.listRuns(ctx, tx, "finished_at IS NULL", "enqueued_at ASC", 0)
		return err
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

func (p *PG) ListFinishedRuns(ctx context.Context, limit int) ([]*tester.Run, error) {
	var runs []*tester.Run
	err := p.tx(ctx, func(tx pgx.Tx) error {
		var err error
		runs, err = p.listRuns(ctx, tx, "finished_at IS NOT NULL", "finished_at DESC", limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

func (p *PG) ListRunsForPackage(ctx context.Context, pkg string, limit int) ([]*tester.Run, error) {
	var runs []*tester.Run
	err := p.tx(ctx, func(tx pgx.Tx) error {
		var err error
		runs, err = p.listRuns(ctx, tx, sq.Eq{"package": pkg}, "enqueued_at DESC", limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

func (p *PG) ListRunSummariesInRange(ctx context.Context, begin, end time.Time, window time.Duration) ([]*tester.RunSummary, error) {
	begin = begin.UTC()
	end = end.UTC()

	buckets := int(math.Ceil(float64(end.Sub(begin)) / float64(window)))
	summaries := make([]*tester.RunSummary, buckets)
	for i := 0; i < buckets; i++ {
		summaries[i] = &tester.RunSummary{
			Time:           begin.Add(time.Duration(i) * window),
			Duration:       window,
			PackageSummary: make(map[string]*tester.PackageSummary),
		}
	}

	err := p.tx(ctx, func(tx pgx.Tx) error {
		q := psq.Select("runs.package", "runs.id", "runs.started_at", "runs.error", "tests.id", "tests.result").
			From("tests").
			Join("runs ON tests.run_id = runs.id").
			Where("runs.started_at IS NOT NULL").
			Where("runs.started_at >= ?", begin).
			Where("runs.started_at <= ?", end).
			Where("runs.finished_at IS NOT NULL").
			OrderBy("runs.started_at ASC")

		query, args, err := q.ToSql()
		if err != nil {
			return err
		}

		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				packageName  string
				runID        uuid.UUID
				runStartedAt time.Time
				runError     sql.NullString
				testID       uuid.UUID
				result       tester.T
			)
			err := rows.Scan(&packageName, &runID, &runStartedAt, &runError, &testID, &result)
			if err != nil {
				return err
			}
			runStartedAt = runStartedAt.UTC()

			bucketIndex := int(runStartedAt.Sub(begin) / window)
			summary := summaries[bucketIndex]

			packageSummary, ok := summary.PackageSummary[packageName]
			if !ok {
				packageSummary = &tester.PackageSummary{
					Package:      packageName,
					PassedTests:  make(map[string][]uuid.UUID),
					FailedTests:  make(map[string][]uuid.UUID),
					SkippedTests: make(map[string][]uuid.UUID),
				}
				summary.PackageSummary[packageName] = packageSummary
			}

			// NOTE(nan) we blindly add here and uniquify later.
			if runError.Valid {
				packageSummary.ErrorRunIDs = append(packageSummary.ErrorRunIDs, runID)
				continue
			}
			packageSummary.RunIDs = append(packageSummary.RunIDs, runID)

			switch result.State {
			case tester.TBStatePassed:
				packageSummary.PassedTests[result.Name] = append(packageSummary.PassedTests[result.Name], testID)
			case tester.TBStateFailed:
				packageSummary.FailedTests[result.Name] = append(packageSummary.FailedTests[result.Name], testID)
			case tester.TBStateSkipped:
				packageSummary.SkippedTests[result.Name] = append(packageSummary.SkippedTests[result.Name], testID)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	for _, summary := range summaries {
		for _, packageSummary := range summary.PackageSummary {
			if len(packageSummary.RunIDs) == 0 {
				continue
			}

			var runIDs []uuid.UUID
			uniqueRunIDs := make(map[uuid.UUID]struct{})
			for _, id := range packageSummary.RunIDs {
				if _, exists := uniqueRunIDs[id]; exists {
					continue
				}
				uniqueRunIDs[id] = struct{}{}
				runIDs = append(runIDs, id)
			}
			packageSummary.RunIDs = runIDs

			var errorRunIDs []uuid.UUID
			uniqueErrorRunIDs := make(map[uuid.UUID]struct{})
			for _, id := range packageSummary.ErrorRunIDs {
				if _, exists := uniqueErrorRunIDs[id]; exists {
					continue
				}
				uniqueErrorRunIDs[id] = struct{}{}
				errorRunIDs = append(errorRunIDs, id)
			}
			packageSummary.ErrorRunIDs = errorRunIDs
		}
	}
	return summaries, nil
}
