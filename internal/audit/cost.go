package audit

import (
	"context"
	"fmt"

	"github.com/bnaylor/perry/internal/llm"
)

// CostGate is an audit gate that checks if the predicted cost of a task
// is within the budget defined in policy.
type CostGate struct {
	MaxCost      float64
	CurrentModel string
	// PredictedTokens is an estimate of how many tokens the next step will use.
	PredictedTokens int
}

func (g *CostGate) Name() string {
	return "cost_auditor"
}

func (g *CostGate) Run(ctx context.Context, input AuditInput) GateResult {
	// For "Pre-flight" checks, we estimate cost based on input tokens or
	// requirements length.
	inputTokens := g.PredictedTokens
	if inputTokens == 0 {
		inputTokens = len(input.Requirements) / 4 
	}
	outputTokens := inputTokens / 4

	cost, err := llm.EstimateCost(g.CurrentModel, inputTokens, outputTokens)
	if err != nil {
		return GateResult{
			Pass:     true, // Don't block if pricing is unknown
			Gate:     g.Name(),
			Findings: []string{fmt.Sprintf("Warning: %v", err)},
		}
	}

	if cost > g.MaxCost {
		return GateResult{
			Pass:     false,
			Gate:     g.Name(),
			Findings: []string{fmt.Sprintf("Estimated cost $%.4f exceeds budget $%.4f", cost, g.MaxCost)},
		}
	}

	return GateResult{
		Pass:     true,
		Gate:     g.Name(),
		Findings: []string{fmt.Sprintf("Estimated cost: $%.4f", cost)},
	}
}
