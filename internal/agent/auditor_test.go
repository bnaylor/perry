package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditorRole(t *testing.T) {
	a := NewAuditor()
	assert.Equal(t, RoleAuditor, a.Role())
}

func TestAuditorBuildMessages(t *testing.T) {
	a := NewAuditor()
	tk := task.New("test", "user-1")
	input := `{"files": [{"path": "main.py", "content": "print('hello')"}]}`
	msgs := a.BuildMessages(tk, input)
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Auditor")
	assert.Contains(t, msgs[0].Content, "verdict")
	assert.Contains(t, msgs[0].Content, "APPROVE")
	assert.Contains(t, msgs[0].Content, "REJECT")
	assert.Contains(t, msgs[0].Content, "ESCALATE")
}

func TestAuditorSystemPromptDefinesOutputFormat(t *testing.T) {
	a := NewAuditor()
	tk := task.New("test", "user")
	msgs := a.BuildMessages(tk, "{}")
	system := msgs[0].Content
	assert.Contains(t, system, "intent_alignment")
	assert.Contains(t, system, "security_review")
	assert.Contains(t, system, "findings")
}
