package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Command represents an inbound instruction from Discord or another relay.
type Command struct {
	ID        int64
	TaskID    string
	Command   string
	Args      string
	Status    string // 'pending', 'processing', 'completed', 'failed'
	CreatedAt time.Time
}

// GetInternalState retrieves a value from the internal_state table.
func (s *Store) GetInternalState(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM internal_state WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil // Not found is not an error for this purpose
	}
	if err != nil {
		return "", fmt.Errorf("get internal state %s: %w", key, err)
	}
	return value, nil
}

// SetInternalState inserts or updates a value in the internal_state table.
func (s *Store) SetInternalState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO internal_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value)
	if err != nil {
		return fmt.Errorf("set internal state %s: %w", key, err)
	}
	return nil
}

// GetPendingCommands retrieves all commands with 'pending' status.
func (s *Store) GetPendingCommands(ctx context.Context) ([]Command, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, task_id, command, args, status, created_at FROM commands WHERE status = 'pending' ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("get pending commands: %w", err)
	}
	defer rows.Close()

	var cmds []Command
	for rows.Next() {
		var c Command
		var taskID sql.NullString
		var args sql.NullString
		if err := rows.Scan(&c.ID, &taskID, &c.Command, &args, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		c.TaskID = taskID.String
		c.Args = args.String
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// AddCommand inserts a new command into the commands table.
func (s *Store) AddCommand(ctx context.Context, taskID, command, args string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO commands (task_id, command, args, status) VALUES (?, ?, ?, 'pending')",
		sql.NullString{String: taskID, Valid: taskID != ""},
		command,
		sql.NullString{String: args, Valid: args != ""},
	)
	if err != nil {
		return 0, fmt.Errorf("add command: %w", err)
	}
	return res.LastInsertId()
}

// UpdateCommandStatus updates the status of a command.
func (s *Store) UpdateCommandStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE commands SET status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("update command %d status: %w", id, err)
	}
	return nil
}

// TransitionRecord matches the database 'transitions' table.
type TransitionRecord struct {
	ID           int64
	TaskID       string
	FromState    string
	ToState      string
	Reason       string
	ExitCode     sql.NullInt64
	Logs         sql.NullString
	ArtifactPath sql.NullString
	CreatedAt    time.Time
}

// GetTransitionsSince returns all transitions with an ID greater than the given cursor.
func (s *Store) GetTransitionsSince(ctx context.Context, cursor int64) ([]TransitionRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, task_id, from_state, to_state, reason, exit_code, logs, artifact_path, created_at FROM transitions WHERE id > ? ORDER BY id",
		cursor)
	if err != nil {
		return nil, fmt.Errorf("get transitions since %d: %w", cursor, err)
	}
	defer rows.Close()

	var records []TransitionRecord
	for rows.Next() {
		var r TransitionRecord
		if err := rows.Scan(&r.ID, &r.TaskID, &r.FromState, &r.ToState, &r.Reason, &r.ExitCode, &r.Logs, &r.ArtifactPath, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan transition: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// AgentCallRecord matches the database 'agent_calls' table.
type AgentCallRecord struct {
	ID           int64
	TaskID       string
	Role         string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	Content      string
	CreatedAt    time.Time
}

// GetAgentCallsSince returns all agent calls with an ID greater than the given cursor.
func (s *Store) GetAgentCallsSince(ctx context.Context, cursor int64) ([]AgentCallRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, task_id, role, provider, model, input_tokens, output_tokens, content, created_at FROM agent_calls WHERE id > ? ORDER BY id",
		cursor)
	if err != nil {
		return nil, fmt.Errorf("get agent calls since %d: %w", cursor, err)
	}
	defer rows.Close()

	var records []AgentCallRecord
	for rows.Next() {
		var r AgentCallRecord
		if err := rows.Scan(&r.ID, &r.TaskID, &r.Role, &r.Provider, &r.Model, &r.InputTokens, &r.OutputTokens, &r.Content, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent call: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
