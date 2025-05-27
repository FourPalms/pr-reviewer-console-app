package review

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadDesignDocument loads the design document if it exists
func (w *Workflow) LoadDesignDocument() error {
	if w.Ctx.DesignDocPath == "" {
		return nil // No design doc specified, nothing to do
	}

	// Construct the full path
	fullPath := filepath.Join(".context", "design", w.Ctx.DesignDocPath)

	// Check if the file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fmt.Printf("Warning: Design document %s not found at %s\n", w.Ctx.DesignDocPath, fullPath)
		return nil // Continue without the design doc
	}

	// Read the file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("error reading design document: %w", err)
	}

	// Store the content
	w.Ctx.DesignDocContent = string(content)
	fmt.Printf("Design document %s loaded successfully\n", w.Ctx.DesignDocPath)
	return nil
}
