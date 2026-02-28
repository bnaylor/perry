package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// AgentOutput is what an agent produces.
type AgentOutput struct {
	Role    Role
	Content string
	Usage   llm.Usage
}

// Agent defines the interface for all agent roles.
type Agent interface {
	// Role returns this agent's role.
	Role() Role

	// BuildMessages constructs the LLM messages for this agent's task.
	BuildMessages(tk *task.Task, input string) []llm.Message
}
