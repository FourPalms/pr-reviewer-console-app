// Package review provides functionality for reviewing pull requests using LLMs.
package review

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

	// Add token count to the diff file
	tokenInfoDiff := fmt.Sprintf("\n\nThis diff contains **%d tokens** when processed by %s.\n\n", diffTokens, w.Ctx.Model)
	newDiffContent := tokenInfoDiff + string(diffContent)
	err = os.WriteFile(w.Ctx.DiffPath, []byte(newDiffContent), 0644)
	if err != nil {
		return fmt.Errorf("error updating diff file with token count: %w", err)
	}

	// Add token count to the files list file
	tokenInfoFiles := fmt.Sprintf("\n\nThis file list contains **%d tokens** when processed by %s.\n\n", filesTokens, w.Ctx.Model)
	newFilesContent := strings.Replace(string(filesContent), "\n\n## Modified Files", tokenInfoFiles+"## Modified Files", 1)
	err = os.WriteFile(w.Ctx.FilesPath, []byte(newFilesContent), 0644)
	if err != nil {
		return fmt.Errorf("error updating files list with token count: %w", err)
	}

	// Update the content in context with the new versions that include token counts
	w.Ctx.DiffContent = newDiffContent
	w.Ctx.FilesContent = newFilesContent

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
	return fmt.Sprintf(`You are a senior software engineer reviewing a pull request in a PHP codebase that uses a custom architecture with silo/service/domain/repository/applicationservice patterns.

Here is a list of the files that were changed:
%s

Here is the full diff of the changes:
%s

Please provide your analysis in the following format with EXACTLY these section headings:

## 1. Comprehensive Summary
[Your one-paragraph summary of the changes here]

## 2. Flow of Logic
[Your trace of the logic flow through files and functions here]

## 3. Recommended File Order
[Your recommended order for analyzing the files here]

For the recommended file order section:
- Always include the COMPLETE file paths exactly as they appear in the file list above
- List each file path inside backticks
- Number each file (1, 2, 3, etc.)
- Include a brief explanation of why you chose this sequence

Consider the architecture patterns where:
- DataObjects define data structures
- Repositories handle data access
- Domains contain business logic
- Services orchestrate operations
- ApplicationServices provide functionality across domains

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

// ParseRecommendedFileOrder extracts the recommended file analysis order from the initial discovery
func (w *Workflow) ParseRecommendedFileOrder() ([]string, error) {
	// Read the initial discovery file
	initialDiscoveryPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-initial-discovery.md", w.Ctx.Ticket))
	content, err := os.ReadFile(initialDiscoveryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read initial discovery: %w", err)
	}

	// Convert to string for easier processing
	contentStr := string(content)

	// Find the recommended file order section using the exact heading we specified
	orderSectionPattern := "(?s)## 3. Recommended File Order.*?(?:\\n##|$)"
	orderSectionRegex := regexp.MustCompile(orderSectionPattern)
	orderSection := orderSectionRegex.FindString(contentStr)
	if orderSection == "" {
		return nil, fmt.Errorf("could not find recommended file order section in initial discovery")
	}

	// Extract filenames using regex
	// Looking for patterns like: `app/PayrollServices/Silo/Client/Domain/PayCycleDomain.php`
	filePattern := "`([^`]+\\.php)`"
	fileRegex := regexp.MustCompile(filePattern)
	matches := fileRegex.FindAllStringSubmatch(orderSection, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("could not find any filenames in recommended order section")
	}

	// Extract the filenames from the regex matches
	files := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			files = append(files, match[1])
		}
	}

	return files, nil
}

// GetOriginalFileContent retrieves the content of a file from before the PR changes
func (w *Workflow) GetOriginalFileContent(file string) (string, error) {
	// Get the merge-base (common ancestor) of main and PR branch
	cmd := exec.Command("git", "merge-base", "main", w.Ctx.Branch)
	cmd.Dir = w.Ctx.RepoDir
	mergeBase, err := cmd.Output()
	if err != nil {
		// Try with master if main fails
		cmd = exec.Command("git", "merge-base", "master", w.Ctx.Branch)
		cmd.Dir = w.Ctx.RepoDir
		mergeBase, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to find merge-base: %w", err)
		}
	}
	baseCommit := strings.TrimSpace(string(mergeBase))

	// Get the file content at the merge-base commit
	cmd = exec.Command("git", "show", fmt.Sprintf("%s:%s", baseCommit, file))
	cmd.Dir = w.Ctx.RepoDir
	content, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get content for %s: %w", file, err)
	}

	return string(content), nil
}

// FileAnalysisPrompt generates a prompt for analyzing a single file
func (w *Workflow) FileAnalysisPrompt(filename, content string) string {
	prompt := "You are a senior PHP developer analyzing a file from a codebase that uses a custom silo/service/domain/repository/applicationservice architecture.\n\n"
	prompt += "Your goal is to understand how the specific feature being changed in this PR worked BEFORE the changes were applied.\n\n"
	prompt += fmt.Sprintf("File: %s\n\n", filename)
	prompt += "Here's the original content of the file before changes:\n```php\n"
	prompt += content
	prompt += "\n```\n\n"
	prompt += "Here's the diff showing what's changing in the PR:\n"
	prompt += w.Ctx.DiffContent
	prompt += "\n\nFocus on:\n"
	prompt += "1. What specific feature or functionality does this file contribute to, based on the PR changes?\n"
	prompt += "2. How did the key functions/methods work before the changes, especially those affected by the PR?\n"
	prompt += "3. What were the inputs, outputs, and dependencies of these specific functions?\n"
	prompt += "4. What business rules or validation logic was implemented in this specific feature?\n"
	prompt += "5. How did this component interact with other parts of the system for this specific feature?\n\n"
	prompt += "IMPORTANT: Your analysis will be used by another LLM to understand the pre-change implementation when reviewing the PR changes. Focus on the specific feature being modified, not the general system architecture.\n\n"
	prompt += "Provide a clear, concise analysis that explains how this component functioned before the changes, focusing on the specific feature being modified."

	return prompt
}

// AnalyzeFile sends a file to the LLM for analysis and returns the result
func (w *Workflow) AnalyzeFile(filename, content string) (string, error) {
	prompt := w.FileAnalysisPrompt(filename, content)

	fmt.Printf("Analyzing file: %s\n", filename)
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return "", fmt.Errorf("error analyzing file %s: %w", filename, err)
	}

	return response, nil
}

// AnalyzeOriginalImplementation analyzes each file to understand the original implementation
func (w *Workflow) AnalyzeOriginalImplementation() error {
	// 1. Parse the recommended file order
	orderedFiles, err := w.ParseRecommendedFileOrder()
	if err != nil {
		return fmt.Errorf("error parsing recommended file order: %w", err)
	}

	// 2. Create the output file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-original-implementation.md", w.Ctx.Ticket))
	var sb strings.Builder
	sb.WriteString("# Original Implementation Analysis\n\n")
	sb.WriteString("This document provides an analysis of how the code worked before the changes in this PR.\n\n")

	// 3. Process each file in order
	for i, file := range orderedFiles {
		fmt.Printf("Analyzing file %d/%d: %s\n", i+1, len(orderedFiles), file)

		// Get original file content
		content, err := w.GetOriginalFileContent(file)
		if err != nil {
			fmt.Printf("Warning: could not get content for %s: %v\n", file, err)
			continue
		}

		// Analyze with LLM
		analysis, err := w.AnalyzeFile(file, content)
		if err != nil {
			fmt.Printf("Warning: analysis failed for %s: %v\n", file, err)
			continue
		}

		// Add to output
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", file, analysis))
	}

	// 4. Count tokens in the result
	outputContent := sb.String()
	tokenCount, err := w.Ctx.TokenCounter.CountText(outputContent, w.Ctx.Model)
	if err == nil {
		sb.WriteString(fmt.Sprintf("\n\n---\n\nThis analysis contains **%d tokens** when processed by %s.\n", tokenCount, w.Ctx.Model))
		outputContent = sb.String()
	}

	// 5. Write the result to a file
	err = os.WriteFile(outputPath, []byte(outputContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write analysis: %w", err)
	}

	fmt.Printf("Original implementation analysis saved to %s\n", outputPath)
	return nil
}

// SynthesizeOriginalImplementation takes the individual file analyses and creates a synthesized understanding
func (w *Workflow) SynthesizeOriginalImplementation() error {
	// 1. Read the original implementation analysis file
	originalImplementationPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-original-implementation.md", w.Ctx.Ticket))
	content, err := os.ReadFile(originalImplementationPath)
	if err != nil {
		return fmt.Errorf("failed to read original implementation analysis: %w", err)
	}

	// 2. Create the prompt for synthesis
	prompt := "You are a senior software engineer tasked with synthesizing individual file analyses into a cohesive understanding of a specific feature.\n\n"
	prompt += "Below are detailed analyses of each file involved in a feature that's being changed in a PR.\n\n"
	prompt += "Your task is to synthesize these individual analyses into a comprehensive understanding of how the specific feature worked as a system BEFORE the changes.\n\n"
	prompt += "Focus on:\n"
	prompt += "1. Identify the specific feature being modified based on the file analyses and PR changes\n"
	prompt += "2. The complete data and control flow through the system for this specific feature\n"
	prompt += "3. The business rules and validation logic specific to this feature\n"
	prompt += "4. How the different components interacted with each other to implement this feature\n"
	prompt += "5. Potential edge cases or limitations in the original implementation of this feature\n\n"
	prompt += "IMPORTANT: Your synthesis will be used as context by another LLM for reviewing the PR changes. Therefore:\n"
	prompt += "- Focus on the specific feature being modified, not the general system architecture\n"
	prompt += "- Provide concrete details about how this specific feature worked\n"
	prompt += "- Highlight specific methods, parameters, and business rules that are directly relevant\n"
	prompt += "- Structure your response to be maximally useful as context for understanding the changes\n\n"
	prompt += "Here are the individual file analyses:\n\n"
	prompt += string(content)
	prompt += "\n\nProvide a clear, comprehensive synthesis that explains how this specific feature functioned as a cohesive system before the changes."

	// 3. Send to LLM for synthesis
	fmt.Println("Synthesizing file analyses...")
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error synthesizing original implementation: %w", err)
	}

	// 4. Create the output file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-original-synthesis.md", w.Ctx.Ticket))
	var sb strings.Builder
	sb.WriteString("# Original Implementation Synthesis\n\n")
	sb.WriteString("This document provides a synthesized understanding of how the feature worked as a cohesive system before the changes in this PR.\n\n")
	sb.WriteString(response)

	// 5. Count tokens in the result
	outputContent := sb.String()
	tokenCount, err := w.Ctx.TokenCounter.CountText(outputContent, w.Ctx.Model)
	if err == nil {
		sb.WriteString(fmt.Sprintf("\n\n---\n\nThis synthesis contains **%d tokens** when processed by %s.\n", tokenCount, w.Ctx.Model))
		outputContent = sb.String()
	}

	// 6. Write the result to a file
	err = os.WriteFile(outputPath, []byte(outputContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write synthesis: %w", err)
	}

	fmt.Printf("Original implementation synthesis saved to %s\n", outputPath)
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

	// Step 4: Analyze original implementation
	fmt.Println("Step 4: Analyzing original implementation...")
	err = w.AnalyzeOriginalImplementation()
	if err != nil {
		return fmt.Errorf("error analyzing original implementation: %w", err)
	}
	fmt.Println("Original implementation analysis completed successfully.")

	// Step 5: Synthesize original implementation
	fmt.Println("Step 5: Synthesizing original implementation...")
	err = w.SynthesizeOriginalImplementation()
	if err != nil {
		return fmt.Errorf("error synthesizing original implementation: %w", err)
	}
	fmt.Println("Original implementation synthesis completed successfully.")

	return nil
}
