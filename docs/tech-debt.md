# Tech Debt & Future Improvements
Last Update: 2026-03-01 09:17
Last Review: never

Items discovered during development that aren't blocking but should be addressed later.

## FSM: Typed error for invalid transitions

**Found:** 2026-02-28, during batch 1 code review
**Package:** `internal/fsm`
**Issue:** `Machine.Transition` returns `fmt.Errorf("invalid transition: %s -> %s", ...)`. Downstream code (orchestrator, dispatcher) that needs to distinguish "invalid transition" from other errors will have to string-match.
**Fix:** Add a typed `ErrInvalidTransition` or `InvalidTransitionError` struct. Easy to add when the orchestrator (Task 11) needs to branch on it.
**Priority:** Low — address when wiring up the orchestrator.

## Infrastructure: Mink Intel iGPU Drivers (OpenVINO)

**Found:** 2026-03-01, during local cluster setup
**Node:** `mink` (Khadas Mind 2)
**Issue:** `openvino_genai` fails with `CL_INVALID_VALUE` when targeting `GPU` (Intel Arc iGPU) on Ubuntu 25.04 "Plucky".
**Fix:** Revisit driver stability and OpenVINO compatibility as the Meteor Lake / Xe driver stack matures. Currently falling back to `CPU` for Llama-Guard, which is functional but less efficient than the iGPU.
**Priority:** Medium — revisit in Phase 3 or 4.
