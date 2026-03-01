# Implementation Plan: Mirror Cage (Codebase-Aware Orchestration)

**Goal:** Implement the "Mirror Cage" layer to provide agents with a semantic, read-only view of the codebase during the `RESEARCHING` phase, enabling architectural alignment and reducing "grepping like a caveman."

**Architecture:**
- **`internal/codebase`**: A new Go package using `go/ast` to extract exported symbols (types, functions, interfaces) from a list of packages.
- **Enriched Orchestrator**: Invokes the codebase snapshot logic during `StateResearching` based on the Strategist's "Working Set."
- **Updated `ContextPacket`**: Includes a `codebase_context` field to carry semantic metadata to the Coder.
- **Updated Agents**: Strategist (identifies packages), Researcher (incorporates context), and Coder (consumes context).

**Tech Stack:** Go, `go/ast`, `go/parser`, `go/token`.

---

### Task 1: Update ContextPacket Schema

**Files:**
- Modify: `python/context_packet/schema.json`

**Step 1: Add `codebase_context` to the root properties.**
The new field will store the semantic map of the codebase relevant to the task.

```json
"codebase_context": {
  "type": "object",
  "description": "Semantic snapshot of relevant codebase parts (types, functions, interfaces).",
  "additionalProperties": {
    "type": "object",
    "properties": {
      "types": { "type": "object" },
      "functions": { "type": "array" },
      "interfaces": { "type": "array" },
      "imports": { "type": "array" }
    }
  }
}
```

**Step 2: Update "required" fields if necessary.**
(Optional: Decide if it should be required or optional).

---

### Task 2: Implement `internal/codebase` package

**Files:**
- Create: `internal/codebase/snapshot.go`
- Create: `internal/codebase/snapshot_test.go`

**Step 1: Implement `TakeSnapshot(pkgPaths []string) (map[string]any, error)`.**
Use `go/parser.ParseDir` and `ast.Inspect` to walk the AST of the target packages.

**Step 2: Extract Exported Symbols.**
- **Structs:** Field names and types.
- **Functions:** Names, parameters, and return types.
- **Interfaces:** Method signatures.
- **Imports:** List of package imports.

**Step 3: Verification.**
Write a test that parses a small mock package within the `perry` codebase (e.g., `internal/task`) and verifies the output JSON structure.

---

### Task 3: Update Strategist to identify "Working Set"

**Files:**
- Modify: `internal/agent/strategist.go`

**Step 1: Update System Prompt.**
Instruct the Strategist to include a `working_set` field in its JSON output—a list of relative package paths (e.g., `["internal/task", "internal/fsm"]`) relevant to the task.

---

### Task 4: Orchestrator Wiring (The Cage)

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Capture Working Set.**
In `StatePlanning`, store the Strategist's `working_set` output.

**Step 2: Invoke Snapshot.**
In `StateResearching`, before executing the Researcher:
1. Call `codebase.TakeSnapshot(workingSet)`.
2. Pass this snapshot data to the Researcher as additional input.

**Step 3: Enrich ContextPacket.**
Update the `StatePacketValidation` or `StateResearching` logic to ensure the Researcher includes the `codebase_context` in its final output.

---

### Task 5: Agent Prompt Updates (Consuming the Context)

**Files:**
- Modify: `internal/agent/researcher.go`
- Modify: `internal/agent/coder.go`

**Step 1: Researcher Prompt.**
Instruct the Researcher to use the provided `CodebaseContext` to "map the territory" and identify existing patterns.

**Step 2: Coder Prompt.**
Instruct the Coder to treat the `codebase_context` in the `ContextPacket` as the "Source of Truth" for existing types and interfaces.

---

### Task 6: Final Verification

**Step 1: Integration Test.**
Create a task like "Add a field to the Task struct and a method to update it."
1. Verify Strategist identifies `internal/task`.
2. Verify Researcher output contains the `Task` struct AST data.
3. Verify Coder generates code that uses the correct struct fields.

---

## Status: 📝 Plan Ready
Approved by BTGemini (Jarvis) on 2026-03-01.
