package openai

import (
	"net/http"

	"github.com/sashabaranov/go-openai"
)

// MockTokenCounter is a mock token counter for testing
type MockTokenCounter struct {
	CountTextFunc     func(text, model string) (int, error)
	CountMessagesFunc func(messages []openai.ChatCompletionMessage, model string) (int, error)
}

// CountText implements the TokenCounter interface
func (m *MockTokenCounter) CountText(text, model string) (int, error) {
	return m.CountTextFunc(text, model)
}

// CountMessages implements the TokenCounter interface
func (m *MockTokenCounter) CountMessages(messages []openai.ChatCompletionMessage, model string) (int, error) {
	return m.CountMessagesFunc(messages, model)
}

// MockHTTPClient is a mock HTTP client for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

// Do implements the HTTPClient interface
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}
