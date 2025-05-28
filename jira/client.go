package jira

import (
	"fmt"

	jiralib "github.com/andygrunwald/go-jira"
	"github.com/jeremyhunt/agent-runner/config"
)

// Client is a wrapper around the Jira client
type Client struct {
	jiraClient *jiralib.Client
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

	return &Client{
		jiraClient: jiraClient,
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
