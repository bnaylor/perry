package audit

import "context"

// Gate is a single audit check.
type Gate interface {
	Name() string
	Run(ctx context.Context, input AuditInput) GateResult
}

// Pipeline runs audit gates sequentially with fail-fast behavior.
type Pipeline struct {
	gates []Gate
}

// NewPipeline creates an audit pipeline from an ordered list of gates.
func NewPipeline(gates ...Gate) *Pipeline {
	return &Pipeline{gates: gates}
}

// Run executes all gates in order. Stops at first failure.
func (p *Pipeline) Run(ctx context.Context, input AuditInput) (AuditResult, error) {
	var results []GateResult

	for _, gate := range p.gates {
		result := gate.Run(ctx, input)
		results = append(results, result)

		if !result.Pass {
			return AuditResult{
				Verdict:     VerdictReject,
				GateResults: results,
			}, nil
		}
	}

	return AuditResult{
		Verdict:     VerdictApprove,
		GateResults: results,
	}, nil
}
