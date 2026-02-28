# Tech Debt & Future Improvements

Items discovered during development that aren't blocking but should be addressed later.

## FSM: Typed error for invalid transitions

**Found:** 2026-02-28, during batch 1 code review
**Package:** `internal/fsm`
**Issue:** `Machine.Transition` returns `fmt.Errorf("invalid transition: %s -> %s", ...)`. Downstream code (orchestrator, dispatcher) that needs to distinguish "invalid transition" from other errors will have to string-match.
**Fix:** Add a typed `ErrInvalidTransition` or `InvalidTransitionError` struct. Easy to add when the orchestrator (Task 11) needs to branch on it.
**Priority:** Low — address when wiring up the orchestrator.
