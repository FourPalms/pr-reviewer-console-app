package config

import (
	"errors"
	"os"

	"github.com/jeremyhunt/agent-runner/logger"
	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	OpenAIAPIKey string
	Model        string

	// Jira settings
	JiraURL   string
	JiraEmail string
	JiraToken string

	// Logging settings
	Verbosity logger.VerbosityLevel
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
		model = "gpt-4o" // Default model
	}

	// Get Jira settings (optional)
	jiraURL := os.Getenv("JIRA_URL")
	jiraEmail := os.Getenv("JIRA_EMAIL")
	jiraToken := os.Getenv("JIRA_API_TOKEN")

	// Default to normal verbosity
	verbosity := logger.VerbosityNormal

	return &Config{
		OpenAIAPIKey: apiKey,
		Model:        model,
		JiraURL:      jiraURL,
		JiraEmail:    jiraEmail,
		JiraToken:    jiraToken,
		Verbosity:    verbosity,
	}, nil
}

// HasJiraCredentials checks if all required Jira credentials are available
func (c *Config) HasJiraCredentials() bool {
	return c.JiraURL != "" && c.JiraEmail != "" && c.JiraToken != ""
}
