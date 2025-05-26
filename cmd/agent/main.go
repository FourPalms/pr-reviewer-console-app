package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/openai"
	"github.com/jeremyhunt/agent-runner/review"
)

func main() {
	// Define command-line flags
	modelFlag := flag.String("model", "", "OpenAI model to use (overrides env variable)")
	reviewFlag := flag.Bool("review", false, "Run PR review workflow")
	ticketFlag := flag.String("ticket", "", "Ticket number for PR review (e.g., WIRE-1231)")
	repoFlag := flag.String("repo", "", "Repository name for PR review (e.g., BambooHR/payroll-gateway)")
	branchFlag := flag.String("branch", "", "PR branch name for review (e.g., username/WIRE-1231)")

	// Parse flags
	flag.Parse()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Override model if specified via flag
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	// Create OpenAI client
	client := openai.NewClient(cfg.OpenAIAPIKey, cfg.Model)

	// Print which model we're using
	fmt.Printf("Using model: %s\n", cfg.Model)

	// Check if review mode is enabled
	if *reviewFlag {
		if *ticketFlag == "" {
			fmt.Fprintf(os.Stderr, "Error: ticket is required for review mode\n")
			flag.Usage()
			os.Exit(1)
		}

		handleReview(client, *ticketFlag, *repoFlag, *branchFlag)
		return
	}

	// Check if prompt was provided as command line argument
	if flag.NArg() > 0 {
		// Join all non-flag arguments as the prompt
		prompt := strings.Join(flag.Args(), " ")
		handlePrompt(client, prompt)
		return
	}

	// Interactive mode
	fmt.Println("Agent Runner v0.1.0")
	fmt.Println("Type your prompt and press Enter. Type 'exit' to quit.")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "exit" {
			break
		}

		handlePrompt(client, input)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
	}
}

func handlePrompt(client *openai.Client, prompt string) {
	// Count tokens in the prompt first
	tokenCount, err := client.CountText(prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error counting tokens: %v\n", err)
		return
	}

	fmt.Printf("Sending prompt to OpenAI... (%d tokens)\n", tokenCount)

	response, err := client.Complete(context.Background(), prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	fmt.Println("\nResponse:")
	fmt.Println(response)
}

// handleReview runs the PR review workflow
func handleReview(client *openai.Client, ticket string, repo string, branch string) {
	fmt.Printf("Starting PR review for ticket %s...\n", ticket)

	// Create review context
	ctx := review.NewReviewContext(ticket, client)

	// Set repository directory and branch if provided
	if repo != "" {
		// Extract repo name from full path (e.g., "BambooHR/payroll-gateway" -> "payroll-gateway")
		repoName := repo
		if idx := strings.LastIndex(repo, "/"); idx != -1 {
			repoName = repo[idx+1:]
		}
		ctx.RepoDir = filepath.Join(".context", "projects", repoName)
		fmt.Printf("Using repository at %s\n", ctx.RepoDir)
	}

	if branch != "" {
		ctx.Branch = branch
		fmt.Printf("Using PR branch %s\n", ctx.Branch)
	}

	// Create workflow
	workflow := review.NewWorkflow(ctx)

	// Run the workflow
	err := workflow.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running review workflow: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("PR review completed successfully.")
}
