package storage

const currentVersion = 1

var migrations = map[int]string{
	1: `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
	id          TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	created_by  TEXT NOT NULL,
	state       TEXT NOT NULL,
	created_at  TIMESTAMP NOT NULL,
	updated_at  TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS transitions (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id       TEXT NOT NULL REFERENCES tasks(id),
	from_state    TEXT NOT NULL,
	to_state      TEXT NOT NULL,
	reason        TEXT NOT NULL,
	exit_code     INTEGER,
	logs          TEXT,
	artifact_path TEXT,
	created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_records (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id    TEXT NOT NULL REFERENCES tasks(id),
	pipeline   TEXT NOT NULL,
	gate       TEXT NOT NULL,
	pass       BOOLEAN NOT NULL,
	findings   TEXT,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_calls (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id       TEXT NOT NULL REFERENCES tasks(id),
	role          TEXT NOT NULL,
	provider      TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	content       TEXT,
	created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_transitions_task_id ON transitions(task_id);
CREATE INDEX IF NOT EXISTS idx_audit_records_task_id ON audit_records(task_id);
CREATE INDEX IF NOT EXISTS idx_agent_calls_task_id ON agent_calls(task_id);
`,
}
