package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrategistRole(t *testing.T) {
	s := NewStrategist()
	assert.Equal(t, RoleStrategist, s.Role())
}

func TestStrategistBuildMessages(t *testing.T) {
	s := NewStrategist()
	tk := task.New("Build a Python script that fetches weather data", "user-1")
	msgs := s.BuildMessages(tk, "Build a Python script that fetches weather data")
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Strategist")
	assert.Contains(t, msgs[0].Content, "JSON")
	last := msgs[len(msgs)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, last.Content, "weather data")
}

func TestStrategistSystemPromptRequiresJSON(t *testing.T) {
	s := NewStrategist()
	tk := task.New("test", "user")
	msgs := s.BuildMessages(tk, "test")
	system := msgs[0].Content
	assert.Contains(t, system, "requirements")
	assert.Contains(t, system, "complexity")
	assert.Contains(t, system, "task_summary")
}
