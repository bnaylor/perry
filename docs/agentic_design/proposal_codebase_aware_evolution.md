# Phase 2 Design: Workspace-Aware Orchestration (v2)

## The Problem
The Phase 1 "Airlock" design is optimized for a code generator that produces a standalone artifact (a "perfectly-formed nugget"). However, real-world development involves modifying an established, multi-file codebase. 

Providing the entire repository to an ephemeral Coder agent violates least-privilege, and providing no repository context makes the agent's code un-runnable and un-testable.

## The Solution: Workspace-Aware Evolution
We will evolve the Strategist and Executor to support a "Layered Sandbox" model that allows agents to work *within* an existing project without compromising the host filesystem.

### 1. The Strategist as "Workspace Orchestrator"
The Strategist's role is expanded to define a **Working Set** based on user intent.

- **Explicit Declaration:** Instead of complex auto-discovery (Phase 3+), the Strategist explicitly declares which files the task needs to touch or see.
- **Context Packet Expansion:** A new `workspace_context` block is added to the JSON schema:
    - `read_only_files`: Code the Coder needs to see to understand interfaces, types, and existing utilities.
    - `writable_files`: The specific files (or file paths for new files) the Coder is authorized to modify.
    - `runtime_environment`: The required toolchain (e.g., "golang:1.25", "python:3.14-slim", "node:22").
    - `build_and_test_commands`: The deterministic commands (e.g., `go test ./...`) the Executor should run to verify the changes.

### 2. The Executor: Layered Filesystem & Language-Specific Containers
The Executor's sandbox is upgraded to a language-aware "Mirror Cage."

- **Layered Mount (The Mirror Cage):**
    - **Lower Layer (Read-Only):** A filtered view of the host repository, containing only the files specified by the Strategist.
    - **Upper Layer (Writable):** An ephemeral, isolated scratchpad where the Coder applies their changes.
- **Dynamic Toolchain:** The Executor uses the `runtime_environment` field to select or build a language-specific container image (DevContainer style). This ensures the Coder has access to `go test`, `pytest`, or `npm test` without bloating the core orchestrator image.

### 3. The Auditor: Patch-Based Enforcement
The Auditor's deterministic gates are updated to validate **Changes (Diffs)** rather than just full files.

- **Authorized Edit Check:** The Auditor verifies that the diff generated in the sandbox *only* modifies files listed in the `writable_files` field of the Context Packet. This is a hard, deterministic boundary that no LLM prompt can override.
- **Style/Security Scan:** Traditional static analysis (Bandit, secrets scan) is run specifically on the diff to ensure the new code doesn't introduce vulnerabilities.

### 4. The Notary: Patch Validation & Delivery
The Notary's role moves from "artifact check" to "patch validation."

- **The Patchset:** The Coder's output is now a structured Patchset (e.g., a series of Unified Diffs).
- **Final Approval:** The Notary validates the final diff against the host's current state, ensuring it applies cleanly and matches the PM's manifest of expected changes.
- **Host Sync:** Only after the Notary's "APPROVE" does the orchestrator apply the patches to the actual host repository (e.g., via `git apply` or a similar tool).

## Impact on State Machine
The FSM remains largely the same, but the `CODING` -> `AUDITING` -> `EXECUTING` loop becomes a "Real-World Integration" loop:

1. **CODING:** Coder writes code/patches inside the Mirror Cage.
2. **AUDITING:** Auditor checks the patches for safety and "Working Set" compliance.
3. **EXECUTING:** Executor runs the `build_and_test_commands` *within the sandbox context*.
4. **LOOP:** If tests fail, the FSM transitions from `EXECUTING` back to `CODING` with the test logs.

## Security Advantages
- **Least Privilege:** The agent never sees the `.git` folder, `.env` files, or unrelated subsystems unless explicitly authorized by the Strategist.
- **Safe Execution:** "Malicious code" is executed against a read-only mirror of the codebase. It cannot "jailbreak" into the host repository.
- **Deterministic Bounds:** The Strategist's `writable_files` list acts as a hard boundary that prevents unauthorized lateral movement within the repo.
