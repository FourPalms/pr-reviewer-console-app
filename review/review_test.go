package review

import (
	"os"
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

func TestLoadDesignDocument(t *testing.T) {
	// Skip test if we're just checking compilation
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create a temporary directory for our test files
	tempDir := t.TempDir()
	contextDir := filepath.Join(tempDir, ".context")
	designDir := filepath.Join(contextDir, "design")
	err := os.MkdirAll(designDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a test design document
	testDocName := "test-design.md"
	testDocPath := filepath.Join(designDir, testDocName)
	testContent := "# Test Design\n\nThis is a test design document."
	err = os.WriteFile(testDocPath, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test cases
	tests := []struct {
		name            string
		designDocPath   string
		expectedError   bool
		expectedContent string
	}{
		{
			name:            "Existing design document",
			designDocPath:   testDocName,
			expectedError:   false,
			expectedContent: testContent,
		},
		{
			name:            "Non-existent design document",
			designDocPath:   "non-existent.md",
			expectedError:   false, // Should not error, just log warning
			expectedContent: "",
		},
		{
			name:            "Empty design document path",
			designDocPath:   "",
			expectedError:   false,
			expectedContent: "",
		},
	}

	// Save the original working directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	// Change to the temporary directory for the test
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Ensure we change back to the original directory when the test is done
	defer func() {
		err := os.Chdir(originalWd)
		if err != nil {
			t.Fatalf("Failed to restore working directory: %v", err)
		}
	}()

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a workflow with a context that has the design doc path set
			ctx := &ReviewContext{
				DesignDocPath: tc.designDocPath,
			}
			workflow := NewWorkflow(ctx)

			// Call the method
			err := workflow.LoadDesignDocument()

			// Check results
			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if ctx.DesignDocContent != tc.expectedContent {
				t.Errorf("Expected content %q, got %q", tc.expectedContent, ctx.DesignDocContent)
			}
		})
	}
}

func TestGenerateSyntaxReviewPrompt(t *testing.T) {
	// Skip test if we're just checking compilation
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Set up temporary directory and files
	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "reviews")
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a test synthesis file
	ticket := "TEST-123"
	synthesisPath := filepath.Join(outputDir, ticket+"-original-synthesis.md")
	synthesisContent := "Test synthesis content"
	err = os.WriteFile(synthesisPath, []byte(synthesisContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test cases
	tests := []struct {
		name             string
		designDocContent string
		expectDesignDoc  bool
		expectComparison bool
	}{
		{
			name:             "Without design document",
			designDocContent: "",
			expectDesignDoc:  false,
			expectComparison: false,
		},
		{
			name:             "With design document",
			designDocContent: "# Test Design\n\nThis is a test design document.",
			expectDesignDoc:  true,
			expectComparison: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a context with test content
			ctx := &ReviewContext{
				Ticket:           ticket,
				OutputDir:        outputDir,
				DiffContent:      "Test diff content",
				DesignDocContent: tc.designDocContent,
			}

			// Create a workflow with our context
			workflow := NewWorkflow(ctx)

			// Get the prompt
			prompt := workflow.GenerateSyntaxReviewPrompt()

			// Check that the prompt contains our test content
			if prompt == "" {
				t.Error("GenerateSyntaxReviewPrompt() returned an empty string")
			}

			// Check that the prompt contains our test content
			if !strings.Contains(prompt, "Test diff content") {
				t.Error("Prompt does not contain diff content")
			}

			if !strings.Contains(prompt, synthesisContent) {
				t.Error("Prompt does not contain synthesis content")
			}

			// Check for design document content
			hasDesignDoc := strings.Contains(prompt, "Design Document")
			if tc.expectDesignDoc && !hasDesignDoc {
				t.Error("Prompt does not contain design document section when it should")
			}
			if !tc.expectDesignDoc && hasDesignDoc {
				t.Error("Prompt contains design document section when it should not")
			}

			// Our new syntax review prompt doesn't have comparison instructions
			// Instead, check for PHP-specific syntax review content
			hasPHPSyntax := strings.Contains(prompt, "PHP Syntax and Best Practices Review")
			if !hasPHPSyntax {
				t.Error("Prompt does not contain PHP syntax review heading")
			}

			// If we expect design document content, check that it's actually there
			if tc.expectDesignDoc && !strings.Contains(prompt, tc.designDocContent) {
				t.Error("Prompt does not contain the actual design document content")
			}
		})
	}
}

func TestInitialDiscoveryPrompt(t *testing.T) {
	// Skip test if we're just checking compilation
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Test cases
	tests := []struct {
		name             string
		designDocContent string
		expectDesignDoc  bool
		expectAlignment  bool
	}{
		{
			name:             "Without design document",
			designDocContent: "",
			expectDesignDoc:  false,
			expectAlignment:  false,
		},
		{
			name:             "With design document",
			designDocContent: "# Test Design\n\nThis is a test design document.",
			expectDesignDoc:  true,
			expectAlignment:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a review context with test content
			ctx := &ReviewContext{
				DiffContent:      "Test diff content",
				FilesContent:     "Test files content",
				DesignDocContent: tc.designDocContent,
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

			// Check for design document content
			hasDesignDoc := strings.Contains(prompt, "Design Document")
			if tc.expectDesignDoc && !hasDesignDoc {
				t.Error("Prompt does not contain design document section when it should")
			}
			if !tc.expectDesignDoc && hasDesignDoc {
				t.Error("Prompt contains design document section when it should not")
			}

			// Check for design alignment section
			hasAlignment := strings.Contains(prompt, "Design Alignment")
			if tc.expectAlignment && !hasAlignment {
				t.Error("Prompt does not contain design alignment section when it should")
			}
			if !tc.expectAlignment && hasAlignment {
				t.Error("Prompt contains design alignment section when it should not")
			}

			// If we expect design document content, check that it's actually there
			if tc.expectDesignDoc && !strings.Contains(prompt, tc.designDocContent) {
				t.Error("Prompt does not contain the actual design document content")
			}
		})
	}
}
