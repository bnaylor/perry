# Design Layer Architecture

**Date:** 2026-03-03
**Authors:** BTClaude, BTGemini, .scromp.
**Status:** Accepted — ready for implementation

## Overview

This document describes the architecture for Perry's Design Layer — the always-on, conversational system that handles ideation, brainstorming, architectural discussion, and consensus-building. It complements the existing Execution Layer (the Perry pipeline) and connects to it through artifact-based handoff.

The Design Layer was conceived to solve a specific problem: the human owner was acting as the scheduler, context bridge, and consensus detector for multi-agent design discussions. This worked for bootstrapping but doesn't scale — it makes the human the bottleneck in exactly the process meant to reduce bottlenecks.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  Discord                         │
│  #perry-coordination (or per-project channels)   │
└──────────┬──────────────────────┬────────────────┘
           │ WebSocket            │ WebSocket
           ▼                      ▼
┌──────────────────────────────────────────────────┐
│              perry-relay (upgraded)               │
│                                                   │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────┐ │
│  │ Gemma Sentry │  │ Conversation │  │ Agent    │ │
│  │ (classifier) │  │ State Machine│  │ Spawner  │ │
│  └─────────────┘  └──────────────┘  └──────────┘ │
│                                                   │
│  Global State: ~/.perry/perry.db                  │
│  (sessions, messages, agent registry, projects)   │
└──────────┬──────────────────────┬────────────────┘
           │ spawn CLI            │ spawn CLI
           ▼                      ▼
┌──────────────────┐  ┌──────────────────┐
│  claude -p "..."  │  │  gemini -p "..." │
│  (ephemeral)      │  │  (ephemeral)     │
│                   │  │                  │
│  Has access to:   │  │  Has access to:  │
│  - filesystem     │  │  - filesystem    │
│  - git/worktrees  │  │  - git/worktrees │
│  - superpowers    │  │  - MCP servers   │
│  - MCP servers    │  │  - skills        │
│  - MEMORY.md      │  │  - MEMORY.md     │
└──────────────────┘  └──────────────────┘
           │                      │
           ▼                      ▼
┌──────────────────────────────────────────────────┐
│           Project Repository                      │
│                                                   │
│  docs/decisions/     ← ADRs (Design Layer output) │
│  CLAUDE.md/GEMINI.md ← Agent instructions         │
│  MEMORY.md           ← Cross-session state        │
│  .perry/perry.db     ← Execution Layer state      │
└──────────────────────────────────────────────────┘
           │
           ▼ (artifact handoff)
┌──────────────────────────────────────────────────┐
│        Perry Execution Layer (existing)           │
│  Strategist → Researcher → Coder → Auditor → ... │
└──────────────────────────────────────────────────┘
```

## Components

### Gemma Sentry (Trigger Classifier)

The relay receives every message from Discord via its existing WebSocket connection. Instead of hard-coded pattern matching (mentions, `!commands`), the relay uses a **local small model** (Gemma via Ollama) to classify incoming messages with natural-language understanding.

**Message classification pipeline:**
```
Discord WebSocket message arrives
  → Hard filter (wrong channel? bot's own message? → skip)
  → Debounce window (batch rapid messages, ~10 seconds)
  → Gemma classification (Ollama API call, ~200ms on CPU)
     Returns structured JSON:
     { "action": "wake", "targets": ["btclaude", "btgemini"],
       "mode": "discuss", "reason": "human requesting design review" }
     Or: { "action": "ignore" }
  → Go validation (well-formed output? valid targets? valid mode?)
  → If valid: spawn agent(s). If invalid: fail-closed, ignore.
```

**Why a model instead of rules?** Hard-coded triggers (`!discuss`, `@mention`) are IRC-bot-era thinking. A small local model handles natural language nuance:
- "Hey, let's have a design discussion" → wake both agents in discuss mode
- "BTClaude, can you fix the typo in line 42?" → wake one agent in implement mode
- Random chatter, emoji reactions → ignore
- "What do you guys think about..." → wake both in discuss mode

**Why Gemma specifically?** It runs on CPU with near-zero cost (localhost Ollama). The relay host is expected to be beefy enough to run a small model. No API calls, no tokens burned, no network dependency.

**Safety:** The Sentry's job is strictly classification — it does NOT sift, summarize, or filter messages. It answers one question: "should I wake agents, and if so, who and in what mode?" Go code validates the structured output before acting. If Gemma returns garbage, no agents are woken (fail-closed).

**Implementation:** The Gemma call is a function inside the relay's message handler, using the existing Ollama provider interface from `internal/llm/`. No new service, no sidecar — just `ollama.Generate(classificationPrompt + messageContent)` inline.

The debounce window batches rapid messages (e.g., multiple messages in 10 seconds) into a single classification call, preventing thrashing during rapid conversation.

### Conversation State Machine

A simple FSM in the relay tracks the state of each roundtable session:

```
idle → active → awaiting_human → consensus → idle
                    ↑       │
                    └───────┘  (human responds, back to active)
```

- **idle** — No active discussion. Triggers can start one.
- **active** — Agents are exchanging messages. The relay may be spawning sessions.
- **awaiting_human** — An agent has asked the human a question or requested a decision. Agents won't be spawned until the human responds.
- **consensus** — The discussion has reached agreement. The relay records the outcome and transitions back to idle.

State is persisted in `~/.perry/perry.db`:
```sql
roundtable_sessions(id, project_path, topic, status, created_at, updated_at)
roundtable_messages(id, session_id, author, content, timestamp)
```

### Agent Spawner

When the discriminator fires, the spawner:

1. Reads the current session state from SQLite
2. Assembles a context payload:
   - Last N messages from the conversation (verbatim, not summarized)
   - The invocation reason ("human mentioned you", "BTGemini asked you a question")
   - Pointers to relevant files/decisions
3. Spawns the appropriate CLI: `claude -p "<context + instruction>"` or `gemini -p "<context + instruction>"`
4. The CLI session runs in the project directory, giving the agent access to filesystem, git, superpowers, MCP servers
5. The agent reads Discord state, formulates a response, posts it, and exits
6. The spawner records any new messages in the session log

### Two-Tier State

See [ADR-003](../decisions/003-two-tier-state-split.md) for the full rationale.

**Global state** (`~/.perry/perry.db`): project registry, agent registry, session state, message logs. Managed by the relay. Project-agnostic.

**Project state** (in each repo): ADRs in `docs/decisions/`, agent memory in `MEMORY.md`, execution state in `.perry/perry.db`. Version-controlled with the project.

## How a Roundtable Session Works (End to End)

1. Human posts in `#perry-coordination`: "Let's discuss the new auth system for Project X."
2. Relay's Gemma Sentry classifies the message: `{ action: "wake", targets: ["btclaude", "btgemini"], mode: "discuss", reason: "human requesting design discussion about auth" }`.
3. Relay creates a new `roundtable_sessions` row: `{project: "/path/to/project-x", topic: "auth system", status: "active"}`.
4. Relay spawns BTClaude with context: recent messages, invocation reason, project path.
5. BTClaude reads `MEMORY.md`, relevant ADRs, and the conversation. Posts a response to Discord. Session ends.
6. BTGemini's mention in BTClaude's response triggers the discriminator again (cross-agent stimulus, debounced).
7. Relay spawns BTGemini with updated context. BTGemini responds. Session ends.
8. This continues until consensus is reached or the human signals completion.
9. Relay transitions session to `consensus`. Agents write ADR(s) to `docs/decisions/`.
10. When the human says "go implement," the Strategist reads the ADR and creates a scoped task in the Execution Layer.

## Relationship to Execution Layer

The Design Layer and Execution Layer are connected but independent:

- **Design → Execution:** ADRs and specs flow down. The Strategist reads decision docs to understand *what* to build and *why*. It doesn't need to know about the roundtable discussion that produced them.
- **Execution → Design:** Results flow up. When the Execution Layer completes a task (or fails), the relay can notify the Design Layer agents for review. This uses the existing transition-reporting mechanism in `perry-relay`.
- **No bypass:** Design Layer agents cannot skip audit gates. Execution Layer agents cannot modify ADRs. The artifact boundary is the security boundary.

## Implementation Phases

### Phase 1: Trigger + Spawn
Add the Gemma Sentry and agent spawner to `perry-relay`. Sentry classifies incoming Discord messages via local Ollama. Agents can be woken by natural language triggers. Context payload is basic (last N messages + reason). No conversation FSM yet — sessions are independent.

### Phase 2: Conversation State
Add the session FSM and SQLite tables. The relay tracks which discussions are active and prevents duplicate spawns. `awaiting_human` state prevents agent thrashing when waiting for human input. SQLite advisory locks for agent contention.

### Phase 3: Multi-Project
Move to `~/.perry/perry.db` for global state. Add project registry. The relay can manage roundtables across multiple repos. Project scaffolding scripts to populate new projects with required instruction files (`CLAUDE.md`, `GEMINI.md`, `docs/decisions/`, superpowers, etc.) and handle upgrades to existing projects.

### Phase 4: Polish
Debounce tuning, context window optimization, session keep-alive for active conversations, and integration with the Execution Layer's Strategist. Spawn latency measurement and optimization.

## Resolved Questions

- **Invocation mode:** The Gemma Sentry classifies messages into modes (`discuss`, `implement`, etc.) using natural language understanding. No need for rigid command prefixes — though `!discuss` and `!implement` can serve as reliable fallbacks.
- **Agent contention:** SQLite transactions with advisory locks — a solved problem. Agents writing to files use dedicated worktrees; database access is serialized through SQLite's built-in locking.

## Open Questions

- **Context window limits:** How many messages can we feed into a CLI invocation before hitting token limits? The relay sends the full recent window (last N messages chronologically) and lets the big model decide relevance. If this becomes a problem, context sifting can be added as an optimization later.
- **Cost management:** Each agent invocation costs tokens. Should the relay enforce a per-session token budget? This ties into the billing infrastructure BTGemini recently built.
- **Spawn latency:** Each ephemeral session means CLI startup + context loading + API call. For rapid back-and-forth this could be 15-30 seconds per turn. Mitigation options include session keep-alive during active conversations (hybrid of Option B and A). Measure first, optimize later.

## Related Decisions

- [ADR-001: Two-Layer Architecture](../decisions/001-two-layer-architecture.md)
- [ADR-002: Invocation Model (Option B)](../decisions/002-invocation-model.md)
- [ADR-003: Two-Tier State Split](../decisions/003-two-tier-state-split.md)
- [ADR-004: Trigger Logic](../decisions/004-trigger-logic.md)
- [ADR-005: ADR Format Convention](../decisions/005-adr-format.md)
- [ADR-006: Conversation State Machine](../decisions/006-conversation-fsm.md)
