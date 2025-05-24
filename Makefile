.PHONY: build test clean run

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
run:
	go run ./cmd/agent

# Install dependencies
deps:
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	go vet ./...

# Help target
help:
	@echo "Available targets:"
	@echo "  build   - Build the application"
	@echo "  test    - Run all tests"
	@echo "  clean   - Clean build artifacts"
	@echo "  run     - Run the application"
	@echo "  deps    - Install dependencies"
	@echo "  fmt     - Format code"
	@echo "  lint    - Run linter"
	@echo "  help    - Show this help message"
