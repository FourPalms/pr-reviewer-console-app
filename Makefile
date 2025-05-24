.PHONY: build test clean run setup-hooks check clone-repo pull-repo

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

# Check compilation without producing executable
check:
	go build -o /dev/null ./...

# Setup Git hooks
setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed successfully!"

# Clone a GitHub repository
# Usage: make clone-repo REPO=username/repo-name
clone-repo:
	@if [ -z "$(REPO)" ]; then \
		echo "Error: REPO parameter is required. Usage: make clone-repo REPO=username/repo-name"; \
		exit 1; \
	fi
	@REPO_NAME=$$(echo $(REPO) | sed 's/.*\///');\
	echo "Cloning $(REPO) into .context/projects/$$REPO_NAME (shallow clone)";\
	git clone --depth 1 git@github.com:$(REPO).git .context/projects/$$REPO_NAME;\
	echo "Clone completed successfully."

# Pull the main/master branch of a cloned repository
# Usage: make pull-repo REPO=username/repo-name
pull-repo:
	@if [ -z "$(REPO)" ]; then \
		echo "Error: REPO parameter is required. Usage: make pull-repo REPO=username/repo-name"; \
		exit 1; \
	fi
	@REPO_NAME=$$(echo $(REPO) | sed 's/.*\///');\
	if [ ! -d ".context/projects/$$REPO_NAME" ]; then \
		echo "Error: Repository $$REPO_NAME not found in .context/projects/. Clone it first using make clone-repo REPO=$(REPO)"; \
		exit 1; \
	fi;\
	echo "Pulling latest changes for $$REPO_NAME (shallow pull)...";\
	cd .context/projects/$$REPO_NAME && \
	git fetch --depth 1 && \
	git checkout main 2>/dev/null || git checkout master 2>/dev/null || echo "Neither main nor master branch found, staying on current branch" && \
	git pull --depth 1;\
	echo "Pull completed successfully."

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
	@echo "  check       - Compile without producing executable"
	@echo "  setup-hooks - Configure Git to use project hooks"
	@echo "  clone-repo  - Clone a GitHub repository (usage: make clone-repo REPO=username/repo-name)"
	@echo "  pull-repo   - Pull latest changes from main/master branch (usage: make pull-repo REPO=username/repo-name)"
	@echo "  help        - Show this help message"
