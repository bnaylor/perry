package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// Runner executes agents by dispatching to LLM providers.
type Runner struct {
	agents    map[Role]Agent
	providers map[string]llm.Provider
}

// NewRunner creates an agent runner with a provider map.
func NewRunner(agents map[Role]Agent, providers map[string]llm.Provider) *Runner {
	return &Runner{agents: agents, providers: providers}
}

// Execute runs the agent for the given role using the routed provider.
func (r *Runner) Execute(ctx context.Context, tk *task.Task, role Role, input string, decision dispatch.Decision) (AgentOutput, error) {
	agent, ok := r.agents[role]
	if !ok {
		return AgentOutput{}, fmt.Errorf("no agent registered for role %q", role)
	}

	provider, ok := r.providers[decision.Provider]
	if !ok {
		return AgentOutput{}, fmt.Errorf("no provider %q in provider map", decision.Provider)
	}

	messages := agent.BuildMessages(tk, input)
	resp, err := provider.Complete(ctx, llm.CompletionRequest{
		Model:    decision.Model,
		Messages: messages,
	})
	if err != nil {
		return AgentOutput{}, fmt.Errorf("LLM call failed for %s: %w", role, err)
	}

	output := AgentOutput{
		Role:    role,
		Content: resp.Content,
		Usage:   resp.Usage,
	}

	// Attempt JSON parsing — non-JSON content is not an error
	var parsed map[string]any
	cleaned := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
		output.Parsed = parsed
	}

	return output, nil
}

// extractJSON strips markdown code fences from content before JSON parsing.
func extractJSON(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.SplitN(trimmed, "\n", 2)
		if len(lines) == 2 {
			rest := lines[1]
			if idx := strings.LastIndex(rest, "```"); idx >= 0 {
				return strings.TrimSpace(rest[:idx])
			}
		}
	}
	return trimmed
}
