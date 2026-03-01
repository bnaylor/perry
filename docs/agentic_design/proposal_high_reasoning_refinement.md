# Proposal: High-Reasoning Refinement Layer (Two-Pass Coding)

## The Problem
While local 14B models (e.g., Qwen2.5-Coder) are highly capable for routine implementation, they can suffer from "logic drift" or architectural blindness when handling complex, multi-module tasks. Relying solely on a smaller model for critical systems (auth, crypto, core orchestration) introduces risk that even a robust audit pipeline might struggle to catch if the initial implementation is fundamentally misaligned.

## The Solution: Two-Pass Refinement Loop
We propose an optional **High-Reasoning Refinement** stage that leverages "planetary-scale" cloud models (Claude 3.5 Sonnet, Gemini 1.5 Pro) to review and guide the local model's output before it proceeds to the auditing and execution phases.

### 1. Architectural Simplicity: The Internal Loop
To avoid bloating the Finite State Machine (FSM), the Refinement Layer is implemented as an **internal loop** within the `CODING` state.
- **Pass 1:** The 14B local Coder generates an initial implementation.
- **Refinement:** If the task is tagged `[CRITICAL]`, the Pass 1 output is sent to a High-Reasoning Cloud model (the "Refiner").
- **The Critique:** The Refiner provides a detailed architectural critique and specific logic suggestions (not a direct diff).
- **Pass 2:** The 14B local Coder receives the critique and generates the final revised implementation.

### 2. Criticality Heuristics (Auto-Tagging)
The **Strategist** will automatically tag tasks as `[CRITICAL]` based on the following heuristics identified during the `PLANNING` phase:
- **Security-Sensitive Patterns:** Any task involving authentication, cryptography, network protocols, or sensitive filesystem operations.
- **Logic Complexity:** Any task touching more than three internal modules (derived from the `ContextPacket` dependency graph).
- **Mutation Risk:** Any task that **modifies existing code** (regression risk) rather than generating greenfield implementations.
- **Human Override:** The human owner can manually prepend `[CRITICAL]` or `[SKIP_REFINEMENT]` to any task description to force or bypass this layer.

### 3. "Critique-and-Revise" vs. "Direct Diff"
The Refinement Layer uses a **Critique-and-Revise** protocol rather than having the cloud model generate code directly:
- **Maintains Agency:** The 14B Coder remains the primary author, ensuring the code is compatible with the local environment and style.
- **Digestible Reasoning:** Forcing the 14B model to "digest" the cloud model's critique ensures the final output is a coherent synthesis, not a blind patch.
- **Robustness:** Critiques are less sensitive to minor versioning or pathing differences than automated diffs.

### 4. Observability & Alignment Drift Analysis
With the introduction of the SQLite-based **Audit Record Persistence** layer, we will capture both iterations:
- **Data Collection:** Every `agent_call` (Pass 1, Critique, Pass 2) is recorded with full request/response payloads.
- **Drift Metrics:** We will analyze the delta between Pass 1 and Pass 2 to quantify the value added by the High-Reasoning model.
- **Threshold Tuning:** If Pass 1 and Pass 2 are consistently identical for certain heuristics, we will relax the auto-tagging to conserve tokens.

## Impact on Perry
- **Enhanced Security:** Critical code is reviewed by a superior reasoning engine before it even hits the sandbox.
- **Optimized Compute:** We utilize cheap local compute for the bulk of the work, only burning expensive cloud tokens for high-leverage architectural guidance.
- **Dataset Generation:** We begin building a proprietary dataset of "14B errors vs. Cloud corrections," which is invaluable for future fine-tuning of our local tier.

## Status: ✅ Consensus Reached (Architect Roundtable)
This proposal was designed and approved by the Perry Architect Ensemble (BTGemini & BTClaude) on 2026-03-01.
