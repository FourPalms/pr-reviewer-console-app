package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Save original environment variables
	originalOpenAIKey := os.Getenv("OPENAI_API_KEY")
	originalModel := os.Getenv("OPENAI_MODEL")
	originalJiraURL := os.Getenv("JIRA_URL")
	originalJiraEmail := os.Getenv("JIRA_EMAIL")
	originalJiraToken := os.Getenv("JIRA_API_TOKEN")

	// Restore environment variables after test
	defer func() {
		os.Setenv("OPENAI_API_KEY", originalOpenAIKey)
		os.Setenv("OPENAI_MODEL", originalModel)
		os.Setenv("JIRA_URL", originalJiraURL)
		os.Setenv("JIRA_EMAIL", originalJiraEmail)
		os.Setenv("JIRA_API_TOKEN", originalJiraToken)
	}()

	// Test cases
	tests := []struct {
		name          string
		envVars       map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name: "All required variables set",
			envVars: map[string]string{
				"OPENAI_API_KEY": "test-key",
				"OPENAI_MODEL":   "gpt-4o",
				"JIRA_URL":       "https://example.atlassian.net",
				"JIRA_EMAIL":     "test@example.com",
				"JIRA_API_TOKEN": "test-token",
			},
			expectError: false,
		},
		{
			name: "Missing OpenAI API key",
			envVars: map[string]string{
				"OPENAI_API_KEY": "",
				"OPENAI_MODEL":   "gpt-4o",
			},
			expectError:   true,
			errorContains: "OPENAI_API_KEY environment variable is not set",
		},
		{
			name: "Default model when not specified",
			envVars: map[string]string{
				"OPENAI_API_KEY": "test-key",
				"OPENAI_MODEL":   "",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Clear any variables not explicitly set
			if _, exists := tt.envVars["OPENAI_API_KEY"]; !exists {
				os.Unsetenv("OPENAI_API_KEY")
			}
			if _, exists := tt.envVars["OPENAI_MODEL"]; !exists {
				os.Unsetenv("OPENAI_MODEL")
			}
			if _, exists := tt.envVars["JIRA_URL"]; !exists {
				os.Unsetenv("JIRA_URL")
			}
			if _, exists := tt.envVars["JIRA_EMAIL"]; !exists {
				os.Unsetenv("JIRA_EMAIL")
			}
			if _, exists := tt.envVars["JIRA_API_TOKEN"]; !exists {
				os.Unsetenv("JIRA_API_TOKEN")
			}

			// Load configuration
			cfg, err := Load()

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q but got %q", tt.errorContains, err.Error())
				}
				return
			}

			// Check successful configuration loading
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if cfg == nil {
				t.Error("Expected config but got nil")
				return
			}

			// Check specific configuration values
			if tt.envVars["OPENAI_API_KEY"] != "" && cfg.OpenAIAPIKey != tt.envVars["OPENAI_API_KEY"] {
				t.Errorf("Expected OpenAIAPIKey %q but got %q", tt.envVars["OPENAI_API_KEY"], cfg.OpenAIAPIKey)
			}
			if tt.envVars["OPENAI_MODEL"] != "" && cfg.Model != tt.envVars["OPENAI_MODEL"] {
				t.Errorf("Expected Model %q but got %q", tt.envVars["OPENAI_MODEL"], cfg.Model)
			}
			if tt.envVars["JIRA_URL"] != "" && cfg.JiraURL != tt.envVars["JIRA_URL"] {
				t.Errorf("Expected JiraURL %q but got %q", tt.envVars["JIRA_URL"], cfg.JiraURL)
			}
			if tt.envVars["JIRA_EMAIL"] != "" && cfg.JiraEmail != tt.envVars["JIRA_EMAIL"] {
				t.Errorf("Expected JiraEmail %q but got %q", tt.envVars["JIRA_EMAIL"], cfg.JiraEmail)
			}
			if tt.envVars["JIRA_API_TOKEN"] != "" && cfg.JiraToken != tt.envVars["JIRA_API_TOKEN"] {
				t.Errorf("Expected JiraToken %q but got %q", tt.envVars["JIRA_API_TOKEN"], cfg.JiraToken)
			}

			// Check default values
			if tt.envVars["OPENAI_MODEL"] == "" && cfg.Model == "" {
				t.Error("Expected default model but got empty string")
			}
		})
	}
}

func TestHasJiraCredentials(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name: "All Jira credentials present",
			config: &Config{
				JiraURL:   "https://example.atlassian.net",
				JiraEmail: "test@example.com",
				JiraToken: "test-token",
			},
			expected: true,
		},
		{
			name: "Missing Jira URL",
			config: &Config{
				JiraEmail: "test@example.com",
				JiraToken: "test-token",
			},
			expected: false,
		},
		{
			name: "Missing Jira email",
			config: &Config{
				JiraURL:   "https://example.atlassian.net",
				JiraToken: "test-token",
			},
			expected: false,
		},
		{
			name: "Missing Jira token",
			config: &Config{
				JiraURL:   "https://example.atlassian.net",
				JiraEmail: "test@example.com",
			},
			expected: false,
		},
		{
			name:     "All Jira credentials missing",
			config:   &Config{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasJiraCredentials()
			if result != tt.expected {
				t.Errorf("Expected HasJiraCredentials() to return %v but got %v", tt.expected, result)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
