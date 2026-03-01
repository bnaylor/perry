package executor

import "context"

// RunRequest describes what to execute in the sandbox.
type RunRequest struct {
	Code         string
	Files        map[string]string // Multi-file support: filename -> content
	Entrypoint   string            // Entrypoint file to run, defaults to main.py or language equivalent if empty
	Language     string
	Dependencies []string
	EnvVars      map[string]string
	TimeoutSec   int
}

// Result is the outcome of a sandbox execution.
type Result struct {
	Success  bool
	Output   string // path to output artifact(s)
	ExitCode int
	Logs     string
}

// Executor runs code in an isolated sandbox.
type Executor interface {
	Run(ctx context.Context, req RunRequest) (Result, error)
}
