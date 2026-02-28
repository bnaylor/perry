package audit

// Verdict is the overall outcome of the audit pipeline.
type Verdict string

const (
	VerdictApprove  Verdict = "APPROVE"
	VerdictReject   Verdict = "REJECT"
	VerdictEscalate Verdict = "ESCALATE"
)

// GateResult is the outcome of a single audit gate.
type GateResult struct {
	Pass     bool     `json:"pass"`
	Gate     string   `json:"gate"`
	Findings []string `json:"findings,omitempty"`
}

// AuditInput is what gets fed into the audit pipeline.
type AuditInput struct {
	Code          string
	ContextPacket map[string]any // the validated context packet
	Requirements  string         // PM's requirements summary
}

// AuditResult is the complete audit outcome.
type AuditResult struct {
	Verdict     Verdict
	GateResults []GateResult
}
