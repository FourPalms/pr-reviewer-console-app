# Agent Runner Project Overview

## Project Summary
Agent Runner is a Golang console application that interacts with OpenAI's API to send prompts and receive responses. It allows users to interact with OpenAI models through both command-line and interactive modes.

## Project Structure
This project follows Ben Johnson's package layout approach, which organizes code by domain rather than by technical function:

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
└── README.md             # Project documentation
```

## Key Components

### Main Application (cmd/agent/main.go)
- Entry point for the application
- Supports command-line and interactive modes
- Handles user input and displays responses
- Configurable via command-line flags and environment variables

### Configuration (config/config.go)
- Loads configuration from environment variables
- Supports .env file for local development
- Manages OpenAI API key and model selection
- Default model: gpt-3.5-turbo (overridable via env or flag)

### OpenAI Client (openai/client.go)
- Implements communication with OpenAI API
- Uses the chat completions endpoint
- Handles request/response formatting
- Manages API authentication and error handling

## Current Configuration
- Using OpenAI model: gpt-4o
- API key is stored in .env file (not committed to version control)

## Dependencies
- github.com/joho/godotenv v1.5.1 - For loading environment variables from .env file
- Go version: 1.20

## Usage
- Command-line mode: `./bin/agent "Your prompt here"`
- Interactive mode: `./bin/agent` (type prompts at the prompt, 'exit' to quit)

## Build Instructions
```
go mod tidy
go build -o bin/agent ./cmd/agent
```

## Potential Improvements
- Add support for streaming responses
- Implement conversation history/context
- Add more configuration options
- Create a more sophisticated prompt template system
- Add support for function calling and other advanced OpenAI features
