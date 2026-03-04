# ADR-003: Two-Tier State Split

**Status:** Accepted
**Date:** 2026-03-03
**Participants:** BTClaude, BTGemini, .scromp.

## Context

The human owner raised the Outside Context Problem (OCP): if all Design Layer state lives inside the Perry repository, the system can only think about Perry. The relay needs to be project-agnostic so it can eventually manage multiple projects — initiating new projects, switching between them, and maintaining awareness of a larger universe.

This mirrors a real architectural concern: the mechanism for managing things shouldn't be rooted inside one of the things being managed.

## Decision

State is split into two tiers:

### Tier 1: Global Orchestration State (Relay-Level)

**Location:** `~/.perry/perry.db` (SQLite, outside any project repo)

**Contents:**
- **Project registry** — Paths to managed repos/worktrees
- **Agent registry** — Who's available, what CLI to spawn, identity info
- **Conversation state** — Active roundtable sessions, their status, participants
- **Trigger configuration** — What events wake which agents
- **Message log** — Recent conversation history for rehydration

**Schema (core tables):**
```sql
roundtable_sessions(id, project_path, topic, status, created_at, updated_at)
roundtable_messages(id, session_id, author, content, timestamp)
```

### Tier 2: Project Design State (Repo-Level)

**Location:** Inside each managed repository

**Contents:**
- `docs/decisions/` — ADR-style decision documents specific to that project
- `CLAUDE.md` / `MEMORY.md` — Agent context within that project
- Implementation plans, specs, design docs
- `.discord-coordination.json` — Discord channel mappings for that project

### How They Interact

The relay (Tier 1) is the "outer shell" — it knows about all projects. Each project (Tier 2) is a leaf that only knows about itself. When the relay wakes an agent, it tells it which project directory to work in. The agent reads project-level state from the filesystem and gets conversation context from the relay's invocation payload.

## Consequences

- The relay is no longer Perry-specific. It can manage N projects with the same infrastructure.
- `perry-relay` may eventually migrate out of `cmd/perry-relay/` into its own repository or a more generic location. For bootstrapping, it stays where it is.
- Project-level ADRs are version-controlled with the project's source code — they travel with the repo.
- Global state (sessions, agent registry) is not version-controlled — it's operational state, like a database.
- The existing `perry.db` in `.perry/perry.db` (project-local) remains for Execution Layer state (tasks, transitions, audit records). The global `~/.perry/perry.db` is a separate database for Design Layer orchestration.
