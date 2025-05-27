# Agent Runner

A Golang console application that leverages OpenAI's API for automated Pull Request review and code analysis.

## Project Structure

This project follows Ben Johnson's domain-oriented design approach:

```
agent-runner/
├── cmd/
│   └── agent/            # Command-line application
│       └── main.go       # Entry point
├── config/               # Application configuration
│   └── config.go
├── openai/               # OpenAI domain package
│   └── client.go         # OpenAI client implementation
├── review/               # PR review functionality
│   ├── review.go         # Core review logic
│   └── review_test.go    # Tests for review package
├── tokens/               # Token counting utilities
│   ├── counter.go        # Token counter implementation
│   └── counter_test.go   # Tests for token counter
├── .context/             # Context directory for reviews and projects
├── .env                  # Environment variables (not committed to version control)
├── .gitignore            # Git ignore file
├── .githooks/            # Git hooks for quality control
├── go.mod                # Go module definition
└── README.md             # This file
```

## Getting Started

1. Make sure you have Go installed (1.16+)
2. Set up your OpenAI API key in the `.env` file:
   ```
   OPENAI_API_KEY=your_api_key_here
   ```
3. Install dependencies:
   ```
   make deps
   ```
   or
   ```
   go mod tidy
   ```
4. Create the necessary directories for PR reviews:
   ```
   mkdir -p .context/projects .context/reviews
   ```
5. Run your first PR review:
   ```
   make review REPO=BambooHR/repo-name PR-BRANCH=username/TICKET-NUMBER
   ```

## Usage

### PR Review

The agent-runner is designed to analyze pull requests and provide detailed code reviews using LLMs. It can be run in two ways:

#### Using the Go command directly:

```
go run ./cmd/agent --review --ticket=TICKET-NUMBER --repo=BambooHR/repo-name [--branch=username/TICKET-NUMBER]
```

#### Using the Makefile (recommended):

```
make review REPO=BambooHR/repo-name PR-BRANCH=username/TICKET-NUMBER
```

### Review Process

When you run the review command, the tool will:

1. Clone the repository if it doesn't exist locally
2. Generate a diff between master and the PR branch
3. List all changed files (added, modified, deleted)
4. Analyze the original implementation of affected code
5. Synthesize a comprehensive understanding of the code before changes

### Review Artifacts

All review artifacts are saved in the `.context/reviews/` directory with the ticket number as prefix:

- `TICKET-diff.md`: The complete diff between master and PR branch
- `TICKET-files.md`: List of all changed files with statistics
- `TICKET-initial-discovery.md`: Initial analysis of the changes
- `TICKET-original-synthesis.md`: Synthesized understanding of the original implementation
- `TICKET-review-result.md`: Comprehensive PR review with blockers and non-blockers in GitHub-compatible markdown

These artifacts provide a comprehensive analysis that helps reviewers understand both the original code and the proposed changes.

## Makefile

The project includes a Makefile with several useful targets, primarily focused on PR review functionality:

### PR Review Targets

```
make review       # Main command: Set up repo and run full PR review
                  # Usage: make review REPO=BambooHR/repo-name PR-BRANCH=username/TICKET-NUMBER

make run-review   # Run review on an already prepared repository
                  # Usage: make run-review TICKET=TICKET-NUMBER

make clone-repo   # Clone a GitHub repository for review
                  # Usage: make clone-repo REPO=BambooHR/repo-name

make pull-repo    # Pull latest changes from main/master branch
                  # Usage: make pull-repo REPO=BambooHR/repo-name

make diff-pr      # Generate a diff between master and PR branch
                  # Usage: make diff-pr REPO=BambooHR/repo-name PR-BRANCH=username/TICKET-NUMBER

make list-changes # List changed files in a PR
                  # Usage: make list-changes REPO=BambooHR/repo-name PR-BRANCH=username/TICKET-NUMBER
```

### Development Targets

```
make build        # Build the application
make test         # Run all tests
make clean        # Clean build artifacts
make deps         # Install dependencies
make fmt          # Format code
make lint         # Run linter
make check        # Compile without producing executable
make setup-hooks  # Configure Git to use project hooks
make help         # Show help message
```

## Git Hooks

This project includes Git hooks to ensure code quality. The pre-commit hook runs formatting, linting, and tests on all Go files in the project before allowing commits, ensuring the entire codebase maintains quality standards.

To set up the Git hooks, run:

```
make setup-hooks
```

This configures Git to use the hooks in the `.githooks` directory.

## Testing

Run the test suite using the Makefile:

```
make test
```

Or run tests directly with Go:

```
go test ./...
```

## Dependencies

- github.com/joho/godotenv - For loading environment variables from .env file
- github.com/pkoukk/tiktoken-go - For token counting
- github.com/sashabaranov/go-openai - For OpenAI API types
