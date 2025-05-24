package openai

import (
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestCountText(t *testing.T) {
	// Create a client with a dummy API key and model
	client := NewClient("dummy-api-key", "gpt-4o")
	
	tests := []struct {
		name     string
		text     string
		wantErr  bool
	}{
		{
			name:    "Empty string",
			text:    "",
			wantErr: false,
		},
		{
			name:    "Simple text",
			text:    "Hello, world!",
			wantErr: false,
		},
		{
			name:    "Longer text",
			text:    "This is a longer text that should be tokenized correctly by the OpenAI tokenizer.",
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := client.CountText(tt.text)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("CountText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			// We're not testing exact token counts here since that's covered in the tokens package tests
			// Just verify that we get a reasonable count (greater than 0 for non-empty strings)
			if tt.text != "" && count <= 0 {
				t.Errorf("CountText() returned unreasonable count = %v for non-empty text", count)
			}
		})
	}
}

func TestCountTokens(t *testing.T) {
	// Create a client with a dummy API key and model
	client := NewClient("dummy-api-key", "gpt-4o")
	
	tests := []struct {
		name     string
		messages []openai.ChatCompletionMessage
		wantErr  bool
	}{
		{
			name: "Empty messages",
			messages: []openai.ChatCompletionMessage{},
			wantErr: false,
		},
		{
			name: "Single message",
			messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: "Hello, world!",
				},
			},
			wantErr: false,
		},
		{
			name: "Multiple messages",
			messages: []openai.ChatCompletionMessage{
				{
					Role:    "system",
					Content: "You are a helpful assistant.",
				},
				{
					Role:    "user",
					Content: "Tell me about token counting.",
				},
			},
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := client.CountTokens(tt.messages)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("CountTokens() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			// For non-empty messages, we should get a positive count
			if len(tt.messages) > 0 && count <= 0 {
				t.Errorf("CountTokens() returned unreasonable count = %v for non-empty messages", count)
			}
		})
	}
}
