package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

// TestComplete tests the Complete function
func TestComplete(t *testing.T) {
	// Test cases
	tests := []struct {
		name           string
		prompt         string
		mockResponse   string
		mockStatusCode int
		expectedResult string
		expectError    bool
		tokenCount     int
	}{
		{
			name:   "Successful completion",
			prompt: "Hello, world!",
			mockResponse: `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1620000000,
				"model": "gpt-4o",
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "Hello! How can I help you today?"
						},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`,
			mockStatusCode: http.StatusOK,
			expectedResult: "Hello! How can I help you today?",
			expectError:    false,
			tokenCount:     3, // Mocked token count for "Hello, world!"
		},
		{
			name:           "API error",
			prompt:         "Hello, world!",
			mockResponse:   `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`,
			mockStatusCode: http.StatusUnauthorized,
			expectedResult: "",
			expectError:    true,
			tokenCount:     3,
		},
		{
			name:           "Empty response",
			prompt:         "Hello, world!",
			mockResponse:   `{"choices":[]}`,
			mockStatusCode: http.StatusOK,
			expectedResult: "",
			expectError:    true,
			tokenCount:     3,
		},
		{
			name:           "Token limit exceeded",
			prompt:         "This is a very long prompt that exceeds the token limit",
			mockResponse:   `{}`,
			mockStatusCode: http.StatusOK,
			expectedResult: "",
			expectError:    true,
			tokenCount:     130000, // More than the 120000 limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock token counter
			mockCounter := &MockTokenCounter{
				CountTextFunc: func(text, model string) (int, error) {
					return tt.tokenCount, nil
				},
				CountMessagesFunc: func(messages []openai.ChatCompletionMessage, model string) (int, error) {
					return tt.tokenCount, nil
				},
			}

			// Create a mock HTTP client
			mockHTTPClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					// Check if we should test token limit exceeded
					if tt.name == "Token limit exceeded" {
						// Don't actually make the request, just return the error
						return nil, nil
					}

					// Check request method
					if req.Method != http.MethodPost {
						t.Errorf("Expected POST request, got %s", req.Method)
					}

					// Check request URL
					if !strings.HasSuffix(req.URL.String(), "/chat/completions") {
						t.Errorf("Expected URL to end with /chat/completions, got %s", req.URL.String())
					}

					// Check request headers
					if req.Header.Get("Content-Type") != "application/json" {
						t.Errorf("Expected Content-Type header to be application/json, got %s", req.Header.Get("Content-Type"))
					}
					if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
						t.Errorf("Expected Authorization header to start with 'Bearer ', got %s", req.Header.Get("Authorization"))
					}

					// Check request body
					bodyBytes, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatalf("Error reading request body: %v", err)
					}
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // Reset the body for further reading

					var reqBody ChatCompletionRequest
					if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
						t.Fatalf("Error unmarshaling request body: %v", err)
					}

					if len(reqBody.Messages) != 1 {
						t.Errorf("Expected 1 message, got %d", len(reqBody.Messages))
					}
					if reqBody.Messages[0].Role != "user" {
						t.Errorf("Expected message role to be 'user', got %s", reqBody.Messages[0].Role)
					}
					if reqBody.Messages[0].Content != tt.prompt {
						t.Errorf("Expected message content to be %q, got %q", tt.prompt, reqBody.Messages[0].Content)
					}

					// Create the response
					response := &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(strings.NewReader(tt.mockResponse)),
						Header:     make(http.Header),
					}
					response.Header.Set("Content-Type", "application/json")

					return response, nil
				},
			}

			// Create the client
			client := &Client{
				baseURL:      "https://api.openai.com/v1",
				apiKey:       "test-key",
				model:        "gpt-4o",
				httpClient:   mockHTTPClient,
				tokenCounter: mockCounter,
			}

			// Call the function
			result, err := client.Complete(context.Background(), tt.prompt)

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			// Check result expectations
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tt.expectedResult {
				t.Errorf("Expected result %q, got %q", tt.expectedResult, result)
			}
		})
	}
}
