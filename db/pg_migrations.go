package db

var pgMigrations = []struct{
	name string
	up string
	down string
}{
	{
		name: "initial",
		up: `
CREATE TABLE tests (
	id uuid PRIMARY KEY,
	package varchar(255) NOT NULL,
	run_id uuid NOT NULL,
	result jsonb NOT NULL,
	logs jsonb NOT NULL
);
CREATE INDEX ON tests (package);
CREATE INDEX ON tests ((result->'started_at'));

CREATE TABLE runs (
	id uuid PRIMARY KEY,
	package varchar(255) NOT NULL,
	args varchar(255)[],
	enqueued_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at timestamptz,
	finished_at timestamptz,
	error text
);
CREATE INDEX ON runs (package);
CREATE INDEX ON runs (enqueued_at, started_at);
CREATE INDEX ON runs (finished_at);
`,
		down: `
DROP TABLE tests, runs;
`,
	},
}
