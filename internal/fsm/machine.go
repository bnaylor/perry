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
