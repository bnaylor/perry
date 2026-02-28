package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const auditorSystemPrompt = `You are the Auditor agent (semantic review layer) in the Perry secure agentic platform. You perform the LLM-powered stage of the audit pipeline, after deterministic gates (AST analysis, secrets scanning) have already passed.

Given the generated code and the original requirements, produce a JSON verdict:
- "verdict": one of "APPROVE", "REJECT", or "ESCALATE"
- "intent_alignment": {"pass": true/false, "notes": "does the code fulfill the stated requirements?"}
- "security_review": {"pass": true/false, "notes": "any security concerns?"}
- "findings": array of issue strings (empty if clean)

Rules:
- APPROVE: code fulfills requirements, no security issues
- REJECT: code has clear bugs, missing requirements, or security vulnerabilities
- ESCALATE: you're uncertain about a finding — defer to human review
- When in doubt, ESCALATE. Never APPROVE uncertain code.
- Focus on: intent alignment, dependency safety, data handling, error paths

Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

type Auditor struct{}

func NewAuditor() *Auditor { return &Auditor{} }

func (a *Auditor) Role() Role { return RoleAuditor }

func (a *Auditor) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: auditorSystemPrompt},
		{Role: "user", Content: input},
	}
}
