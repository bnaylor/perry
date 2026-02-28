# Orchestration Skeleton Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build perry's Go orchestration skeleton with a working FSM, Dispatcher, Policy Engine, LLM provider layer, audit pipeline coordination, and mock agents — so tasks flow through the full state machine with deterministic enforcement.

**Architecture:** Go binary with internal packages. Python audit tooling invoked as subprocesses. No external frameworks — plain state machine with explicit transition table. See `docs/plans/2026-02-27-orchestration-skeleton-design.md` for full design.

**Tech Stack:** Go 1.25, Python 3.14, `github.com/stretchr/testify` for Go test assertions, `jsonschema` for Python schema validation, `bandit` for static analysis.

---

### Task 1: Project Scaffold and Go Module

**Files:**
- Create: `go.mod`
- Create: `cmd/perry/main.go`
- Create: `Makefile`
- Create: `.gitignore` (update existing)

**Step 1: Initialize Go module**

Run: `go mod init github.com/bnaylor/perry`

**Step 2: Create minimal main.go**

```go
// cmd/perry/main.go
package main

import "fmt"

func main() {
	fmt.Println("perry: secure agentic platform")
}
```

**Step 3: Create Makefile**

```makefile
.PHONY: build test lint clean

build:
	go build -o bin/perry ./cmd/perry

test:
	go test ./... -v -race

lint:
	go vet ./...

clean:
	rm -rf bin/
```

**Step 4: Update .gitignore**

```
bin/
*.exe
*.test
*.out
__pycache__/
*.pyc
.venv/
```

**Step 5: Verify build and run**

Run: `make build && ./bin/perry`
Expected: `perry: secure agentic platform`

**Step 6: Commit**

```bash
git add go.mod cmd/ Makefile .gitignore
git commit -m "feat: initialize Go module and project scaffold"
```

---

### Task 2: Task Model

The task is the central data object that flows through the FSM. Every other package depends on this.

**Files:**
- Create: `internal/task/task.go`
- Create: `internal/task/task_test.go`

**Step 1: Write the failing test**

```go
// internal/task/task_test.go
package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTask(t *testing.T) {
	tk := New("Build a weather CLI", "user-1")
	assert.NotEmpty(t, tk.ID)
	assert.Equal(t, "Build a weather CLI", tk.Description)
	assert.Equal(t, "user-1", tk.CreatedBy)
	assert.Equal(t, StateSubmitted, tk.State)
	assert.NotZero(t, tk.CreatedAt)
	assert.Empty(t, tk.History)
}

func TestTaskRecordTransition(t *testing.T) {
	tk := New("test task", "user-1")
	tk.RecordTransition(StateSubmitted, StatePlanning, "auto")

	require.Len(t, tk.History, 1)
	assert.Equal(t, StateSubmitted, tk.History[0].From)
	assert.Equal(t, StatePlanning, tk.History[0].To)
	assert.Equal(t, "auto", tk.History[0].Reason)
	assert.NotZero(t, tk.History[0].At)
}

func TestTaskRetryCount(t *testing.T) {
	tk := New("test task", "user-1")
	assert.Equal(t, 0, tk.RetryCount(StateCoding, StateAuditing))

	tk.RecordTransition(StateCoding, StateAuditing, "submit")
	tk.RecordTransition(StateAuditing, StateCoding, "revision")
	tk.RecordTransition(StateCoding, StateAuditing, "submit")

	assert.Equal(t, 2, tk.RetryCount(StateCoding, StateAuditing))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/task/ -v`
Expected: Compilation failure — package doesn't exist yet.

**Step 3: Install testify and write implementation**

Run: `go get github.com/stretchr/testify`

```go
// internal/task/task.go
package task

import (
	"crypto/rand"
	"fmt"
	"time"
)

// State represents a task's position in the FSM.
type State string

const (
	StateSubmitted        State = "SUBMITTED"
	StatePlanning         State = "PLANNING"
	StateResearching      State = "RESEARCHING"
	StatePacketValidation State = "PACKET_VALIDATION"
	StateCoding           State = "CODING"
	StateAuditing         State = "AUDITING"
	StateExecuting        State = "EXECUTING"
	StateOutputReview     State = "OUTPUT_REVIEW"
	StateCompleted        State = "COMPLETED"
	StateFailed           State = "FAILED"
	StateHumanReview      State = "HUMAN_REVIEW"
)

// Transition records a single state change.
type Transition struct {
	From   State
	To     State
	Reason string
	At     time.Time
}

// Task is the central data object flowing through the FSM.
type Task struct {
	ID          string
	Description string
	CreatedBy   string
	State       State
	CreatedAt   time.Time
	History     []Transition
}

// New creates a task in the SUBMITTED state.
func New(description, createdBy string) *Task {
	b := make([]byte, 8)
	rand.Read(b)
	return &Task{
		ID:          fmt.Sprintf("task-%x", b),
		Description: description,
		CreatedBy:   createdBy,
		State:       StateSubmitted,
		CreatedAt:   time.Now(),
	}
}

// RecordTransition appends a transition to the task's history.
func (t *Task) RecordTransition(from, to State, reason string) {
	t.History = append(t.History, Transition{
		From:   from,
		To:     to,
		Reason: reason,
		At:     time.Now(),
	})
}

// RetryCount returns how many times a specific transition has occurred.
func (t *Task) RetryCount(from, to State) int {
	count := 0
	for _, h := range t.History {
		if h.From == from && h.To == to {
			count++
		}
	}
	return count
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/task/ -v`
Expected: All 3 tests PASS.

**Step 5: Commit**

```bash
git add internal/task/ go.mod go.sum
git commit -m "feat: add task model with state, history, and retry tracking"
```

---

### Task 3: State Machine (FSM)

The core spine. Enforces legal transitions and fires hooks.

**Files:**
- Create: `internal/fsm/states.go`
- Create: `internal/fsm/machine.go`
- Create: `internal/fsm/machine_test.go`

**Step 1: Write failing tests**

```go
// internal/fsm/machine_test.go
package fsm

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidTransition(t *testing.T) {
	m := New()
	tk := task.New("test", "user-1")

	err := m.Transition(context.Background(), tk, task.StatePlanning, "start")
	require.NoError(t, err)
	assert.Equal(t, task.StatePlanning, tk.State)
	assert.Len(t, tk.History, 1)
}

func TestInvalidTransition(t *testing.T) {
	m := New()
	tk := task.New("test", "user-1")

	// Can't go directly from SUBMITTED to CODING
	err := m.Transition(context.Background(), tk, task.StateCoding, "skip")
	assert.Error(t, err)
	assert.Equal(t, task.StateSubmitted, tk.State) // unchanged
	assert.Empty(t, tk.History)
}

func TestTransitionHookFires(t *testing.T) {
	m := New()
	hookCalled := false
	m.OnTransition(func(tk *task.Task, from, to task.State) {
		hookCalled = true
	})

	tk := task.New("test", "user-1")
	err := m.Transition(context.Background(), tk, task.StatePlanning, "start")
	require.NoError(t, err)
	assert.True(t, hookCalled)
}

func TestFullHappyPath(t *testing.T) {
	m := New()
	tk := task.New("test", "user-1")

	happyPath := []task.State{
		task.StatePlanning,
		task.StateResearching,
		task.StatePacketValidation,
		task.StateCoding,
		task.StateAuditing,
		task.StateExecuting,
		task.StateOutputReview,
		task.StateCompleted,
	}

	for _, next := range happyPath {
		err := m.Transition(context.Background(), tk, next, "auto")
		require.NoError(t, err, "transition to %s should succeed", next)
	}
	assert.Equal(t, task.StateCompleted, tk.State)
	assert.Len(t, tk.History, len(happyPath))
}

func TestAuditRetryLoop(t *testing.T) {
	m := New()
	tk := task.New("test", "user-1")

	// Get to AUDITING
	for _, s := range []task.State{task.StatePlanning, task.StateResearching, task.StatePacketValidation, task.StateCoding, task.StateAuditing} {
		require.NoError(t, m.Transition(context.Background(), tk, s, "auto"))
	}

	// Bounce back to CODING (revision)
	require.NoError(t, m.Transition(context.Background(), tk, task.StateCoding, "revision"))
	// Back to AUDITING
	require.NoError(t, m.Transition(context.Background(), tk, task.StateAuditing, "retry"))

	assert.Equal(t, task.StateAuditing, tk.State)
	assert.Equal(t, 2, tk.RetryCount(task.StateCoding, task.StateAuditing))
}

func TestHumanReviewEscalation(t *testing.T) {
	m := New()
	tk := task.New("test", "user-1")

	// Get to AUDITING
	for _, s := range []task.State{task.StatePlanning, task.StateResearching, task.StatePacketValidation, task.StateCoding, task.StateAuditing} {
		require.NoError(t, m.Transition(context.Background(), tk, s, "auto"))
	}

	// Escalate to HUMAN_REVIEW
	require.NoError(t, m.Transition(context.Background(), tk, task.StateHumanReview, "circuit breaker"))
	// Human sends back to PLANNING
	require.NoError(t, m.Transition(context.Background(), tk, task.StatePlanning, "human guidance"))

	assert.Equal(t, task.StatePlanning, tk.State)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/fsm/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/fsm/states.go
package fsm

import "github.com/bnaylor/perry/internal/task"

// transitions defines every legal state transition.
var transitions = map[task.State][]task.State{
	task.StateSubmitted:        {task.StatePlanning},
	task.StatePlanning:         {task.StateResearching, task.StateHumanReview},
	task.StateResearching:      {task.StatePacketValidation},
	task.StatePacketValidation: {task.StateCoding, task.StateResearching, task.StateHumanReview},
	task.StateCoding:           {task.StateAuditing},
	task.StateAuditing:         {task.StateExecuting, task.StateCoding, task.StateHumanReview},
	task.StateExecuting:        {task.StateOutputReview, task.StateCoding, task.StateFailed},
	task.StateOutputReview:     {task.StateCompleted, task.StateHumanReview},
	task.StateHumanReview:      {task.StatePlanning, task.StateFailed},
	task.StateCompleted:        {},
	task.StateFailed:           {},
}

// IsValid returns true if transitioning from -> to is allowed.
func IsValid(from, to task.State) bool {
	allowed, ok := transitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
```

```go
// internal/fsm/machine.go
package fsm

import (
	"context"
	"fmt"

	"github.com/bnaylor/perry/internal/task"
)

// TransitionHook is called after a successful transition.
type TransitionHook func(tk *task.Task, from, to task.State)

// Machine enforces state transitions for tasks.
type Machine struct {
	hooks []TransitionHook
}

// New creates a new FSM.
func New() *Machine {
	return &Machine{}
}

// OnTransition registers a hook that fires after every successful transition.
func (m *Machine) OnTransition(hook TransitionHook) {
	m.hooks = append(m.hooks, hook)
}

// Transition attempts to move a task to a new state.
// Returns an error if the transition is not allowed.
func (m *Machine) Transition(_ context.Context, tk *task.Task, to task.State, reason string) error {
	from := tk.State
	if !IsValid(from, to) {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}

	tk.RecordTransition(from, to, reason)
	tk.State = to

	for _, hook := range m.hooks {
		hook(tk, from, to)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/fsm/ -v`
Expected: All 6 tests PASS.

**Step 5: Commit**

```bash
git add internal/fsm/
git commit -m "feat: add FSM with transition table, hooks, and enforcement"
```

---

### Task 4: Task Store Interface + In-Memory Implementation

**Files:**
- Create: `internal/task/store.go`
- Create: `internal/task/memstore.go`
- Create: `internal/task/memstore_test.go`

**Step 1: Write failing tests**

```go
// internal/task/memstore_test.go
package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemStoreCreateAndGet(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	tk := New("test task", "user-1")
	err := store.Create(ctx, tk)
	require.NoError(t, err)

	got, err := store.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, tk.ID, got.ID)
	assert.Equal(t, tk.Description, got.Description)
}

func TestMemStoreGetNotFound(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestMemStoreUpdate(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	tk := New("test task", "user-1")
	require.NoError(t, store.Create(ctx, tk))

	tk.State = StatePlanning
	err := store.Update(ctx, tk)
	require.NoError(t, err)

	got, err := store.Get(ctx, tk.ID)
	require.NoError(t, err)
	assert.Equal(t, StatePlanning, got.State)
}

func TestMemStoreList(t *testing.T) {
	store := NewMemStore()
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, New("task 1", "user-1")))
	require.NoError(t, store.Create(ctx, New("task 2", "user-1")))

	tasks, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/task/ -v -run MemStore`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/task/store.go
package task

import "context"

// Store persists tasks. Implementations may be in-memory, SQLite, etc.
type Store interface {
	Create(ctx context.Context, t *Task) error
	Get(ctx context.Context, id string) (*Task, error)
	Update(ctx context.Context, t *Task) error
	List(ctx context.Context) ([]*Task, error)
}
```

```go
// internal/task/memstore.go
package task

import (
	"context"
	"fmt"
	"sync"
)

// MemStore is a thread-safe in-memory task store.
type MemStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewMemStore() *MemStore {
	return &MemStore{tasks: make(map[string]*Task)}
}

func (s *MemStore) Create(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[t.ID]; exists {
		return fmt.Errorf("task %s already exists", t.ID)
	}
	s.tasks[t.ID] = t
	return nil
}

func (s *MemStore) Get(_ context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return t, nil
}

func (s *MemStore) Update(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[t.ID]; !exists {
		return fmt.Errorf("task %s not found", t.ID)
	}
	s.tasks[t.ID] = t
	return nil
}

func (s *MemStore) List(_ context.Context) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/task/ -v`
Expected: All tests PASS (both Task and MemStore tests).

**Step 5: Commit**

```bash
git add internal/task/store.go internal/task/memstore.go internal/task/memstore_test.go
git commit -m "feat: add task store interface with in-memory implementation"
```

---

### Task 5: LLM Provider Interface and Types

The thin abstraction layer for calling LLMs. No real provider implementations yet — just the interface and request/response types, plus a mock provider for testing.

**Files:**
- Create: `internal/llm/provider.go`
- Create: `internal/llm/types.go`
- Create: `internal/llm/mock.go`
- Create: `internal/llm/mock_test.go`

**Step 1: Write failing test**

```go
// internal/llm/mock_test.go
package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider(t *testing.T) {
	mock := NewMockProvider("test-model", "This is the response.")
	resp, err := mock.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "This is the response.", resp.Content)
	assert.Equal(t, "test-model", resp.Model)
	assert.Greater(t, resp.Usage.OutputTokens, 0)
}

func TestMockProviderName(t *testing.T) {
	mock := NewMockProvider("test-model", "resp")
	assert.Equal(t, "mock", mock.Name())
}

func TestMockProviderRecordsRequests(t *testing.T) {
	mock := NewMockProvider("test-model", "resp")
	_, _ = mock.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	_, _ = mock.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "world"}},
	})
	assert.Len(t, mock.Requests, 2)
	assert.Equal(t, "hello", mock.Requests[0].Messages[0].Content)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/llm/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/llm/types.go
package llm

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// CompletionRequest is the provider-agnostic request.
type CompletionRequest struct {
	Messages    []Message      `json:"messages"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Extras      map[string]any `json:"extras,omitempty"` // provider-specific
}

// CompletionResponse is the provider-agnostic response.
type CompletionResponse struct {
	Content  string `json:"content"`
	Model    string `json:"model"`
	Usage    Usage  `json:"usage"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
```

```go
// internal/llm/provider.go
package llm

import "context"

// Provider is the interface for all LLM backends.
type Provider interface {
	// Name returns the provider identifier (e.g., "anthropic", "google", "ollama").
	Name() string

	// Complete sends a completion request and returns the response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
```

```go
// internal/llm/mock.go
package llm

import "context"

// MockProvider returns a fixed response for testing.
type MockProvider struct {
	model    string
	response string
	Requests []CompletionRequest // records all requests for assertions
}

func NewMockProvider(model, response string) *MockProvider {
	return &MockProvider{model: model, response: response}
}

func (m *MockProvider) Name() string { return "mock" }

func (m *MockProvider) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	m.Requests = append(m.Requests, req)
	return CompletionResponse{
		Content: m.response,
		Model:   m.model,
		Usage:   Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/llm/ -v`
Expected: All 3 tests PASS.

**Step 5: Commit**

```bash
git add internal/llm/
git commit -m "feat: add LLM provider interface with mock implementation"
```

---

### Task 6: Policy Engine

Deterministic rules enforcement — dependency allowlists, budget limits, safety invariants.

**Files:**
- Create: `internal/policy/engine.go`
- Create: `internal/policy/engine_test.go`
- Create: `configs/policy.yaml`

**Step 1: Write failing tests**

```go
// internal/policy/engine_test.go
package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckDependencyAllowed(t *testing.T) {
	e := NewEngine(Config{
		AllowedDependencies: []string{"requests", "json", "pandas"},
	})
	result := e.CheckDependency("requests")
	assert.True(t, result.Allowed)
}

func TestCheckDependencyDenied(t *testing.T) {
	e := NewEngine(Config{
		AllowedDependencies: []string{"requests", "json"},
	})
	result := e.CheckDependency("subprocess")
	assert.False(t, result.Allowed)
	assert.Equal(t, "escalate", result.Action)
}

func TestCheckBudgetWithinLimits(t *testing.T) {
	e := NewEngine(Config{
		MaxTokensPerTask: 100000,
		MaxCostPerTask:   5.00,
	})
	result := e.CheckBudget(50000, 2.50)
	assert.True(t, result.Allowed)
}

func TestCheckBudgetExceeded(t *testing.T) {
	e := NewEngine(Config{
		MaxTokensPerTask: 100000,
		MaxCostPerTask:   5.00,
	})
	result := e.CheckBudget(150000, 2.50)
	assert.False(t, result.Allowed)
	assert.Equal(t, "escalate", result.Action)
	assert.Contains(t, result.Reason, "token")
}

func TestCheckBudgetCostExceeded(t *testing.T) {
	e := NewEngine(Config{
		MaxTokensPerTask: 100000,
		MaxCostPerTask:   5.00,
	})
	result := e.CheckBudget(50000, 7.00)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "cost")
}

func TestImmutableSafetyRules(t *testing.T) {
	e := NewEngine(Config{
		ProhibitedImports: []string{"eval", "exec", "os.system"},
	})
	violations := e.CheckProhibitedImports([]string{"json", "os.system", "requests"})
	assert.Len(t, violations, 1)
	assert.Equal(t, "os.system", violations[0])
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/policy/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/policy/engine.go
package policy

import "fmt"

// Config holds all policy rules loaded from config file.
type Config struct {
	AllowedDependencies []string `yaml:"allowed_dependencies"`
	ProhibitedImports   []string `yaml:"prohibited_imports"`
	MaxTokensPerTask    int      `yaml:"max_tokens_per_task"`
	MaxCostPerTask      float64  `yaml:"max_cost_per_task"`
}

// Decision is the result of a policy check.
type Decision struct {
	Allowed bool
	Action  string // "allow", "escalate", "deny"
	Reason  string
}

// Engine evaluates deterministic policy rules.
type Engine struct {
	config           Config
	allowedDepsIndex map[string]bool
	prohibitedIndex  map[string]bool
}

// NewEngine creates a policy engine from config.
func NewEngine(cfg Config) *Engine {
	depsIdx := make(map[string]bool, len(cfg.AllowedDependencies))
	for _, d := range cfg.AllowedDependencies {
		depsIdx[d] = true
	}
	prohibIdx := make(map[string]bool, len(cfg.ProhibitedImports))
	for _, p := range cfg.ProhibitedImports {
		prohibIdx[p] = true
	}
	return &Engine{
		config:           cfg,
		allowedDepsIndex: depsIdx,
		prohibitedIndex:  prohibIdx,
	}
}

// CheckDependency returns whether a package is on the allowlist.
func (e *Engine) CheckDependency(pkg string) Decision {
	if e.allowedDepsIndex[pkg] {
		return Decision{Allowed: true, Action: "allow"}
	}
	return Decision{
		Allowed: false,
		Action:  "escalate",
		Reason:  fmt.Sprintf("package %q not on approved list", pkg),
	}
}

// CheckBudget returns whether token/cost usage is within limits.
func (e *Engine) CheckBudget(tokens int, cost float64) Decision {
	if tokens > e.config.MaxTokensPerTask {
		return Decision{
			Allowed: false,
			Action:  "escalate",
			Reason:  fmt.Sprintf("token usage %d exceeds limit %d", tokens, e.config.MaxTokensPerTask),
		}
	}
	if cost > e.config.MaxCostPerTask {
		return Decision{
			Allowed: false,
			Action:  "escalate",
			Reason:  fmt.Sprintf("cost $%.2f exceeds limit $%.2f", cost, e.config.MaxCostPerTask),
		}
	}
	return Decision{Allowed: true, Action: "allow"}
}

// CheckProhibitedImports returns any imports that match the prohibited list.
func (e *Engine) CheckProhibitedImports(imports []string) []string {
	var violations []string
	for _, imp := range imports {
		if e.prohibitedIndex[imp] {
			violations = append(violations, imp)
		}
	}
	return violations
}
```

**Step 4: Create default config**

```yaml
# configs/policy.yaml
allowed_dependencies:
  - json
  - math
  - os
  - sys
  - pathlib
  - datetime
  - requests
  - pandas
  - numpy
  - typing

prohibited_imports:
  - eval
  - exec
  - os.system
  - subprocess
  - shutil.rmtree

max_tokens_per_task: 500000
max_cost_per_task: 10.00
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/policy/ -v`
Expected: All 6 tests PASS.

**Step 6: Commit**

```bash
git add internal/policy/ configs/
git commit -m "feat: add policy engine with allowlists and budget enforcement"
```

---

### Task 7: Dispatcher

Config-driven compute tier routing.

**Files:**
- Create: `internal/dispatch/dispatcher.go`
- Create: `internal/dispatch/dispatcher_test.go`
- Create: `configs/routing.yaml`

**Step 1: Write failing tests**

```go
// internal/dispatch/dispatcher_test.go
package dispatch

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultConfig() Config {
	return Config{
		Defaults: map[string]RouteConfig{
			"strategist":       {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
			"researcher":       {Tier: "cloud", Provider: "google", Model: "gemini-2.5-pro"},
			"coder":            {Tier: "local", Provider: "ollama", Model: "qwen2.5-coder:32b"},
			"auditor_semantic": {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
		},
		Escalation: EscalationConfig{
			MaxLocalAttempts: 3,
			PromoteTo:        RouteConfig{Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
		},
	}
}

func TestRouteDefaultCoder(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	decision, err := d.Route(context.Background(), tk, "coder")
	require.NoError(t, err)
	assert.Equal(t, "local", decision.Tier)
	assert.Equal(t, "ollama", decision.Provider)
	assert.Equal(t, "qwen2.5-coder:32b", decision.Model)
}

func TestRouteDefaultStrategist(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	decision, err := d.Route(context.Background(), tk, "strategist")
	require.NoError(t, err)
	assert.Equal(t, "cloud", decision.Tier)
	assert.Equal(t, "anthropic", decision.Provider)
}

func TestRouteEscalatesAfterRetries(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	// Simulate 3 failed coding->auditing cycles
	for i := 0; i < 3; i++ {
		tk.RecordTransition(task.StateCoding, task.StateAuditing, "submit")
		tk.RecordTransition(task.StateAuditing, task.StateCoding, "revision")
	}

	decision, err := d.Route(context.Background(), tk, "coder")
	require.NoError(t, err)
	assert.Equal(t, "cloud", decision.Tier)
	assert.Equal(t, "anthropic", decision.Provider)
	assert.Contains(t, decision.Reason, "escalat")
}

func TestRouteUnknownRole(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	_, err := d.Route(context.Background(), tk, "nonexistent")
	assert.Error(t, err)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/dispatch/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/dispatch/dispatcher.go
package dispatch

import (
	"context"
	"fmt"

	"github.com/bnaylor/perry/internal/task"
)

// RouteConfig specifies where to run an agent.
type RouteConfig struct {
	Tier     string `yaml:"tier"`     // "local" or "cloud"
	Provider string `yaml:"provider"` // "anthropic", "google", "ollama"
	Model    string `yaml:"model"`
}

// EscalationConfig defines when to promote from local to cloud.
type EscalationConfig struct {
	MaxLocalAttempts int         `yaml:"max_local_attempts"`
	PromoteTo        RouteConfig `yaml:"promote_to"`
}

// Config holds dispatch routing configuration.
type Config struct {
	Defaults   map[string]RouteConfig `yaml:"defaults"`
	Escalation EscalationConfig       `yaml:"escalation"`
}

// Decision is the routing result.
type Decision struct {
	Tier     string
	Provider string
	Model    string
	Reason   string
}

// Dispatcher routes tasks to compute tiers.
type Dispatcher struct {
	config Config
}

// New creates a dispatcher from config.
func New(cfg Config) *Dispatcher {
	return &Dispatcher{config: cfg}
}

// Route decides where to run an agent for a given task and role.
func (d *Dispatcher) Route(_ context.Context, tk *task.Task, role string) (Decision, error) {
	defaults, ok := d.config.Defaults[role]
	if !ok {
		return Decision{}, fmt.Errorf("no routing config for role %q", role)
	}

	// Check if the task has exhausted local retries (only applies to local-tier roles)
	if defaults.Tier == "local" {
		retries := tk.RetryCount(task.StateCoding, task.StateAuditing)
		if retries >= d.config.Escalation.MaxLocalAttempts {
			promo := d.config.Escalation.PromoteTo
			return Decision{
				Tier:     promo.Tier,
				Provider: promo.Provider,
				Model:    promo.Model,
				Reason:   fmt.Sprintf("escalated after %d local attempts", retries),
			}, nil
		}
	}

	return Decision{
		Tier:     defaults.Tier,
		Provider: defaults.Provider,
		Model:    defaults.Model,
		Reason:   "default routing",
	}, nil
}
```

**Step 4: Create routing config**

```yaml
# configs/routing.yaml
defaults:
  strategist:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6
  researcher:
    tier: cloud
    provider: google
    model: gemini-2.5-pro
  coder:
    tier: local
    provider: ollama
    model: qwen2.5-coder:32b
  auditor_semantic:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6

escalation:
  max_local_attempts: 3
  promote_to:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/dispatch/ -v`
Expected: All 4 tests PASS.

**Step 6: Commit**

```bash
git add internal/dispatch/ configs/routing.yaml
git commit -m "feat: add dispatcher with config-driven routing and escalation"
```

---

### Task 8: Agent Interface and Mock Agents

The Agent interface and mock implementations that log what they'd do.

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/role.go`
- Create: `internal/agent/mock.go`
- Create: `internal/agent/runner.go`
- Create: `internal/agent/runner_test.go`

**Step 1: Write failing tests**

```go
// internal/agent/runner_test.go
package agent

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerExecutesMockAgent(t *testing.T) {
	provider := llm.NewMockProvider("test-model", "mock LLM response")
	agent := NewMockAgent(RoleStrategist)
	runner := NewRunner(map[Role]Agent{RoleStrategist: agent}, provider)

	tk := task.New("Build a weather CLI", "user-1")
	output, err := runner.Execute(context.Background(), tk, RoleStrategist, "Break this into requirements")
	require.NoError(t, err)
	assert.NotEmpty(t, output.Content)
	assert.Equal(t, RoleStrategist, output.Role)
}

func TestRunnerUnknownRole(t *testing.T) {
	provider := llm.NewMockProvider("test-model", "resp")
	runner := NewRunner(map[Role]Agent{}, provider)

	tk := task.New("test", "user-1")
	_, err := runner.Execute(context.Background(), tk, RoleStrategist, "do something")
	assert.Error(t, err)
}

func TestMockAgentRecordsCalls(t *testing.T) {
	agent := NewMockAgent(RoleCoder)
	provider := llm.NewMockProvider("test-model", "generated code here")
	runner := NewRunner(map[Role]Agent{RoleCoder: agent}, provider)

	tk := task.New("test", "user-1")
	_, _ = runner.Execute(context.Background(), tk, RoleCoder, "write a script")

	require.Len(t, agent.Calls, 1)
	assert.Equal(t, "write a script", agent.Calls[0])
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/agent/role.go
package agent

// Role identifies an agent's function in the pipeline.
type Role string

const (
	RoleStrategist Role = "strategist"
	RoleResearcher Role = "researcher"
	RoleCoder      Role = "coder"
	RoleAuditor    Role = "auditor_semantic"
)
```

```go
// internal/agent/agent.go
package agent

import (
	"context"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// AgentOutput is what an agent produces.
type AgentOutput struct {
	Role    Role
	Content string
	Usage   llm.Usage
}

// Agent defines the interface for all agent roles.
type Agent interface {
	// Role returns this agent's role.
	Role() Role

	// BuildMessages constructs the LLM messages for this agent's task.
	BuildMessages(tk *task.Task, input string) []llm.Message
}
```

```go
// internal/agent/mock.go
package agent

import (
	"fmt"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// MockAgent logs what it would do instead of calling an LLM.
type MockAgent struct {
	role  Role
	Calls []string
}

func NewMockAgent(role Role) *MockAgent {
	return &MockAgent{role: role}
}

func (m *MockAgent) Role() Role { return m.role }

func (m *MockAgent) BuildMessages(tk *task.Task, input string) []llm.Message {
	m.Calls = append(m.Calls, input)
	return []llm.Message{
		{Role: "system", Content: fmt.Sprintf("You are the %s agent.", m.role)},
		{Role: "user", Content: fmt.Sprintf("Task: %s\n\nInput: %s", tk.Description, input)},
	}
}
```

```go
// internal/agent/runner.go
package agent

import (
	"context"
	"fmt"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// Runner executes agents by dispatching to LLM providers.
type Runner struct {
	agents   map[Role]Agent
	provider llm.Provider
}

// NewRunner creates an agent runner.
func NewRunner(agents map[Role]Agent, provider llm.Provider) *Runner {
	return &Runner{agents: agents, provider: provider}
}

// Execute runs the agent for the given role and returns its output.
func (r *Runner) Execute(ctx context.Context, tk *task.Task, role Role, input string) (AgentOutput, error) {
	agent, ok := r.agents[role]
	if !ok {
		return AgentOutput{}, fmt.Errorf("no agent registered for role %q", role)
	}

	messages := agent.BuildMessages(tk, input)
	resp, err := r.provider.Complete(ctx, llm.CompletionRequest{
		Messages: messages,
	})
	if err != nil {
		return AgentOutput{}, fmt.Errorf("LLM call failed for %s: %w", role, err)
	}

	return AgentOutput{
		Role:    role,
		Content: resp.Content,
		Usage:   resp.Usage,
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -v`
Expected: All 3 tests PASS.

**Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat: add agent interface, mock agents, and runner"
```

---

### Task 9: Audit Pipeline Coordinator (Go side)

The Go package that orchestrates calls to Python audit tools. For now, uses a mock Python runner — real subprocess invocation comes when we build the Python tools.

**Files:**
- Create: `internal/audit/pipeline.go`
- Create: `internal/audit/result.go`
- Create: `internal/audit/pipeline_test.go`

**Step 1: Write failing tests**

```go
// internal/audit/pipeline_test.go
package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func passingGate(name string) Gate {
	return &mockGate{name: name, result: GateResult{Pass: true, Gate: name}}
}

func failingGate(name string, findings []string) Gate {
	return &mockGate{name: name, result: GateResult{Pass: false, Gate: name, Findings: findings}}
}

type mockGate struct {
	name   string
	result GateResult
}

func (g *mockGate) Name() string                                   { return g.name }
func (g *mockGate) Run(_ context.Context, _ AuditInput) GateResult { return g.result }

func TestPipelineAllPass(t *testing.T) {
	p := NewPipeline(
		passingGate("ast"),
		passingGate("secrets"),
		passingGate("static"),
	)

	result, err := p.Run(context.Background(), AuditInput{Code: "print('hello')"})
	require.NoError(t, err)
	assert.Equal(t, VerdictApprove, result.Verdict)
	assert.Len(t, result.GateResults, 3)
}

func TestPipelineFailFast(t *testing.T) {
	secondGate := passingGate("secrets")
	p := NewPipeline(
		failingGate("ast", []string{"disallowed import: os"}),
		secondGate,
	)

	result, err := p.Run(context.Background(), AuditInput{Code: "import os"})
	require.NoError(t, err)
	assert.Equal(t, VerdictReject, result.Verdict)
	// Only 1 gate ran (fail-fast)
	assert.Len(t, result.GateResults, 1)
	assert.Equal(t, "ast", result.GateResults[0].Gate)
}

func TestPipelinePartialFailure(t *testing.T) {
	p := NewPipeline(
		passingGate("ast"),
		failingGate("secrets", []string{"possible API key at line 5"}),
		passingGate("static"),
	)

	result, err := p.Run(context.Background(), AuditInput{Code: "key = 'sk-abc123...'"})
	require.NoError(t, err)
	assert.Equal(t, VerdictReject, result.Verdict)
	assert.Len(t, result.GateResults, 2) // stopped after secrets
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/audit/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/audit/result.go
package audit

// Verdict is the overall outcome of the audit pipeline.
type Verdict string

const (
	VerdictApprove  Verdict = "APPROVE"
	VerdictReject   Verdict = "REJECT"
	VerdictEscalate Verdict = "ESCALATE"
)

// GateResult is the outcome of a single audit gate.
type GateResult struct {
	Pass     bool     `json:"pass"`
	Gate     string   `json:"gate"`
	Findings []string `json:"findings,omitempty"`
}

// AuditInput is what gets fed into the audit pipeline.
type AuditInput struct {
	Code           string
	ContextPacket  map[string]any // the validated context packet
	Requirements   string         // PM's requirements summary
}

// AuditResult is the complete audit outcome.
type AuditResult struct {
	Verdict     Verdict
	GateResults []GateResult
}
```

```go
// internal/audit/pipeline.go
package audit

import "context"

// Gate is a single audit check.
type Gate interface {
	Name() string
	Run(ctx context.Context, input AuditInput) GateResult
}

// Pipeline runs audit gates sequentially with fail-fast behavior.
type Pipeline struct {
	gates []Gate
}

// NewPipeline creates an audit pipeline from an ordered list of gates.
func NewPipeline(gates ...Gate) *Pipeline {
	return &Pipeline{gates: gates}
}

// Run executes all gates in order. Stops at first failure.
func (p *Pipeline) Run(ctx context.Context, input AuditInput) (AuditResult, error) {
	var results []GateResult

	for _, gate := range p.gates {
		result := gate.Run(ctx, input)
		results = append(results, result)

		if !result.Pass {
			return AuditResult{
				Verdict:     VerdictReject,
				GateResults: results,
			}, nil
		}
	}

	return AuditResult{
		Verdict:     VerdictApprove,
		GateResults: results,
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/audit/ -v`
Expected: All 3 tests PASS.

**Step 5: Commit**

```bash
git add internal/audit/
git commit -m "feat: add audit pipeline with gate interface and fail-fast execution"
```

---

### Task 10: Executor and Notary Interfaces (Mocked)

Stub interfaces for the sandbox executor and output notary. No real implementation — just the contract and mocks.

**Files:**
- Create: `internal/executor/executor.go`
- Create: `internal/executor/mock.go`
- Create: `internal/notary/notary.go`
- Create: `internal/notary/mock.go`
- Create: `internal/executor/mock_test.go`
- Create: `internal/notary/mock_test.go`

**Step 1: Write failing tests**

```go
// internal/executor/mock_test.go
package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockExecutorSuccess(t *testing.T) {
	exec := NewMockExecutor(Result{
		Success:  true,
		Output:   "result.json",
		ExitCode: 0,
		Logs:     "executed successfully",
	})

	result, err := exec.Run(context.Background(), RunRequest{Code: "print('hi')"})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
}

func TestMockExecutorFailure(t *testing.T) {
	exec := NewMockExecutor(Result{
		Success:  false,
		ExitCode: 1,
		Logs:     "NameError: name 'foo' is not defined",
	})

	result, err := exec.Run(context.Background(), RunRequest{Code: "foo()"})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.ExitCode)
}
```

```go
// internal/notary/mock_test.go
package notary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockNotaryApproves(t *testing.T) {
	n := NewMockNotary(true)
	result, err := n.Review(context.Background(), ReviewRequest{
		ArtifactPath: "/out/result.json",
		ExpectedType: "json",
	})
	require.NoError(t, err)
	assert.True(t, result.Approved)
}

func TestMockNotaryRejects(t *testing.T) {
	n := NewMockNotary(false)
	result, err := n.Review(context.Background(), ReviewRequest{
		ArtifactPath: "/out/backdoor.sh",
		ExpectedType: "json",
	})
	require.NoError(t, err)
	assert.False(t, result.Approved)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/executor/ ./internal/notary/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/executor/executor.go
package executor

import "context"

// RunRequest describes what to execute in the sandbox.
type RunRequest struct {
	Code         string
	Language     string
	Dependencies []string
	EnvVars      map[string]string
	TimeoutSec   int
}

// Result is the outcome of a sandbox execution.
type Result struct {
	Success  bool
	Output   string // path to output artifact(s)
	ExitCode int
	Logs     string
}

// Executor runs code in an isolated sandbox.
type Executor interface {
	Run(ctx context.Context, req RunRequest) (Result, error)
}
```

```go
// internal/executor/mock.go
package executor

import "context"

// MockExecutor returns a fixed result for testing.
type MockExecutor struct {
	result   Result
	Requests []RunRequest
}

func NewMockExecutor(result Result) *MockExecutor {
	return &MockExecutor{result: result}
}

func (m *MockExecutor) Run(_ context.Context, req RunRequest) (Result, error) {
	m.Requests = append(m.Requests, req)
	return m.result, nil
}
```

```go
// internal/notary/notary.go
package notary

import "context"

// ReviewRequest describes an artifact to validate.
type ReviewRequest struct {
	ArtifactPath string
	ExpectedType string // "json", "csv", "py", etc.
	MaxSizeBytes int64
}

// ReviewResult is the notary's verdict on an artifact.
type ReviewResult struct {
	Approved bool
	Reason   string
}

// Notary validates executor output before delivery to the host.
type Notary interface {
	Review(ctx context.Context, req ReviewRequest) (ReviewResult, error)
}
```

```go
// internal/notary/mock.go
package notary

import "context"

// MockNotary returns a fixed verdict for testing.
type MockNotary struct {
	approved bool
}

func NewMockNotary(approved bool) *MockNotary {
	return &MockNotary{approved: approved}
}

func (m *MockNotary) Review(_ context.Context, req ReviewRequest) (ReviewResult, error) {
	reason := "mock approved"
	if !m.approved {
		reason = "mock rejected"
	}
	return ReviewResult{Approved: m.approved, Reason: reason}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/executor/ ./internal/notary/ -v`
Expected: All 4 tests PASS.

**Step 5: Commit**

```bash
git add internal/executor/ internal/notary/
git commit -m "feat: add executor and notary interfaces with mock implementations"
```

---

### Task 11: Orchestrator — Wiring It All Together

The central component that owns the FSM and calls the Dispatcher, Agent Runner, Audit Pipeline, Executor, and Notary for each state transition.

**Files:**
- Create: `internal/orchestrator/orchestrator.go`
- Create: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write failing tests**

```go
// internal/orchestrator/orchestrator_test.go
package orchestrator

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allPassingPipeline() *audit.Pipeline {
	return audit.NewPipeline(&passingGate{})
}

type passingGate struct{}

func (g *passingGate) Name() string { return "mock" }
func (g *passingGate) Run(_ context.Context, _ audit.AuditInput) audit.GateResult {
	return audit.GateResult{Pass: true, Gate: "mock"}
}

func testOrchestrator() *Orchestrator {
	provider := llm.NewMockProvider("test-model", "mock response")
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewMockAgent(agent.RoleStrategist),
		agent.RoleResearcher: agent.NewMockAgent(agent.RoleResearcher),
		agent.RoleCoder:      agent.NewMockAgent(agent.RoleCoder),
		agent.RoleAuditor:    agent.NewMockAgent(agent.RoleAuditor),
	}
	return NewOrchestrator(Config{
		FSM:        fsm.New(),
		Store:      task.NewMemStore(),
		Runner:     agent.NewRunner(agents, provider),
		Dispatcher: dispatch.New(dispatch.Config{
			Defaults: map[string]dispatch.RouteConfig{
				"strategist":       {Tier: "cloud", Provider: "anthropic", Model: "test"},
				"researcher":       {Tier: "cloud", Provider: "google", Model: "test"},
				"coder":            {Tier: "local", Provider: "ollama", Model: "test"},
				"auditor_semantic": {Tier: "cloud", Provider: "anthropic", Model: "test"},
			},
			Escalation: dispatch.EscalationConfig{MaxLocalAttempts: 3, PromoteTo: dispatch.RouteConfig{Tier: "cloud", Provider: "anthropic", Model: "test"}},
		}),
		Policy:   policy.NewEngine(policy.Config{MaxTokensPerTask: 100000, MaxCostPerTask: 5.0}),
		Audit:    allPassingPipeline(),
		Executor: executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:   notary.NewMockNotary(true),
	})
}

func TestOrchestratorSubmitTask(t *testing.T) {
	orch := testOrchestrator()
	ctx := context.Background()

	tk, err := orch.Submit(ctx, "Build a weather CLI", "user-1")
	require.NoError(t, err)
	assert.NotEmpty(t, tk.ID)
	assert.Equal(t, task.StateSubmitted, tk.State)
}

func TestOrchestratorStepThroughHappyPath(t *testing.T) {
	orch := testOrchestrator()
	ctx := context.Background()

	tk, err := orch.Submit(ctx, "Build a weather CLI", "user-1")
	require.NoError(t, err)

	// Step through the entire happy path
	expectedStates := []task.State{
		task.StatePlanning,
		task.StateResearching,
		task.StatePacketValidation,
		task.StateCoding,
		task.StateAuditing,
		task.StateExecuting,
		task.StateOutputReview,
		task.StateCompleted,
	}

	for _, expected := range expectedStates {
		err = orch.Step(ctx, tk)
		require.NoError(t, err, "step to %s should succeed", expected)
		assert.Equal(t, expected, tk.State)
	}
}

func TestOrchestratorStepAtCompletedIsNoop(t *testing.T) {
	orch := testOrchestrator()
	ctx := context.Background()

	tk, _ := orch.Submit(ctx, "test", "user-1")

	// Run to completion
	for tk.State != task.StateCompleted {
		require.NoError(t, orch.Step(ctx, tk))
	}

	// Stepping again should be a no-op
	err := orch.Step(ctx, tk)
	require.NoError(t, err)
	assert.Equal(t, task.StateCompleted, tk.State)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/orchestrator/ -v`
Expected: Compilation failure.

**Step 3: Write implementation**

```go
// internal/orchestrator/orchestrator.go
package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
)

// Config holds all dependencies for the orchestrator.
type Config struct {
	FSM        *fsm.Machine
	Store      task.Store
	Runner     *agent.Runner
	Dispatcher *dispatch.Dispatcher
	Policy     *policy.Engine
	Audit      *audit.Pipeline
	Executor   executor.Executor
	Notary     notary.Notary
}

// Orchestrator drives tasks through the FSM.
type Orchestrator struct {
	fsm        *fsm.Machine
	store      task.Store
	runner     *agent.Runner
	dispatcher *dispatch.Dispatcher
	policy     *policy.Engine
	audit      *audit.Pipeline
	executor   executor.Executor
	notary     notary.Notary
}

// NewOrchestrator creates an orchestrator with all dependencies.
func NewOrchestrator(cfg Config) *Orchestrator {
	return &Orchestrator{
		fsm:        cfg.FSM,
		store:      cfg.Store,
		runner:     cfg.Runner,
		dispatcher: cfg.Dispatcher,
		policy:     cfg.Policy,
		audit:      cfg.Audit,
		executor:   cfg.Executor,
		notary:     cfg.Notary,
	}
}

// Submit creates a new task and persists it.
func (o *Orchestrator) Submit(ctx context.Context, description, createdBy string) (*task.Task, error) {
	tk := task.New(description, createdBy)
	if err := o.store.Create(ctx, tk); err != nil {
		return nil, fmt.Errorf("failed to store task: %w", err)
	}
	slog.Info("task submitted", "id", tk.ID, "description", description)
	return tk, nil
}

// Step advances a task by one state transition.
func (o *Orchestrator) Step(ctx context.Context, tk *task.Task) error {
	next, reason, err := o.determineNextState(ctx, tk)
	if err != nil {
		return err
	}
	if next == "" {
		return nil // terminal state, nothing to do
	}

	if err := o.fsm.Transition(ctx, tk, next, reason); err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	slog.Info("state transition", "task", tk.ID, "to", next, "reason", reason)

	if err := o.store.Update(ctx, tk); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	return nil
}

// determineNextState figures out what the next state should be based on
// the current state and the results of running the appropriate handler.
func (o *Orchestrator) determineNextState(ctx context.Context, tk *task.Task) (task.State, string, error) {
	switch tk.State {
	case task.StateSubmitted:
		return task.StatePlanning, "auto", nil

	case task.StatePlanning:
		_, err := o.runner.Execute(ctx, tk, agent.RoleStrategist, tk.Description)
		if err != nil {
			return task.StateHumanReview, "strategist error", nil
		}
		return task.StateResearching, "requirements ready", nil

	case task.StateResearching:
		_, err := o.runner.Execute(ctx, tk, agent.RoleResearcher, "gather context")
		if err != nil {
			return "", "", fmt.Errorf("researcher failed: %w", err)
		}
		return task.StatePacketValidation, "context packet produced", nil

	case task.StatePacketValidation:
		// In phase 1, validation is a pass-through. Real validation comes with Python tooling.
		return task.StateCoding, "packet validated", nil

	case task.StateCoding:
		_, err := o.runner.Execute(ctx, tk, agent.RoleCoder, "generate code")
		if err != nil {
			return "", "", fmt.Errorf("coder failed: %w", err)
		}
		return task.StateAuditing, "code ready for audit", nil

	case task.StateAuditing:
		result, err := o.audit.Run(ctx, audit.AuditInput{Code: "placeholder"})
		if err != nil {
			return task.StateHumanReview, "audit error", nil
		}
		switch result.Verdict {
		case audit.VerdictApprove:
			return task.StateExecuting, "audit passed", nil
		case audit.VerdictReject:
			return task.StateCoding, "audit rejected, revision needed", nil
		default:
			return task.StateHumanReview, "audit escalated", nil
		}

	case task.StateExecuting:
		result, err := o.executor.Run(ctx, executor.RunRequest{Code: "placeholder"})
		if err != nil {
			return task.StateFailed, "executor error", nil
		}
		if !result.Success {
			return task.StateCoding, "execution failed, retry", nil
		}
		return task.StateOutputReview, "execution complete", nil

	case task.StateOutputReview:
		result, err := o.notary.Review(ctx, notary.ReviewRequest{ArtifactPath: "placeholder"})
		if err != nil {
			return task.StateHumanReview, "notary error", nil
		}
		if !result.Approved {
			return task.StateHumanReview, "output rejected", nil
		}
		return task.StateCompleted, "output approved", nil

	case task.StateCompleted, task.StateFailed:
		return "", "", nil // terminal

	case task.StateHumanReview:
		return "", "", nil // waiting for human, can't auto-advance

	default:
		return "", "", fmt.Errorf("unhandled state: %s", tk.State)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/orchestrator/ -v`
Expected: All 3 tests PASS.

**Step 5: Run full test suite**

Run: `make test`
Expected: All tests across all packages PASS.

**Step 6: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: add orchestrator wiring FSM, dispatcher, agents, audit, executor, and notary"
```

---

### Task 12: Wire Up main.go with a Demo Run

Update the binary entrypoint to create an orchestrator with mock components and run a task through the full happy path. This is the "smoke test" — you can run the binary and watch a task flow through every state.

**Files:**
- Modify: `cmd/perry/main.go`

**Step 1: Update main.go**

```go
// cmd/perry/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/orchestrator"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// Build all components with mocks
	provider := llm.NewMockProvider("mock-model", "mock LLM output")
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewMockAgent(agent.RoleStrategist),
		agent.RoleResearcher: agent.NewMockAgent(agent.RoleResearcher),
		agent.RoleCoder:      agent.NewMockAgent(agent.RoleCoder),
		agent.RoleAuditor:    agent.NewMockAgent(agent.RoleAuditor),
	}

	fsmMachine := fsm.New()
	fsmMachine.OnTransition(func(tk *task.Task, from, to task.State) {
		fmt.Printf("  %s → %s\n", from, to)
	})

	orch := orchestrator.NewOrchestrator(orchestrator.Config{
		FSM:    fsmMachine,
		Store:  task.NewMemStore(),
		Runner: agent.NewRunner(agents, provider),
		Dispatcher: dispatch.New(dispatch.Config{
			Defaults: map[string]dispatch.RouteConfig{
				"strategist":       {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
				"researcher":       {Tier: "cloud", Provider: "google", Model: "gemini-2.5-pro"},
				"coder":            {Tier: "local", Provider: "ollama", Model: "qwen2.5-coder:32b"},
				"auditor_semantic": {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
			},
			Escalation: dispatch.EscalationConfig{
				MaxLocalAttempts: 3,
				PromoteTo:        dispatch.RouteConfig{Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
			},
		}),
		Policy: policy.NewEngine(policy.Config{
			MaxTokensPerTask: 500000,
			MaxCostPerTask:   10.0,
		}),
		Audit:    audit.NewPipeline(), // no gates yet — auto-pass
		Executor: executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:   notary.NewMockNotary(true),
	})

	// Submit and run a task
	fmt.Println("perry: secure agentic platform")
	fmt.Println("================================")
	fmt.Println()

	tk, err := orch.Submit(ctx, "Build a Python script that fetches weather data and saves it as JSON", "user-1")
	if err != nil {
		slog.Error("failed to submit task", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Task %s submitted\n\n", tk.ID)
	fmt.Println("Running through state machine:")

	for tk.State != task.StateCompleted && tk.State != task.StateFailed {
		if err := orch.Step(ctx, tk); err != nil {
			slog.Error("step failed", "error", err, "state", tk.State)
			os.Exit(1)
		}
	}

	fmt.Printf("\nFinal state: %s (%d transitions)\n", tk.State, len(tk.History))
}
```

**Step 2: Build and run**

Run: `make build && ./bin/perry`
Expected output showing the task flowing through all states:
```
perry: secure agentic platform
================================

Task task-XXXXXXXX submitted

Running through state machine:
  SUBMITTED → PLANNING
  PLANNING → RESEARCHING
  RESEARCHING → PACKET_VALIDATION
  PACKET_VALIDATION → CODING
  CODING → AUDITING
  AUDITING → EXECUTING
  EXECUTING → OUTPUT_REVIEW
  OUTPUT_REVIEW → COMPLETED

Final state: COMPLETED (8 transitions)
```

**Step 3: Run full test suite to confirm nothing broke**

Run: `make test`
Expected: All tests PASS.

**Step 4: Commit**

```bash
git add cmd/perry/main.go
git commit -m "feat: wire up main.go with mock components and demo task flow"
```

---

### Task 13: Python Audit Tooling — AST Analyzer

The first real Python audit gate. Parses code AST and checks for disallowed imports and dangerous function calls.

**Files:**
- Create: `python/audit/ast_analyzer.py`
- Create: `python/audit/test_ast_analyzer.py`
- Create: `python/requirements.txt`

**Step 1: Write failing test**

```python
# python/audit/test_ast_analyzer.py
import json
import subprocess
import sys

def run_analyzer(code, allowed_imports=None):
    """Run ast_analyzer.py as a subprocess and return parsed JSON."""
    input_data = json.dumps({
        "code": code,
        "allowed_imports": allowed_imports or []
    })
    result = subprocess.run(
        [sys.executable, "python/audit/ast_analyzer.py"],
        input=input_data,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"Script failed: {result.stderr}"
    return json.loads(result.stdout)

def test_clean_code_passes():
    result = run_analyzer(
        "import json\nx = json.loads('{}')",
        allowed_imports=["json"]
    )
    assert result["pass"] is True
    assert result["findings"] == []

def test_disallowed_import_fails():
    result = run_analyzer(
        "import os\nos.system('rm -rf /')",
        allowed_imports=["json", "requests"]
    )
    assert result["pass"] is False
    assert any("os" in f for f in result["findings"])

def test_subprocess_import_fails():
    result = run_analyzer(
        "import subprocess\nsubprocess.run(['ls'])",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("subprocess" in f for f in result["findings"])

def test_eval_call_detected():
    result = run_analyzer(
        "x = eval('1+1')",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("eval" in f for f in result["findings"])

def test_exec_call_detected():
    result = run_analyzer(
        "exec('print(1)')",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("exec" in f for f in result["findings"])

def test_from_import_checked():
    result = run_analyzer(
        "from os.path import join",
        allowed_imports=["json"]
    )
    assert result["pass"] is False
    assert any("os" in f for f in result["findings"])
```

**Step 2: Run tests to verify they fail**

Run: `python3 -m pytest python/audit/test_ast_analyzer.py -v`
Expected: Failures (script doesn't exist yet).

**Step 3: Write implementation**

```python
# python/audit/ast_analyzer.py
"""AST-based code analyzer for the perry audit pipeline.

Reads JSON from stdin: {"code": "...", "allowed_imports": ["json", ...]}
Writes JSON to stdout: {"pass": bool, "gate": "ast", "findings": [...]}
"""
import ast
import json
import sys

DANGEROUS_CALLS = {"eval", "exec", "compile", "__import__"}


def analyze(code: str, allowed_imports: list[str]) -> dict:
    findings = []

    try:
        tree = ast.parse(code)
    except SyntaxError as e:
        return {"pass": False, "gate": "ast", "findings": [f"SyntaxError: {e}"]}

    for node in ast.walk(tree):
        # Check imports
        if isinstance(node, ast.Import):
            for alias in node.names:
                root_module = alias.name.split(".")[0]
                if root_module not in allowed_imports:
                    findings.append(f"disallowed import: {alias.name}")

        elif isinstance(node, ast.ImportFrom):
            if node.module:
                root_module = node.module.split(".")[0]
                if root_module not in allowed_imports:
                    findings.append(f"disallowed import: from {node.module}")

        # Check dangerous function calls
        elif isinstance(node, ast.Call):
            if isinstance(node.func, ast.Name) and node.func.id in DANGEROUS_CALLS:
                findings.append(f"dangerous call: {node.func.id}()")

    return {
        "pass": len(findings) == 0,
        "gate": "ast",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = analyze(input_data["code"], input_data.get("allowed_imports", []))
    print(json.dumps(result))
```

**Step 4: Create requirements.txt**

```
# python/requirements.txt
pytest>=8.0
bandit>=1.8
jsonschema>=4.20
```

**Step 5: Run tests to verify they pass**

Run: `python3 -m pytest python/audit/test_ast_analyzer.py -v`
Expected: All 6 tests PASS.

**Step 6: Commit**

```bash
git add python/
git commit -m "feat: add Python AST analyzer audit gate"
```

---

### Task 14: Python Audit Tooling — Secrets Scanner

**Files:**
- Create: `python/audit/secrets_scanner.py`
- Create: `python/audit/test_secrets_scanner.py`

**Step 1: Write failing test**

```python
# python/audit/test_secrets_scanner.py
import json
import subprocess
import sys

def run_scanner(code):
    input_data = json.dumps({"code": code})
    result = subprocess.run(
        [sys.executable, "python/audit/secrets_scanner.py"],
        input=input_data,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"Script failed: {result.stderr}"
    return json.loads(result.stdout)

def test_clean_code_passes():
    result = run_scanner("x = 42\nprint(x)")
    assert result["pass"] is True

def test_aws_key_detected():
    result = run_scanner("key = 'AKIAIOSFODNN7EXAMPLE'")
    assert result["pass"] is False
    assert any("AWS" in f or "AKIA" in f for f in result["findings"])

def test_github_token_detected():
    result = run_scanner("token = 'ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234'")
    assert result["pass"] is False

def test_generic_api_key_detected():
    result = run_scanner("API_KEY = 'sk-abcdefghijklmnopqrstuvwxyz123456'")
    assert result["pass"] is False

def test_env_var_reference_is_fine():
    result = run_scanner("key = os.environ['API_KEY']")
    assert result["pass"] is True

def test_bearer_token_detected():
    result = run_scanner("headers = {'Authorization': 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc123'}")
    assert result["pass"] is False
```

**Step 2: Run tests to verify they fail**

Run: `python3 -m pytest python/audit/test_secrets_scanner.py -v`
Expected: Failures.

**Step 3: Write implementation**

```python
# python/audit/secrets_scanner.py
"""Secrets and credential scanner for the perry audit pipeline.

Reads JSON from stdin: {"code": "..."}
Writes JSON to stdout: {"pass": bool, "gate": "secrets", "findings": [...]}
"""
import json
import re
import sys

PATTERNS = [
    (r"(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*[\"']?[a-zA-Z0-9_\-]{20,}", "possible hardcoded credential"),
    (r"(?i)bearer\s+[a-zA-Z0-9_\-\.]{20,}", "possible bearer token"),
    (r"ghp_[a-zA-Z0-9]{36}", "GitHub personal access token"),
    (r"sk-[a-zA-Z0-9]{32,}", "OpenAI-style API key"),
    (r"AIza[a-zA-Z0-9_\-]{35}", "Google API key"),
    (r"AKIA[A-Z0-9]{16}", "AWS access key"),
]


def scan(code: str) -> dict:
    findings = []
    for line_num, line in enumerate(code.splitlines(), 1):
        for pattern, description in PATTERNS:
            if re.search(pattern, line):
                findings.append(f"line {line_num}: {description}")
                break  # one finding per line is enough

    return {
        "pass": len(findings) == 0,
        "gate": "secrets",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = scan(input_data["code"])
    print(json.dumps(result))
```

**Step 4: Run tests to verify they pass**

Run: `python3 -m pytest python/audit/test_secrets_scanner.py -v`
Expected: All 6 tests PASS.

**Step 5: Commit**

```bash
git add python/audit/secrets_scanner.py python/audit/test_secrets_scanner.py
git commit -m "feat: add Python secrets scanner audit gate"
```

---

### Task 15: Python Context Packet Validation

**Files:**
- Copy: `docs/agentic_design/claude_artifacts/context_packet_schema.json` → `python/context_packet/schema.json`
- Create: `python/context_packet/validate.py`
- Create: `python/context_packet/scan.py`
- Create: `python/context_packet/test_validate.py`

**Step 1: Write failing test**

```python
# python/context_packet/test_validate.py
import json
import subprocess
import sys
import os

SCHEMA_PATH = os.path.join(os.path.dirname(__file__), "schema.json")

def run_validator(packet):
    input_data = json.dumps({"packet": packet})
    result = subprocess.run(
        [sys.executable, "python/context_packet/validate.py"],
        input=input_data,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"Script failed: {result.stderr}"
    return json.loads(result.stdout)

def minimal_valid_packet():
    return {
        "packet_meta": {
            "packet_id": "CP-20260227-a3f8c012",
            "schema_version": "1.0.0",
            "created_at": "2026-02-27T14:30:00Z",
            "researcher_model": "gemini-2.5-pro",
            "source_count": 1,
            "parent_packet_id": None,
        },
        "task_reference": {
            "task_id": "TASK-001",
            "prd_summary": "Test task",
            "cuj_ids": ["CUJ-001"],
        },
        "external_apis": [],
        "data_schemas": [],
        "constraints": {
            "permitted_operations": ["write_stdout"],
            "prohibited_operations": ["network_listen"],
        },
        "researcher_notes": {
            "summary": "Test summary.",
            "open_questions": [],
        },
    }

def test_valid_packet_passes():
    result = run_validator(minimal_valid_packet())
    assert result["pass"] is True

def test_missing_required_field_fails():
    packet = minimal_valid_packet()
    del packet["task_reference"]
    result = run_validator(packet)
    assert result["pass"] is False
    assert any("task_reference" in f for f in result["findings"])

def test_invalid_packet_id_format_fails():
    packet = minimal_valid_packet()
    packet["packet_meta"]["packet_id"] = "bad-id"
    result = run_validator(packet)
    assert result["pass"] is False

def test_invalid_schema_version_fails():
    packet = minimal_valid_packet()
    packet["packet_meta"]["schema_version"] = "2.0.0"
    result = run_validator(packet)
    assert result["pass"] is False
```

**Step 2: Run tests to verify they fail**

Run: `python3 -m pytest python/context_packet/test_validate.py -v`
Expected: Failures.

**Step 3: Copy schema and write implementation**

Copy the existing schema:
```bash
cp docs/agentic_design/claude_artifacts/context_packet_schema.json python/context_packet/schema.json
```

```python
# python/context_packet/validate.py
"""Context Packet schema validator for the perry audit pipeline.

Reads JSON from stdin: {"packet": {...}}
Writes JSON to stdout: {"pass": bool, "gate": "schema_validation", "findings": [...]}
"""
import json
import os
import sys

import jsonschema


def validate(packet: dict) -> dict:
    schema_path = os.path.join(os.path.dirname(__file__), "schema.json")
    with open(schema_path) as f:
        schema = json.load(f)

    validator = jsonschema.Draft7Validator(schema)
    findings = []
    for error in sorted(validator.iter_errors(packet), key=str):
        path = ".".join(str(p) for p in error.absolute_path)
        if path:
            findings.append(f"{path}: {error.message}")
        else:
            findings.append(error.message)

    return {
        "pass": len(findings) == 0,
        "gate": "schema_validation",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = validate(input_data["packet"])
    print(json.dumps(result))
```

```python
# python/context_packet/scan.py
"""Content scanner for Context Packets — checks for secrets and injection patterns.

Reads JSON from stdin: {"packet": {...}}
Writes JSON to stdout: {"pass": bool, "gate": "content_scan", "findings": [...]}
"""
import json
import re
import sys

SECRETS_PATTERNS = [
    r"(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*[\"']?[a-zA-Z0-9_\-]{20,}",
    r"(?i)bearer\s+[a-zA-Z0-9_\-\.]{20,}",
    r"ghp_[a-zA-Z0-9]{36}",
    r"sk-[a-zA-Z0-9]{32,}",
    r"AIza[a-zA-Z0-9_\-]{35}",
    r"AKIA[A-Z0-9]{16}",
]

INJECTION_PATTERNS = [
    r"(?i)ignore\s+(previous|above|all)\s+(instructions?|prompts?)",
    r"(?i)you\s+are\s+now\s+",
    r"(?i)system\s*:\s*",
    r"(?i)forget\s+(everything|your|all)",
    r"(?i)<\s*/?system\s*>",
]


def scan(packet: dict) -> dict:
    findings = []

    def walk(obj, path="root"):
        if isinstance(obj, str):
            for pattern in SECRETS_PATTERNS:
                if re.search(pattern, obj):
                    findings.append(f"SECRETS_LEAK at {path}")
                    break
            for pattern in INJECTION_PATTERNS:
                if re.search(pattern, obj):
                    findings.append(f"INJECTION_SUSPECT at {path}")
                    break
        elif isinstance(obj, dict):
            for k, v in obj.items():
                walk(v, f"{path}.{k}")
        elif isinstance(obj, list):
            for i, v in enumerate(obj):
                walk(v, f"{path}[{i}]")

    walk(packet)
    return {
        "pass": len(findings) == 0,
        "gate": "content_scan",
        "findings": findings,
    }


if __name__ == "__main__":
    input_data = json.loads(sys.stdin.read())
    result = scan(input_data["packet"])
    print(json.dumps(result))
```

**Step 4: Install jsonschema**

Run: `pip install jsonschema`

**Step 5: Run tests to verify they pass**

Run: `python3 -m pytest python/context_packet/test_validate.py -v`
Expected: All 4 tests PASS.

**Step 6: Commit**

```bash
git add python/context_packet/
git commit -m "feat: add context packet schema validation and content scanning"
```

---

### Task 16: Update Makefile and Run Full Suite

Update the Makefile to include Python tests, then verify everything works end to end.

**Files:**
- Modify: `Makefile`

**Step 1: Update Makefile**

```makefile
.PHONY: build test test-go test-python lint clean

build:
	go build -o bin/perry ./cmd/perry

test: test-go test-python

test-go:
	go test ./... -v -race

test-python:
	python3 -m pytest python/ -v

lint:
	go vet ./...

clean:
	rm -rf bin/
```

**Step 2: Run full suite**

Run: `make test`
Expected: All Go and Python tests PASS.

**Step 3: Run the binary demo**

Run: `make build && ./bin/perry`
Expected: Task flows through all states to COMPLETED.

**Step 4: Commit**

```bash
git add Makefile
git commit -m "feat: update Makefile with Python test target and full suite"
```

---

## Summary

16 tasks building bottom-up:

| Task | Package | What it delivers |
|------|---------|-----------------|
| 1 | scaffold | Go module, Makefile, .gitignore |
| 2 | task | Task model with state, history, retry counting |
| 3 | fsm | State machine with transition table and hooks |
| 4 | task/store | Store interface + in-memory implementation |
| 5 | llm | Provider interface + mock |
| 6 | policy | Policy engine — allowlists, budget, safety rules |
| 7 | dispatch | Dispatcher — config-driven tier routing + escalation |
| 8 | agent | Agent interface, mock agents, runner |
| 9 | audit | Audit pipeline with gate interface, fail-fast |
| 10 | executor, notary | Stub interfaces + mocks |
| 11 | orchestrator | Wires everything together, drives tasks through FSM |
| 12 | cmd/perry | Demo binary running a task through the full loop |
| 13 | python/audit | AST analyzer gate |
| 14 | python/audit | Secrets scanner gate |
| 15 | python/context_packet | Schema validation + content scanning |
| 16 | Makefile | Full test suite (Go + Python) |

After task 12, you have a running binary. After task 16, you have the complete phase 1 orchestration skeleton with real Python audit tooling.
