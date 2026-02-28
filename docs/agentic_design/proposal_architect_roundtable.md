# Proposal: The Architect Roundtable (Multi-Agent Design Loop)

## The Problem
Currently, the "Three-Way Brainstorming" (Human + Claude + Gemini) that developed Perry's architecture is a manual, human-mediated process. The human owner acts as a "go-between" for different LLMs on different platforms. This is slow and doesn't scale to iterative refinement or new feature design within the platform.

## The Solution: The "Roundtable" Orchestration
We propose a new **Architect** role and a **Roundtable Protocol** that allows multiple LLM agents (Claude, Gemini, Local Models) to collaborate simultaneously in a Discord thread to design, review, and refine project documentation.

### 1. The "Architect" Role
The Architect is a new, high-reasoning agent role. Unlike the Strategist (who owns "Why") or the Coder (who owns "How"), the Architect owns the **"What"** and the **"Design."**

- **Multi-Model Ensemble:** The platform spins up two or more Architects using different providers (e.g., `architect-anthropic` and `architect-google`).
- **Tools:**
    - `brainstorm`: Generates initial design ideas based on requirements.
    - `critique`: Reviews another Architect's proposal for flaws, security gaps, or complexity.
    - `finalize`: Synthesizes the discussion into a final Markdown document in `docs/`.

### 2. The Roundtable Protocol (Moderated Discord Chat)
To ensure the discussion is robust and available, we implement a **Roundtable Moderator Service** (within the Go control plane) that manages an internal `RoundtableLog` (JSON) and mirrors it to Discord.

- **Discord as Mirror:** The native substrate is the internal JSON log; Discord acts as a subscriber for visibility and human intervention. If Discord is unreachable, the discussion can still proceed.
- **The Token:** Only one agent holds "the token" to speak at a time. The Moderator assigns the token based on a simple turn-taking or "Request to Speak" (RTS) queue.
- **V1 Moderation Scope:** For Phase 1, the Moderator's consensus detection is a simple prompt asking "Do we have consensus?" rather than complex free-text parsing.
- **Threaded Discussions:** Every new design task creates a dedicated Discord thread.
- **Stateful Emojis:**
    - 🗣️ **Speaking:** Agent is currently posting.
    - ⏳ **Thinking:** Agent is generating a response.
    - 🙋 **RTS:** Agent wants to reply to a specific point.
    - ✅ **Approved:** Agent agrees with the current draft.

### 3. FSM Integration: The `IDEATING` & `RE_IDEATING` States
We introduce a new state before `PLANNING`:

```
SUBMITTED → IDEATING → PLANNING → ...
```

- **IDEATING:** The Roundtable is active. Architects are debating the design.
- **RE_IDEATING (Safe Re-Entry):** To avoid infinite agentic cycles, we add an explicit `RE_IDEATING` state. This state can **only** be triggered by a `HumanIntervention` event (e.g., from Discord). This allows the human owner to send a design back to the drawing board if a major blocker is found, without creating uncontrolled loops in the FSM.

### 4. Iterative Refinement
The approved design is broken down into the 16-task TDD-style plan we saw in Phase 1. If consensus isn't reached in $N$ rounds, the Moderator pings the **Human Owner** for a tie-breaker.

## Impact on User Experience
- **Real-Time Collaboration:** The human owner can jump into the Discord thread at any time to steer the discussion, provide constraints, or "like" a specific proposal.
- **Transparent Reasoning:** The entire design history is preserved in Discord threads, making it easy to see *why* a certain architectural decision was made.
- **Scalable Design:** New models (like a local `DeepSeek-R1` or `Qwen2.5-Coder`) can be added to the Roundtable simply by updating the `routing.yaml` config.

## Security Advantages
- **Heterogeneous Review:** By forcing different models to critique each other, we catch "model-specific blind spots" early.
- **Human-in-the-Loop Design:** The final design must be committed to git, providing a clear audit trail of what the Architects "agreed" to build.
