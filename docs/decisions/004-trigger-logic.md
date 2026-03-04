# ADR-004: Reactive Trigger Logic — Gemma Sentry

**Status:** Accepted
**Date:** 2026-03-03
**Participants:** BTClaude, BTGemini, .scromp.

## Context

The Roundtable is moving to Option B (Relay-invoked ephemeral CLI sessions). To prevent redundant or unnecessary agent invocations, the `perry-relay` requires a logic layer to discriminate meaningful design-level stimuli from Discord channel noise.

Hard-coded pattern matching (mentions, `!commands`) works but is rigid — it can't handle natural language like "Hey, let's have a design discussion" or distinguish between discussion requests and implementation requests without explicit syntax.

## Decision

The `perry-relay` will use a **Gemma Sentry** — a local small model (Gemma via Ollama on CPU) — as an intelligent trigger classifier, wrapped in a two-stage pipeline:

### Stage 1: Hard Filter (Go code, deterministic)
Skip messages that are obviously irrelevant:
- Wrong channel (not a coordination channel)
- Bot's own messages (prevent self-triggering loops)
- Messages from unknown/unauthorized users

### Stage 2: Gemma Classification (Ollama API call, ~200ms)
The Sentry classifies the message into a structured JSON response:
```json
{
  "action": "wake",
  "targets": ["btclaude", "btgemini"],
  "mode": "discuss",
  "reason": "human requesting design review of auth system"
}
```
Or simply: `{ "action": "ignore" }`

**Valid modes:** `discuss` (open-ended design conversation), `implement` (scoped task for a single agent), `review` (feedback on existing work).

### Stage 3: Go Validation (deterministic, fail-closed)
- Is the JSON well-formed?
- Are targets valid registered agents?
- Is the mode a recognized value?
- If any check fails: **ignore the message** (fail-closed). Do not wake agents on garbage output.

### Debounce Window
The relay batches rapid messages (e.g., multiple messages within 10 seconds) into a single classification call. This prevents thrashing during rapid-fire human typing.

### Fallback Triggers
`!discuss`, `!roundtable`, `!implement`, and direct `@mentions` remain as **hard-coded fallbacks** that bypass the Sentry. These guarantee agents can always be woken even if Ollama is down or Gemma is misbehaving.

## Why a Model Instead of Rules?

Natural language triggers like "Hey guys, you awake?" or "What do you think about..." are more intuitive than `!discuss`. The Sentry also handles invocation mode detection — distinguishing "let's brainstorm" from "fix this bug" — without requiring the human to learn command syntax.

Cost is near-zero: Gemma runs on localhost CPU. No API calls, no tokens burned, no network dependency.

## Scope Boundary

The Sentry's job is strictly **classification**. It does NOT:
- Summarize or sift messages (the big models handle relevance)
- Make decisions about conversation state (the FSM handles that)
- Modify or filter the context payload (the spawner assembles context)

One question: "Should I wake agents, and if so, who and in what mode?"

## Consequences

- Significant token savings by eliminating idle polling loops.
- Natural language interaction — no need to learn IRC-bot-style commands.
- Requires Ollama running on the relay host (already available on the local cluster).
- Fail-closed design means a Gemma failure degrades gracefully to hard-coded fallback triggers.
- The Sentry's classification prompt becomes a tunable artifact — adjusting behavior without code changes.
