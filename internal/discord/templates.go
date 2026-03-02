package discord

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/bnaylor/perry/internal/task"
)

// EventData contains context for formatting a Discord message.
type EventData struct {
	TaskID      string
	Description string
	FromState   string
	ToState     string
	Reason      string
	AgentName   string
	Role        string
	Content     string
	Findings    string
}

var templates = map[string]string{
	"transition": `**State Transition: {{.FromState}} ➔ {{.ToState}}**
> {{.Reason}}`,

	"task_submitted": `📢 **New Mission Briefing**
**ID:** ` + "`" + `{{.TaskID}}` + "`" + `
**Task:** {{.Description}}
*Phineas and Ferb, I know what we're going to do today!*`,

	"agent_call": `**{{.AgentName}}** ({{.Role}}):
{{.Content}}`,

	"audit_rejection": `⚠️ **Audit Rejected by {{.AgentName}}**
The code failed the security gates.
**Findings:**
{{.Findings}}`,

	"completion": `✅ **Mission Accomplished!**
Task ` + "`" + `{{.TaskID}}` + "`" + ` has been successfully completed.
*Curse you, Perry the Platypus!*`,

	"failure": `❌ **Mission Failed**
Task ` + "`" + `{{.TaskID}}` + "`" + ` encountered a critical error.
**Reason:** {{.Reason}}`,
}

// Format returns a human-readable string for the given event type.
func Format(templateName string, data EventData) (string, error) {
	tmplStr, ok := templates[templateName]
	if !ok {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	tmpl, err := template.New(templateName).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	if err := tmpl.Execute(buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// MapStateToTemplate returns the appropriate template name for a given state transition.
func MapStateToTemplate(prev, next task.State) string {
	if prev == "" && next == task.StateSubmitted {
		return "task_submitted"
	}
	if next == task.StateCompleted {
		return "completion"
	}
	if next == task.StateFailed {
		return "failure"
	}
	return "transition"
}
