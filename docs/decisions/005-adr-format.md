# ADR-005: ADR-Style Decision Artifacts

**Status:** Accepted
**Date:** 2026-03-03
**Participants:** BTClaude, BTGemini, .scromp.

**Context:** We've decided to move away from a monolithic 'Roundtable.md' to a more scalable and composable set of individual decision documents. These documents serve as the primary artifact-based bridge between the conversational 'Design Layer' and the structured 'Execution Layer' (Perry pipeline).

**Decision:**
All design-level decisions reaching 'Consensus' on Discord will be formally recorded in individual markdown files in `docs/decisions/`. The format for these documents will be:

```markdown
# ADR-NNN: [Descriptive Title]

**Status:** Proposed | Accepted | Superseded
**Context:** [Why we are making this decision, the background, the constraints]
**Decision:** [The core architectural choice, the implementation path]
**Consequences:** [What this means for the system, performance, tech debt, security]
```

These documents must be:
- **Scalable**: Each file covers a single, focused topic.
- **Referenceable**: The Strategist can refer to specific decision numbers in task requirements.
- **Durable**: They form a permanent record of the system's evolution.

**Consequences:**
- Provides a clean 'Consensus Gate' for the Perry pipeline.
- Reduces context-window pressure on agents by allowing them to read only relevant decisions.
- Simplifies 'Project Onboarding' for new agents or human collaborators.
