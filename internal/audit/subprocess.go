package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// InputMapper transforms an AuditInput into the JSON structure expected by a
// specific Python script. Each gate provides its own mapper.
type InputMapper func(AuditInput) (map[string]any, error)

// SubprocessGateOption configures a SubprocessGate.
type SubprocessGateOption func(*SubprocessGate)

// WithTimeout sets a per-gate execution timeout.
func WithTimeout(d time.Duration) SubprocessGateOption {
	return func(g *SubprocessGate) { g.timeout = d }
}

// SubprocessGate runs a Python script as a subprocess, feeding it JSON on
// stdin and reading a GateResult from stdout. It fails closed on every error
// path: non-zero exit, bad JSON, timeout, context cancellation.
type SubprocessGate struct {
	name    string
	script  string
	mapper  InputMapper
	timeout time.Duration
}

// NewSubprocessGate creates a gate that delegates to an external script.
func NewSubprocessGate(name, script string, mapper InputMapper, opts ...SubprocessGateOption) *SubprocessGate {
	g := &SubprocessGate{
		name:   name,
		script: script,
		mapper: mapper,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Name returns the gate's name.
func (g *SubprocessGate) Name() string { return g.name }

// Run executes the subprocess gate. Fails closed on any error.
func (g *SubprocessGate) Run(ctx context.Context, input AuditInput) GateResult {
	fail := func(reason string) GateResult {
		return GateResult{Pass: false, Gate: g.name, Findings: []string{reason}}
	}

	mapped, err := g.mapper(input)
	if err != nil {
		if errors.Is(err, ErrSkipGate) {
			return GateResult{Pass: true, Gate: g.name}
		}
		return fail(fmt.Sprintf("input mapping error: %v", err))
	}

	inputJSON, err := json.Marshal(mapped)
	if err != nil {
		return fail(fmt.Sprintf("JSON marshal error: %v", err))
	}

	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "python3", g.script)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := err.Error()
		if stderr.Len() > 0 {
			detail += ": " + stderr.String()
		}
		return fail(fmt.Sprintf("subprocess error: %s", detail))
	}

	var result GateResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return fail(fmt.Sprintf("invalid JSON from subprocess: %v", err))
	}

	// Always tag with our gate name, regardless of what the script says.
	result.Gate = g.name
	return result
}
