package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResearcherRole(t *testing.T) {
	r := NewResearcher()
	assert.Equal(t, RoleResearcher, r.Role())
}

func TestResearcherBuildMessages(t *testing.T) {
	r := NewResearcher()
	tk := task.New("fetch weather data", "user-1")
	input := `{"requirements": ["fetch weather API"], "complexity": "standard", "task_summary": "weather fetcher"}`
	msgs := r.BuildMessages(tk, input)
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Researcher")
	assert.Contains(t, msgs[0].Content, "Context Packet")
	assert.Contains(t, msgs[0].Content, "JSON")
	last := msgs[len(msgs)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, last.Content, "weather")
}

func TestResearcherSystemPromptDefinesPacketSchema(t *testing.T) {
	r := NewResearcher()
	tk := task.New("test", "user")
	msgs := r.BuildMessages(tk, "{}")
	system := msgs[0].Content
	assert.Contains(t, system, "packet_meta")
	assert.Contains(t, system, "external_apis")
	assert.Contains(t, system, "constraints")
}
