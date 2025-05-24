// Package review provides functionality for reviewing pull requests using LLMs.
package review

import (
	"context"
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

// RunLLMStep executes a generic LLM step (prepare prompt, send to LLM, save response)
func (w *Workflow) RunLLMStep(stepName string, promptFunc func() string, outputPath string) error {
	// 1. Prepare prompt using the provided function
	prompt := promptFunc()

	// 2. Send to LLM
	fmt.Printf("Sending %s prompt to OpenAI...\n", stepName)
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error in %s step: %w", stepName, err)
	}

	// 3. Save response
	err = os.WriteFile(outputPath, []byte(response), 0644)
	if err != nil {
		return fmt.Errorf("error saving %s output: %w", stepName, err)
	}

	fmt.Printf("%s step completed successfully. Output saved to %s\n", stepName, outputPath)
	return nil
}

// InitialDiscoveryPrompt generates the prompt for the initial discovery step
func (w *Workflow) InitialDiscoveryPrompt() string {
	return fmt.Sprintf(`You are a senior software engineer reviewing a pull request.

Here is a list of the files that were changed:
%s

Here is the full diff of the changes:
%s

Please provide:
1. A comprehensive summary of these changes in one paragraph.
2. The flow of logic through the files and functions formatted nicely as a trace.

Format your response in markdown with clear sections and code references.`,
		w.Ctx.FilesContent, w.Ctx.DiffContent)
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

	// Step 2: Initial discovery
	fmt.Println("Step 2: Performing initial discovery...")
	err = w.RunLLMStep(
		"Initial Discovery",
		w.InitialDiscoveryPrompt,
		filepath.Join(".context", "reviews", w.Ctx.Ticket+"-initial-discovery.md"),
	)
	if err != nil {
		return err
	}

	return nil
}
