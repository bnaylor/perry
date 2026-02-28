package agent

import (
	"context"
	"fmt"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// Runner executes agents by dispatching to LLM providers.
type Runner struct {
	agents   map[Role]Agent
	provider llm.Provider
}

// NewRunner creates an agent runner.
func NewRunner(agents map[Role]Agent, provider llm.Provider) *Runner {
	return &Runner{agents: agents, provider: provider}
}

// Execute runs the agent for the given role and returns its output.
func (r *Runner) Execute(ctx context.Context, tk *task.Task, role Role, input string) (AgentOutput, error) {
	agent, ok := r.agents[role]
	if !ok {
		return AgentOutput{}, fmt.Errorf("no agent registered for role %q", role)
	}

	messages := agent.BuildMessages(tk, input)
	resp, err := r.provider.Complete(ctx, llm.CompletionRequest{
		Messages: messages,
	})
	if err != nil {
		return AgentOutput{}, fmt.Errorf("LLM call failed for %s: %w", role, err)
	}

	return AgentOutput{
		Role:    role,
		Content: resp.Content,
		Usage:   resp.Usage,
	}, nil
}
