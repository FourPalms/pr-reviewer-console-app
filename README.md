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
   go mod tidy
   ```
4. Build the application:
   ```
   go build -o bin/agent ./cmd/agent
   ```
5. Run the application:
   ```
   ./bin/agent "Your prompt here"
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

## Dependencies

- github.com/joho/godotenv - For loading environment variables from .env file
