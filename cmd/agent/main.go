package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jeremyhunt/agent-runner/config"
	"github.com/jeremyhunt/agent-runner/openai"
)

func main() {
	// Define command-line flags
	modelFlag := flag.String("model", "", "OpenAI model to use (overrides env variable)")
	
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
