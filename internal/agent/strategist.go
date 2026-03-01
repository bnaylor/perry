package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const strategistSystemPrompt = `You are the Strategist agent in the Perry secure agentic platform. Your role is to translate user intent into clear, actionable requirements.

Given a task description, produce a JSON object with these fields:
- "requirements": array of specific, testable requirements
- "cuj_ids": array of Critical User Journey IDs (e.g., "CUJ-001") — assign sequentially
- "complexity": either "standard" or "complex" — "complex" means the task likely needs multiple files, external APIs, or careful error handling
- "working_set": array of relative Go package paths (e.g., ["internal/task", "internal/executor"]) that are relevant to this task for codebase analysis. If it's a new project, identify where the core logic should go.
- "task_summary": a one-sentence summary of what needs to be built

Respond ONLY with the JSON object. No markdown fencing, no explanatory text.

The Perry project structure is:
- cmd/perry: CLI entry point
- internal/agent: Agent definitions and logic
- internal/audit: Code and packet audit pipelines
- internal/codebase: Codebase snapshot logic (new)
- internal/executor: Sandbox execution logic
- internal/fsm: State machine definitions
- internal/orchestrator: Central control logic
- internal/task: Task and state definitions
- internal/llm: LLM provider integrations`

type Strategist struct{}

func NewStrategist() *Strategist { return &Strategist{} }

func (s *Strategist) Role() Role { return RoleStrategist }

func (s *Strategist) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: strategistSystemPrompt},
		{Role: "user", Content: input},
	}
}
