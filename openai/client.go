package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jeremyhunt/agent-runner/logger"
	"github.com/jeremyhunt/agent-runner/tokens"
	"github.com/sashabaranov/go-openai"
)

// HTTPClient is an interface for HTTP clients
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// TokenCounter is an interface for token counting
type TokenCounter interface {
	CountText(text, model string) (int, error)
	CountMessages(messages []openai.ChatCompletionMessage, model string) (int, error)
}

// Client represents an OpenAI API client
type Client struct {
	apiKey       string
	httpClient   HTTPClient
	baseURL      string
	model        string
	tokenCounter TokenCounter
}

// NewClient creates a new OpenAI client
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		baseURL:      "https://api.openai.com/v1",
		model:        model,
		tokenCounter: tokens.NewCounter(),
	}
}

// ChatCompletionRequest represents a request to the chat completion API
type ChatCompletionRequest struct {
	Model    string                         `json:"model"`
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

// ChatCompletionResponse represents a response from the chat completion API
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends a prompt to the OpenAI API and returns the response
func (c *Client) Complete(ctx context.Context, prompt string) (string, error) {
	// Create the message
	messages := []openai.ChatCompletionMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Count tokens in the prompt
	tokenCount, err := c.CountTokens(messages)
	if err != nil {
		return "", fmt.Errorf("error counting tokens: %w", err)
	}

	// Get the maximum token limit for the model
	// GPT-4o has a 128K token limit, but we'll be conservative
	maxTokens := 120000

	// Check if the token count exceeds the maximum limit
	if tokenCount > maxTokens {
		return "", fmt.Errorf("token count (%d) exceeds maximum limit (%d)", tokenCount, maxTokens)
	}

	// Log the token count
	logger.Verbose("Sending prompt to %s (token count: %d)", c.model, tokenCount)

	// Create the request body
	reqBody := ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/chat/completions", c.baseURL),
		bytes.NewReader(reqBytes),
	)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// CountTokens counts the number of tokens in a slice of chat messages
func (c *Client) CountTokens(messages []openai.ChatCompletionMessage) (int, error) {
	return c.tokenCounter.CountMessages(messages, c.model)
}

// CountText counts the number of tokens in a text string
func (c *Client) CountText(text string) (int, error) {
	return c.tokenCounter.CountText(text, c.model)
}
