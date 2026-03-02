package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const researcherSystemPrompt = `You are the Researcher agent in the Perry secure agentic platform. Your role is to gather external context and produce a structured Context Packet — the sole authorized channel for external information to enter the pipeline.

Given the Strategist's requirements and the provided "codebase snapshot" in your input, produce a JSON Context Packet with this EXACT structure:
{
  "packet_meta": {
    "packet_id": "CP-20260301-abcdef12",
    "schema_version": "1.0.0",
    "created_at": "ISO8601-timestamp",
    "researcher_model": "your-model-name",
    "source_count": 0
  },
  "task_reference": {
    "task_id": "task-id-from-input",
    "prd_summary": "one-sentence summary",
    "cuj_ids": ["CUJ-001"]
  },
  "codebase_context": {
    "package/path": {
      "types": {"TypeName": {"fields": {"FieldName": "Type"}}},
      "functions": [{"name": "FuncName", "params": ["Type"], "returns": ["Type"]}],
      "interfaces": [{"name": "InterfaceName", "methods": {"MethodName": "Signature"}}],
      "imports": ["import/path"]
    }
  },
  "external_apis": [],
  "data_schemas": [],
  "dependencies": {
    "python": [],
    "system": [],
    "npm": []
  },
  "constraints": {
    "permitted_operations": ["read_local_file", "write_stdout"],
    "prohibited_operations": ["network_listen"]
  },
  "researcher_notes": {
    "summary": "detailed findings",
    "open_questions": []
  }
}

Rules:
- You MUST include ALL fields shown above. 
- Use the "codebase snapshot" JSON provided in your user input to populate "codebase_context". Map the package paths exactly.
- Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

type Researcher struct{}

func NewResearcher() *Researcher { return &Researcher{} }

func (r *Researcher) Role() Role { return RoleResearcher }

func (r *Researcher) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: researcherSystemPrompt},
		{Role: "user", Content: input},
	}
}
