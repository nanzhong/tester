package db

import (
	"database/sql"

	"github.com/jackc/pgx/v4"
	"github.com/lib/pq"
	"github.com/nanzhong/tester"
)

type pgTest tester.Test

func (t *pgTest) Columns() []string {
	return []string{
		"id",
		"package",
		"run_id",
		"result",
		"logs",
	}
}

func (t *pgTest) Values() []interface{} {
	return []interface{}{
		t.ID,
		t.Package,
		t.RunID,
		t.Result,
		t.Logs,
	}
}

func (t *pgTest) Scan(row pgx.Row) error {
	err := row.Scan(
		&t.ID,
		&t.Package,
		&t.RunID,
		&t.Result,
		&t.Logs,
	)
	if err != nil && err == pgx.ErrNoRows {
		err = ErrNotFound
	}
	return err
}

type pgRun tester.Run

func (r *pgRun) Columns() []string {
	return []string{
		"id",
		"package",
		"args",
		"enqueued_at",
		"started_at",
		"finished_at",
		"error",
	}
}

func (r *pgRun) Values() []interface{} {
	startedAt := sql.NullTime{Valid: !r.StartedAt.IsZero(), Time: r.StartedAt}
	finishedAt := sql.NullTime{Valid: !r.FinishedAt.IsZero(), Time: r.FinishedAt}
	error := sql.NullString{Valid: r.Error != "", String: r.Error}

	return []interface{}{
		r.ID,
		r.Package,
		pq.Array(r.Args),
		r.EnqueuedAt,
		startedAt,
		finishedAt,
		error,
	}
}

func (r *pgRun) Scan(row pgx.Row) error {
	var (
		startedAt  sql.NullTime
		finishedAt sql.NullTime
		error      sql.NullString
	)

	err := row.Scan(
		&r.ID,
		&r.Package,
		pq.Array(&r.Args),
		&r.EnqueuedAt,
		&startedAt,
		&finishedAt,
		&error,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			err = ErrNotFound
		}
		return err
	}

	if startedAt.Valid {
		r.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		r.FinishedAt = finishedAt.Time
	}
	if error.Valid {
		r.Error = error.String
	}
	return nil
}
