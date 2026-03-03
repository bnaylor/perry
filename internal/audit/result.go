package audit

// Verdict is the overall outcome of the audit pipeline.
type Verdict string

const (
	VerdictApprove  Verdict = "APPROVE"
	VerdictReject   Verdict = "REJECT"
	VerdictEscalate Verdict = "ESCALATE"
)

// FailureReason categorizes why an audit might have failed.
type FailureReason string

const (
	ReasonNone           FailureReason = ""
	ReasonModelIncapable FailureReason = "model_incapable"
	ReasonInvalidInput   FailureReason = "invalid_input"
	ReasonSystemError    FailureReason = "system_error"
	ReasonPolicyViolation FailureReason = "policy_violation"
)

// GateResult is the outcome of a single audit gate.
type GateResult struct {
	Pass          bool          `json:"pass"`
	Gate          string        `json:"gate"`
	Findings      []string      `json:"findings,omitempty"`
	FailureReason FailureReason `json:"failure_reason,omitempty"`
}

// AuditInput is what gets fed into the audit pipeline.
type AuditInput struct {
	Code          string
	Language      string
	ContextPacket map[string]any // the validated context packet
	Requirements  string         // PM's requirements summary
}

// AuditResult is the complete audit outcome.
type AuditResult struct {
	Verdict       Verdict
	GateResults   []GateResult
	FailureReason FailureReason
}
