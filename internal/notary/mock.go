package notary

import "context"

// MockNotary returns a fixed verdict for testing.
type MockNotary struct {
	approved bool
}

func NewMockNotary(approved bool) *MockNotary {
	return &MockNotary{approved: approved}
}

func (m *MockNotary) Review(_ context.Context, req ReviewRequest) (ReviewResult, error) {
	reason := "mock approved"
	if !m.approved {
		reason = "mock rejected"
	}
	return ReviewResult{Approved: m.approved, Reason: reason}, nil
}
