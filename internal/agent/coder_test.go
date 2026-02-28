package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoderRole(t *testing.T) {
	c := NewCoder()
	assert.Equal(t, RoleCoder, c.Role())
}

func TestCoderBuildMessages(t *testing.T) {
	c := NewCoder()
	tk := task.New("build weather fetcher", "user-1")
	input := `{"task_reference": {"task_summary": "weather fetcher"}, "external_apis": [], "constraints": {"language": "python"}}`
	msgs := c.BuildMessages(tk, input)
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Coder")
	assert.Contains(t, msgs[0].Content, "files")
	assert.Contains(t, msgs[0].Content, "JSON")
	last := msgs[len(msgs)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, last.Content, "weather")
}

func TestCoderSystemPromptDefinesOutputFormat(t *testing.T) {
	c := NewCoder()
	tk := task.New("test", "user")
	msgs := c.BuildMessages(tk, "{}")
	system := msgs[0].Content
	assert.Contains(t, system, "path")
	assert.Contains(t, system, "content")
	assert.Contains(t, system, "dependencies")
	assert.Contains(t, system, "explanation")
}
