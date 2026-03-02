# Perry Development Best Practices for Agents

**Last Updated:** March 2, 2026

## Regular Hygiene
- Always run relevant tests after making changes to confirm proper function.
- Remember to commit changes when reaching a good stopping point.
- Use Conventional Commits formatting.
- **Self-Evolution:** Add new items to this file as lessons are learned during development.

## Perry-specific Items
- When beginning a new session, consult `docs/tech-debt.md`.
    - If it has been more than 5 days since last review, ask the user if they would like to do a tech debt review. If so, list open issues and discuss whether to address any of them. Update the review date either way.
- Remember to update `docs/VISION.md` after significant architectural decisions have been made.
- Update `docs/local_llm_reference.md` when changes to local models or roles are made.

## Core Conventions
- **Fail-Closed by Default:** All security boundaries (audit gates, sandbox setup, subprocess runners) must fail closed. If anything goes wrong — non-zero exit, bad JSON, timeout, context cancellation — reject, don't pass through.
- **Integration Tests:** Tests tagged with `//go:build integration` require external services (API keys, Docker daemon, etc.). Run with `go test -tags integration ./...`. Unit tests should always pass without external dependencies.
- **Agent Output Keying:** The orchestrator stores agent outputs in a map keyed by `taskID:role`. When passing data between pipeline stages, use this convention consistently.

## Infrastructure & Configuration
- **Distributed Node Readiness:** When adding or troubleshooting local LLM nodes (e.g., Node 2/mink), verify the service is listening on `0.0.0.0` (not just `127.0.0.1`) and check connectivity from the host.
- **Model Availability Discovery:** If a provider returns a 404 for a model, use the `perry --models` flag to verify the exact names and capabilities supported by the current API version for all configured providers.
- **Configuration Integrity:** For small YAML files (like `configs/routing.yaml`), prefer `write_file` over `edit_file` to avoid accidental key duplications during complex updates.

## Docker Executor Gotchas
- **Bind mount for `/out`:** Use a bind mount, not tmpfs — data is lost on container exit with tmpfs.
- **No `ReadonlyRootfs`:** `CopyToContainer` writes to the rootfs layer pre-start, so read-only root breaks setup.
- **Log demuxing:** Docker's attached streams use 8-byte frame headers. Always use `stdcopy.StdCopy` to demux stdout/stderr, never read raw.
- **Security defaults:** `NetworkDisabled: true`, all caps dropped, UID 1000, resource limits enforced. See `docs/plans/2026-03-01-phase3b-docker-executor-design.md` for rationale.

## Debugging & Diagnostics
- **Error Logging:** Ensure that all `runner.Execute` calls in `internal/orchestrator` log their errors *before* transitioning to a failure or review state. This is critical for diagnosing failures that are otherwise swallowed by the FSM.
