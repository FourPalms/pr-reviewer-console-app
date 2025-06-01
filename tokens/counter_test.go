package tokens

import (
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestCountText(t *testing.T) {
	counter := NewCounter()

	tests := []struct {
		name      string
		text      string
		model     string
		minTokens int // Using min/max since token counts can vary slightly between tiktoken versions
		maxTokens int
	}{
		{
			name:      "Empty string",
			text:      "",
			model:     "gpt-4o",
			minTokens: 0,
			maxTokens: 0,
		},
		{
			name:      "Short text",
			text:      "Hello, world!",
			model:     "gpt-4o",
			minTokens: 3,
			maxTokens: 5,
		},
		{
			name:      "Medium text",
			text:      "This is a test of the token counter. It should count approximately 15-20 tokens for this text.",
			model:     "gpt-4o",
			minTokens: 15,
			maxTokens: 25,
		},
		{
			name:      "Different model",
			text:      "Hello, world!",
			model:     "gpt-3.5-turbo",
			minTokens: 3,
			maxTokens: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := counter.CountText(tt.text, tt.model)
			if err != nil {
				t.Errorf("CountText() error = %v", err)
				return
			}

			if count < tt.minTokens || count > tt.maxTokens {
				t.Errorf("CountText() = %v, want between %v and %v", count, tt.minTokens, tt.maxTokens)
			}
		})
	}
}

func TestCountMessages(t *testing.T) {
	counter := NewCounter()

	tests := []struct {
		name      string
		messages  []openai.ChatCompletionMessage
		model     string
		minTokens int
		maxTokens int
	}{
		{
			name: "Single message",
			messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: "Hello, world!",
				},
			},
			model:     "gpt-4o",
			minTokens: 7, // 3 tokens for the message + 3-5 for content + 1 for role
			maxTokens: 12,
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
			model:     "gpt-4o",
			minTokens: 15,
			maxTokens: 25,
		},
		{
			name: "Message with name",
			messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Name:    "Jeremy",
					Content: "Hello!",
				},
			},
			model:     "gpt-4o",
			minTokens: 7,
			maxTokens: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := counter.CountMessages(tt.messages, tt.model)
			if err != nil {
				t.Errorf("CountMessages() error = %v", err)
				return
			}

			if count < tt.minTokens || count > tt.maxTokens {
				t.Errorf("CountMessages() = %v, want between %v and %v", count, tt.minTokens, tt.maxTokens)
			}
		})
	}
}
