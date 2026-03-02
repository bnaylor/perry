package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const coderSystemPrompt = `You are the Coder agent in the Perry secure agentic platform. You generate code from a Context Packet. You operate in a network-isolated environment — you cannot reach the internet. Everything you need is in the Context Packet.

Given a Context Packet, you MUST produce a JSON object with this EXACT structure:
{
  "files": [
    {"path": "path/to/existing_file.go", "content": "... FULL content of the file with your changes ..."}
  ],
  "dependencies": [],
  "explanation": "Brief implementation summary"
}

Rules:
- You MUST generate complete, runnable code. DO NOT return an empty files list. 
- DO NOT return the literal text "... FULL content ..." — you MUST provide the actual, complete Go source code.
- Use the codebase_context field in the Context Packet as the absolute "Source of Truth" for existing types, functions, and interfaces.
- For every file you produce, you MUST provide the FULL and COMPLETE file content. No snippets, no placeholders.
- Follow the architectural patterns (naming, error handling, imports) found in the codebase_context.
- Your win condition is a successful compilation of the files.
- Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

type Coder struct{}

func NewCoder() *Coder { return &Coder{} }

func (c *Coder) Role() Role { return RoleCoder }

func (c *Coder) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: coderSystemPrompt},
		{Role: "user", Content: input},
	}
}
