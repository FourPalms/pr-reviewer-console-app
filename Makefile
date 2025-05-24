.PHONY: build test clean run setup-hooks

# Default Go build flags
GOFLAGS := -v

# Binary output
BINARY_NAME := agent

# Build the application
build:
	go build $(GOFLAGS) -o bin/$(BINARY_NAME) ./cmd/agent

# Run all tests
test:
	go test ./... -v

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Run the application
# Usage: make run PROMPT="Your prompt here"
run:
	@if [ -z "$(PROMPT)" ]; then \
		go run ./cmd/agent; \
	else \
		go run ./cmd/agent "$(PROMPT)"; \
	fi

# Install dependencies
deps:
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	go vet ./...

# Setup Git hooks
setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed successfully!"

# Help target
help:
	@echo "Available targets:"
	@echo "  build       - Build the application"
	@echo "  test        - Run all tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  run         - Run the application"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Run linter"
	@echo "  setup-hooks - Configure Git to use project hooks"
	@echo "  help        - Show this help message"
