package agent

import (
	"fmt"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// MockAgent logs what it would do instead of calling an LLM.
type MockAgent struct {
	role  Role
	Calls []string
}

func NewMockAgent(role Role) *MockAgent {
	return &MockAgent{role: role}
}

func (m *MockAgent) Role() Role { return m.role }

func (m *MockAgent) BuildMessages(tk *task.Task, input string) []llm.Message {
	m.Calls = append(m.Calls, input)
	return []llm.Message{
		{Role: "system", Content: fmt.Sprintf("You are the %s agent.", m.role)},
		{Role: "user", Content: fmt.Sprintf("Task: %s\n\nInput: %s", tk.Description, input)},
	}
}
