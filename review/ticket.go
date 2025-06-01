package review

import (
	"context"
	"fmt"
	"os"

	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/jira"
	"github.com/jeremyhunt/agent-runner/logger"
)

// LoadTicketDetails fetches and formats the Jira ticket information
func (w *Workflow) LoadTicketDetails() error {
	// Create a config for the Jira client
	cfg := &config.Config{
		// Get Jira credentials from environment
		JiraURL:   os.Getenv("JIRA_URL"),
		JiraEmail: os.Getenv("JIRA_EMAIL"),
		JiraToken: os.Getenv("JIRA_API_TOKEN"),
	}

	// Check if Jira credentials are available
	if !cfg.HasJiraCredentials() {
		return fmt.Errorf("missing Jira credentials in environment variables - please set JIRA_URL, JIRA_EMAIL, and JIRA_API_TOKEN")
	}

	// Create Jira client
	client, err := jira.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Jira client: %w", err)
	}

	// Get the ticket
	// We don't need to log here since we're already logging in the Run method
	ticket, err := client.GetTicket(w.Ctx.Ticket)
	if err != nil {
		return fmt.Errorf("failed to get ticket %s: %w", w.Ctx.Ticket, err)
	}

	// Format the ticket as markdown using our existing LLM client
	logger.Verbose("Formatting ticket as markdown...")

	// Create a prompt for the LLM to format the ticket
	prompt := fmt.Sprintf(`You are a technical documentation expert tasked with formatting a Jira ticket for use in a code review context.

The ticket information will be used to perform an in-depth code review of a pull request.

Format the ticket information as clean, well-structured markdown that highlights the most important aspects relevant for code review.

Focus on technical requirements, acceptance criteria, and implementation details that would help evaluate code changes.

Be concise but comprehensive - include all relevant information while keeping the format clean and readable.

Here is the Jira ticket information:

Ticket Key: %s
Summary: %s
Status: %s
Description: %s

Format this as markdown, with appropriate sections and highlighting of key information.`,
		ticket.Key,
		ticket.Fields.Summary,
		ticket.Fields.Status.Name,
		ticket.Fields.Description)

	// Count tokens in the prompt
	tokenCount, err := w.Ctx.TokenCounter.CountText(prompt, w.Ctx.Model)
	if err != nil {
		logger.Debug("Warning: Could not count tokens in ticket prompt: %v", err)
	} else {
		logger.Verbose("Ticket formatting prompt contains %d tokens", tokenCount)
		if tokenCount > w.Ctx.MaxTokens/2 {
			return fmt.Errorf("ticket formatting prompt is too large (%d tokens, max is %d)", tokenCount, w.Ctx.MaxTokens/2)
		}
	}

	// Send to LLM
	formattedTicket, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("failed to format ticket: %w", err)
	}

	// Store the formatted ticket
	w.Ctx.TicketDetails = formattedTicket
	return nil
}
