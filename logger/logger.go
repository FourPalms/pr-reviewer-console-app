// Package logger provides a simple logging system with verbosity levels
package logger

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// VerbosityLevel defines how verbose the logging should be
type VerbosityLevel int

const (
	// VerbosityQuiet shows only essential output
	VerbosityQuiet VerbosityLevel = iota
	// VerbosityNormal shows important information (default)
	VerbosityNormal
	// VerbosityVerbose shows detailed information
	VerbosityVerbose
	// VerbosityDebug shows all information including debug data
	VerbosityDebug
)

// Symbols for log messages
const (
	checkmark = "✓"
	arrow     = "→"
	dash      = "-"
)

// Arrow returns the arrow symbol for use in logging
func Arrow() string {
	return arrow
}

var verbosity VerbosityLevel = VerbosityNormal
var startTime time.Time
var currentSection string
var totalSteps int
var currentStep int

// Initialize sets the verbosity level and records the start time
func Initialize(level VerbosityLevel) {
	verbosity = level
	startTime = time.Now()
	currentSection = ""
	totalSteps = 0
	currentStep = 0

	// Print a nice header at the start if not in quiet mode
	if verbosity >= VerbosityNormal {
		fmt.Println()
		fmt.Println("AUTOMATED AGENTIC PR REVIEW TOOL")
		fmt.Println(strings.Repeat("-", 30))
		fmt.Println()
	}
}

// SetTotalSteps sets the total number of steps in the process
func SetTotalSteps(steps int) {
	totalSteps = steps
}

// Info prints information at normal verbosity and above
func Info(format string, args ...interface{}) {
	if verbosity >= VerbosityNormal {
		fmt.Printf(format+"\n", args...)
	}
}

// Section starts a new logical section in the output
func Section(name string) {
	if verbosity >= VerbosityNormal {
		// Add spacing before new sections (but not for the first section)
		if currentSection != "" {
			fmt.Println()
		}

		// Print section header
		fmt.Printf("%s:\n", strings.ToUpper(name))
	}

	currentSection = name
}

// Verbose prints information at verbose level and above
func Verbose(format string, args ...interface{}) {
	if verbosity >= VerbosityVerbose {
		fmt.Printf(format+"\n", args...)
	}
}

// Debug prints information at debug level only
func Debug(format string, args ...interface{}) {
	if verbosity >= VerbosityDebug {
		fmt.Printf("DEBUG: "+format+"\n", args...)
	}
}

// IsDebugEnabled returns true if debug logging is enabled
func IsDebugEnabled() bool {
	return verbosity >= VerbosityDebug
}

// Error prints error information at all verbosity levels
func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
}

// Step prints a step message with step number
func Step(stepName string) {
	if verbosity >= VerbosityNormal {
		currentStep++
		// Add spacing before each step
		fmt.Println()
		// Print the step information with step number
		fmt.Printf("%s Step %d/%d: %s\n", arrow, currentStep, totalSteps, stepName)
	}
}

// StepDetail prints a detail message for the current step
func StepDetail(format string, args ...interface{}) {
	if verbosity >= VerbosityNormal {
		// Print the message with indentation
		fmt.Printf("  %s\n", fmt.Sprintf(format, args...))
	}
}

// AnalysisItem prints an analysis item with a worker number
func AnalysisItem(workerNum int, filename string) {
	if verbosity >= VerbosityNormal {
		fmt.Printf("  [%d] Analyzing: %s\n", workerNum, filename)
	}
}

// AnalysisCompleted prints a message when a file analysis is completed
func AnalysisCompleted(workerNum int, filename string) {
	if verbosity >= VerbosityNormal {
		fmt.Printf("  [%d] %s Completed: %s\n", workerNum, checkmark, filename)
	}
}

// AnalysisFailure prints a message when a file analysis fails
func AnalysisFailure(workerNum int, filename string, reason string) {
	if verbosity >= VerbosityNormal {
		fmt.Printf("  [%d] ✗ Failure: %s - %s\n", workerNum, filename, reason)
	}
}

// Success prints a success message
func Success(format string, args ...interface{}) {
	if verbosity >= VerbosityNormal {
		fmt.Printf("%s %s\n", checkmark, fmt.Sprintf(format, args...))
	}
}

// Complete prints a completion message for the entire process
func Complete() {
	if verbosity >= VerbosityNormal {
		elapsed := time.Since(startTime)

		// Print a separator and completion message
		fmt.Println()
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println("REVIEW COMPLETED SUCCESSFULLY")
		fmt.Printf("Total time: %s\n", formatDuration(elapsed))
		fmt.Println()
	}

	// Reset step counter for next run
	currentStep = 0
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	// Round to seconds for cleaner output
	seconds := int(d.Seconds())
	minutes := seconds / 60
	seconds %= 60

	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
