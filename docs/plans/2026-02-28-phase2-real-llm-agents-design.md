# Phase 2: Real LLM Agents — Design

**Date:** 2026-02-28
**Status:** Approved
**Prerequisite:** Phase 1 (orchestration skeleton) — complete

## Summary

Phase 2 replaces mock LLM providers and stub agent prompts with real implementations. When complete, perry can accept a task description, route it through real LLMs (Claude, Gemini, local Ollama models), and produce real output — requirements, Context Packets, and generated code — flowing through the full state machine.

## Scope

**In scope:**
- Three LLM provider implementations (Anthropic, Gemini, Ollama)
- Provider map for Dispatcher-driven routing
- Real agent system prompts with structured JSON output
- Runner upgrade to use provider map + routing decisions
- YAML config loading
- CLI that accepts a task description argument
- CompletionRequest model field for per-call model selection

**Out of scope (deferred):**
- Python audit subprocess calls (Phase 3)
- Container runtime / sandbox (Phase 3)
- Layered filesystem / Mirror Cage (Phase 3)
- Patch-based auditing (Phase 3)
- Notary file scanning (Phase 3)
- Discord bot (Phase 4)
- Observability (Phase 4)

## LLM Provider Implementations

Three providers, each ~200-300 lines, satisfying the existing `llm.Provider` interface.

### Anthropic Provider

- Anthropic Messages API (`POST /v1/messages`) via direct HTTP
- System message extracted from messages array (Anthropic puts it top-level)
- `ANTHROPIC_API_KEY` from environment
- `Extras` field supports provider-specific features
- Token usage mapped from response

### Gemini Provider

- Gemini `generateContent` REST API via direct HTTP
- Role mapping: `user`/`assistant` → `user`/`model`
- `GEMINI_API_KEY` from environment
- `usageMetadata` mapped to `Usage` struct

### Ollama Provider

- Ollama chat completions API (`POST /api/chat`) via direct HTTP
- Configurable base URL (default `http://localhost:11434`)
- Model name from routing config
- No API key — local service
- Graceful handling of connection errors

### Provider Map

```go
providers := map[string]llm.Provider{
    "anthropic": anthropic.New(os.Getenv("ANTHROPIC_API_KEY")),
    "google":    gemini.New(os.Getenv("GEMINI_API_KEY")),
    "ollama":    ollama.New("http://localhost:11434"),
    "mock":      llm.NewMockProvider("mock", "mock response"),
}
```

### Testing Strategy

- Unit tests per provider using `httptest.NewServer` — no real API calls in CI
- Integration test files (skipped unless env var set) for real API verification
- Mock provider stays for all other package tests

## Agent Structured Output

Agents produce JSON responses. The Runner parses them into a typed struct.

```go
type AgentOutput struct {
    Role    Role
    Content string         // raw LLM response
    Parsed  map[string]any // parsed JSON (nil if parse fails)
    Usage   llm.Usage
}
```

### Agent Output Formats

**Strategist** — produces requirements:
```json
{
  "requirements": ["..."],
  "cuj_ids": ["CUJ-001"],
  "complexity": "standard|complex",
  "task_summary": "..."
}
```

**Researcher** — produces Context Packet (matching existing JSON schema):
```json
{
  "packet_meta": {...},
  "task_reference": {...},
  "external_apis": [...],
  "constraints": {...},
  "researcher_notes": {...}
}
```

**Coder** — produces code files:
```json
{
  "files": [
    {"path": "main.py", "content": "..."}
  ],
  "dependencies": ["requests>=2.28"],
  "explanation": "..."
}
```

**Auditor (semantic)** — produces verdict:
```json
{
  "verdict": "APPROVE|REJECT|ESCALATE",
  "intent_alignment": {"pass": true, "notes": "..."},
  "security_review": {"pass": true, "notes": "..."},
  "findings": []
}
```

## Dispatcher Wiring

### Phase 1 flow
```
Orchestrator → Runner.Execute(task, role, input)
                 └→ calls hardcoded mock provider
```

### Phase 2 flow
```
Orchestrator → Dispatcher.Route(task, role) → RoutingDecision
             → Runner.Execute(task, role, input, decision)
                 └→ looks up provider from map by decision.Provider
                 └→ sets decision.Model on CompletionRequest
                 └→ calls real provider
```

### Runner Change

```go
type Runner struct {
    agents    map[Role]Agent
    providers map[string]llm.Provider  // was: single provider
}
```

### CompletionRequest Change

```go
type CompletionRequest struct {
    Model       string         // added
    Messages    []Message
    MaxTokens   int
    Temperature float64
    Extras      map[string]any
}
```

## Config Loading

New `internal/config` package that loads YAML files into existing config structs. Replaces hardcoded config in `main.go`.

## End-to-End Demo

```bash
./bin/perry "Build a Python script that fetches weather data and saves it as JSON"
```

Produces real LLM output at each stage, printed to stdout, with structured logging of state transitions. Audit gates and executor remain mocks.
