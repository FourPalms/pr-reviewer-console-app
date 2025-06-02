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
	"sync"
	"sync/atomic"

	"github.com/jeremyhunt/agent-runner/logger"
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

	// DesignDocPath is the name of the design document to include in the review
	DesignDocPath string

	// DesignDocContent is the content of the design document
	DesignDocContent string

	// TicketDetails is the formatted Jira ticket information
	TicketDetails string

	// Results from processing steps
	DiffContent      string
	FilesContent     string
	SynthesisContent string
	DiffTokens       int
	FilesTokens      int
	TotalTokens      int
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

	// Log token information
	logger.Verbose("Token counts for ticket %s:", w.Ctx.Ticket)
	logger.Verbose("  Diff file:  %d tokens", diffTokens)
	logger.Verbose("  Files list: %d tokens", filesTokens)
	logger.Verbose("  Total:      %d tokens", totalTokens)
	logger.Verbose("  Max tokens: %d", w.Ctx.MaxTokens)

	if totalTokens > w.Ctx.MaxTokens {
		logger.Error("Content exceeds token limit by %d tokens", totalTokens-w.Ctx.MaxTokens)
		return fmt.Errorf("token limit exceeded: %d tokens (limit: %d)", totalTokens, w.Ctx.MaxTokens)
	}

	logger.Verbose("Tokens remaining: %d", w.Ctx.MaxTokens-totalTokens)
	return nil
}

// RunLLMStep executes a generic LLM step (prepare prompt, send to LLM, save response)
func (w *Workflow) RunLLMStep(stepName string, promptFunc func() string, outputPath string) error {
	// 1. Prepare prompt using the provided function
	prompt := promptFunc()

	// 2. Send to LLM
	// Note: We don't need to log this here since it's already logged in the Step functions
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error in %s step: %w", stepName, err)
	}

	// 3. Save response
	err = os.WriteFile(outputPath, []byte(response), 0644)
	if err != nil {
		return fmt.Errorf("error saving %s output: %w", stepName, err)
	}

	logger.Success("%s step completed", stepName)
	logger.Debug("Output saved to %s", outputPath)
	return nil
}

// InitialDiscoveryPrompt generates the prompt for the initial discovery step
func (w *Workflow) InitialDiscoveryPrompt() string {
	// Base prompt template
	promptTemplate := `You are a senior software engineer reviewing a pull request in a PHP codebase that uses a custom architecture with silo/service/domain/repository/applicationservice patterns.

Here is a list of the files that were changed:
%s

Here is the full diff of the changes:
%s%s%s

Please provide your analysis in the following format with EXACTLY these section headings:

## 1. Comprehensive Summary
[Your one-paragraph summary of the changes here]

## 2. Flow of Logic
[Your trace of the logic flow through files and functions here]

## 3. Recommended File Order
[Your recommended order for analyzing the files here]%s%s

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

Format your response in markdown with clear sections and code references.`

	// Add design document section if available
	designDocSection := ""
	designDocInstruction := ""
	if w.Ctx.DesignDocContent != "" {
		designDocSection = fmt.Sprintf("\n\n## Design Document\n\nThe following design document provides context for this PR:\n\n%s\n\nPlease consider this design document when analyzing the PR changes.", w.Ctx.DesignDocContent)
		designDocInstruction = "\n\n## 4. Design Alignment\n[Your assessment of how well the changes align with the design document]"
	}

	// Add ticket details if available
	ticketSection := ""
	ticketInstruction := ""
	if w.Ctx.TicketDetails != "" {
		ticketSection = fmt.Sprintf("\n\n## Jira Ticket\n\nThe following Jira ticket provides context for this PR:\n\n%s\n\nPlease consider this ticket information when analyzing the PR changes.", w.Ctx.TicketDetails)
		ticketInstruction = "\n\n## 5. Ticket Alignment\n[Your assessment of how well the changes address the requirements in the ticket]"
	}

	return fmt.Sprintf(promptTemplate, w.Ctx.FilesContent, w.Ctx.DiffContent, designDocSection, ticketSection, designDocInstruction, ticketInstruction)
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
			logger.Debug("Warning: failed to get content for %s: %v", file, err)
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
			logger.Debug("Warning: failed to get content for deleted file %s: %v", file, err)
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

	logger.Debug("Original file contents saved (%d tokens)", tokenCount)
	logger.Debug("Output path: %s", outputPath)
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

	// We don't need to log here since we're already logging in the worker
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

	// 3. Process files using a worker pool
	type analysisResult struct {
		file     string
		analysis string
		err      error
		index    int
	}

	// Create a slice to store results in the correct order
	results := make([]analysisResult, len(orderedFiles))

	// Create a mutex to protect shared resources
	var resultsMutex sync.Mutex

	// Create a WaitGroup to track when all files have been processed
	var wg sync.WaitGroup
	wg.Add(len(orderedFiles))

	// Create a channel for job distribution
	jobs := make(chan int, len(orderedFiles))

	// Determine the number of workers (max 10)
	numWorkers := 10
	if len(orderedFiles) < numWorkers {
		numWorkers = len(orderedFiles)
	}

	// Create an atomic counter for active workers
	var activeWorkers int32

	// We're already logging this in the Step function, so we don't need to log it here

	// Launch worker goroutines
	for workerID := 0; workerID < numWorkers; workerID++ {
		go func(id int) {
			// Process jobs until the channel is closed
			for i := range jobs {
				file := orderedFiles[i]

				// Increment active workers counter
				atomic.AddInt32(&activeWorkers, 1)

				// Print progress (protected by mutex to avoid garbled output)
				resultsMutex.Lock()
				logger.AnalysisItem(id+1, file) // Use worker ID + 1 so it starts from 1 instead of 0
				// Only print debug info in debug mode
				if logger.IsDebugEnabled() {
					logger.Debug("[Worker %d] Analyzing file %d/%d: %s (Active workers: %d)",
						id, i+1, len(orderedFiles), file, atomic.LoadInt32(&activeWorkers))
				}
				resultsMutex.Unlock()

				// Get original file content
				content, err := w.GetOriginalFileContent(file)
				if err != nil {
					resultsMutex.Lock()
					logger.Debug("Warning: could not get content for %s: %v", file, err)
					resultsMutex.Unlock()

					// Store the error result
					results[i] = analysisResult{file: file, err: err, index: i}
					wg.Done()
					continue
				}

				// Analyze with LLM
				analysis, err := w.AnalyzeFile(file, content)
				if err != nil {
					resultsMutex.Lock()
					logger.Debug("Warning: analysis failed for %s: %v", file, err)
					resultsMutex.Unlock()

					// Store the error result
					results[i] = analysisResult{file: file, err: err, index: i}
					wg.Done()
					continue
				}

				// Store the successful result
				results[i] = analysisResult{file: file, analysis: analysis, index: i}

				// Log completion with the same mutex protection
				resultsMutex.Lock()
				logger.AnalysisCompleted(id+1, file)
				resultsMutex.Unlock()

				// Decrement active workers counter
				atomic.AddInt32(&activeWorkers, -1)
				wg.Done()
			}
		}(workerID)
	}

	// Send jobs to the workers
	for i := range orderedFiles {
		jobs <- i
	}

	// Close the jobs channel to signal that no more jobs are coming
	close(jobs)

	// Wait for all files to be processed
	wg.Wait()

	// Add a blank line after all workers have completed
	fmt.Println()

	// Process results in the original order
	for _, result := range results {
		// Skip files that had errors
		if result.err != nil {
			continue
		}

		// Add to output
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", result.file, result.analysis))
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

	logger.Debug("Original implementation analysis saved")
	logger.Debug("Output path: %s", outputPath)
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
	logger.Debug("Synthesizing file analyses...")
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

	// 6. Store the synthesis content in the context for later use
	w.Ctx.SynthesisContent = outputContent

	// 7. Write the result to a file
	err = os.WriteFile(outputPath, []byte(outputContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write synthesis: %w", err)
	}

	logger.Debug("Original implementation synthesis saved")
	logger.Debug("Output path: %s", outputPath)
	return nil
}

// GenerateSyntaxReviewPrompt creates a prompt for the syntax and best practices review step
func (w *Workflow) GenerateSyntaxReviewPrompt() string {
	// Use the synthesis content stored in the context
	synthesisContent := "No synthesis available."
	if w.Ctx.SynthesisContent != "" {
		synthesisContent = w.Ctx.SynthesisContent
	}

	// Build the prompt using a string builder for better maintainability
	var sb strings.Builder

	// Overview section - common across all review types
	sb.WriteString("# Code Review: Syntax and Best Practices\n\n")
	sb.WriteString("You are a Senior Engineer with expertise in PHP and general software development. ")
	sb.WriteString("You're reviewing code for other senior developers who value helpfulness, brevity, and professionalism. ")
	sb.WriteString("Your goal is to identify syntax issues and best practice violations that could cause the team problems. ")
	sb.WriteString("Focus on substance over form, and avoid stating things that would be obvious to experienced developers.\n\n")

	// Review focus section
	sb.WriteString("## Review Focus\n\n")
	sb.WriteString("For this syntax review, focus on:\n\n")
	sb.WriteString("1. **Syntax Issues**: Identify errors that would cause runtime failures, typos in names, missing syntax elements, and namespace issues.\n\n")
	sb.WriteString("2. **Logic and Variable Usage**: Examine parameter usage, type handling, null/undefined access, conditional logic, and error handling patterns.\n\n")
	sb.WriteString("3. **Review Limitations**: Explicitly state if you have sufficient context and what additional information would improve the review.\n\n")

	// Machine consumption format section
	sb.WriteString("## Machine Consumption Format\n\n")
	sb.WriteString("IMPORTANT: Your output will be processed by another LLM to create a consolidated review, not read directly by humans.\n\n")
	sb.WriteString("Use these consistent tags and format:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<SYNTAX_REVIEW>\n")
	sb.WriteString("  <REVIEW_SUMMARY>\n")
	sb.WriteString("  Brief assessment of findings and limitations\n")
	sb.WriteString("  </REVIEW_SUMMARY>\n\n")

	sb.WriteString("  <CRITICAL_ISSUES>\n")
	sb.WriteString("  [List syntax errors that would cause runtime failures]\n")
	sb.WriteString("  </CRITICAL_ISSUES>\n\n")

	sb.WriteString("  <LOGIC_ISSUES>\n")
	sb.WriteString("  [List problems with variable/parameter usage and logic]\n")
	sb.WriteString("  </LOGIC_ISSUES>\n\n")

	sb.WriteString("  <IMPROVEMENT_SUGGESTIONS>\n")
	sb.WriteString("  [List best practice violations]\n")
	sb.WriteString("  </IMPROVEMENT_SUGGESTIONS>\n\n")

	sb.WriteString("  <REVIEW_LIMITATIONS>\n")
	sb.WriteString("  [State what additional context would help]\n")
	sb.WriteString("  </REVIEW_LIMITATIONS>\n")
	sb.WriteString("</SYNTAX_REVIEW>\n")
	sb.WriteString("```\n\n")

	sb.WriteString("For each issue, use this format:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("LINE: 42\n")
	sb.WriteString("SEVERITY: [Critical|Major|Minor]\n")
	sb.WriteString("PROBLEM: Brief description\n")
	sb.WriteString("SOLUTION_CODE:\n")
	sb.WriteString("```php\n")
	sb.WriteString("// Original\n")
	sb.WriteString("$original = $code->here();\n\n")
	sb.WriteString("// Fixed\n")
	sb.WriteString("$fixed = $code->here();\n")
	sb.WriteString("```\n")
	sb.WriteString("</ISSUE>\n")
	sb.WriteString("```\n\n")

	sb.WriteString("If no issues found in a category: `<NO_ISSUES_FOUND/>`\n\n")
	sb.WriteString("Focus exclusively on syntax and logic - ignore broader functionality concerns.\n\n")

	// Context section
	sb.WriteString("## Context\n\n")
	sb.WriteString("The following context is provided for your review:\n\n")

	// Original implementation
	sb.WriteString("### Original Implementation\n\n")
	sb.WriteString(synthesisContent)

	// PR changes
	sb.WriteString("\n\n### Changes in this PR\n\n")
	sb.WriteString(w.Ctx.DiffContent)

	// Add design document if available
	if w.Ctx.DesignDocContent != "" {
		sb.WriteString("\n\n### Design Document\n\n")
		sb.WriteString(w.Ctx.DesignDocContent)
	}

	// Add ticket details if available
	if w.Ctx.TicketDetails != "" {
		sb.WriteString("\n\n### Jira Ticket\n\n")
		sb.WriteString(w.Ctx.TicketDetails)
	}

	return sb.String()
}

// GenerateFunctionalityReviewPrompt creates a prompt for the functionality review step
func (w *Workflow) GenerateFunctionalityReviewPrompt() string {
	// Use the synthesis content stored in the context
	synthesisContent := "No synthesis available."
	if w.Ctx.SynthesisContent != "" {
		synthesisContent = w.Ctx.SynthesisContent
	}

	// Build the prompt using a string builder for better maintainability
	var sb strings.Builder

	// Overview section - common across all review types
	sb.WriteString("# Code Review: Implementation vs Requirements\n\n")
	sb.WriteString("You are a Senior Engineer with expertise in PHP and general software development. ")
	sb.WriteString("You're reviewing code for other senior developers who value helpfulness, brevity, and professionalism. ")
	sb.WriteString("Your goal is to identify missing or incorrect functionality that could cause the team problems. ")
	sb.WriteString("Focus on substance over form, and avoid stating things that would be obvious to experienced developers.\n\n")

	// Review focus section
	sb.WriteString("## Review Focus\n\n")
	sb.WriteString("For this functionality review, focus on:\n\n")
	sb.WriteString("Use the included context of Jira ticket content and (if exists) design doc content to determine whether or not the implement code completely and correctly satisfies all requirements.")
	sb.WriteString("List any missing or incorrectly implemented functionality separately")
	sb.WriteString("WITH COMPLETE HONESTY ANSWER: Am I able, with the given context, to review this implementation thoroughly? Yes or No, then why if No.")
	sb.WriteString("WITH COMPLETE HONESTY ANSWER: What context is missing or would be helpful if the above answer is NO. Be specific on WHY this is needed for the review, after asking: would a Sr Dev on the team expect to the have the context I think is missing?.")
	sb.WriteString("Again, NO above, explain exactly HOW you would use the context you suggest is missing, if you had it.")

	// Machine consumption format section
	sb.WriteString("## Machine Consumption Format\n\n")
	sb.WriteString("IMPORTANT: Your output will be processed by another LLM to create a consolidated review, not read directly by humans.\n\n")
	sb.WriteString("Use these consistent tags and format:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<FUNCTIONALITY_REVIEW>\n")
	sb.WriteString("  <REVIEW_SUMMARY>\n")
	sb.WriteString("  Brief assessment of findings and limitations\n")
	sb.WriteString("  </REVIEW_SUMMARY>\n\n")

	sb.WriteString("  <FUNCTIONALITY_ISSUES>\n")
	sb.WriteString("  [List misses or errors in implemented functionality, as compared to ticket/design doc reqs]\n")
	sb.WriteString("  </FUNCTIONALITY_ISSUES>\n\n")

	sb.WriteString("  <REVIEW_LIMITATIONS>\n")
	sb.WriteString("  [State if able to do a thorough review]\n")
	sb.WriteString("  [State what additional context would help]\n")
	sb.WriteString("  </REVIEW_LIMITATIONS>\n")
	sb.WriteString("</FUNCTIONALITY_REVIEW>\n")
	sb.WriteString("```\n\n")

	sb.WriteString("If no issues found in a category: `<NO_ISSUES_FOUND/>`\n\n")
	sb.WriteString("Focus exclusively on functionality implementation per ticket/design doc - ignore broader syntax and logic concerns.\n\n")

	// Context section
	sb.WriteString("## Context\n\n")
	sb.WriteString("The following context is provided for your review:\n\n")

	// Original implementation
	sb.WriteString("### Original Implementation\n\n")
	sb.WriteString(synthesisContent)

	// PR changes
	sb.WriteString("\n\n### Changes in this PR\n\n")
	sb.WriteString(w.Ctx.DiffContent)

	// Add design document if available
	if w.Ctx.DesignDocContent != "" {
		sb.WriteString("\n\n### Design Document\n\n")
		sb.WriteString(w.Ctx.DesignDocContent)
	}

	// Add ticket details if available
	if w.Ctx.TicketDetails != "" {
		sb.WriteString("\n\n### Jira Ticket\n\n")
		sb.WriteString(w.Ctx.TicketDetails)
	}

	return sb.String()
}

// GenerateDefensiveReviewPrompt creates a prompt for the defensive programming review step
func (w *Workflow) GenerateDefensiveReviewPrompt() string {
	// Use the synthesis content stored in the context
	synthesisContent := "No synthesis available."
	if w.Ctx.SynthesisContent != "" {
		synthesisContent = w.Ctx.SynthesisContent
	}

	// Build the prompt using a string builder for better maintainability
	var sb strings.Builder

	// Overview section - common across all review types
	sb.WriteString("# Code Review: Defensive Programming\n\n")
	sb.WriteString("You are a Senior Engineer with expertise in PHP and general software development. ")
	sb.WriteString("You're reviewing code for other senior developers who value helpfulness, brevity, and professionalism. ")
	sb.WriteString("Your goal is to identify security issues, error handling gaps, and edge cases that could cause production problems. ")
	sb.WriteString("Focus on substance over form, and avoid stating things that would be obvious to experienced developers.\n\n")

	// Review focus section
	sb.WriteString("## Review Focus\n\n")
	sb.WriteString("For this defensive programming review, focus on:\n\n")
	sb.WriteString("1. Based on what we know of the original functionality (see context below), could this new functionality break anything?\n\n")
	sb.WriteString("2. Deep dive into funcs - check to see whether the vars being passed in are actually what the func content expects\n\n")
	sb.WriteString("3. Look specifically for areas that could expose uncaught errors/exceptions that would break or interrupt calling code\n\n")
	sb.WriteString("4. **Review Limitations**: Explicitly state if you have sufficient context and what additional information would improve the review.\n\n")

	// Machine consumption format section
	sb.WriteString("## Machine Consumption Format\n\n")
	sb.WriteString("IMPORTANT: Your output will be processed by another LLM to create a consolidated review, not read directly by humans.\n\n")
	sb.WriteString("Use these consistent tags and format:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<DEFENSIVE_REVIEW>\n")
	sb.WriteString("  <REVIEW_SUMMARY>\n")
	sb.WriteString("  Brief assessment of findings and limitations\n")
	sb.WriteString("  </REVIEW_SUMMARY>\n\n")

	sb.WriteString("  <ERROR_HANDLING_ISSUES>\n")
	sb.WriteString("  [List missing or inadequate error handling]\n")
	sb.WriteString("  </ERROR_HANDLING_ISSUES>\n\n")

	sb.WriteString("  <EDGE_CASE_ISSUES>\n")
	sb.WriteString("  [List unhandled edge cases]\n")
	sb.WriteString("  </EDGE_CASE_ISSUES>\n\n")

	sb.WriteString("  <SECURITY_ISSUES>\n")
	sb.WriteString("  [List potential security concerns]\n")
	sb.WriteString("  </SECURITY_ISSUES>\n\n")

	sb.WriteString("  <RESOURCE_ISSUES>\n")
	sb.WriteString("  [List resource management issues]\n")
	sb.WriteString("  </RESOURCE_ISSUES>\n\n")

	sb.WriteString("  <REVIEW_LIMITATIONS>\n")
	sb.WriteString("  [State what additional context would help]\n")
	sb.WriteString("  </REVIEW_LIMITATIONS>\n")
	sb.WriteString("</DEFENSIVE_REVIEW>\n")
	sb.WriteString("```\n\n")

	sb.WriteString("For each issue, use this format:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("LINE: 42\n")
	sb.WriteString("SEVERITY: [Critical|Major|Minor]\n")
	sb.WriteString("PROBLEM: Brief description\n")
	sb.WriteString("SOLUTION_CODE:\n")
	sb.WriteString("```php\n")
	sb.WriteString("// Original\n")
	sb.WriteString("$original = $code->here();\n\n")
	sb.WriteString("// Fixed\n")
	sb.WriteString("$fixed = $code->here();\n")
	sb.WriteString("```\n")
	sb.WriteString("</ISSUE>\n")
	sb.WriteString("```\n\n")

	sb.WriteString("If no issues found in a category: `<NO_ISSUES_FOUND/>`\n\n")
	sb.WriteString("Focus exclusively on defensive programming concerns - ignore syntax and functionality issues already covered in other reviews.\n\n")

	// Context section
	sb.WriteString("## Context\n\n")
	sb.WriteString("The following context is provided for your review:\n\n")

	// Original implementation
	sb.WriteString("### Original Implementation\n\n")
	sb.WriteString(synthesisContent)

	// PR changes
	sb.WriteString("\n\n### Changes in this PR\n\n")
	sb.WriteString(w.Ctx.DiffContent)

	// Add design document if available
	if w.Ctx.DesignDocContent != "" {
		sb.WriteString("\n\n### Design Document\n\n")
		sb.WriteString(w.Ctx.DesignDocContent)
	}

	// Add ticket details if available
	if w.Ctx.TicketDetails != "" {
		sb.WriteString("\n\n### Jira Ticket\n\n")
		sb.WriteString(w.Ctx.TicketDetails)
	}

	return sb.String()
}

// GenerateFinalSummaryPrompt creates a prompt for the final summary step
func (w *Workflow) GenerateFinalSummaryPrompt() string {
	// This will be implemented in Phase 2
	return ""
}

// GenerateSyntaxReview generates a review focusing on PHP syntax and best practices
func (w *Workflow) GenerateSyntaxReview() error {
	// 1. Generate the prompt
	prompt := w.GenerateSyntaxReviewPrompt()

	// 2. Count tokens in the prompt
	tokenCount, err := w.Ctx.TokenCounter.CountText(prompt, w.Ctx.Model)
	if err != nil {
		logger.Debug("Warning: Could not count tokens in syntax review prompt: %v", err)
	} else {
		logger.Verbose("Syntax review prompt contains %d tokens", tokenCount)
		if tokenCount > w.Ctx.MaxTokens/2 {
			logger.Debug("Warning: Syntax review prompt is very large (%d tokens)", tokenCount)
		}
	}

	// 3. Send to LLM for review
	logger.Debug("Generating syntax review...")
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error generating syntax review: %w", err)
	}

	// 4. Create or append to the output file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-review-result.md", w.Ctx.Ticket))

	// Check if file exists
	var content []byte
	fileExists := false
	if _, err := os.Stat(outputPath); err == nil {
		// File exists, read it
		content, err = os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("error reading existing review file: %w", err)
		}
		fileExists = true
	}

	// Prepare content to write
	var sb strings.Builder
	if !fileExists {
		// Create new file with header
		sb.WriteString("# PR Review Results\n\n")
		sb.WriteString("This document contains a thorough review of the PR changes from multiple perspectives.\n\n")
	}

	// Append existing content if any
	if fileExists {
		sb.Write(content)
		// Add a separator
		sb.WriteString("\n\n---\n\n")
	}

	// Add the syntax review section
	sb.WriteString(response)

	// 5. Count tokens in the result
	outputContent := sb.String()
	tokenCount, err = w.Ctx.TokenCounter.CountText(outputContent, w.Ctx.Model)
	if err == nil && !fileExists {
		// Only add token count info if this is a new file
		sb.WriteString(fmt.Sprintf("\n\n---\n\nThis review contains **%d tokens** when processed by %s.\n", tokenCount, w.Ctx.Model))
		outputContent = sb.String()
	}

	// 6. Write the result to a file
	err = os.WriteFile(outputPath, []byte(outputContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write syntax review: %w", err)
	}

	logger.Debug("Syntax review saved")
	logger.Debug("Output path: %s", outputPath)
	return nil
}

// GenerateFunctionalityReview generates a review focusing on functionality against requirements
func (w *Workflow) GenerateFunctionalityReview() error {
	// 1. Generate the prompt
	prompt := w.GenerateFunctionalityReviewPrompt()

	// 2. Count tokens in the prompt
	tokenCount, err := w.Ctx.TokenCounter.CountText(prompt, w.Ctx.Model)
	if err != nil {
		logger.Debug("Warning: Could not count tokens in functionality review prompt: %v", err)
	} else {
		logger.Verbose("Functionality review prompt contains %d tokens", tokenCount)
		if tokenCount > w.Ctx.MaxTokens/2 {
			logger.Debug("Warning: Functionality review prompt is very large (%d tokens)", tokenCount)
		}
	}

	// 3. Send to LLM for review
	logger.Debug("Generating functionality review...")
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error generating functionality review: %w", err)
	}

	// 4. Create or append to the output file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-review-result.md", w.Ctx.Ticket))

	// Check if file exists
	var content []byte
	fileExists := false
	if _, err := os.Stat(outputPath); err == nil {
		// File exists, read it
		content, err = os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("error reading existing review file: %w", err)
		}
		fileExists = true
	}

	// Prepare content to write
	var sb strings.Builder
	if !fileExists {
		// Create new file with header
		sb.WriteString("# PR Review Results\n\n")
		sb.WriteString("This document contains a thorough review of the PR changes from multiple perspectives.\n\n")
	}

	// Append existing content if any
	if fileExists {
		sb.Write(content)
		// Add a separator
		sb.WriteString("\n\n---\n\n")
	}

	// Add the functionality review section
	sb.WriteString(response)

	// 5. Count tokens in the result
	outputContent := sb.String()
	tokenCount, err = w.Ctx.TokenCounter.CountText(outputContent, w.Ctx.Model)
	if err == nil && !fileExists {
		// Only add token count info if this is a new file
		sb.WriteString(fmt.Sprintf("\n\n---\n\nThis review contains **%d tokens** when processed by %s.\n", tokenCount, w.Ctx.Model))
		outputContent = sb.String()
	}

	// 6. Write the result to a file
	err = os.WriteFile(outputPath, []byte(outputContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write functionality review: %w", err)
	}

	logger.Debug("Functionality review saved")
	logger.Debug("Output path: %s", outputPath)
	return nil
}

// GenerateDefensiveReview generates a review focusing on defensive programming
func (w *Workflow) GenerateDefensiveReview() error {
	// 1. Generate the prompt
	prompt := w.GenerateDefensiveReviewPrompt()

	// 2. Count tokens in the prompt
	tokenCount, err := w.Ctx.TokenCounter.CountText(prompt, w.Ctx.Model)
	if err != nil {
		logger.Debug("Warning: Could not count tokens in defensive review prompt: %v", err)
	} else {
		logger.Verbose("Defensive review prompt contains %d tokens", tokenCount)
		if tokenCount > w.Ctx.MaxTokens/2 {
			logger.Debug("Warning: Defensive review prompt is very large (%d tokens)", tokenCount)
		}
	}

	// 3. Send to LLM for review
	logger.Debug("Generating defensive programming review...")
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error generating defensive programming review: %w", err)
	}

	// 4. Create or append to the output file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-review-result.md", w.Ctx.Ticket))

	// Check if file exists
	var content []byte
	fileExists := false
	if _, err := os.Stat(outputPath); err == nil {
		// File exists, read it
		content, err = os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("error reading existing review file: %w", err)
		}
		fileExists = true
	}

	// Prepare content to write
	var sb strings.Builder
	if !fileExists {
		// Create new file with header
		sb.WriteString("# PR Review Results\n\n")
		sb.WriteString("This document contains a thorough review of the PR changes from multiple perspectives.\n\n")
	}

	// Append existing content if any
	if fileExists {
		sb.Write(content)
		// Add a separator
		sb.WriteString("\n\n---\n\n")
	}

	// Add the defensive review section
	sb.WriteString(response)

	// 5. Count tokens in the result
	outputContent := sb.String()
	tokenCount, err = w.Ctx.TokenCounter.CountText(outputContent, w.Ctx.Model)
	if err == nil && !fileExists {
		// Only add token count info if this is a new file
		sb.WriteString(fmt.Sprintf("\n\n---\n\nThis review contains **%d tokens** when processed by %s.\n", tokenCount, w.Ctx.Model))
		outputContent = sb.String()
	}

	// 6. Write the result to a file
	err = os.WriteFile(outputPath, []byte(outputContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write defensive programming review: %w", err)
	}

	logger.Debug("Defensive programming review saved")
	logger.Debug("Output path: %s", outputPath)
	return nil
}

// GenerateFinalSummary generates a human-friendly summary of all reviews
func (w *Workflow) GenerateFinalSummary() error {
	// This will be implemented in Phase 3
	return nil
}

// Run executes the PR review workflow
func (w *Workflow) Run() error {
	// Set the total number of steps (we're skipping the token counting step)
	logger.SetTotalSteps(8)

	// Assemble PR context section
	// Add an extra blank line before the first section
	fmt.Println()
	logger.Section("ASSEMBLING PR CONTEXT")

	// Load design document if specified
	if w.Ctx.DesignDocPath != "" {
		logger.Info("%s Loading design document", logger.Arrow())
		err := w.LoadDesignDocument()
		if err != nil {
			return fmt.Errorf("error loading design document: %w", err)
		}
		// Success message is printed in LoadDesignDocument, so we don't need to print it here
	}

	// Fetch and format Jira ticket information if a ticket is specified
	if w.Ctx.Ticket != "" {
		logger.Info("%s Fetching Jira ticket", logger.Arrow())
		err := w.LoadTicketDetails()
		if err != nil {
			// If we can't load the ticket details, log the error and exit
			logger.Error("Failed to load Jira ticket details: %v", err)
			logger.Error("Please check your Jira credentials and try again.")
			os.Exit(1)
		}
		logger.Success("Jira ticket %s loaded successfully", w.Ctx.Ticket)
	}

	// We'll still count tokens internally, but not show it as a numbered step
	err := w.CountTokens()
	if err != nil {
		return fmt.Errorf("error counting tokens: %w", err)
	}

	// Step 1: Initial discovery
	logger.Step("Performing initial discovery")
	logger.StepDetail("Sending Initial Discovery prompt to OpenAI")
	err = w.RunLLMStep(
		"Initial Discovery",
		w.InitialDiscoveryPrompt,
		filepath.Join(w.Ctx.OutputDir, w.Ctx.Ticket+"-initial-discovery.md"),
	)
	if err != nil {
		return err
	}

	// Step 2: Collect original file contents
	logger.Step("Collecting original file contents")
	err = w.CollectOriginalFileContents()
	if err != nil {
		return fmt.Errorf("error collecting original file contents: %w", err)
	}
	logger.Success("Original file content collection completed")

	// Step 3: Analyze original implementation
	logger.Section("CODE ANALYSIS")
	logger.Step("Analyzing original implementation")
	// Get the number of files to analyze from the recommended file order
	orderedFiles, err := w.ParseRecommendedFileOrder()
	if err != nil {
		logger.Debug("Could not parse recommended file order: %v", err)
		logger.StepDetail("Starting file analysis using concurrent workers")
		// Add a blank line after the message
		fmt.Println()
	} else {
		logger.StepDetail("Starting analysis of %d files using up to %d concurrent workers", len(orderedFiles), 5)
		// Add a blank line after the message
		fmt.Println()
	}
	err = w.AnalyzeOriginalImplementation()
	if err != nil {
		return fmt.Errorf("error analyzing original implementation: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Original implementation analysis completed")

	// Step 4: Synthesize original implementation
	logger.Step("Synthesizing original implementation")
	logger.StepDetail("Synthesizing file analyses")
	err = w.SynthesizeOriginalImplementation()
	if err != nil {
		return fmt.Errorf("error synthesizing original implementation: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Original implementation synthesis completed")

	// Begin the PR review section
	logger.Section("PR REVIEW GENERATION")

	// Delete any existing review file to start fresh
	reviewFilePath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-review-result.md", w.Ctx.Ticket))
	if _, err := os.Stat(reviewFilePath); err == nil {
		logger.Debug("Removing existing review file: %s", reviewFilePath)
		err = os.Remove(reviewFilePath)
		if err != nil {
			logger.Debug("Warning: Could not remove existing review file: %v", err)
		}
	}

	// Step 5: Generate Syntax Review
	logger.Step("Generating syntax and best practices review")
	logger.StepDetail("Analyzing PHP syntax and best practices")
	err = w.GenerateSyntaxReview()
	if err != nil {
		return fmt.Errorf("error generating syntax review: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Syntax review completed")

	// Step 6: Generate Functionality Review
	logger.Step("Generating functionality review")
	logger.StepDetail("Analyzing functionality against requirements")
	err = w.GenerateFunctionalityReview()
	if err != nil {
		return fmt.Errorf("error generating functionality review: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Functionality review completed")

	// Step 7: Generate Defensive Programming Review
	logger.Step("Generating defensive programming review")
	logger.StepDetail("Analyzing defensive programming aspects")
	err = w.GenerateDefensiveReview()
	if err != nil {
		return fmt.Errorf("error generating defensive programming review: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Defensive programming review completed")

	// Step 8: Generate Final Summary
	logger.Step("Generating final review summary")
	logger.StepDetail("Creating human-friendly review summary")
	err = w.GenerateFinalSummary()
	if err != nil {
		return fmt.Errorf("error generating final summary: %w", err)
	}
	// Add a blank line before the success messages
	fmt.Println()
	logger.Success("Final review summary saved")
	logger.Success("PR review generation completed")

	// Complete the process with timing information
	logger.Complete()

	return nil
}
