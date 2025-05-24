package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	OpenAIAPIKey string
	Model        string
}

// Load loads the configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY environment variable is not set")
	}

	// Get model from env or use default
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-3.5-turbo" // Default model
	}

	return &Config{
		OpenAIAPIKey: apiKey,
		Model:        model,
	}, nil
}
