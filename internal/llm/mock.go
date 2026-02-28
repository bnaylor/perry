package llm

import "context"

// MockProvider returns a fixed response for testing.
type MockProvider struct {
	model    string
	response string
	Requests []CompletionRequest // records all requests for assertions
}

func NewMockProvider(model, response string) *MockProvider {
	return &MockProvider{model: model, response: response}
}

func (m *MockProvider) Name() string { return "mock" }

func (m *MockProvider) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	m.Requests = append(m.Requests, req)
	return CompletionResponse{
		Content: m.response,
		Model:   m.model,
		Usage:   Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}
