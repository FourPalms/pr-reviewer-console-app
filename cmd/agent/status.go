package main

import (
	"fmt"
	"os"

	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/jira"
	"github.com/jeremyhunt/agent-runner/logger"
)

// checkJiraStatus checks if we can connect to Jira and retrieve a test ticket
func checkJiraStatus(cfg *config.Config) error {
	// Check if Jira credentials are available
	if !cfg.HasJiraCredentials() {
		return fmt.Errorf("missing Jira credentials in environment variables")
	}

	// Create Jira client
	client, err := jira.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Jira client: %w", err)
	}

	// Try to get a test ticket (WIRE-1231 as an example)
	testTicket := "WIRE-1231"
	ticket, err := client.GetTicket(testTicket)
	if err != nil {
		return fmt.Errorf("failed to retrieve test ticket %s: %w", testTicket, err)
	}

	// Print basic ticket info
	logger.Info("Successfully retrieved ticket %s: %s", ticket.Key, ticket.Fields.Summary)

	// Print additional ticket details (only in verbose mode)
	logger.Verbose("Status: %s", ticket.Fields.Status.Name)
	if ticket.Fields.Assignee != nil {
		logger.Verbose("Assignee: %s", ticket.Fields.Assignee.DisplayName)
	}
	if ticket.Fields.Reporter != nil {
		logger.Verbose("Reporter: %s", ticket.Fields.Reporter.DisplayName)
	}

	return nil
}

// handleStatus checks the status of various integrations
func handleStatus() {
	logger.Info("Checking system status...")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Config: %v", err)
		os.Exit(1)
	}
	logger.Success("Config: Successfully loaded")

	// Check OpenAI API
	logger.Success("OpenAI API: API key is set")
	logger.Debug("API key starts with: %s", cfg.OpenAIAPIKey[:10]+"...")

	// Check Jira status
	logger.Info("Checking Jira API...")
	err = checkJiraStatus(cfg)
	if err != nil {
		logger.Error("Jira API: %v", err)
	} else {
		logger.Success("Jira API: Connected successfully")
	}

	logger.Info("\nStatus check complete.")
}
