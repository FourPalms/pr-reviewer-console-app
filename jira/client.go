package jira

import (
	"context"
	"encoding/json"
	"fmt"

	jiralib "github.com/andygrunwald/go-jira"
	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/openai"
)

// Client is a wrapper around the Jira client
type Client struct {
	jiraClient *jiralib.Client
	openaiClient *openai.Client
	config     *config.Config
}

// NewClient creates a new Jira client
func NewClient(cfg *config.Config) (*Client, error) {
	// Check if Jira credentials are available
	if !cfg.HasJiraCredentials() {
		return nil, fmt.Errorf("missing Jira credentials")
	}

	// Create a basic auth transport for authentication
	tp := jiralib.BasicAuthTransport{
		Username: cfg.JiraEmail,
		Password: cfg.JiraToken,
	}

	// Create a new Jira client
	jiraClient, err := jiralib.NewClient(tp.Client(), cfg.JiraURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}

	// Create OpenAI client
	openaiClient := openai.NewClient(cfg.OpenAIAPIKey, cfg.Model)

	return &Client{
		jiraClient: jiraClient,
		openaiClient: openaiClient,
		config:     cfg,
	}, nil
}

// GetTicket retrieves a ticket from Jira
func (c *Client) GetTicket(ticketID string) (*jiralib.Issue, error) {
	issue, _, err := c.jiraClient.Issue.Get(ticketID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket %s: %w", ticketID, err)
	}
	return issue, nil
}

// FormatTicketAsMarkdown formats a Jira ticket as markdown using the LLM
func (c *Client) FormatTicketAsMarkdown(ticket *jiralib.Issue) (string, error) {
	// Convert the ticket to JSON for the LLM
	ticketJSON, err := json.Marshal(ticket)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ticket to JSON: %w", err)
	}

	// Create the prompt for the LLM
	prompt := fmt.Sprintf(`You are a technical documentation expert tasked with formatting a Jira ticket for use in a code review context. 

The ticket information will be used by an LLM (like yourself) to perform an in-depth code review of a pull request.

Format the ticket information as clean, well-structured markdown that highlights the most important aspects relevant for code review.

Focus on technical requirements, acceptance criteria, and any implementation details that would help evaluate the code changes.

Be concise but comprehensive - include all relevant information while keeping the format clean and readable.



Here is the raw Jira ticket data in JSON format:

%s



Format this as markdown, with appropriate sections and highlighting of key information.

Do not include any explanations or commentary outside of the formatted ticket content.`, string(ticketJSON))

	// Send the prompt to the LLM
	response, err := c.openaiClient.Complete(context.Background(), prompt)
	if err != nil {
		return "", fmt.Errorf("failed to get LLM response: %w", err)
	}

	return response, nil
}
