package review

import (
	"fmt"
	"os"

	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/jira"
)

// LoadTicketDetails fetches and formats the Jira ticket information
func (w *Workflow) LoadTicketDetails() error {
	// Create a config for the Jira client
	cfg := &config.Config{
		// Get the OpenAI API key from environment
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		Model:        w.Ctx.Model,
		// Get Jira credentials from environment
		JiraURL:      os.Getenv("JIRA_URL"),
		JiraEmail:    os.Getenv("JIRA_EMAIL"),
		JiraToken:    os.Getenv("JIRA_API_TOKEN"),
	}

	// Check if Jira credentials are available
	if !cfg.HasJiraCredentials() {
		return fmt.Errorf("missing Jira credentials in environment variables - please set JIRA_URL, JIRA_EMAIL, and JIRA_API_TOKEN")
	}

	// Check if OpenAI API key is available
	if cfg.OpenAIAPIKey == "" {
		return fmt.Errorf("missing OpenAI API key in environment variables - please set OPENAI_API_KEY")
	}

	// Create Jira client
	client, err := jira.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Jira client: %w", err)
	}

	// Get the ticket
	fmt.Printf("Fetching Jira ticket %s...\n", w.Ctx.Ticket)
	ticket, err := client.GetTicket(w.Ctx.Ticket)
	if err != nil {
		return fmt.Errorf("failed to get ticket %s: %w", w.Ctx.Ticket, err)
	}

	// Format the ticket as markdown
	fmt.Println("Formatting ticket as markdown...")
	formattedTicket, err := client.FormatTicketAsMarkdown(ticket)
	if err != nil {
		return fmt.Errorf("failed to format ticket: %w", err)
	}

	// Store the formatted ticket
	w.Ctx.TicketDetails = formattedTicket
	return nil
}
