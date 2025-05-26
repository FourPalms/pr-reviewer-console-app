// Package review provides functionality for reviewing pull requests using LLMs.
package review

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	// RepoDir is the path to the cloned repository
	RepoDir string

	// Branch is the PR branch name
	Branch string

	// OutputDir is the directory where output files are stored
	OutputDir string

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
	outputDir := filepath.Join(".context", "reviews")
	return &ReviewContext{
		Ticket:       ticket,
		DiffPath:     filepath.Join(outputDir, ticket+"-diff.md"),
		FilesPath:    filepath.Join(outputDir, ticket+"-files.md"),
		RepoDir:      "", // Will be set when needed
		Branch:       "", // Will be set when needed
		OutputDir:    outputDir,
		MaxTokens:    120000, // Default for GPT-4o
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

// CollectOriginalFileContents reads the original content of modified and deleted files
func (w *Workflow) CollectOriginalFileContents() error {
	// 1. Read the files.md to get the list of modified and deleted files
	filesContent, err := os.ReadFile(w.Ctx.FilesPath)
	if err != nil {
		return fmt.Errorf("failed to read files list: %w", err)
	}

	// 2. Parse the content to extract modified and deleted files
	modifiedFiles := []string{}
	deletedFiles := []string{}

	scanner := bufio.NewScanner(strings.NewReader(string(filesContent)))
	section := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimPrefix(line, "## ")
			continue
		}

		// Skip empty lines, headers, and the stats section
		if line == "" || strings.HasPrefix(line, "#") || section == "Stats" {
			continue
		}

		if section == "Modified Files" {
			modifiedFiles = append(modifiedFiles, line)
		} else if section == "Deleted Files" {
			deletedFiles = append(deletedFiles, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning files list: %w", err)
	}

	// 3. Get the merge-base (common ancestor) of main and PR branch
	cmd := exec.Command("git", "merge-base", "main", w.Ctx.Branch)
	cmd.Dir = w.Ctx.RepoDir
	mergeBase, err := cmd.Output()
	if err != nil {
		// Try with master if main fails
		cmd = exec.Command("git", "merge-base", "master", w.Ctx.Branch)
		cmd.Dir = w.Ctx.RepoDir
		mergeBase, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to find merge-base: %w", err)
		}
	}
	baseCommit := strings.TrimSpace(string(mergeBase))

	// 4. Build the markdown content
	var sb strings.Builder
	sb.WriteString("# Original File Contents\n\n")
	// We'll add the token count later
	sb.WriteString("_Token count will be added here_\n\n")

	// 5. For each modified file
	for _, file := range modifiedFiles {
		// Execute git command to get content from the merge-base
		cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", baseCommit, file))
		cmd.Dir = w.Ctx.RepoDir
		content, err := cmd.Output()
		if err != nil {
			fmt.Printf("Warning: failed to get content for %s: %v\n", file, err)
			continue
		}

		// Add to our markdown
		sb.WriteString(fmt.Sprintf("## %s\n```php\n%s\n```\n\n", file, content))
	}

	// 6. Same for deleted files
	for _, file := range deletedFiles {
		cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", baseCommit, file))
		cmd.Dir = w.Ctx.RepoDir
		content, err := cmd.Output()
		if err != nil {
			fmt.Printf("Warning: failed to get content for deleted file %s: %v\n", file, err)
			continue
		}

		sb.WriteString(fmt.Sprintf("## %s (DELETED)\n```php\n%s\n```\n\n", file, content))
	}

	// Get the content as a string
	content := sb.String()

	// 7. Count tokens
	tokenCount, err := w.Ctx.TokenCounter.CountText(content, w.Ctx.Model)
	if err != nil {
		return fmt.Errorf("failed to count tokens: %w", err)
	}

	// 8. Replace the placeholder with the actual token count
	content = strings.Replace(
		content,
		"_Token count will be added here_",
		fmt.Sprintf("This file contains **%d tokens** when processed by %s.", tokenCount, w.Ctx.Model),
		1,
	)

	// 9. Write the result to a file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-original-file-content.md", w.Ctx.Ticket))
	err = os.WriteFile(outputPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write original file content: %w", err)
	}

	fmt.Printf("Original file contents saved to %s (%d tokens)\n", outputPath, tokenCount)
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

	// Step 2: Initial discovery
	fmt.Println("Step 2: Performing initial discovery...")
	err = w.RunLLMStep(
		"Initial Discovery",
		w.InitialDiscoveryPrompt,
		filepath.Join(w.Ctx.OutputDir, w.Ctx.Ticket+"-initial-discovery.md"),
	)
	if err != nil {
		return err
	}

	// Step 3: Collect original file contents
	fmt.Println("Step 3: Collecting original file contents...")
	err = w.CollectOriginalFileContents()
	if err != nil {
		return fmt.Errorf("error collecting original file contents: %w", err)
	}
	fmt.Println("Original file content collection completed successfully.")

	return nil
}
