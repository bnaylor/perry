# Tech Debt & Future Improvements
Last Update: 2026-03-02
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

## Notary: Real implementation needed

**Found:** 2026-03-02, during executor gap analysis
**Package:** `internal/notary`
**Issue:** The Notary is currently a mock that always approves. VISION.md describes an "Airlock" model where the Notary validates executor output against a manifest (expected file types, checksums) and scans artifacts before delivery. The interface exists (`notary.Notary`) and the artifact path is now correctly wired from executor → orchestrator → Notary, but the implementation just returns `Approved: true`.
**Fix:** Implement a real Notary that: (1) enumerates files in the artifact directory, (2) validates MIME types against expected output from the Strategist's manifest, (3) computes checksums, (4) optionally runs a malware scanner. Return `Approved: false` on any violation, routing to `StateHumanReview`.
**Priority:** High — this is the last missing security gate in the end-to-end pipeline.

## Executor: Bind mount allows host writes during execution

**Found:** 2026-03-02, during executor gap analysis
**Package:** `internal/executor`
**Issue:** VISION.md's Airlock model says the Notary validates output *before* files reach the host. In practice, the `/out` bind mount means container writes land on the host filesystem in real time during execution. A true airlock would require a different approach — e.g., `CopyFromContainer` after the container exits (but before removal) instead of a bind mount, or a staging area with restricted permissions.
**Fix:** Evaluate whether to switch from bind mount to post-exit copy-out. Tradeoff: bind mount is simpler and the output size limit (now enforced) mitigates the blast radius. The current approach may be acceptable with the size limit in place — document the decision either way.
**Priority:** Low — mitigated by output size enforcement. Revisit if the threat model tightens.

## Integration tests: Missing build tag guards

**Found:** 2026-03-02, during full test suite run
**Package:** `internal/llm/anthropic`, `internal/llm/gemini`
**Issue:** Integration tests in the Anthropic and Gemini provider packages run during `go test ./...` even without API keys, causing expected but noisy failures. Other integration tests (e.g., Docker executor) correctly use `//go:build integration` tags.
**Fix:** Add `//go:build integration` tags to `anthropic/integration_test.go` and `gemini/integration_test.go`, or add skip guards that check for the API key env var before running.
**Priority:** Low — cosmetic, but noisy test output masks real failures.
