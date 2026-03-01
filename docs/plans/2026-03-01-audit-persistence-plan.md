# Audit Record Persistence Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the in-memory `MemStore` with a SQLite-backed store that persists tasks, transitions, audit gate results, agent LLM calls, and executor outputs.

**Architecture:** New `internal/storage` package wraps `modernc.org/sqlite`. Implements `task.Store` interface for drop-in replacement. Adds audit-specific recording methods called by the orchestrator after each gate, LLM call, and execution. Executor output files are relocated to `.perry/artifacts/<taskID>/`.

**Tech Stack:** `modernc.org/sqlite` (pure Go, no CGO), `database/sql` stdlib

---

### Task 1: Add SQLite dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run: `cd /Users/bnaylor/src/perry && go get modernc.org/sqlite`

**Step 2: Verify it resolved**

Run: `grep modernc go.mod`
Expected: `modernc.org/sqlite` appears in require block

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add modernc.org/sqlite for audit persistence"
```

---

### Task 2: Create storage package with schema migrations

**Files:**
- Create: `internal/storage/store.go`
- Create: `internal/storage/migrations.go`
- Create: `internal/storage/store_test.go`

**Step 1: Write the failing test**

```go
// internal/storage/store_test.go
package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore_CreatesTablesInMemory(t *testing.T) {
	s, err := NewStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	// Verify all tables exist by querying sqlite_master
	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}

	assert.Contains(t, tables, "tasks")
	assert.Contains(t, tables, "transitions")
	assert.Contains(t, tables, "audit_records")
	assert.Contains(t, tables, "agent_calls")
	assert.Contains(t, tables, "schema_version")
}

func TestNewStore_MigrationsAreIdempotent(t *testing.T) {
	s, err := NewStore(":memory:")
	require.NoError(t, err)

	// Run migrations again — should not error
	err = s.migrate()
	assert.NoError(t, err)
	s.Close()
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run TestNewStore`
Expected: FAIL — package does not exist

**Step 3: Write migrations**

```go
// internal/storage/migrations.go
package storage

import "database/sql"

const currentVersion = 1

var migrations = map[int]string{
	1: `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
	id         TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	created_by TEXT NOT NULL,
	state      TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
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
```

**Step 4: Write Store struct and constructor**

```go
// internal/storage/store.go
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store provides durable persistence for tasks, audit records, and agent calls.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database and runs migrations.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	// Get current version
	var version int
	row := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&version); err != nil {
		// Table doesn't exist yet — that's fine, version stays 0
		version = 0
	}

	for v := version + 1; v <= currentVersion; v++ {
		sql, ok := migrations[v]
		if !ok {
			return fmt.Errorf("missing migration for version %d", v)
		}
		if _, err := s.db.Exec(sql); err != nil {
			return fmt.Errorf("migration v%d: %w", v, err)
		}
		if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", v); err != nil {
			return fmt.Errorf("record migration v%d: %w", v, err)
		}
	}
	return nil
}
```

**Step 5: Run tests to verify they pass**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run TestNewStore`
Expected: PASS (both tests)

**Step 6: Commit**

```bash
git add internal/storage/
git commit -m "feat(storage): add SQLite store with schema migrations"
```

---

### Task 3: Implement task.Store interface on storage.Store

**Files:**
- Modify: `internal/storage/store.go` (add methods)
- Create: `internal/storage/tasks_test.go`

**Step 1: Write the failing tests**

```go
// internal/storage/tasks_test.go
package storage

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_ImplementsTaskStore(t *testing.T) {
	s := newTestStore(t)
	// Compile-time check
	var _ task.Store = s
}

func TestStore_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(ctx, tk))

	got, err := s.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, tk.ID, got.ID)
	assert.Equal(t, tk.Description, got.Description)
	assert.Equal(t, tk.CreatedBy, got.CreatedBy)
	assert.Equal(t, tk.State, got.State)
}

func TestStore_CreateDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(ctx, tk))
	err := s.Create(ctx, tk)
	assert.Error(t, err)
}

func TestStore_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestStore_Update(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(ctx, tk))

	tk.State = task.StatePlanning
	require.NoError(t, s.Update(ctx, tk))

	got, err := s.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, task.StatePlanning, got.State)
}

func TestStore_UpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := task.New("test task", "tester")
	err := s.Update(ctx, tk)
	assert.Error(t, err)
}

func TestStore_List(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk1 := task.New("task one", "tester")
	tk2 := task.New("task two", "tester")
	require.NoError(t, s.Create(ctx, tk1))
	require.NoError(t, s.Create(ctx, tk2))

	tasks, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run TestStore_`
Expected: FAIL — methods not defined, interface not satisfied

**Step 3: Implement task.Store methods**

Add to `internal/storage/store.go`:

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/bnaylor/perry/internal/task"
	_ "modernc.org/sqlite"
)

func (s *Store) Create(_ context.Context, t *task.Task) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, description, created_by, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.Description, t.CreatedBy, string(t.State), t.CreatedAt, t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create task %s: %w", t.ID, err)
	}
	return nil
}

func (s *Store) Get(_ context.Context, id string) (*task.Task, error) {
	row := s.db.QueryRow(
		`SELECT id, description, created_by, state, created_at FROM tasks WHERE id = ?`, id,
	)
	t := &task.Task{}
	var state string
	if err := row.Scan(&t.ID, &t.Description, &t.CreatedBy, &state, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("task %s not found", id)
	}
	t.State = task.State(state)

	// Load transitions as History
	rows, err := s.db.Query(
		`SELECT from_state, to_state, reason, created_at FROM transitions
		 WHERE task_id = ? ORDER BY id`, id,
	)
	if err != nil {
		return nil, fmt.Errorf("load transitions for %s: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var tr task.Transition
		var from, to string
		if err := rows.Scan(&from, &to, &tr.Reason, &tr.At); err != nil {
			return nil, fmt.Errorf("scan transition: %w", err)
		}
		tr.From = task.State(from)
		tr.To = task.State(to)
		t.History = append(t.History, tr)
	}
	return t, nil
}

func (s *Store) Update(_ context.Context, t *task.Task) error {
	res, err := s.db.Exec(
		`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`,
		string(t.State), time.Now(), t.ID,
	)
	if err != nil {
		return fmt.Errorf("update task %s: %w", t.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %s not found", t.ID)
	}
	return nil
}

func (s *Store) List(_ context.Context) ([]*task.Task, error) {
	rows, err := s.db.Query(
		`SELECT id, description, created_by, state, created_at FROM tasks ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var tasks []*task.Task
	for rows.Next() {
		t := &task.Task{}
		var state string
		if err := rows.Scan(&t.ID, &t.Description, &t.CreatedBy, &state, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		t.State = task.State(state)
		tasks = append(tasks, t)
	}
	return tasks, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run TestStore_`
Expected: PASS (all 6 tests)

**Step 5: Commit**

```bash
git add internal/storage/
git commit -m "feat(storage): implement task.Store interface on SQLite"
```

---

### Task 4: Add audit record and agent call recording methods

**Files:**
- Modify: `internal/storage/store.go` (add methods)
- Create: `internal/storage/audit_test.go`

**Step 1: Write the failing tests**

```go
// internal/storage/audit_test.go
package storage

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestTask(t *testing.T, s *Store) *task.Task {
	t.Helper()
	tk := task.New("test task", "tester")
	require.NoError(t, s.Create(context.Background(), tk))
	return tk
}

func TestStore_RecordTransition(t *testing.T) {
	s := newTestStore(t)
	tk := createTestTask(t, s)

	err := s.RecordTransition(tk.ID, "SUBMITTED", "PLANNING", "task accepted")
	require.NoError(t, err)

	// Verify via Get (transitions load as History)
	got, err := s.Get(context.Background(), tk.ID)
	require.NoError(t, err)
	require.Len(t, got.History, 1)
	assert.Equal(t, task.State("SUBMITTED"), got.History[0].From)
	assert.Equal(t, task.State("PLANNING"), got.History[0].To)
	assert.Equal(t, "task accepted", got.History[0].Reason)
}

func TestStore_RecordTransitionWithExecution(t *testing.T) {
	s := newTestStore(t)
	tk := createTestTask(t, s)

	err := s.RecordExecution(tk.ID, 0, "hello world\n", "/tmp/artifacts/test")
	require.NoError(t, err)

	// Verify the execution record exists
	var exitCode int
	var logs, artifactPath string
	row := s.db.QueryRow(
		`SELECT exit_code, logs, artifact_path FROM transitions WHERE task_id = ? AND exit_code IS NOT NULL`, tk.ID,
	)
	require.NoError(t, row.Scan(&exitCode, &logs, &artifactPath))
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello world\n", logs)
	assert.Equal(t, "/tmp/artifacts/test", artifactPath)
}

func TestStore_RecordAuditGate(t *testing.T) {
	s := newTestStore(t)
	tk := createTestTask(t, s)

	err := s.RecordAuditGate(tk.ID, "code", "ast", true, nil)
	require.NoError(t, err)

	err = s.RecordAuditGate(tk.ID, "code", "secrets", false, []string{"hardcoded password on line 5"})
	require.NoError(t, err)

	// Verify
	rows, err := s.db.Query(`SELECT pipeline, gate, pass, findings FROM audit_records WHERE task_id = ? ORDER BY id`, tk.ID)
	require.NoError(t, err)
	defer rows.Close()

	var records []struct {
		pipeline, gate, findings string
		pass                     bool
	}
	for rows.Next() {
		var r struct {
			pipeline, gate, findings string
			pass                     bool
		}
		require.NoError(t, rows.Scan(&r.pipeline, &r.gate, &r.pass, &r.findings))
		records = append(records, r)
	}
	require.Len(t, records, 2)
	assert.True(t, records[0].pass)
	assert.False(t, records[1].pass)
	assert.Contains(t, records[1].findings, "hardcoded password")
}

func TestStore_RecordAgentCall(t *testing.T) {
	s := newTestStore(t)
	tk := createTestTask(t, s)

	err := s.RecordAgentCall(tk.ID, "coder", "anthropic", "claude-sonnet-4-20250514", 500, 1200, `{"code": "print('hello')"}`)
	require.NoError(t, err)

	var role, provider, model, content string
	var inputTokens, outputTokens int
	row := s.db.QueryRow(`SELECT role, provider, model, input_tokens, output_tokens, content FROM agent_calls WHERE task_id = ?`, tk.ID)
	require.NoError(t, row.Scan(&role, &provider, &model, &inputTokens, &outputTokens, &content))
	assert.Equal(t, "coder", role)
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, 500, inputTokens)
	assert.Equal(t, 1200, outputTokens)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run "TestStore_Record"`
Expected: FAIL — methods not defined

**Step 3: Implement recording methods**

Add to `internal/storage/store.go`:

```go
import "encoding/json"

func (s *Store) RecordTransition(taskID, from, to, reason string) error {
	_, err := s.db.Exec(
		`INSERT INTO transitions (task_id, from_state, to_state, reason) VALUES (?, ?, ?, ?)`,
		taskID, from, to, reason,
	)
	if err != nil {
		return fmt.Errorf("record transition: %w", err)
	}
	return nil
}

func (s *Store) RecordExecution(taskID string, exitCode int, logs, artifactPath string) error {
	_, err := s.db.Exec(
		`INSERT INTO transitions (task_id, from_state, to_state, reason, exit_code, logs, artifact_path)
		 VALUES (?, 'EXECUTING', '', 'execution result', ?, ?, ?)`,
		taskID, exitCode, logs, artifactPath,
	)
	if err != nil {
		return fmt.Errorf("record execution: %w", err)
	}
	return nil
}

func (s *Store) RecordAuditGate(taskID, pipeline, gate string, pass bool, findings []string) error {
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return fmt.Errorf("marshal findings: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO audit_records (task_id, pipeline, gate, pass, findings) VALUES (?, ?, ?, ?, ?)`,
		taskID, pipeline, gate, pass, string(findingsJSON),
	)
	if err != nil {
		return fmt.Errorf("record audit gate: %w", err)
	}
	return nil
}

func (s *Store) RecordAgentCall(taskID, role, provider, model string, inputTokens, outputTokens int, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO agent_calls (task_id, role, provider, model, input_tokens, output_tokens, content)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, role, provider, model, inputTokens, outputTokens, content,
	)
	if err != nil {
		return fmt.Errorf("record agent call: %w", err)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run "TestStore_Record"`
Expected: PASS (all 4 tests)

**Step 5: Commit**

```bash
git add internal/storage/
git commit -m "feat(storage): add audit gate, agent call, and execution recording"
```

---

### Task 5: Add MoveArtifacts method

**Files:**
- Modify: `internal/storage/store.go` (add method)
- Create: `internal/storage/artifacts_test.go`

**Step 1: Write the failing test**

```go
// internal/storage/artifacts_test.go
package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_MoveArtifacts(t *testing.T) {
	s := newTestStore(t)
	baseDir := t.TempDir()
	s.artifactDir = baseDir

	// Create a fake executor output dir with a file
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "output.txt"), []byte("hello"), 0644))

	managedPath, err := s.MoveArtifacts("task-abc123", srcDir)
	require.NoError(t, err)

	// Verify file was moved
	assert.DirExists(t, managedPath)
	content, err := os.ReadFile(filepath.Join(managedPath, "output.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))

	// Verify source dir no longer exists
	_, err = os.Stat(srcDir)
	assert.True(t, os.IsNotExist(err))
}

func TestStore_MoveArtifacts_EmptyDir(t *testing.T) {
	s := newTestStore(t)
	baseDir := t.TempDir()
	s.artifactDir = baseDir

	srcDir := t.TempDir()
	managedPath, err := s.MoveArtifacts("task-abc123", srcDir)
	require.NoError(t, err)
	assert.Empty(t, managedPath)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run TestStore_MoveArtifacts`
Expected: FAIL — method and field not defined

**Step 3: Implement MoveArtifacts**

Add `artifactDir` field to `Store` struct and update constructor. Add to `internal/storage/store.go`:

```go
type Store struct {
	db          *sql.DB
	artifactDir string
}

// Update NewStore to accept and store artifactDir:
func NewStore(dbPath string) (*Store, error) {
	// ... existing code ...
	artifactDir := ""
	if dbPath != ":memory:" {
		artifactDir = filepath.Join(filepath.Dir(dbPath), "artifacts")
	}
	s := &Store{db: db, artifactDir: artifactDir}
	// ...
}

func (s *Store) MoveArtifacts(taskID, srcDir string) (string, error) {
	// Check if source dir has any files
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", fmt.Errorf("read source dir: %w", err)
	}
	if len(entries) == 0 {
		os.RemoveAll(srcDir)
		return "", nil
	}

	destDir := filepath.Join(s.artifactDir, taskID)
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return "", fmt.Errorf("create artifact parent dir: %w", err)
	}

	if err := os.Rename(srcDir, destDir); err != nil {
		return "", fmt.Errorf("move artifacts: %w", err)
	}
	return destDir, nil
}
```

Add `"os"` and `"path/filepath"` to imports.

**Step 4: Run tests to verify they pass**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v -run TestStore_MoveArtifacts`
Expected: PASS

**Step 5: Run all storage tests**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/storage/ -v`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add internal/storage/
git commit -m "feat(storage): add MoveArtifacts for executor output relocation"
```

---

### Task 6: Wire storage.Store into orchestrator

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:19-58` (Config and constructor)
- Modify: `internal/orchestrator/orchestrator.go:129-290` (state handlers — add recording calls)

This is the largest task. The orchestrator needs to:
1. Accept `*storage.Store` in Config (replacing `task.Store`)
2. After each audit gate result: call `RecordAuditGate`
3. After each LLM agent call: call `RecordAgentCall`
4. In `StateExecuting`: call `MoveArtifacts` + `RecordExecution`
5. On each transition: call `RecordTransition`
6. Store strategist output (currently discarded)

**Step 1: Write the failing test**

Add a new test file that verifies audit records are created after a pipeline run:

```go
// internal/orchestrator/persistence_test.go
package orchestrator

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_PersistsAuditRecords(t *testing.T) {
	store, err := storage.NewStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Build orchestrator with store instead of MemStore
	// Use existing test helpers/mocks from orchestrator_test.go
	orch := NewOrchestrator(Config{
		// ... wire with store and mocks ...
		Store: store,
	})
	_ = orch // Compile check that Config.Store accepts *storage.Store
}
```

The exact test wiring depends on what mocks exist in `orchestrator_test.go`. The implementer should adapt using the existing test patterns.

**Step 2: Run test to verify it fails**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/orchestrator/ -v -run TestOrchestrator_Persists`
Expected: FAIL — Config.Store type mismatch

**Step 3: Update Config to accept *storage.Store**

In `internal/orchestrator/orchestrator.go`, change the `Config` struct:

```go
import "github.com/bnaylor/perry/internal/storage"

type Config struct {
	FSM         *fsm.Machine
	Store       *storage.Store    // was: task.Store
	Runner      *agent.Runner
	Dispatcher  *dispatch.Dispatcher
	Policy      *policy.Engine
	Audit       *audit.Pipeline
	PacketAudit *audit.Pipeline
	Executor    executor.Executor
	Notary      notary.Notary
}
```

Update the `Orchestrator` struct field and all method calls that use `o.store` to work with `*storage.Store`.

**Step 4: Add recording calls to state handlers**

In each state handler within `determineNextState`, add recording calls. These calls should **log and swallow errors** (not return them). Example pattern:

```go
// After audit.Pipeline.Run in StateAuditing handler:
result, err := o.audit.Run(ctx, audit.AuditInput{Code: code})
// Record each gate result (log errors, don't fail the pipeline)
for _, gr := range result.GateResults {
	if recErr := o.store.RecordAuditGate(tk.ID, "code", gr.Gate, gr.Pass, gr.Findings); recErr != nil {
		log.Printf("WARN: failed to record audit gate: %v", recErr)
	}
}
```

Apply the same pattern to:
- `StatePacketValidation` — record with pipeline="packet"
- `StateExecuting` — call `MoveArtifacts`, then `RecordExecution`
- Every LLM agent call (strategist, researcher, coder, shadow_auditor) — `RecordAgentCall`
- Every state transition — `RecordTransition` (in the FSM `OnTransition` callback or in `Step`)

**Step 5: Store strategist output**

In the `StatePlanning` handler (around line 115), the runner result is currently used and discarded. Add:

```go
o.outputs[outputKey(tk.ID, agent.RoleStrategist)] = output
```

**Step 6: Run all orchestrator tests**

Run: `cd /Users/bnaylor/src/perry && go test ./internal/orchestrator/ -v`
Expected: Existing tests may need updating to use `storage.NewStore(":memory:")` instead of `task.NewMemStore()`. Fix as needed.

**Step 7: Run full test suite**

Run: `cd /Users/bnaylor/src/perry && go test ./... -count=1`
Expected: All PASS (except integration tests which require Docker/env vars)

**Step 8: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat(orchestrator): wire storage.Store for audit persistence"
```

---

### Task 7: Wire storage.Store into CLI

**Files:**
- Modify: `cmd/perry/main.go:76-95` (replace MemStore with storage.Store, add --db-path flag)

**Step 1: Update CLI to use storage.Store**

```go
// In main.go, replace:
//   Store: task.NewMemStore(),
// With:
dbPath := ".perry/perry.db"
// Parse --db-path flag if provided (use flag package or os.Args)
if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
	log.Fatalf("create .perry dir: %v", err)
}
store, err := storage.NewStore(dbPath)
if err != nil {
	log.Fatalf("open database: %v", err)
}
defer store.Close()

// ...
orch := orchestrator.NewOrchestrator(orchestrator.Config{
	// ...
	Store: store,
	// ...
})
```

**Step 2: Add .perry/ to .gitignore**

Check if `.gitignore` exists. If so, append `.perry/`. If not, create it with `.perry/` and `/bin/`.

**Step 3: Build and smoke test**

Run: `cd /Users/bnaylor/src/perry && go build -o bin/perry ./cmd/perry/ && ls -la .perry/`
Expected: Binary builds. After first run, `.perry/perry.db` exists.

**Step 4: Commit**

```bash
git add cmd/perry/main.go .gitignore
git commit -m "feat(cli): wire SQLite store with --db-path flag"
```

---

### Task 8: Run full test suite and verify

**Step 1: Run all unit tests**

Run: `cd /Users/bnaylor/src/perry && go test ./... -count=1 -v 2>&1 | tail -30`
Expected: All PASS

**Step 2: Run go vet**

Run: `cd /Users/bnaylor/src/perry && go vet ./...`
Expected: Clean

**Step 3: Verify no MemStore references remain (except the file itself)**

Run: `grep -r "NewMemStore" --include="*.go" .`
Expected: Only `internal/task/memstore.go` and its test file. No references in `cmd/` or `orchestrator/`.

**Step 4: Commit any fixes, then final commit**

```bash
git add -A
git commit -m "chore: clean up MemStore references, all tests green"
```
