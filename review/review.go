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
	"time"

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

// GetCommonPromptIntro returns a standardized introduction for prompts
func (w *Workflow) GetCommonPromptIntro(role string) string {
	// Common beginning for all roles
	commonIntro := "You are a skeptical and methodical Sr Developer with expertise in PHP and general software development. "
	commonIntro += "You assume there are issues, misses, and mistakes unless proven otherwise. "

	// Role-specific additions
	switch role {
	case "reviewer":
		return commonIntro + "You're reviewing code for other senior developers who value helpfulness, brevity, and professionalism. "
	case "analyzer":
		return commonIntro + "You're analyzing a file from a codebase that uses a custom silo/service/domain/repository/applicationservice architecture.\n\n"
	case "discoverer":
		return commonIntro + "You're reviewing a pull request in a PHP codebase that uses a custom architecture with silo/service/domain/repository/applicationservice patterns."
	case "summarizer":
		return commonIntro + "You're creating a final summary of a PR review for GitHub. "
	default:
		return commonIntro
	}
}

// InitialDiscoveryPrompt generates the prompt for the initial discovery step
func (w *Workflow) InitialDiscoveryPrompt() string {
	// Base prompt template
	promptTemplate := `%s

Here is a list of the files that were changed:
%s

Here is the full diff of the changes:
%s%s%s

Please provide your analysis in the following format with EXACTLY these section headings:

## 1. Comprehensive Summary
[Your one-paragraph summary of the changes here]

## 2. Framework Detection
[Identify the framework(s) being used (e.g., Laravel, Symfony, CodeIgniter, custom) and provide specific evidence from the code that supports your identification]

## 3. Flow of Logic
[Your trace of the logic flow through files and functions here]

## 4. Recommended File Order
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
		designDocInstruction = "\n\n## 5. Design Alignment\n[Your assessment of how well the changes align with the design document]"
	}

	// Add ticket details if available
	ticketSection := ""
	ticketInstruction := ""
	if w.Ctx.TicketDetails != "" {
		ticketSection = fmt.Sprintf("\n\n## Jira Ticket\n\nThe following Jira ticket provides context for this PR:\n\n%s\n\nPlease consider this ticket information when analyzing the PR changes.", w.Ctx.TicketDetails)
		ticketInstruction = "\n\n## 6. Ticket Alignment\n[Your assessment of how well the changes address the requirements in the ticket]"
	}

	return fmt.Sprintf(promptTemplate, w.GetCommonPromptIntro("discoverer"), w.Ctx.FilesContent, w.Ctx.DiffContent, designDocSection, ticketSection, designDocInstruction, ticketInstruction)
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

// ParseRecommendedFileOrder gets the list of files to analyze
// It uses the actual list of changed files from the PR, not LLM recommendations
func (w *Workflow) ParseRecommendedFileOrder() ([]string, error) {
	// Get the complete list of changed files from the PR
	changedFiles, err := w.ParseChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to parse changed files: %w", err)
	}

	// Log the number of files found
	logger.Debug("Found %d changed files in the PR", len(changedFiles))

	// Return the complete list of changed files
	return changedFiles, nil
}

// ParseChangedFiles extracts the list of all changed files from the PR
func (w *Workflow) ParseChangedFiles() ([]string, error) {
	// Extract all changed files from the files content
	// The files content is in the format:
	// # Changed Files for tharris/check-bank-for-paygroup
	//
	// ## Modified Files
	// app/PayrollServices/Silo/Client/Domain/PayCycleDomain.php
	// ...
	// ## Added Files
	// ...
	// ## Deleted Files
	// ...

	// Read the files content
	filesContent := w.Ctx.FilesContent
	if filesContent == "" {
		return nil, fmt.Errorf("files content is empty")
	}

	// Split the content into sections
	sections := map[string][]string{
		"Modified": {},
		"Added":    {},
		"Deleted":  {},
	}

	// Parse the file content into sections
	scanner := bufio.NewScanner(strings.NewReader(filesContent))
	currentSection := ""
	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is a section header
		if strings.HasPrefix(line, "## ") {
			sectionName := strings.TrimPrefix(line, "## ")
			currentSection = sectionName
			continue
		}

		// Skip empty lines and lines that don't look like file paths
		if line == "" || !strings.Contains(line, ".") {
			continue
		}

		// Add the file to the appropriate section
		switch currentSection {
		case "Modified Files":
			sections["Modified"] = append(sections["Modified"], line)
		case "Added Files":
			sections["Added"] = append(sections["Added"], line)
		case "Deleted Files":
			sections["Deleted"] = append(sections["Deleted"], line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning files content: %w", err)
	}

	// For original implementation analysis, we only want modified and deleted files
	// since we need to analyze what they were like before the changes
	files := append(sections["Modified"], sections["Deleted"]...)

	// If we didn't find any files, try the old regex method as a fallback
	if len(files) == 0 {
		logger.Debug("No files found in sections, trying regex fallback")
		filePattern := "(?m)^([a-zA-Z0-9_\\-./]+\\.[a-zA-Z0-9]+)$"
		fileRegex := regexp.MustCompile(filePattern)
		matches := fileRegex.FindAllStringSubmatch(filesContent, -1)

		if len(matches) == 0 {
			return nil, fmt.Errorf("could not find any filenames in files content")
		}

		// Extract the filenames from the regex matches
		for _, match := range matches {
			if len(match) >= 2 {
				files = append(files, match[1])
			}
		}
	}

	logger.Debug("Found %d files for analysis (modified: %d, deleted: %d, added files excluded)",
		len(files), len(sections["Modified"]), len(sections["Deleted"]))

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
	prompt := w.GetCommonPromptIntro("analyzer")
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

	// 3. Process files individually with goroutines
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

	// Create an atomic counter for active goroutines
	var activeWorkers int32

	// We're already logging this in the Step function, so we don't need to log it here

	// Launch a goroutine for each file
	for i, file := range orderedFiles {
		// Increment the WaitGroup counter
		wg.Add(1)

		// Create a goroutine for this file
		go func(index int, filename string) {
			// Increment active workers counter
			atomic.AddInt32(&activeWorkers, 1)
			workerNum := atomic.LoadInt32(&activeWorkers)

			// Recover from any panics in this goroutine
			defer func() {
				if r := recover(); r != nil {
					panicErr := fmt.Sprintf("panic: %v", r)
					resultsMutex.Lock()
					logger.Debug("PANIC in goroutine processing %s: %v", filename, r)
					logger.AnalysisFailure(int(workerNum), filename, panicErr)
					resultsMutex.Unlock()

					// Store the error result
					results[index] = analysisResult{file: filename, err: fmt.Errorf(panicErr), index: index}
				}
			}()

			// Make sure we mark this file as done when the goroutine exits
			defer func() {
				// Only log completion if there was no error (errors are logged separately)
				if results[index].err == nil {
					resultsMutex.Lock()
					logger.AnalysisCompleted(int(workerNum), filename)
					resultsMutex.Unlock()
				}

				// Decrement active workers counter
				atomic.AddInt32(&activeWorkers, -1)

				wg.Done()
			}()

			// Print progress (protected by mutex to avoid garbled output)
			resultsMutex.Lock()
			logger.AnalysisItem(int(workerNum), filename)
			// Only print debug info in debug mode
			if logger.IsDebugEnabled() {
				logger.Debug("[Worker %d] Analyzing file %d/%d: %s (Active workers: %d)",
					workerNum, index+1, len(orderedFiles), filename, atomic.LoadInt32(&activeWorkers))
			}
			resultsMutex.Unlock()

			// Get original file content
			content, err := w.GetOriginalFileContent(filename)
			if err != nil {
				errMsg := fmt.Sprintf("could not get content: %v", err)
				resultsMutex.Lock()
				logger.Debug("Warning: could not get content for %s: %v", filename, err)
				logger.AnalysisFailure(int(workerNum), filename, errMsg)
				resultsMutex.Unlock()

				// Store the error result
				results[index] = analysisResult{file: filename, err: err, index: index}
				return
			}

			// Analyze with LLM
			analysis, err := w.AnalyzeFile(filename, content)
			if err != nil {
				errMsg := fmt.Sprintf("LLM analysis failed: %v", err)
				resultsMutex.Lock()
				logger.Debug("Warning: analysis failed for %s: %v", filename, err)
				logger.AnalysisFailure(int(workerNum), filename, errMsg)
				resultsMutex.Unlock()

				// Store the error result
				results[index] = analysisResult{file: filename, err: err, index: index}
				return
			}

			// Store the successful result
			results[index] = analysisResult{file: filename, analysis: analysis, index: index}
		}(i, file)

		// Wait a bit between launching goroutines to avoid API overload
		time.Sleep(250 * time.Millisecond)
	}

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
	sb.WriteString(w.GetCommonPromptIntro("reviewer"))
	sb.WriteString("Your goal is to identify syntax issues and best practice violations that could cause the team problems. ")
	sb.WriteString("Focus on substance over form, and avoid stating things that would be obvious to experienced developers.\n\n")

	// Review focus section
	sb.WriteString("## Review Focus\n\n")
	sb.WriteString("For this syntax review, focus on:\n\n")
	sb.WriteString("1. **Syntax Issues**: Identify errors that would cause runtime failures, typos in names, missing syntax elements, and namespace issues.\n\n")
	sb.WriteString("2. **Logic and Variable Usage**: Examine parameter usage, type handling, null/undefined access, conditional logic, and error handling patterns.\n\n")
	sb.WriteString("3. **Duplicate Implementations**: Check if the PR is implementing functionality that already exists elsewhere in the codebase. Look for similar class/method names or functionality across different namespaces.\n\n")
	sb.WriteString("4. **Namespace Conflicts**: Identify any duplicate class/interface names across different namespaces that could cause confusion or import conflicts.\n\n")
	sb.WriteString("5. **Review Limitations**: Explicitly state if you have sufficient context and what additional information would improve the review.\n\n")

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
	sb.WriteString("... several lines of prior context with line numbers...\n")
	sb.WriteString("SOLUTION_CODE:\n")
	sb.WriteString("```php\n")
	sb.WriteString("// Original\n")
	sb.WriteString("$original = $code->here();\n\n")
	sb.WriteString("// Fixed\n")
	sb.WriteString("$fixed = $code->here();\n")
	sb.WriteString("```\n")
	sb.WriteString("... more lines of prior context with line numbers if available...\n")
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
	sb.WriteString(w.GetCommonPromptIntro("reviewer"))
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

	sb.WriteString("For each issue, use this format:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("LINE: 42\n")
	sb.WriteString("SEVERITY: [Critical|Major|Minor]\n")
	sb.WriteString("PROBLEM: Brief description\n")
	sb.WriteString("... several lines of prior context with line numbers...\n")
	sb.WriteString("SOLUTION_CODE:\n")
	sb.WriteString("```php\n")
	sb.WriteString("// Original\n")
	sb.WriteString("$original = $code->here();\n\n")
	sb.WriteString("// Fixed\n")
	sb.WriteString("$fixed = $code->here();\n")
	sb.WriteString("```\n")
	sb.WriteString("... more lines of prior context with line numbers if available...\n")
	sb.WriteString("</ISSUE>\n")
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
	sb.WriteString(w.GetCommonPromptIntro("reviewer"))
	sb.WriteString("Your goal is to identify security issues, error handling gaps, and edge cases that could cause production problems. ")
	sb.WriteString("Focus on substance over form, and avoid stating things that would be obvious to experienced developers.\n\n")

	// Review focus section
	sb.WriteString("## Review Focus\n\n")
	sb.WriteString("CRITICAL: First thoroughly understand the existing code logic and its purpose before suggesting any changes.\n\n")
	sb.WriteString("For this defensive programming review, focus on:\n\n")
	sb.WriteString("1. **Understand before suggesting**: ALWAYS make sure you fully understand what the existing code is trying to accomplish before suggesting changes. This is especially important for conditional logic.\n\n")
	sb.WriteString("2. **Preserve functionality**: Never suggest changes that would remove intended functionality or edge case handling. If the original code handles a specific case, your suggestion must also handle it.\n\n")
	sb.WriteString("3. **Provide context**: For every suggestion, include the function/method signature and several lines of context before and after the change to help developers locate the code.\n\n")
	sb.WriteString("4. **Based on what we know of the original functionality (see context below), could this new functionality break anything?\n\n")
	sb.WriteString("5. **Deep dive into funcs**: Check to see whether the vars being passed in are actually what the func content expects\n\n")
	sb.WriteString("6. **Look for uncaught errors**: Identify areas that could expose uncaught errors/exceptions that would break or interrupt calling code\n\n")
	sb.WriteString("7. **Carefully analyze edge cases**: Consider edge cases within the context of what the code is trying to accomplish. Only suggest changes if they actually improve handling of edge cases.\n\n")
	sb.WriteString("8. **Pay close attention to conditional logic**: When suggesting changes to if/else statements or other conditionals, ensure you fully understand the business logic and don't remove intended functionality.\n\n")
	sb.WriteString("9. **Review Limitations**: Explicitly state if you have sufficient context and what additional information would improve the review.\n\n")

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
	sb.WriteString("... several lines of prior context with line numbers...\n")
	sb.WriteString("SOLUTION_CODE:\n")
	sb.WriteString("```php\n")
	sb.WriteString("// Original\n")
	sb.WriteString("$original = $code->here();\n\n")
	sb.WriteString("// Fixed\n")
	sb.WriteString("$fixed = $code->here();\n")
	sb.WriteString("```\n")
	sb.WriteString("... more lines of prior context with line numbers if available...\n")
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
	// Read the existing review result file
	reviewPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-review-result.md", w.Ctx.Ticket))
	reviewContent, err := os.ReadFile(reviewPath)
	if err != nil {
		logger.Debug("Warning: Could not read review file: %v", err)
		reviewContent = []byte("No review content available.")
	}

	// Read the validation result file
	validationPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-validation.md", w.Ctx.Ticket))
	validationContent, err := os.ReadFile(validationPath)
	if err != nil {
		logger.Debug("Warning: Could not read validation file: %v", err)
		validationContent = []byte("No validation content available.")
	}

	// Build the prompt using a string builder for better maintainability
	var sb strings.Builder

	// Overview section
	sb.WriteString("# PR Review Summary Generation\n\n")
	sb.WriteString(w.GetCommonPromptIntro("summarizer"))
	sb.WriteString("Your task is to synthesize the machine-generated review phases AND their validation into a concise, actionable, and professional summary. ")
	sb.WriteString("The team values clear communication, actionable feedback, and a focus on what matters most.\n\n")
	sb.WriteString("IMPORTANT: The validation results should take precedence over the original review when there are conflicts. ")
	sb.WriteString("Focus on confirmed issues, adjusted issues with their corrections, and any newly identified issues from the validation step.\n\n")
	sb.WriteString("CRITICAL INSTRUCTION: Your output MUST be in clean GitHub-flavored markdown format ONLY. ")
	sb.WriteString("DO NOT use any XML-style tags (like <SYNTAX_REVIEW> or <CRITICAL_ISSUES>) in your response. ")
	sb.WriteString("The output should be a professional markdown document that looks good when viewed on GitHub.\n\n")

	// Instructions section
	sb.WriteString("## HOW TO CREATE THE SUMMARY\n\n")
	sb.WriteString("IMPORTANT: Only report issues that are explicitly found in the review content. Do NOT invent or fabricate issues that aren't clearly mentioned in the input reviews. If you're uncertain about an issue, exclude it rather than risk reporting something inaccurate.\n\n")
	sb.WriteString("Your primary task is to faithfully summarize the existing reviews, not to perform a new review or analysis. Focus on accurately representing what the previous review phases found.\n\n")
	sb.WriteString("As a senior engineer, ask: what are the blocker issues? Issues that would block this from release until fixes are applied? These should only be issues explicitly identified in the review content.\n\n")
	sb.WriteString("Ask: what issues are 'Non Blockers', but still important to address? Again, only include issues that were actually identified in the reviews.\n\n")
	sb.WriteString("Ask: How can I best summarize the findings in a concise, actionable, and professional manner?\n\n")
	sb.WriteString("If no blocker issues were identified in any of the review phases, state \"No blocker issues were identified\" rather than attempting to elevate non-blocker issues or create new ones.\n\n")
	sb.WriteString("## Verification Process\n\n")
	sb.WriteString("Before finalizing your summary, follow this verification process:\n\n")
	sb.WriteString("1. For each issue you've identified, verify that it has clear evidence in the original review content.\n")
	sb.WriteString("2. If you can't find direct support for an issue in the reviews, remove it from your summary.\n")
	sb.WriteString("3. Double-check that you haven't misinterpreted or exaggerated any issues.\n")
	sb.WriteString("4. Ensure you've maintained the original severity assessment - don't escalate minor issues to blockers.\n\n")

	// Format section
	sb.WriteString("## Format Guidelines\n\n")
	sb.WriteString("Structure your summary with these sections:\n\n")
	sb.WriteString("1. **Overview**: A brief assessment of the PR quality and purpose (2-3 sentences).\n\n")
	sb.WriteString("2. **Positive Aspects**: Highlight what was done well (if applicable).\n\n")
	sb.WriteString("3. **Blocker Issues**: List the blocker issues that would prevent release. Format as follows:\n\n")
	sb.WriteString("   ### 1. [Issue Title]\n")
	sb.WriteString("   **Issue**: [Clear description of the problem]\n\n")
	sb.WriteString("   **WHY**: [Explanation of why this is important]\n\n")
	sb.WriteString("   **Suggested Fix**:\n")
	sb.WriteString("   ```diff\n")
	sb.WriteString("   // Show function/class signature and at least 3-5 lines of context before the change\n")
	sb.WriteString("   public function processPayment($clientId) {\n")
	sb.WriteString("       // Several lines of context...\n")
	sb.WriteString("   -   if ($condition) {\n")
	sb.WriteString("   +   if ($condition && $additionalCheck) {\n")
	sb.WriteString("       // More context lines...\n")
	sb.WriteString("   }\n")
	sb.WriteString("   ```\n\n")
	sb.WriteString("4. **Non-Blocker Issues**: List the non-blocker issues that are still important to address. Format as follows:\n\n")
	sb.WriteString("   ### 1. [Suggestion Title]\n")
	sb.WriteString("   **Suggestion**: [Description of the suggestion]\n\n")
	sb.WriteString("   **Benefit**: [Explanation of the benefit]\n\n")
	sb.WriteString("   **File**: /full/path/to/file.php\n")
	sb.WriteString("   **Line**: ~142 (approximate line number)\n\n")
	sb.WriteString("   **Example**:\n")
	sb.WriteString("   ```diff\n")
	sb.WriteString("   // Show function/class signature and at least 3-5 lines of context before the change\n")
	sb.WriteString("   public function processPayment($clientId) {\n")
	sb.WriteString("       // Several lines of context...\n")
	sb.WriteString("   -   if ($condition) {\n")
	sb.WriteString("   +   if ($condition && $additionalCheck) {\n")
	sb.WriteString("       // More context lines...\n")
	sb.WriteString("   }\n")
	sb.WriteString("   ```\n\n")
	sb.WriteString("5. For each issue, also include these fields in your internal analysis (but they don't need to appear in the final output):\n\n")
	sb.WriteString("   - Source: Which review phase identified it (Syntax, Functionality, or Defensive)\n")
	sb.WriteString("   - Confidence: High/Medium/Low based on how clearly it was identified\n")
	sb.WriteString("   - Current Logic: Explain what the current code is trying to accomplish\n")
	sb.WriteString("   - Functionality Check: Confirm that the suggested change preserves all existing functionality\n\n")
	sb.WriteString("   Example format:\n\n")
	sb.WriteString("   **Issue: [Brief Title]**\n")
	sb.WriteString("   - **Source**: Syntax Review\n")
	sb.WriteString("   - **Confidence**: High - Brief explanation of confidence level\n")
	sb.WriteString("   - **Description**: [Detailed explanation of why this is a problem and what edge cases it addresses]\n")
	sb.WriteString("   - **Current Logic**: [Explain what the current code is trying to accomplish]\n")
	sb.WriteString("   - **Functionality Check**: [Confirm that the suggested change preserves all existing functionality]\n")
	sb.WriteString("   - **File**: app/PayrollServices/Silo/Client/Domain/PayCycleDomain.php\n")
	sb.WriteString("   - **Line**: ~142 (approximate line number)\n")
	sb.WriteString("   - **Code**:\n")
	sb.WriteString("   ```diff\n")
	sb.WriteString("   /**\n")
	sb.WriteString("    * Process a payment for the given client ID\n")
	sb.WriteString("    * @param int $clientId The client ID\n")
	sb.WriteString("    * @return PaymentStatus\n")
	sb.WriteString("    */\n")
	sb.WriteString("   public function processPayment($clientId) {\n")
	sb.WriteString("       $client = $this->getClient($clientId);\n")
	sb.WriteString("       $account = $client->getAccount();\n")
	sb.WriteString("       \n")
	sb.WriteString("       // Check if client has sufficient balance\n")
	sb.WriteString("   -   $balance = $this->getBalance($client);\n")
	sb.WriteString("   -   if ($balance > 0) {\n")
	sb.WriteString("   +   $balance = $this->getBalanceWithRetry($client);\n")
	sb.WriteString("   +   if ($balance !== null && $balance > 0) {\n")
	sb.WriteString("           $this->processPaymentWithBalance($client, $balance);\n")
	sb.WriteString("       } else {\n")
	sb.WriteString("           $this->logInsufficientFunds($client);\n")
	sb.WriteString("       }\n")
	sb.WriteString("       \n")
	sb.WriteString("       return $this->getPaymentStatus($client);\n")
	sb.WriteString("   }\n")
	sb.WriteString("   ```\n\n")
	sb.WriteString("   IMPORTANT: Use pure diff format. Do NOT include comments like \"// Original\" or \"// Fixed\". Just show the actual code changes with - and + prefixes. Always include enough surrounding code to help developers locate the right spot.\n")

	sb.WriteString("## IMPORTANT FORMATTING RULES\n\n")
	sb.WriteString("1. Use ONLY GitHub-flavored markdown - NO XML tags or custom formats.\n")
	sb.WriteString("2. DO NOT use any XML-style tags like <SYNTAX_REVIEW>, <CRITICAL_ISSUES>, etc.\n")
	sb.WriteString("3. DO NOT include any XML or HTML formatting in your response.\n")
	sb.WriteString("4. Format your response as clean, professional GitHub-flavored markdown ONLY.\n\n")

	sb.WriteString("Use these markdown formatting elements:\n\n")
	sb.WriteString("- Use `##` and `###` for section headers\n")
	sb.WriteString("- Use bullet points (`*`) for lists\n")
	sb.WriteString("- Use code blocks with syntax highlighting for code examples\n")
	sb.WriteString("- Use bold and italic for emphasis\n")
	sb.WriteString("- Use tables for structured information if helpful\n\n")

	sb.WriteString("REMEMBER: Your output should be a clean, professional markdown document that looks good on GitHub. NO XML TAGS.\n\n")

	// Context section
	sb.WriteString("## Context\n\n")

	// Ticket details if available
	if w.Ctx.TicketDetails != "" {
		sb.WriteString("### Jira Ticket\n\n")
		sb.WriteString(w.Ctx.TicketDetails)
		sb.WriteString("\n\n")
	}

	// Design document if available
	if w.Ctx.DesignDocContent != "" {
		sb.WriteString("### Design Document\n\n")
		sb.WriteString("A design document was provided for this PR.\n\n")
	}

	// Review content
	sb.WriteString("### Original Review Content\n\n")
	sb.WriteString("The following is the machine-generated review content:\n\n")
	sb.WriteString(string(reviewContent))

	// Validation content
	sb.WriteString("\n\n### Validation Results\n\n")
	sb.WriteString("The following is the validation of the review findings, which you should prioritize over the original review when there are conflicts:\n\n")
	sb.WriteString(string(validationContent))

	return sb.String()
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

// ValidateReviewFindings challenges assumptions and validates issues from previous review phases
func (w *Workflow) ValidateReviewFindings() error {
	// 1. Read the existing review result file
	reviewPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-review-result.md", w.Ctx.Ticket))
	reviewContent, err := os.ReadFile(reviewPath)
	if err != nil {
		return fmt.Errorf("error reading review file for validation: %w", err)
	}

	// 2. Read the diff file
	diffPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-diff.md", w.Ctx.Ticket))
	diffContent, err := os.ReadFile(diffPath)
	if err != nil {
		logger.Debug("Warning: Could not read diff file for validation: %v", err)
		diffContent = []byte("No diff content available.")
	}

	// 3. Generate the validation prompt
	prompt := w.GenerateValidationPrompt(string(reviewContent), string(diffContent))

	// 4. Count tokens in the prompt
	tokenCount, err := w.Ctx.TokenCounter.CountText(prompt, w.Ctx.Model)
	if err != nil {
		logger.Debug("Warning: Could not count tokens in validation prompt: %v", err)
	} else {
		logger.Verbose("Validation prompt contains %d tokens", tokenCount)
		if tokenCount > w.Ctx.MaxTokens/2 {
			logger.Debug("Warning: Validation prompt is very large (%d tokens)", tokenCount)
		}
	}

	// 5. Send to LLM for validation
	logger.Debug("Generating review validation...")
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error generating review validation: %w", err)
	}

	// 6. Write the validation result to a file
	validationPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-validation.md", w.Ctx.Ticket))
	err = os.WriteFile(validationPath, []byte(response), 0644)
	if err != nil {
		return fmt.Errorf("failed to write review validation: %w", err)
	}

	logger.Debug("Review validation saved")
	logger.Debug("Validation path: %s", validationPath)
	return nil
}

// GenerateValidationPrompt creates a prompt for the validation step
func (w *Workflow) GenerateValidationPrompt(reviewContent, diffContent string) string {
	// Build the prompt using a string builder for better maintainability
	var sb strings.Builder

	// Overview section
	sb.WriteString("# PR Review Validation\n\n")
	sb.WriteString("You are a skeptical and methodical Sr Developer with expertise in PHP and general software development. ")
	sb.WriteString("You assume there are issues, misses, and mistakes unless proven otherwise. ")
	sb.WriteString("Your task is to critically evaluate the machine-generated review of a PR and validate or challenge its findings.\n\n")

	// Ticket details if available - show this first for proper context
	if w.Ctx.TicketDetails != "" {
		sb.WriteString("## Ticket Context\n\n")
		sb.WriteString("You are reviewing a PR for the following ticket:\n\n")
		sb.WriteString(w.Ctx.TicketDetails)
		sb.WriteString("\n\nKeep this context in mind when evaluating the review findings.\n\n")
	}

	// Instructions section
	sb.WriteString("## YOUR TASK\n\n")
	sb.WriteString("You are given a machine-generated review of a PR. Your job is to act as a second opinion, challenging assumptions and validating or refuting the issues identified.\n\n")
	sb.WriteString("For each issue identified in the review:\n\n")
	sb.WriteString("1. **Challenge the evidence**: Is there sufficient evidence in the diff to support this claim?\n")
	sb.WriteString("2. **Question the severity**: Is the severity rating appropriate given the actual impact?\n")
	sb.WriteString("3. **Verify the solution**: Would the proposed solution actually fix the issue without introducing new problems?\n")
	sb.WriteString("4. **Check for false positives**: Is this actually an issue or just a misunderstanding of the code?\n\n")

	sb.WriteString("## APPROACH\n\n")
	sb.WriteString("1. First, understand the overall PR by examining the diff.\n")
	sb.WriteString("2. Then, examine each issue identified in the review.\n")
	sb.WriteString("3. For each issue, decide if you:\n")
	sb.WriteString("   - **Confirm**: The issue is real and correctly assessed\n")
	sb.WriteString("   - **Adjust**: The issue is real but needs adjustment (e.g., severity, description)\n")
	sb.WriteString("   - **Reject**: The issue is not valid or is a false positive\n\n")

	sb.WriteString("## OUTPUT FORMAT\n\n")
	sb.WriteString("Structure your validation as follows:\n\n")
	sb.WriteString("```xml\n")
	sb.WriteString("<VALIDATION_SUMMARY>\n")
	sb.WriteString("A brief overview of your findings. How many issues did you confirm, adjust, or reject?\n")
	sb.WriteString("</VALIDATION_SUMMARY>\n\n")

	sb.WriteString("<CONFIRMED_ISSUES>\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("ORIGINAL_SEVERITY: Critical/Major/Minor\n")
	sb.WriteString("CONFIRMED_SEVERITY: Critical/Major/Minor\n")
	sb.WriteString("PROBLEM: Brief description of the issue\n")
	sb.WriteString("EVIDENCE: Specific evidence from the diff that confirms this issue\n")
	sb.WriteString("SOLUTION_ASSESSMENT: Is the proposed solution appropriate? Why?\n")
	sb.WriteString("</ISSUE>\n")
	sb.WriteString("</CONFIRMED_ISSUES>\n\n")

	sb.WriteString("<ADJUSTED_ISSUES>\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("ORIGINAL_SEVERITY: Critical/Major/Minor\n")
	sb.WriteString("ADJUSTED_SEVERITY: Critical/Major/Minor\n")
	sb.WriteString("ORIGINAL_PROBLEM: Brief description of the original issue\n")
	sb.WriteString("ADJUSTED_PROBLEM: Your adjusted description\n")
	sb.WriteString("ADJUSTMENT_REASON: Why did you adjust this issue?\n")
	sb.WriteString("EVIDENCE: Specific evidence from the diff\n")
	sb.WriteString("SOLUTION_ASSESSMENT: Is the proposed solution appropriate? If not, what would be better?\n")
	sb.WriteString("</ISSUE>\n")
	sb.WriteString("</ADJUSTED_ISSUES>\n\n")

	sb.WriteString("<REJECTED_ISSUES>\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("ORIGINAL_SEVERITY: Critical/Major/Minor\n")
	sb.WriteString("ORIGINAL_PROBLEM: Brief description of the original issue\n")
	sb.WriteString("REJECTION_REASON: Why is this not a valid issue?\n")
	sb.WriteString("EVIDENCE: Specific evidence from the diff that contradicts this issue\n")
	sb.WriteString("</ISSUE>\n")
	sb.WriteString("</REJECTED_ISSUES>\n\n")

	sb.WriteString("<MISSED_ISSUES>\n")
	sb.WriteString("<ISSUE>\n")
	sb.WriteString("FILE: path/to/file.php\n")
	sb.WriteString("SEVERITY: Critical/Major/Minor\n")
	sb.WriteString("PROBLEM: Description of an issue that was missed in the original review\n")
	sb.WriteString("EVIDENCE: Specific evidence from the diff\n")
	sb.WriteString("SUGGESTED_SOLUTION: How to fix this issue\n")
	sb.WriteString("</ISSUE>\n")
	sb.WriteString("</MISSED_ISSUES>\n")
	sb.WriteString("```\n\n")

	// Context section
	sb.WriteString("## CONTEXT\n\n")

	// Diff content
	sb.WriteString("### Diff Content\n\n")
	sb.WriteString("```diff\n")
	sb.WriteString(diffContent)
	sb.WriteString("\n```\n\n")

	// Review content
	sb.WriteString("### Review Content to Validate\n\n")
	sb.WriteString(reviewContent)

	return sb.String()
}

// GenerateFinalSummary generates the final PR review summary
func (w *Workflow) GenerateFinalSummary() error {
	// 1. Generate the prompt
	prompt := w.GenerateFinalSummaryPrompt()

	// 2. Count tokens in the prompt
	tokenCount, err := w.Ctx.TokenCounter.CountText(prompt, w.Ctx.Model)
	if err != nil {
		logger.Debug("Warning: Could not count tokens in final summary prompt: %v", err)
	} else {
		logger.Verbose("Final summary prompt contains %d tokens", tokenCount)
		if tokenCount > w.Ctx.MaxTokens/2 {
			logger.Debug("Warning: Final summary prompt is very large (%d tokens)", tokenCount)
		}
	}

	// 3. Send to LLM for summary generation
	logger.Debug("Generating final summary...")
	response, err := w.Ctx.Client.Complete(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("error generating final summary: %w", err)
	}

	// 4. Create the output file
	outputPath := filepath.Join(w.Ctx.OutputDir, fmt.Sprintf("%s-final-summary.md", w.Ctx.Ticket))

	// 5. Write the result to a file
	err = os.WriteFile(outputPath, []byte(response), 0644)
	if err != nil {
		return fmt.Errorf("failed to write final summary: %w", err)
	}

	logger.Debug("Final summary saved")
	logger.Debug("Output path: %s", outputPath)
	return nil
}

// Run executes the PR review workflow
func (w *Workflow) Run() error {
	// Set the total number of steps (we're skipping the token counting step)
	logger.SetTotalSteps(9)

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
	logger.Section("PREVIOUS IMPLEMENTATION ANALYSIS")
	logger.Step("Analyzing original implementation")
	// Get the number of files to analyze from the recommended file order
	orderedFiles, err := w.ParseRecommendedFileOrder()
	if err != nil {
		logger.Debug("Could not parse recommended file order: %v", err)
		logger.StepDetail("Starting file analysis using concurrent workers")
		// Add a blank line after the message
		fmt.Println()
	} else {
		logger.StepDetail("Starting analysis of %d files using individual goroutines", len(orderedFiles))
		// Add a blank line after the message
		fmt.Println()
	}
	err = w.AnalyzeOriginalImplementation()
	if err != nil {
		return fmt.Errorf("error analyzing original implementation: %w", err)
	}
	// Add a blank line before the success message
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

	// Step 8: Validate Review Findings
	logger.Step("Validating review findings")
	logger.StepDetail("Challenging assumptions and validating issues")
	err = w.ValidateReviewFindings()
	if err != nil {
		return fmt.Errorf("error validating review findings: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Review validation completed")

	// Step 9: Generate Final Summary
	logger.Step("Generating final review summary")
	logger.StepDetail("Creating human-friendly review summary")
	err = w.GenerateFinalSummary()
	if err != nil {
		return fmt.Errorf("error generating final summary: %w", err)
	}
	// Add a blank line before the success message
	fmt.Println()
	logger.Success("Final review summary saved")
	logger.Success("PR review generation completed")

	// Complete the process with timing information
	logger.Complete()

	return nil
}
