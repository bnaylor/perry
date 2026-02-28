# Phase 2 Design: Workspace-Aware Orchestration (v2)

## Goal: Core Phase 2 Objective
The transition from "standalone code generator" to "secure codebase collaborator" is the primary technical objective for Phase 2.

### 1. The Strategist as "Workspace Orchestrator"
The Strategist's role is expanded to define a **Working Set** based on user intent.

- **Strategist Ownership:** The Strategist (not the Researcher) is the authoritative agent for identifying which files are relevant to the task. The Researcher remains a stateless "fetcher" of these files.
- **Explicit Declaration:** Instead of complex auto-discovery, the Strategist explicitly declares the `Working Set`.
- **Context Packet Expansion:** A new `workspace_context` block is added to the JSON schema:
    - `read_only_files`: Code needed for context (interfaces, types).
    - `writable_files`: Authorized files for modification.
    - `runtime_environment`: Required toolchain (e.g., "golang:1.25").
    - `build_and_test_commands`: Deterministic verification commands.

### 2. The Executor: Layered Sandbox & Toolchains
The Executor is upgraded to a language-aware "Mirror Cage."

- **Layered Mount:** Read-only host mirror + ephemeral writable overlay.
- **Dynamic Toolchain:** The Executor uses language-specific container images (e.g., a Go-specific runner). **Note:** This increases the sandbox's disk footprint and initial spin-up time but is necessary to provide the full toolchain (`go test`, `npm test`) for internal verification.

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
