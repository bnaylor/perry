# Audit Record Persistence — Design

**Date:** 2026-03-01
**Status:** Approved
**Phase:** 3b (remaining)

## Problem

The orchestrator computes rich audit data (gate verdicts, findings, executor results, token usage) at every pipeline stage, then discards it after branching. The only persistence is `task.History[]` with freeform strings like `"audit rejected"`, stored in an in-memory `MemStore` that's lost when the process exits.

## Decision: SQLite

**Backend:** SQLite via `modernc.org/sqlite` (pure Go, no CGO).

**Rationale:** A single-file embedded database gives us both per-task forensics ("why did task X fail?") and cross-task analytics ("how many tokens this week?") with zero infrastructure. Flat JSON files would require custom aggregation code later. SQLite is the standard choice for CLI tools that need local persistence.

**Location:** `.perry/perry.db` by default, overridable with `--db-path` CLI flag. `.perry/` is gitignored.

## Schema

Four tables, versioned via a `schema_version` table with Go-based migrations on startup.

### `tasks`

Replaces `MemStore`. Implements the existing `task.Store` interface.

| Column | Type | Notes |
|---|---|---|
| id | TEXT PK | Task ID |
| description | TEXT | |
| created_by | TEXT | |
| state | TEXT | Current FSM state |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

### `transitions`

Replaces `task.History[]`. Also stores executor metadata when transitioning from `StateExecuting`.

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | Auto-increment |
| task_id | TEXT FK | References tasks(id) |
| from_state | TEXT | |
| to_state | TEXT | |
| reason | TEXT | Human-readable reason |
| exit_code | INTEGER NULL | Executor exit code (only on execution transitions) |
| logs | TEXT NULL | Executor stdout/stderr (only on execution transitions) |
| artifact_path | TEXT NULL | Managed path in .perry/artifacts/ (only on execution transitions) |
| created_at | TIMESTAMP | |

### `audit_records`

One row per gate execution.

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | Auto-increment |
| task_id | TEXT FK | References tasks(id) |
| pipeline | TEXT | "code" or "packet" |
| gate | TEXT | "ast", "secrets", "schema_validation", "content_scan" |
| pass | BOOLEAN | |
| findings | TEXT | JSON array of finding strings |
| created_at | TIMESTAMP | |

### `agent_calls`

One row per LLM invocation. Covers all roles including strategist (currently discarded).

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | Auto-increment |
| task_id | TEXT FK | References tasks(id) |
| role | TEXT | strategist, researcher, coder, auditor, shadow_auditor |
| provider | TEXT | anthropic, gemini, ollama |
| model | TEXT | Specific model name |
| input_tokens | INTEGER | |
| output_tokens | INTEGER | |
| content | TEXT | Raw LLM response |
| created_at | TIMESTAMP | |

## Package: `internal/storage`

New package owns the DB connection, migrations, and all queries.

### API

```go
// NewStore opens (or creates) the SQLite database and runs migrations.
func NewStore(dbPath string) (*Store, error)

// task.Store interface methods (replacing MemStore):
func (s *Store) Create(t task.Task) error
func (s *Store) Get(id string) (task.Task, error)
func (s *Store) Update(t task.Task) error
func (s *Store) List() ([]task.Task, error)

// Audit-specific methods:
func (s *Store) RecordTransition(taskID, from, to, reason string) error
func (s *Store) RecordAuditGate(taskID, pipeline, gate string, pass bool, findings []string) error
func (s *Store) RecordAgentCall(taskID, role, provider, model string, inputTokens, outputTokens int, content string) error
func (s *Store) RecordExecution(taskID string, exitCode int, logs, artifactPath string) error
func (s *Store) MoveArtifacts(taskID, tempDir string) (string, error)
```

`MoveArtifacts` moves files from the executor's temp output dir to `.perry/artifacts/<taskID>/` and returns the managed path.

## Integration Points

### Orchestrator

- Accepts `*storage.Store` instead of `task.Store` (needs audit-specific methods beyond the interface)
- After each gate: `RecordAuditGate` with the `GateResult`
- After each LLM call: `RecordAgentCall` with the `AgentOutput`
- In `StateExecuting`: `MoveArtifacts` to relocate output, `RecordExecution` with exit code/logs/managed path
- Store strategist output (currently discarded)

### CLI

- `--db-path` flag, default `.perry/perry.db`
- Create `.perry/` directory on startup
- Add `.perry/` to `.gitignore` if not present

## Error Handling

Storage failures for audit records (gate results, agent calls) are **logged and swallowed** — they must not crash the pipeline. Audit persistence is observability, not control flow.

Exception: `task.Store` operations (create/update task) remain fatal since the orchestrator cannot function without task state.

## Testing

- **Unit tests** for `storage.Store` using `:memory:` SQLite — fast, no disk cleanup
- **Interface compliance** — existing `MemStore` test patterns port to `storage.Store`
- **Audit record verification** — write records, query them back, assert correctness
- **Integration test** — full orchestrator cycle with `storage.Store`, assert records in all four tables
- **`MoveArtifacts`** — tested with real temp dirs and file contents

## Out of Scope

- Budget enforcement wiring (token usage is persisted but not checked against PolicyEngine)
- Query CLI (e.g. `perry audit show <taskID>`)
- Retention/cleanup policy
- Schema for Shadow Auditor PoC results (Gemini is designing that separately; can be added as a migration)
