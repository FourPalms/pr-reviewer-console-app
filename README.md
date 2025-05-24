# Agent Runner

A Golang console application that interacts with OpenAI's API to send prompts and receive responses.

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
├── .env                  # Environment variables (not committed to version control)
├── .gitignore            # Git ignore file
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
4. Build the application:
   ```
   make build
   ```
   or
   ```
   go build -o bin/agent ./cmd/agent
   ```
5. Run the application:
   ```
   ./bin/agent "Your prompt here"
   ```
   or
   ```
   make run
   ```

## Usage

### Command-line mode

```
./bin/agent "What is the capital of France?"
```

### Interactive mode

```
./bin/agent
```

Then type your prompts at the prompt and press Enter. Type `exit` to quit.

## Makefile

The project includes a Makefile with several useful targets:

```
make build    # Build the application
make test     # Run all tests
make clean    # Clean build artifacts
make run      # Run the application
make deps     # Install dependencies
make fmt      # Format code
make lint     # Run linter
make help     # Show help message
```

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
