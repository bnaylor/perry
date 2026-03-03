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

func TestStore_RecordExecution(t *testing.T) {
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

	err := s.RecordAuditGate(tk.ID, "code", "ast", true, nil, "")
	require.NoError(t, err)

	err = s.RecordAuditGate(tk.ID, "code", "secrets", false, []string{"hardcoded password on line 5"}, "policy_violation")
	require.NoError(t, err)

	// Verify
	rows, err := s.db.Query(`SELECT pipeline, gate, pass, findings, failure_reason FROM audit_records WHERE task_id = ? ORDER BY id`, tk.ID)
	require.NoError(t, err)
	defer rows.Close()

	type record struct {
		pipeline, gate, findings, failureReason string
		pass                                    bool
	}
	var records []record
	for rows.Next() {
		var r record
		require.NoError(t, rows.Scan(&r.pipeline, &r.gate, &r.pass, &r.findings, &r.failureReason))
		records = append(records, r)
	}
	require.Len(t, records, 2)
	assert.True(t, records[0].pass)
	assert.False(t, records[1].pass)
	assert.Contains(t, records[1].findings, "hardcoded password")
	assert.Equal(t, "policy_violation", records[1].failureReason)
}

func TestStore_RecordAgentCall(t *testing.T) {
	s := newTestStore(t)
	tk := createTestTask(t, s)

	err := s.RecordAgentCall(tk.ID, "coder", "anthropic", "claude-3-7-sonnet-20260219", 500, 1200, `{"code": "print('hello')"}`)
	require.NoError(t, err)

	var role, provider, model, content string
	var inputTokens, outputTokens int
	var cost float64
	row := s.db.QueryRow(`SELECT role, provider, model, input_tokens, output_tokens, content, cost FROM agent_calls WHERE task_id = ?`, tk.ID)
	require.NoError(t, row.Scan(&role, &provider, &model, &inputTokens, &outputTokens, &content, &cost))
	assert.Equal(t, "coder", role)
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "claude-3-7-sonnet-20260219", model)
	assert.Equal(t, 500, inputTokens)
	assert.Equal(t, 1200, outputTokens)
	assert.Contains(t, content, "print('hello')")
	assert.Greater(t, cost, 0.0)
}
