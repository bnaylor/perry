// internal/agent/runner_test.go
package agent

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerExecutesMockAgent(t *testing.T) {
	provider := llm.NewMockProvider("test-model", "mock LLM response")
	agent := NewMockAgent(RoleStrategist)
	runner := NewRunner(map[Role]Agent{RoleStrategist: agent}, provider)

	tk := task.New("Build a weather CLI", "user-1")
	output, err := runner.Execute(context.Background(), tk, RoleStrategist, "Break this into requirements")
	require.NoError(t, err)
	assert.NotEmpty(t, output.Content)
	assert.Equal(t, RoleStrategist, output.Role)
}

func TestRunnerUnknownRole(t *testing.T) {
	provider := llm.NewMockProvider("test-model", "resp")
	runner := NewRunner(map[Role]Agent{}, provider)

	tk := task.New("test", "user-1")
	_, err := runner.Execute(context.Background(), tk, RoleStrategist, "do something")
	assert.Error(t, err)
}

func TestMockAgentRecordsCalls(t *testing.T) {
	agent := NewMockAgent(RoleCoder)
	provider := llm.NewMockProvider("test-model", "generated code here")
	runner := NewRunner(map[Role]Agent{RoleCoder: agent}, provider)

	tk := task.New("test", "user-1")
	_, _ = runner.Execute(context.Background(), tk, RoleCoder, "write a script")

	require.Len(t, agent.Calls, 1)
	assert.Equal(t, "write a script", agent.Calls[0])
}
