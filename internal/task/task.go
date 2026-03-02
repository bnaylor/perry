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
	StateShadowAuditing   State = "SHADOW_AUDITING"
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
	ID              string
	Description     string
	CreatedBy       string
	State           State
	CreatedAt       time.Time
	DiscordThreadID string // ID of the Discord thread for this task
	History         []Transition
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
