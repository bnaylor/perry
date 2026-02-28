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
