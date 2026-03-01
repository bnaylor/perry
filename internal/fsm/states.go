package fsm

import "github.com/bnaylor/perry/internal/task"

// transitions defines every legal state transition.
var transitions = map[task.State][]task.State{
	task.StateSubmitted:        {task.StatePlanning},
	task.StatePlanning:         {task.StateResearching, task.StateHumanReview},
	task.StateResearching:      {task.StatePacketValidation},
	task.StatePacketValidation: {task.StateCoding, task.StateResearching, task.StateHumanReview},
	task.StateCoding:           {task.StateAuditing},
	task.StateAuditing:         {task.StateShadowAuditing, task.StateCoding, task.StateHumanReview},
	task.StateShadowAuditing:   {task.StateExecuting, task.StateCoding, task.StateHumanReview},
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
