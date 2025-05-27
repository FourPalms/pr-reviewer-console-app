package tokens

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

// Counter provides methods for counting tokens in text and messages
type Counter struct {
	// Cached encoders for different models to improve performance
	encoders map[string]*tiktoken.Tiktoken
	// Mutex to protect concurrent access to the encoders map
	mutex sync.RWMutex
}

// NewCounter creates a new token counter
func NewCounter() *Counter {
	return &Counter{
		encoders: make(map[string]*tiktoken.Tiktoken),
	}
}

// CountText counts the number of tokens in a plain text string for a specific model
func (c *Counter) CountText(text string, model string) (int, error) {
	encoder, err := c.getEncoderForModel(model)
	if err != nil {
		return 0, err
	}

	tokens := encoder.Encode(text, nil, nil)
	return len(tokens), nil
}

// CountMessages counts the number of tokens in a slice of chat messages for a specific model
// This implementation is based on OpenAI's guidelines for token counting in chat completions
func (c *Counter) CountMessages(messages []openai.ChatCompletionMessage, model string) (int, error) {
	encoder, err := c.getEncoderForModel(model)
	if err != nil {
		return 0, err
	}

	var tokensPerMessage, tokensPerName int
	switch model {
	case "gpt-3.5-turbo-0613",
		"gpt-3.5-turbo-16k-0613",
		"gpt-4-0314",
		"gpt-4-32k-0314",
		"gpt-4-0613",
		"gpt-4-32k-0613",
		"gpt-4o-2024-05-13",
		"gpt-4o":
		tokensPerMessage = 3
		tokensPerName = 1
	case "gpt-3.5-turbo-0301":
		tokensPerMessage = 4 // every message follows <|start|>{role/name}\n{content}<|end|>\n
		tokensPerName = -1   // if there's a name, the role is omitted
	default:
		if strings.Contains(model, "gpt-3.5-turbo") {
			log.Println("warning: gpt-3.5-turbo may update over time. Returning num tokens assuming gpt-3.5-turbo-0613.")
			return c.CountMessages(messages, "gpt-3.5-turbo-0613")
		} else if strings.Contains(model, "gpt-4") {
			log.Println("warning: gpt-4 may update over time. Returning num tokens assuming gpt-4-0613.")
			return c.CountMessages(messages, "gpt-4-0613")
		} else {
			return 0, fmt.Errorf("token counting not implemented for model %s", model)
		}
	}

	numTokens := 0
	for _, message := range messages {
		numTokens += tokensPerMessage
		numTokens += len(encoder.Encode(message.Content, nil, nil))
		numTokens += len(encoder.Encode(message.Role, nil, nil))
		if message.Name != "" {
			numTokens += len(encoder.Encode(message.Name, nil, nil))
			numTokens += tokensPerName
		}
	}

	// Every reply is primed with <|start|>assistant<|message|>
	numTokens += 3
	return numTokens, nil
}

// getEncoderForModel returns a tiktoken encoder for the specified model
// It caches encoders to improve performance on repeated calls
func (c *Counter) getEncoderForModel(model string) (*tiktoken.Tiktoken, error) {
	// First check with a read lock if we already have a cached encoder
	c.mutex.RLock()
	encoder, ok := c.encoders[model]
	c.mutex.RUnlock()

	if ok {
		return encoder, nil
	}

	// If not found, acquire a write lock and check again (double-checked locking)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check again in case another goroutine created it while we were waiting
	if encoder, ok := c.encoders[model]; ok {
		return encoder, nil
	}

	// Get a new encoder for this model
	encoder, err := tiktoken.EncodingForModel(model)
	if err != nil {
		return nil, fmt.Errorf("failed to get encoding for model %s: %w", model, err)
	}

	// Cache the encoder for future use
	c.encoders[model] = encoder
	return encoder, nil
}
