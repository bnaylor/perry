package storage

import (
	"encoding/json"
	"fmt"
)

// RecordTransition inserts a state transition entry into the transitions table.
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

// RecordExecution inserts a container execution result into the transitions table.
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

// RecordAuditGate inserts an audit gate result into the audit_records table.
// findings may be nil (pass with no notes) or a slice of human-readable finding strings.
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

// RecordAgentCall inserts an LLM agent invocation record into the agent_calls table.
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
