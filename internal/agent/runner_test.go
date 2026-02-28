package agent

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func TestRunnerExecuteWithRouting(t *testing.T) {
	mockClaude := llm.NewMockProvider("claude", "strategist response")
	mockOllama := llm.NewMockProvider("ollama", "coder response")
	providers := map[string]llm.Provider{
		"anthropic": mockClaude,
		"ollama":    mockOllama,
	}
	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
		RoleCoder:      NewMockAgent(RoleCoder),
	}
	runner := NewRunner(agents, providers)
	tk := task.New("test task", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan this", dispatch.Decision{
		Provider: "anthropic", Model: "claude-sonnet-4-6",
	})
	require.NoError(t, err)
	assert.Equal(t, "strategist response", out.Content)
	assert.Equal(t, RoleStrategist, out.Role)
	require.Len(t, mockClaude.Requests, 1)
	assert.Equal(t, "claude-sonnet-4-6", mockClaude.Requests[0].Model)

	out, err = runner.Execute(ctx(t), tk, RoleCoder, "write code", dispatch.Decision{
		Provider: "ollama", Model: "qwen2.5-coder:32b",
	})
	require.NoError(t, err)
	assert.Equal(t, "coder response", out.Content)
	require.Len(t, mockOllama.Requests, 1)
	assert.Equal(t, "qwen2.5-coder:32b", mockOllama.Requests[0].Model)
}

func TestRunnerUnknownProvider(t *testing.T) {
	providers := map[string]llm.Provider{"mock": llm.NewMockProvider("m", "r")}
	agents := map[Role]Agent{RoleStrategist: NewMockAgent(RoleStrategist)}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	_, err := runner.Execute(ctx(t), tk, RoleStrategist, "input", dispatch.Decision{Provider: "nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider")
}

func TestRunnerUnknownRole(t *testing.T) {
	providers := map[string]llm.Provider{"mock": llm.NewMockProvider("m", "r")}
	runner := NewRunner(map[Role]Agent{}, providers)
	tk := task.New("test", "user")

	_, err := runner.Execute(ctx(t), tk, RoleStrategist, "input", dispatch.Decision{Provider: "mock"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agent")
}

func TestRunnerParsesJSON(t *testing.T) {
	jsonResp := `{"requirements": ["req1"], "complexity": "standard"}`
	providers := map[string]llm.Provider{"mock": llm.NewMockProvider("m", jsonResp)}
	agents := map[Role]Agent{RoleStrategist: NewMockAgent(RoleStrategist)}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan", dispatch.Decision{Provider: "mock", Model: "m"})
	require.NoError(t, err)
	assert.Equal(t, jsonResp, out.Content)
	require.NotNil(t, out.Parsed)
	assert.Equal(t, "standard", out.Parsed["complexity"])
}

func TestRunnerNonJSONContent(t *testing.T) {
	providers := map[string]llm.Provider{"mock": llm.NewMockProvider("m", "this is not JSON")}
	agents := map[Role]Agent{RoleStrategist: NewMockAgent(RoleStrategist)}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan", dispatch.Decision{Provider: "mock", Model: "m"})
	require.NoError(t, err)
	assert.Equal(t, "this is not JSON", out.Content)
	assert.Nil(t, out.Parsed)
}

func TestMockAgentRecordsCalls(t *testing.T) {
	a := NewMockAgent(RoleCoder)
	providers := map[string]llm.Provider{"mock": llm.NewMockProvider("m", "code")}
	runner := NewRunner(map[Role]Agent{RoleCoder: a}, providers)
	tk := task.New("test", "user")

	_, _ = runner.Execute(ctx(t), tk, RoleCoder, "write a script", dispatch.Decision{Provider: "mock", Model: "m"})
	require.Len(t, a.Calls, 1)
	assert.Equal(t, "write a script", a.Calls[0])
}
