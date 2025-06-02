package jira

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeremyhunt/agent-runner/config"
)

// TestNewClient tests the creation of a new Jira client
func TestNewClient(t *testing.T) {
	// Test cases
	tests := []struct {
		name          string
		config        *config.Config
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			config: &config.Config{
				JiraURL:   "https://example.atlassian.net",
				JiraEmail: "test@example.com",
				JiraToken: "test-token",
			},
			expectError: false,
		},
		{
			name: "Missing Jira URL",
			config: &config.Config{
				JiraEmail: "test@example.com",
				JiraToken: "test-token",
			},
			expectError:   true,
			errorContains: "missing Jira credentials",
		},
		{
			name: "Missing Jira email",
			config: &config.Config{
				JiraURL:   "https://example.atlassian.net",
				JiraToken: "test-token",
			},
			expectError:   true,
			errorContains: "missing Jira credentials",
		},
		{
			name: "Missing Jira token",
			config: &config.Config{
				JiraURL:   "https://example.atlassian.net",
				JiraEmail: "test@example.com",
			},
			expectError:   true,
			errorContains: "missing Jira credentials",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)

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

			// Check successful client creation
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if client == nil {
				t.Error("Expected client but got nil")
				return
			}
			if client.jiraClient == nil {
				t.Error("Expected jiraClient but got nil")
			}
			if client.config != tt.config {
				t.Error("Config not properly set in client")
			}
		})
	}
}

// TestGetTicket tests the GetTicket method
func TestGetTicket(t *testing.T) {
	// Create a test server that mimics Jira API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request path to determine the response
		if r.URL.Path == "/rest/api/2/issue/WIRE-1234" {
			// Return a successful response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"id": "10000",
				"key": "WIRE-1234",
				"fields": {
					"summary": "Test ticket",
					"description": "This is a test ticket",
					"status": {
						"name": "Open"
					}
				}
			}`))
		} else if r.URL.Path == "/rest/api/2/issue/ERROR-404" {
			// Return a 404 error
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"errorMessages":["Issue does not exist or you do not have permission to see it."]}`))
		} else {
			// Return a generic error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errorMessages":["An error occurred"]}`))
		}
	}))
	defer server.Close()

	// Create a client that uses the test server
	cfg := &config.Config{
		JiraURL:   server.URL,
		JiraEmail: "test@example.com",
		JiraToken: "test-token",
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test cases
	tests := []struct {
		name          string
		ticketID      string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid ticket",
			ticketID:    "WIRE-1234",
			expectError: false,
		},
		{
			name:          "Non-existent ticket",
			ticketID:      "ERROR-404",
			expectError:   true,
			errorContains: "failed to get ticket",
		},
		{
			name:          "Server error",
			ticketID:      "SERVER-ERROR",
			expectError:   true,
			errorContains: "failed to get ticket",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue, err := client.GetTicket(tt.ticketID)

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

			// Check successful ticket retrieval
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if issue == nil {
				t.Error("Expected issue but got nil")
				return
			}
			if issue.Key != tt.ticketID {
				t.Errorf("Expected ticket ID %q but got %q", tt.ticketID, issue.Key)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
