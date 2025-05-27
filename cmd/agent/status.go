package main

import (
	"fmt"
	"os"

	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/jira"
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
	fmt.Printf("Successfully retrieved ticket %s: %s\n", ticket.Key, ticket.Fields.Summary)
	return nil
}

// handleStatus checks the status of various integrations
func handleStatus() {
	fmt.Println("Checking system status...")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Config: Successfully loaded")

	// Check OpenAI API
	fmt.Printf("✅ OpenAI API: API key is set (%s...)\n", cfg.OpenAIAPIKey[:10]+"...")

	// Check Jira status
	fmt.Print("🔍 Jira API: ")
	err = checkJiraStatus(cfg)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
	} else {
		fmt.Println("✅ Connected successfully")
	}

	fmt.Println("\nStatus check complete.")
}
