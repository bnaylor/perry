# Design: Phase 4 — Discord Relay & Command Interface

**Date:** 2026-03-02
**Status:** Approved (Architect Roundtable Consensus)

## Overview

Phase 4 formalizes the Perry platform's interaction with Discord. Instead of the Orchestrator talking directly to Discord, we are implementing a **Sidecar Relay** architecture. This maintains security isolation for the core logic while providing a rich, multi-agent presence on Discord.

## Architecture

```
[ Discord UI ]
      ▲
      │ (Websockets/REST)
      ▼
[ perry-relay (Sidecar) ] <───┐
      │                       │ (SQLite WAL Polling)
      ▼                       │
[ perry.db (Event Bus) ] <────┘
      ▲
      │ (SQL)
      ▼
[ perry (Orchestrator) ]
```

### 1. The Relay Sidecar (`cmd/perry-relay`)
A standalone Go process that:
- Connects to Discord using multiple bot tokens.
- Maps Perry `agent.Role` identifiers to themed Phineas & Ferb identities.
- Maintains a `relay_cursor` to ensure every event is processed exactly once.
- Formats structured event data into human-readable Discord messages using `text/template`.

### 2. SQLite as Event Bus
We leverage the existing SQLite database as a durable, inspectable message queue.
- **Outbound Stream:** The `transitions` and `agent_calls` tables act as the source of truth for "what happened."
- **Inbound Stream:** A new `commands` table stores instructions sent from Discord (e.g., `!status`, `!pause`, `!approve`).
- **State Table:** A new `internal_state` table stores the `relay_cursor` (the last processed ID from the event tables).

### 3. Identity Pool
To reinforce the "Architect Roundtable" concept, each role is represented by a specific bot account:

| Role | Persona | Responsibility |
| :--- | :--- | :--- |
| **Strategist** | Major Monogram | Mission briefings, task initialization, threading. |
| **Researcher** | Phineas | Context gathering, "I know what we're gonna do today!" |
| **Coder** | Ferb | Silent, efficient code generation. |
| **Auditor (Semantic)** | Carl | Meticulous technical checklists. |
| **Auditor (Shadow)** | Dr. Doofenshmirtz | Adversarial PoC exploits (-inators). |
| **Orchestrator** | Agent P | The "silent professional" driving the FSM. |

## Communication Protocol

### Threading
- **Task Threads:** Every task creates a dedicated Discord thread on `SUBMITTED`. 
- **Formatting:** All bot updates for a task MUST occur within its thread.
- **Main Channel:** Reserved for high-level announcements and cross-task human directives.

### Polling Logic
- **Interval:** 500ms - 1000ms.
- **WAL Mode:** Both processes operate in Write-Ahead Logging mode to allow concurrent read/write without contention.

## Security Considerations
- **Token Protection:** Discord tokens are stored in `configs/discord.yaml` (excluded from git) or passed via environment variables.
- **Network Isolation:** The core `perry` binary remains restricted from making outbound Discord API calls. 
- **Command Validation:** Inbound commands must be validated by the Orchestrator against the Policy Engine before execution.
