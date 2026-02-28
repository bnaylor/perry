# Plan: Phase 2 Design Refinement & Multi-Node Routing

**Date:** 2026-02-28
**Status:** IMPLEMENTED
**Context:** This plan captures the refinements made to the Phase 2 architecture after the initial "Real LLM Agents" implementation. It addresses the need for multi-node local cluster support and the evolution of the agentic design (Shadow Auditor, Roundtable).

## Completed Refinements

### 1. Multi-Node Dispatch Support
The initial Phase 2 design assumed a single Ollama URL. To support the "Perry Local Cluster" (4070 Ti + Khadas Mind), we expanded the `Dispatcher` and `ProviderMap` to support named instances.

- **Status:** COMPLETED
- **Changes:**
    - Modified `internal/dispatch/Config` to support a `Providers` list of `ProviderInstanceConfig`.
    - Refactored `internal/providers/BuildProviderMap` to instantiate multiple providers (e.g., `local-cuda`, `local-npu`) based on the routing config.
    - Added generic `openai` provider to support OpenVINO and other non-Ollama local endpoints.
    - Updated `configs/routing.yaml` to route `Coder` to `local-cuda` and `Auditor` to `local-npu`.

### 2. Strategy vs. Research Alignment
Refined the role of the Strategist to be the owner of the "Working Set" for workspace-aware tasks.

- **Status:** DESIGN LOCKED
- **Changes:** Updated `docs/agentic_design/proposal_codebase_aware_evolution.md` to shift "Codebase Surgeon" responsibilities from the Researcher to the Strategist.

## Future Phases (Updated Roadmap)

### Phase 3: The Architect Roundtable (Design Loop)
- **Goal:** Implement the `IDEATING` and `RE_IDEATING` states in the FSM.
- **Key Work:** Moderator service, internal `RoundtableLog`, and Discord mirroring sidecar.

### Phase 4: Secure Workspace & Adversarial Audit
- **Goal:** Land the "Shadow Auditor" and the "Mirror Cage" layered sandbox.
- **Key Work:** Layered OverlayFS executor, DeepSeek-R1 adversarial prompting, and PoC verification loop.
