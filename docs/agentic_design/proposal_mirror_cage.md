# Proposal: Mirror Cage (Codebase-Aware Orchestration)

## The Problem
Agents currently operate with a fragmented, "grepping" view of the codebase. They lack a cohesive semantic understanding of existing symbols, type hierarchies, and internal dependencies. This leads to:
- Redundant implementations of existing logic.
- Architectural misalignment (e.g., ignoring established patterns).
- Subtle bugs caused by misunderstanding complex internal state transitions.

## The Solution: The "Mirror Cage"
We propose a **Codebase-Aware Orchestration** layer called the **Mirror Cage**. It provides agents with a read-only, semantic "mirror" of the relevant parts of the codebase, restricted to a "cage" defined by the task's scope.

### 1. FSM Integration: Enriched Researching
Instead of adding a new state, codebase analysis is integrated directly into the `RESEARCHING` phase.
- **The Researcher (Eyes):** Responsible for mapping the territory. It queries the Mirror Cage to understand the existing landscape.
- **The Coder (Hands):** Responsible for implementation. It receives a pre-packaged `ContextPacket` containing all the semantic context it needs.

### 2. The Codebase Snapshot Gate
A new Go-based tool, the `CodebaseSnapshotGate`, acts as a subprocess bridge between the orchestrator and the local source code.
- **Input:** A list of relevant packages (Working Set) identified by the **Strategist** during `PLANNING`.
- **Engine:** Uses `go/ast` and `go/types` for high-fidelity Go code analysis.
- **Output:** A structured JSON map of the identified packages, including:
    - **Exported Types:** Struct fields, interface methods, and underlying types.
    - **Function Signatures:** Parameters, return types, and documentation.
    - **Constants & Enums:** Defined values and types.
    - **Import Graph:** Direct and indirect dependencies to avoid circularities.

### 3. "Strategist-Scoped, Single Pass" Discovery
To maintain efficiency and context window budget:
- The Strategist identifies the relevant packages first.
- The Snapshot Gate performs a **single-pass** extraction of all exported symbols within those packages.
- This avoids expensive iterative discovery and keeps the context size manageable (typically 200-500 lines of JSON).

### 4. Schema Update: `ContextPacket`
The `ContextPacket` schema is updated to include a `codebase_context` field:
```json
{
  "requirements": [...],
  "dependencies": [...],
  "codebase_context": {
    "package_name": {
      "types": {...},
      "functions": [...],
      "interfaces": [...]
    }
  }
}
```

### 5. Sandbox Safety (The Cage)
- **Read-Only Mirror:** The codebase context is injected into the agent's prompt, ensuring no direct filesystem access is required during the reasoning phase.
- **Working Set Bind-Mounts:** When the Coder executes code in the sandbox, the `DockerExecutor` bind-mounts the actual files from the Working Set as a read-only mirror, with a writable overlay for generated changes.

## Impact on Perry
- **Architectural Fidelity:** Agents build *with* the platform, not just *on top* of it.
- **Security:** Maintains strict sandbox isolation while providing high-fidelity semantic intelligence.
- **Efficiency:** Reduces the need for agents to perform multiple "discovery" file reads, saving tokens and time.

## Status: ✅ Consensus Reached (Architect Roundtable)
This proposal was designed and approved by the Perry Architect Ensemble (BTGemini & BTClaude) on 2026-03-01.
