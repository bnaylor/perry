# Perry Orchestration Skeleton Design

**Date:** 2026-02-27
**Status:** Approved
**Approach:** State Machine First (Approach A)

## Summary

Perry is a secure agentic platform built on a Go core with Python tooling. Phase 1 delivers the orchestration skeleton: a working FSM, Dispatcher, Policy Engine, LLM provider layer, audit pipeline coordination, and Discord sidecar — with agent roles as mock/stub implementations that get lit up incrementally in later phases.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Framework | Plain Python/Go — no AG2, no LangGraph | Maximum control, no framework opacity in a security-critical system |
| Core language | Go | Strong typing, native concurrency, single binary for the control plane |
| Tooling language | Python (subprocess) | AST analysis, Bandit, secrets scanning — ecosystem is Python-native |
| LLM providers | Anthropic, Gemini, Ollama via thin Go interface | ~200-300 lines per provider, zero abstraction tax, provider-specific features accessible |
| Container runtime | Deferred — interface mocked | Pick Docker/Podman later, don't couple the skeleton to a runtime |
| Discord | Sidecar observer | Posts status updates, not the control plane yet |
| Task scope | Code-generation focused, extensible | Interfaces designed for future non-coding workflows |
| Context Packet schema | Claude's strict schema (from discussion) | `researcher_notes` provides the escape hatch; easier to loosen than tighten |

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   perry (Go)                     │
│                                                  │
│  ┌──────────┐  ┌────────────┐  ┌──────────────┐ │
│  │  State    │  │ Dispatcher │  │   Policy     │ │
│  │  Machine  │──│            │──│   Engine     │ │
│  │  (FSM)   │  │ (routing)  │  │ (rules)      │ │
│  └────┬─────┘  └─────┬──────┘  └──────────────┘ │
│       │              │                           │
│  ┌────▼──────────────▼───────┐                   │
│  │      Agent Runner         │                   │
│  │  (calls LLM providers)    │                   │
│  └────────────┬──────────────┘                   │
│               │                                  │
│  ┌────────────▼──────────────┐                   │
│  │     LLM Provider Layer    │                   │
│  │  Anthropic│Gemini│Ollama  │                   │
│  └───────────────────────────┘                   │
├─────────────────────────────────────────────────┤
│              Python Tooling (subprocess)          │
│  ┌─────────┐ ┌──────────┐ ┌───────────────────┐ │
│  │ AST     │ │ Secrets  │ │ Static Analysis   │ │
│  │ Analyzer│ │ Scanner  │ │ (Bandit/ShellChk) │ │
│  └─────────┘ └──────────┘ └───────────────────┘ │
├─────────────────────────────────────────────────┤
│              Sidecars                            │
│  ┌──────────────┐  ┌─────────────────────────┐  │
│  │ Discord Bot  │  │ Observability (metrics)  │  │
│  └──────────────┘  └─────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

## State Machine

### States

| State | Owner | Description |
|-------|-------|-------------|
| `SUBMITTED` | System | Task received, not yet triaged |
| `PLANNING` | Strategist (LLM) | Breaking down intent into requirements, CUJs |
| `RESEARCHING` | Researcher (LLM) | Gathering external context, producing Context Packet |
| `PACKET_VALIDATION` | Deterministic | 3-stage Context Packet validation |
| `CODING` | Coder (LLM) | Generating code from validated Context Packet |
| `AUDITING` | Deterministic + LLM | Multi-gate audit pipeline |
| `EXECUTING` | Executor (sandbox) | Running approved code in isolated container |
| `OUTPUT_REVIEW` | Notary (deterministic) | Validating executor output before delivery |
| `COMPLETED` | System | Artifacts delivered, task closed |
| `FAILED` | System | Unrecoverable failure, human escalation |
| `HUMAN_REVIEW` | Human | Awaiting human decision |

### Transitions

```
SUBMITTED ──────► PLANNING
PLANNING ───────► RESEARCHING
PLANNING ───────► HUMAN_REVIEW        (ambiguous requirements)
RESEARCHING ────► PACKET_VALIDATION
PACKET_VALIDATION ► CODING            (pass)
PACKET_VALIDATION ► RESEARCHING       (revision needed)
PACKET_VALIDATION ► HUMAN_REVIEW      (escalation)
CODING ─────────► AUDITING
AUDITING ───────► EXECUTING           (pass)
AUDITING ───────► CODING              (revision, up to N retries)
AUDITING ───────► HUMAN_REVIEW        (circuit breaker tripped)
EXECUTING ──────► OUTPUT_REVIEW
EXECUTING ──────► CODING              (runtime error, retry)
EXECUTING ──────► FAILED              (circuit breaker)
OUTPUT_REVIEW ──► COMPLETED           (pass)
OUTPUT_REVIEW ──► HUMAN_REVIEW        (unexpected output)
HUMAN_REVIEW ───► PLANNING            (restart with new guidance)
HUMAN_REVIEW ───► FAILED              (human cancels)
```

### Circuit Breakers

- Coder → Auditor: max 3 attempts at current tier, then promote. Max 2 at promoted tier before HUMAN_REVIEW.
- Executor: max 2 retries before FAILED.
- Token/cost budget tracked by Policy Engine — exceeding it forces HUMAN_REVIEW regardless of state.

## Dispatcher

Config-driven, deterministic routing. Answers "where does this work run?"

```yaml
routing:
  defaults:
    strategist: { tier: cloud, provider: anthropic, model: claude-sonnet-4-6 }
    researcher: { tier: cloud, provider: google, model: gemini-2.5-pro }
    coder:      { tier: local, provider: ollama, model: qwen2.5-coder:32b }
    auditor_semantic: { tier: cloud, provider: anthropic, model: claude-sonnet-4-6 }
  escalation:
    max_local_attempts: 3
    promote_to: { tier: cloud, provider: anthropic, model: claude-sonnet-4-6 }
```

Routing logic:
1. Check Policy Engine for hard constraints (sensitive data → local only)
2. Check retry history — promote if local has failed N times
3. Check role defaults from config
4. Check resource availability — degrade gracefully if Ollama is down

## Policy Engine

Deterministic rules. Answers "is this allowed?"

- **Dependency allowlist** — known-safe packages, no human approval needed
- **Budget limits** — per-task and global token/cost caps
- **Immutable safety rules** — never allow eval(), never mount host root, etc.

All config, no LLM reasoning. Unknown = escalate to human.

## Audit Pipeline

Go orchestrates, Python executes:

```
Go audit.Pipeline.Run(code, contextPacket)
    │
    ├─► Python: ast_analyzer.py        Gate 1: Structural conformance
    ├─► Python: secrets_scanner.py      Gate 2: Credential/secret patterns
    ├─► Python: static_analysis.py      Gate 3: Bandit + ShellCheck
    ├─► Python: capability_fence.py     Gate 4: Permitted operations check
    ├─► Go: dependency check            Gate 5: Policy Engine allowlist
    │
    │   ── All deterministic gates pass ──
    │
    ├─► LLM: Intent alignment review
    ├─► LLM: Security review
    └─► LLM: Risk synthesis
```

- Gates 1-4: Python subprocesses returning JSON `{"pass": bool, "findings": [...]}`
- Gate 5: Pure Go (Policy Engine)
- Sequential, fail-fast — Gate 1 failure skips Gates 2-5
- LLM reviews only after all deterministic gates pass
- Each LLM review is a separate focused call
- Overall result: `APPROVE | REJECT | ESCALATE`

## Context Packet

Uses Claude's strict JSON Schema from the design discussion. Validated in 3 stages:

1. **Schema validation** (deterministic) — jsonschema library
2. **Content scanning** (deterministic) — regex secrets/injection pattern detection
3. **Semantic review** (LLM) — scope match, constraint appropriateness, escalation check

The `researcher_notes` section is explicitly tagged UNTRUSTED. All other fields are typed and enumerated.

## Go Package Layout

```
perry/
├── cmd/perry/              # Binary entrypoint
├── internal/
│   ├── fsm/                # State machine engine
│   ├── task/               # Task model and persistence interface
│   ├── agent/              # Agent interface, runner, mocks
│   ├── llm/                # Provider interface + implementations
│   ├── dispatch/           # Dispatcher (tier routing)
│   ├── policy/             # Policy Engine (rules)
│   ├── audit/              # Audit pipeline coordinator
│   ├── executor/           # Executor interface + mock
│   ├── notary/             # Output validation interface + mock
│   └── discord/            # Discord sidecar
├── python/
│   ├── audit/              # AST, secrets, Bandit, capability fence
│   └── context_packet/     # Schema + validation
├── configs/                # Default routing, allowlists
└── docs/                   # Design docs
```

## Phase 1 Deliverables

- Working FSM enforcing the full state transition graph
- Dispatcher with config-driven local/cloud routing
- Policy Engine with dependency allowlists and budget tracking
- LLM provider layer (Anthropic, Gemini, Ollama)
- Agent interfaces with mock implementations
- Python audit tooling callable as subprocesses
- Context Packet validation pipeline
- Discord sidecar posting state transitions
- Structured logging / audit trail

## Deferred to Later Phases

| Item | Phase |
|------|-------|
| Real container runtime (Docker/Podman) | 2+ |
| Notary file scanning (ClamAV, MIME) | 2+ |
| Sovereign Cache (agent persistence) | 3+ |
| Output Gate / airlock | 2+ |
| PM Dashboard / web UI | 3+ |
| Long-term state manager | 3+ |
| Packet signing | 3+ |
| Compute health monitoring | 2+ |
| Non-coding task workflows | 3+ |
