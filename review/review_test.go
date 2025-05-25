package review

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNewReviewContext(t *testing.T) {
	// Skip test if we're just checking compilation
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
	
	// Create a new review context
	ctx := NewReviewContext("TEST-123", nil)
	
	// Check that the context was created correctly
	if ctx.Ticket != "TEST-123" {
		t.Errorf("Expected ticket to be TEST-123, got %s", ctx.Ticket)
	}
	
	if ctx.DiffPath != filepath.Join(".context", "reviews", "TEST-123-diff.md") {
		t.Errorf("Expected diff path to be .context/reviews/TEST-123-diff.md, got %s", ctx.DiffPath)
	}
	
	if ctx.FilesPath != filepath.Join(".context", "reviews", "TEST-123-files.md") {
		t.Errorf("Expected files path to be .context/reviews/TEST-123-files.md, got %s", ctx.FilesPath)
	}
	
	if ctx.MaxTokens != 120000 {
		t.Errorf("Expected max tokens to be 120000, got %d", ctx.MaxTokens)
	}
	
	if ctx.Model != "gpt-4o" {
		t.Errorf("Expected model to be gpt-4o, got %s", ctx.Model)
	}
}

func TestInitialDiscoveryPrompt(t *testing.T) {
	// Skip test if we're just checking compilation
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
	
	// Create a review context with test content
	ctx := &ReviewContext{
		DiffContent:  "Test diff content",
		FilesContent: "Test files content",
	}
	
	// Create a workflow with our context
	workflow := NewWorkflow(ctx)
	
	// Get the prompt
	prompt := workflow.InitialDiscoveryPrompt()
	
	// Check that the prompt contains our test content
	if prompt == "" {
		t.Error("InitialDiscoveryPrompt() returned an empty string")
	}
	
	// Check that the prompt contains our test content
	if !strings.Contains(prompt, "Test diff content") {
		t.Error("Prompt does not contain diff content")
	}
	
	if !strings.Contains(prompt, "Test files content") {
		t.Error("Prompt does not contain files content")
	}
}
