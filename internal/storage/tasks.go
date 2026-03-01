package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bnaylor/perry/internal/task"
)

// Create inserts a new task into the database.
// Returns an error if a task with the same ID already exists.
func (s *Store) Create(ctx context.Context, t *task.Task) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, description, created_by, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID,
		t.Description,
		t.CreatedBy,
		string(t.State),
		t.CreatedAt,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("create task %s: %w", t.ID, err)
	}
	return nil
}

// Get retrieves a task by ID, including its full transition history.
// Returns an error if the task does not exist.
func (s *Store) Get(ctx context.Context, id string) (*task.Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, description, created_by, state, created_at FROM tasks WHERE id = ?`,
		id,
	)

	t := &task.Task{}
	var state string
	err := row.Scan(&t.ID, &t.Description, &t.CreatedBy, &state, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get task %s: %w", id, err)
	}
	t.State = task.State(state)

	// Load transition history ordered by insertion id (chronological).
	rows, err := s.db.QueryContext(ctx,
		`SELECT from_state, to_state, reason, created_at
		 FROM transitions WHERE task_id = ? ORDER BY id`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("get transitions for task %s: %w", id, err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transitions for task %s: %w", id, err)
	}

	return t, nil
}

// Update writes the task's current state back to the database.
// Returns an error if no row was affected (task does not exist).
func (s *Store) Update(ctx context.Context, t *task.Task) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`,
		string(t.State),
		time.Now(),
		t.ID,
	)
	if err != nil {
		return fmt.Errorf("update task %s: %w", t.ID, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update task %s rows affected: %w", t.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("task %s not found", t.ID)
	}
	return nil
}

// List returns all tasks ordered by creation time.
// It does not load History for each task (avoids N+1 queries).
func (s *Store) List(ctx context.Context) ([]*task.Task, error) {
	rows, err := s.db.QueryContext(ctx,
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
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		t.State = task.State(state)
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task rows: %w", err)
	}

	return tasks, nil
}
