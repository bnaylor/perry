package executor

import "context"

// MockExecutor returns a fixed result for testing.
type MockExecutor struct {
	result   Result
	Requests []RunRequest
}

func NewMockExecutor(result Result) *MockExecutor {
	return &MockExecutor{result: result}
}

func (m *MockExecutor) Run(_ context.Context, req RunRequest) (Result, error) {
	m.Requests = append(m.Requests, req)
	return m.result, nil
}
