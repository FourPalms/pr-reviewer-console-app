package review

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeremyhunt/agent-runner/logger"
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
		logger.Debug("Warning: Design document %s not found at %s", w.Ctx.DesignDocPath, fullPath)
		return nil // Continue without the design doc
	}

	// Read the file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("error reading design document: %w", err)
	}

	// Store the content
	w.Ctx.DesignDocContent = string(content)
	logger.Success("Design document %s loaded successfully", w.Ctx.DesignDocPath)
	return nil
}
