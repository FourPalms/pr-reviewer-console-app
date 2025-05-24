// Package review provides functionality for reviewing pull requests using LLMs.
package review

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeremyhunt/agent-runner/openai"
	"github.com/jeremyhunt/agent-runner/tokens"
)

// ReviewContext holds all the information needed for a PR review
type ReviewContext struct {
	// Ticket is the ticket number (e.g., "WIRE-1231")
	Ticket string
	
	// DiffPath is the path to the diff file
	DiffPath string
	
	// FilesPath is the path to the changed files list
	FilesPath string
	
	// MaxTokens is the maximum number of tokens allowed for the LLM context
	MaxTokens int
	
	// Model is the OpenAI model to use
	Model string
	
	// Client is the OpenAI client
	Client *openai.Client
	
	// TokenCounter is used to count tokens
	TokenCounter *tokens.Counter
	
	// Results from processing steps
	DiffContent  string
	FilesContent string
	DiffTokens   int
	FilesTokens  int
	TotalTokens  int
}

// NewReviewContext creates a new ReviewContext with default values
func NewReviewContext(ticket string, client *openai.Client) *ReviewContext {
	return &ReviewContext{
		Ticket:       ticket,
		DiffPath:     filepath.Join(".context", "reviews", ticket+"-diff.md"),
		FilesPath:    filepath.Join(".context", "reviews", ticket+"-files.md"),
		MaxTokens:    16000, // Default for GPT-4
		Model:        "gpt-4o",
		Client:       client,
		TokenCounter: tokens.NewCounter(),
	}
}

// Workflow handles the PR review process
type Workflow struct {
	Ctx *ReviewContext
}

// NewWorkflow creates a new workflow for the given context
func NewWorkflow(ctx *ReviewContext) *Workflow {
	return &Workflow{
		Ctx: ctx,
	}
}

// CountTokens reads the diff and files list and counts tokens
func (w *Workflow) CountTokens() error {
	// Read the diff file
	diffContent, err := os.ReadFile(w.Ctx.DiffPath)
	if err != nil {
		return fmt.Errorf("error reading diff file: %w", err)
	}
	
	// Read the files list
	filesContent, err := os.ReadFile(w.Ctx.FilesPath)
	if err != nil {
		return fmt.Errorf("error reading files list: %w", err)
	}
	
	// Count tokens in both files
	diffTokens, err := w.Ctx.TokenCounter.CountText(string(diffContent), w.Ctx.Model)
	if err != nil {
		return fmt.Errorf("error counting diff tokens: %w", err)
	}
	
	filesTokens, err := w.Ctx.TokenCounter.CountText(string(filesContent), w.Ctx.Model)
	if err != nil {
		return fmt.Errorf("error counting files tokens: %w", err)
	}
	
	totalTokens := diffTokens + filesTokens
	
	// Store results in context
	w.Ctx.DiffContent = string(diffContent)
	w.Ctx.FilesContent = string(filesContent)
	w.Ctx.DiffTokens = diffTokens
	w.Ctx.FilesTokens = filesTokens
	w.Ctx.TotalTokens = totalTokens
	
	// Print token information
	fmt.Printf("Token counts for ticket %s:\n", w.Ctx.Ticket)
	fmt.Printf("  Diff file:  %d tokens\n", diffTokens)
	fmt.Printf("  Files list: %d tokens\n", filesTokens)
	fmt.Printf("  Total:      %d tokens\n", totalTokens)
	fmt.Printf("  Max tokens: %d\n", w.Ctx.MaxTokens)
	
	if totalTokens > w.Ctx.MaxTokens {
		fmt.Printf("WARNING: Content exceeds token limit by %d tokens\n", totalTokens-w.Ctx.MaxTokens)
		return fmt.Errorf("token limit exceeded: %d tokens (limit: %d)", totalTokens, w.Ctx.MaxTokens)
	}
	
	fmt.Printf("Tokens remaining: %d\n", w.Ctx.MaxTokens-totalTokens)
	return nil
}

// Run executes the PR review workflow
func (w *Workflow) Run() error {
	// Step 1: Count tokens
	fmt.Println("Step 1: Counting tokens...")
	err := w.CountTokens()
	if err != nil {
		return fmt.Errorf("error counting tokens: %w", err)
	}
	fmt.Println("Token counting completed successfully.")
	
	// Future steps will be added here as we develop them
	// Step 2: Prepare prompt
	// Step 3: Send to LLM
	// Step 4: Process response
	
	return nil
}
