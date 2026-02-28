// internal/audit/pipeline_test.go
package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func passingGate(name string) Gate {
	return &mockGate{name: name, result: GateResult{Pass: true, Gate: name}}
}

func failingGate(name string, findings []string) Gate {
	return &mockGate{name: name, result: GateResult{Pass: false, Gate: name, Findings: findings}}
}

type mockGate struct {
	name   string
	result GateResult
}

func (g *mockGate) Name() string                                   { return g.name }
func (g *mockGate) Run(_ context.Context, _ AuditInput) GateResult { return g.result }

func TestPipelineAllPass(t *testing.T) {
	p := NewPipeline(
		passingGate("ast"),
		passingGate("secrets"),
		passingGate("static"),
	)

	result, err := p.Run(context.Background(), AuditInput{Code: "print('hello')"})
	require.NoError(t, err)
	assert.Equal(t, VerdictApprove, result.Verdict)
	assert.Len(t, result.GateResults, 3)
}

func TestPipelineFailFast(t *testing.T) {
	secondGate := passingGate("secrets")
	p := NewPipeline(
		failingGate("ast", []string{"disallowed import: os"}),
		secondGate,
	)

	result, err := p.Run(context.Background(), AuditInput{Code: "import os"})
	require.NoError(t, err)
	assert.Equal(t, VerdictReject, result.Verdict)
	// Only 1 gate ran (fail-fast)
	assert.Len(t, result.GateResults, 1)
	assert.Equal(t, "ast", result.GateResults[0].Gate)
}

func TestPipelinePartialFailure(t *testing.T) {
	p := NewPipeline(
		passingGate("ast"),
		failingGate("secrets", []string{"possible API key at line 5"}),
		passingGate("static"),
	)

	result, err := p.Run(context.Background(), AuditInput{Code: "key = 'sk-abc123...'"})
	require.NoError(t, err)
	assert.Equal(t, VerdictReject, result.Verdict)
	assert.Len(t, result.GateResults, 2) // stopped after secrets
}
