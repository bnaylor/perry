package audit

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimateCostGemini(t *testing.T) {
	// gemini-3.1-pro: $2.00 input, $12.00 output
	// 1M input + 500k output = $2.00 + $6.00 = $8.00
	cost, err := llm.EstimateCost("gemini-3.1-pro", 1000000, 500000)
	require.NoError(t, err)
	assert.InDelta(t, 8.0, cost, 0.0001)

	// gemini-3.1-flash-lite: $0.05 input, $0.25 output
	// 2M input + 1M output = $0.10 + $0.25 = $0.35
	cost, err = llm.EstimateCost("gemini-3.1-flash-lite", 2000000, 1000000)
	require.NoError(t, err)
	assert.InDelta(t, 0.35, cost, 0.0001)
}

func TestEstimateCostClaude(t *testing.T) {
	// claude-3-7-sonnet: $3.00 input, $15.00 output
	// 1M input + 200k output = $3.00 + $3.00 = $6.00
	cost, err := llm.EstimateCost("claude-3-7-sonnet-20260219", 1000000, 200000)
	require.NoError(t, err)
	assert.InDelta(t, 6.0, cost, 0.0001)
}

func TestEstimateCostUnknownModel(t *testing.T) {
	_, err := llm.EstimateCost("unknown-model", 1000, 1000)
	assert.Error(t, err)
}

func TestCostGate(t *testing.T) {
	gate := &CostGate{
		MaxCost:         1.00,
		CurrentModel:    "gemini-3.1-pro",
		PredictedTokens: 100000, // $2.00 * 0.1 = $0.20 input. Output $12.00 * 0.025 = $0.30. Total $0.50
	}

	input := AuditInput{Requirements: "some requirements"}
	result := gate.Run(context.Background(), input)
	assert.True(t, result.Pass)
	assert.Contains(t, result.Findings[0], "Estimated cost: $0.5000")

	// Set a very low budget to trigger failure
	gate.MaxCost = 0.10
	result = gate.Run(context.Background(), input)
	assert.False(t, result.Pass)
	assert.Contains(t, result.Findings[0], "exceeds budget $0.1000")
}
