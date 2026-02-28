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
To prevent multi-agent discussions from "spinning out of control," we implement a deterministic **Moderator Service** (within the Go control plane) that manages the Discord interaction.

- **The Token:** Only one agent holds "the token" to speak at a time. The Moderator assigns the token based on a simple turn-taking or "Request to Speak" (RTS) queue.
- **Threaded Discussions:** Every new design task creates a dedicated Discord thread. This keeps the main `#agent-coordination` channel for high-level status.
- **Stateful Emojis:**
    - 🗣️ **Speaking:** Agent is currently posting.
    - ⏳ **Thinking:** Agent is generating a response.
    - 🙋 **RTS:** Agent wants to reply to a specific point.
    - ✅ **Approved:** Agent agrees with the current draft.
- **Consensus & Stopping Criteria:**
    - **Convergence:** Discussion ends when all Architects have posted a ✅ on the same draft.
    - **Deadlock:** If consensus isn't reached in $N$ rounds, the Moderator pings the **Human Owner** for a tie-breaker.
    - **Finalization:** Once approved, the Orchestrator commits the design doc to the repository and transitions the FSM to `PLANNING`.

### 3. FSM Integration: The `IDEATING` State
We introduce a new state before `PLANNING`:

```
SUBMITTED → IDEATING → PLANNING → ...
```

- **IDEATING:** The Roundtable is active. Architects are debating the design in Discord.
- **PLANNING:** The approved design is broken down into the 16-task TDD-style plan we saw in Phase 1.

### 4. Iterative Refinement (The "Loop Back")
The FSM allows a "Loop Back to IDEATING" from any state if a major architectural blocker is found (e.g., during `EXECUTING` or `AUDITING`). This triggers a new Roundtable session to solve the specific blocker.

## Impact on User Experience
- **Real-Time Collaboration:** The human owner can jump into the Discord thread at any time to steer the discussion, provide constraints, or "like" a specific proposal.
- **Transparent Reasoning:** The entire design history is preserved in Discord threads, making it easy to see *why* a certain architectural decision was made.
- **Scalable Design:** New models (like a local `DeepSeek-R1` or `Qwen2.5-Coder`) can be added to the Roundtable simply by updating the `routing.yaml` config.

## Security Advantages
- **Heterogeneous Review:** By forcing different models to critique each other, we catch "model-specific blind spots" early.
- **Human-in-the-Loop Design:** The final design must be committed to git, providing a clear audit trail of what the Architects "agreed" to build.
