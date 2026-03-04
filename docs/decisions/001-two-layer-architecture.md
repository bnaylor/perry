# ADR-001: Two-Layer Architecture

**Status:** Accepted
**Date:** 2026-03-03
**Participants:** BTClaude, BTGemini, .scromp.

## Context

Perry's existing pipeline (Strategist → Researcher → Coder → Auditor → Executor) handles structured, gated task execution well. However, the *design and ideation* work that feeds this pipeline has been running ad-hoc — human-orchestrated relay sessions between CLI-based AI agents on Discord. The human owner acts as the scheduler, context bridge, and consensus detector, which doesn't scale.

The core tension: CLI sessions are execution tools being used for open-ended design work, and Discord (a communication channel) is being used as a control plane. Neither is great at the other's job.

## Decision

We adopt a **Two-Layer Architecture** that separates concerns:

### Design Layer
- **Purpose:** Always-on agents that discuss, brainstorm, plan, and reach consensus.
- **Character:** Conversational, stateful, open-ended. Lives in "OCP-land" — aware of the broader universe of projects and concerns.
- **Agents:** BTClaude and BTGemini (and potentially others), operating as co-equal design partners with a human owner.
- **Orchestrator:** The upgraded `perry-relay`, which manages agent lifecycle, conversation state, and trigger logic.

### Execution Layer
- **Purpose:** The existing Perry pipeline — structured, gated, task-scoped work.
- **Character:** Deterministic safety gates, least-privilege agents, fail-closed defaults.
- **Agents:** Strategist, Researcher, Coder, Auditor, Executor — each with scoped authority.
- **Orchestrator:** The existing FSM in `internal/orchestrator/`.

### The Bridge
The two layers connect through **artifacts**: the Design Layer produces decision documents (ADRs), specs, and plans. The Execution Layer consumes these as input to the Strategist, which translates them into scoped tasks. The Execution Layer reports results back to the Design Layer for review.

Neither layer has direct authority over the other. The Design Layer cannot bypass audit gates. The Execution Layer cannot modify design decisions.

## Consequences

- The relay evolves from a passive message forwarder into the Design Layer Orchestrator — a fundamentally different role.
- Design agents (BTClaude, BTGemini) are peers, not pipeline stages. They don't have the same role/privilege model as Execution Layer agents.
- The "Architect Roundtable" becomes a software-supported protocol, not a human-orchestrated workaround.
- The OCP concern (project scoping) is addressable because the Design Layer sits *above* any single project.
- Implementation can be phased: the relay upgrade is independent of changes to the Execution Layer.
