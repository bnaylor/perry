package notary

import "context"

// ReviewRequest describes an artifact to validate.
type ReviewRequest struct {
	ArtifactPath string
	ExpectedType string // "json", "csv", "py", etc.
	MaxSizeBytes int64
}

// ReviewResult is the notary's verdict on an artifact.
type ReviewResult struct {
	Approved bool
	Reason   string
}

// Notary validates executor output before delivery to the host.
type Notary interface {
	Review(ctx context.Context, req ReviewRequest) (ReviewResult, error)
}
