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

### Node & Model Readiness
- **Distributed Node Readiness:** When adding or troubleshooting local LLM nodes (e.g., Node 2/mink), verify the service is listening on `0.0.0.0` (not just `127.0.0.1`) and check connectivity from the host.
- **Model Availability Discovery:** If a provider returns a 404 for a model, use a model-listing utility to verify the exact names and capabilities supported by the current API version.
- **Configuration Integrity:** For small YAML files (like `configs/routing.yaml`), prefer `write_file` over `edit_file` to avoid accidental key duplications during complex updates.

### Observability
- **Error Logging:** Ensure that all `runner.Execute` calls in `internal/orchestrator` log their errors *before* transitioning to a failure or review state. This is critical for diagnosing failures that are otherwise swallowed by the FSM.
