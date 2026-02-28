# Perry: Secure Agentic Platform

## What Is This?

Perry is a security-first platform for orchestrating AI agents that do real work — starting with code generation, but designed to extend to other task types (research, monitoring, automation). The core principle is **managed autonomy**: agents operate with least privilege, deterministic safety gates can't be talked out of their findings, and no single agent holds all the keys.

## The Agent Roles

| Role | What It Does | Compute Tier |
|------|-------------|--------------|
| **Strategist** (narrowed PM) | Translates user intent into requirements. Owns the "why." Talks to the human. | Cloud (Claude/Gemini) |
| **Researcher** | Gathers external context (APIs, docs). Produces a structured **Context Packet** — the sole authorized channel for external info to enter the pipeline. Has network access but cannot write code. | Cloud (Gemini) |
| **Coder** | Generates code from the Context Packet. Operates in a network-isolated environment. Cannot reach the internet. | Local-first (Ollama), escalates to cloud |
| **Auditor** | Mandatory verification gate between code and execution. Two layers: deterministic (AST, secrets scan, Bandit, capability fence) and LLM-powered (intent alignment, security review). Cannot be bypassed. | Deterministic gates run locally; LLM reviews run on cloud |
| **Executor** | Runs approved code in an ephemeral, isolated sandbox. No persistent storage. Output goes through the Notary before reaching the host. | Local (Docker/Podman — TBD) |

## Key Architectural Decisions

**Not everything is an agent.** Three critical components are deliberately non-LLM:
- **Dispatcher** — config-driven routing that decides which compute tier handles a task. Rule-based, not a judgment call.
- **Policy Engine** — dependency allowlists, budget limits, immutable safety rules. Fully deterministic. Can't be persuaded.
- **Notary** — validates executor output (file type, manifest match, scanning) before it touches the host filesystem. No agentic logic.

**The Context Packet is the security boundary.** It's a strict JSON Schema document that forces the Researcher to restate external information in typed, enumerated fields. Free-text fields exist but are tagged UNTRUSTED. Secrets travel as env var references, never values. Three-stage validation: schema check → content scan → semantic review.

**The Auditor pipeline is fail-fast and layered.** Five deterministic gates run before any LLM sees the code. If Gate 1 fails, Gates 2-5 don't run. LLM reviews are three separate focused calls (intent, security, synthesis), not one monolithic prompt. Everything fails closed — timeouts and unparseable responses default to ESCALATE, never APPROVE.

**Circuit breakers are structural.** The Coder gets 3 attempts at the local tier, then the Dispatcher promotes to cloud. 2 more attempts at cloud tier, then human escalation. The Executor gets 2 retries. Token/cost budgets force human review when exceeded. These limits are enforced by the FSM and Policy Engine, not by agent self-discipline.

**Tiered compute: local-first, cloud-escalated.** Local NVIDIA boxes handle high-token grunt work (coding, tests, formatting). Cloud models (Claude, Gemini) handle reasoning-heavy tasks (strategy, auditing, complex code). The Dispatcher manages routing and escalation automatically.

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Control plane | **Go** | Strong typing, native concurrency, single binary |
| Audit tooling | **Python** (subprocess) | AST module, Bandit, jsonschema — ecosystem is Python-native |
| Orchestration | **Custom FSM** (no AG2, no LangGraph) | Maximum control, no framework opacity in security-critical paths |
| LLM providers | **Thin Go interface** | ~200-300 lines per provider (Anthropic, Gemini, Ollama). No abstraction frameworks. |
| Sandbox | **Docker or Podman** (TBD) | Behind an interface, not yet committed to a runtime |
| Human interface | **Discord** (sidecar) | Status updates and commands. Not the control plane — the Go binary is. |
| Observability | **Prometheus-style metrics** (planned) | Each component emits signals for dashboards and the Discord bot |

## State Machine

Every task follows this path (simplified happy path):

```
SUBMITTED → PLANNING → RESEARCHING → PACKET_VALIDATION → CODING → AUDITING → EXECUTING → OUTPUT_REVIEW → COMPLETED
```

Failure paths go to `HUMAN_REVIEW` (recoverable) or `FAILED` (terminal). The FSM enforces that no state can be skipped — you cannot reach EXECUTING without passing through AUDITING.

## The "Airlock" Model for Output

The Executor writes to an ephemeral `/out` directory. The Notary (non-agent service) validates the output against the PM's manifest (expected file types, checksums), scans it, and only then copies it to the host's `~/agent_outputs/` directory. The container is destroyed immediately after. No persistent mounts between containers and the host filesystem — this prevents latent payload, symlink, and permission escalation attacks.

## Phasing

**Phase 1 (current): Orchestration skeleton.** Working FSM, Dispatcher, Policy Engine, LLM provider layer, audit pipeline coordination, mock agents. A task can flow through every state with deterministic enforcement. Python audit tooling (AST, secrets, context packet validation) is real.

**Phase 2: Real execution.** Container runtime (Docker/Podman), Notary file scanning, Output Gate, compute health monitoring. Agents start producing real output.

**Phase 3: Full platform.** Sovereign Cache (agent persistence), PM Dashboard, long-term state manager, packet signing, non-coding task workflows.

## Where The Design Came From

The architecture was developed in a three-way discussion between the project owner, Claude, and Gemini. The full discussion log is in `docs/agentic_design/Secure Agentic Loop Platform Architecture.md` (very long — read this doc instead unless you need deep context on a specific design decision). Claude-generated specs for the Context Packet, Auditor Pipeline, and PM Authority Split are in `docs/agentic_design/claude_artifacts/`.

Key areas where Claude and Gemini diverged and how we resolved them:
- **Context Packet rigidity:** Claude wanted strict typing everywhere; Gemini wanted a "Flexible Core." We went with Claude's strict schema — the `researcher_notes` field provides the escape hatch, explicitly tagged untrusted.
- **Audit latency:** Gemini flagged that full 7-stage audits on every iteration could be slow. We acknowledged this as a future optimization (incremental auditing on diffs) but not phase 1.
- **Framework choice:** The discussion assumed AG2/AutoGen. We decided on plain Python/Go with no framework — maximum control, no framework opacity.
