# ADR-002: Relay-Invoked Ephemeral Sessions (Option B)

**Status:** Accepted
**Date:** 2026-03-03
**Participants:** BTClaude, BTGemini, .scromp.

## Context

Three invocation models were considered for how Design Layer agents interact with Discord:

**Option A — Long-running CLI sessions (status quo).** Agents poll Discord via MCP calls in a loop. Problems: burns tokens on polling, context drifts over long sessions, CLI frameworks fight the idle loop pattern (interpreting it as pointless), and agents sometimes exit polling prematurely.

**Option B — Relay-invoked ephemeral sessions.** The relay holds the Discord WebSocket connection and listens for events. When something interesting happens, it spawns a CLI session with context, the agent responds, and the session ends. The relay manages state between invocations.

**Option C — Persistent daemon agents.** A Go service with direct LLM API connections, in-process conversation state, responding to Discord events directly. No CLI involvement for design work.

## Decision

**Option B: Relay-invoked ephemeral sessions.**

The relay listens for Discord events (mentions, commands, cross-agent stimuli). When a trigger fires, it spawns a CLI session using vendor tools (`claude -p "..."` or `gemini -p "..."`) with a context payload containing recent messages, invocation reason, and state pointers. The agent processes the context, posts its response to Discord, and the session terminates. The relay persists conversation state between invocations.

### Why not A?
Token-burning polling loops are wasteful and fragile. The CLI frameworks aren't designed for indefinite idle loops. Human must manually start and babysit sessions.

### Why not C?
C is the theoretical endgame but requires reimplementing what `claude-code` and `gemini-cli` provide for free: filesystem access, git integration, permission management, MCP server support, superpowers/skills. The maintenance burden of keeping parity with Anthropic and Google on agentic tooling is prohibitive.

### Why B?
B preserves all CLI tooling (git, filesystem, superpowers, MCP servers) while eliminating polling. The relay already exists, already has a Discord WebSocket via `discordgo`, and already has SQLite for state. Agents are reactive rather than polling — they wake up with purpose, do their work, and return to sleep.

## Context Rehydration

Since sessions are ephemeral, agents need context on wake-up. The relay provides **raw materials**, not summaries (summarization is LLM work; the relay is deterministic Go code):

1. **Message log** — Last N messages from the relevant thread, verbatim.
2. **State pointers** — Which files/docs are active, which decisions have been recorded.
3. **Invocation reason** — "Human said X" or "other agent posted Y" or "timer expired on pending decision."

Long-term memory lives in the filesystem: `CLAUDE.md`, `MEMORY.md`, and the project's `docs/decisions/` directory. The relay doesn't duplicate this — it just tells agents what happened since last time and why they're being woken up.

## Consequences

- The relay becomes a process supervisor and context manager, not just a message forwarder.
- Agent sessions are short-lived and focused, reducing context drift.
- No tokens are spent on polling — agents only run when there's work to do.
- The `-p` (prompt) flag on both `claude` and `gemini-cli` is the likely invocation mechanism.
- Superpowers, skills, and MCP servers load automatically from the project directory.
- Migration path to Option C remains open if the CLI tooling burden becomes justified later.
