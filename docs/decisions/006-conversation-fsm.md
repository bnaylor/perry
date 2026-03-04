# ADR-006: Conversation State Machine

**Status:** Accepted
**Date:** 2026-03-03
**Participants:** BTClaude, BTGemini, .scromp.

**Context:** For the Design Layer to be 'always-on' and contextually aware, the `perry-relay` must track the status of active conversations across different projects and agent identities. This state must be persistent and multi-project aware.

**Decision:**
The `perry-relay` will implement a lightweight Finite State Machine (FSM) within its global SQLite database to manage Design Layer sessions.

**Conversation States:**
1.  **Idle**: No active roundtable session for the given project/topic.
2.  **Active**: A discussion is ongoing, agents are being invoked, or a debounce window is active.
3.  **Awaiting Human**: The agents have reached a technical impasse or completed their proposal and are waiting for scromp to provide feedback or a 'Go' signal.
4.  **Consensus**: A final design has been agreed upon, the ADR has been drafted, and the 'Consensus Approved' status is recorded in the ledger.

**Storage Schema:**
The Relay's global database will maintain:
- `roundtable_sessions`: (id, project_path, topic, status, created_at, updated_at)
- `roundtable_messages`: (id, session_id, author, content, timestamp)

**Consequences:**
- Provides the Relay with the ability to manage multiple projects concurrently.
- Ensures design context is never lost across agent CLI invocations.
- Enables the Relay to 'wake up' and resume a discussion exactly where it left off.
