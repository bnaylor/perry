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
		task.StateShadowAuditing,
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
