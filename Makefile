.PHONY: build test clean run run-review setup-hooks check clone-repo pull-repo review diff-pr list-changes status revive

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

# Run the application in review mode
# Usage: make run-review TICKET=WIRE-1231 REPO=BambooHR/repo-name BRANCH=username/WIRE-1231
run-review:
	@if [ -z "$(TICKET)" ]; then \
		echo "Error: TICKET parameter is required. Usage: make run-review TICKET=WIRE-1231 REPO=BambooHR/repo-name BRANCH=username/WIRE-1231"; \
		exit 1; \
	fi
	@if [ -z "$(REPO)" ]; then \
		echo "Error: REPO parameter is required. Usage: make run-review TICKET=WIRE-1231 REPO=BambooHR/repo-name BRANCH=username/WIRE-1231"; \
		exit 1; \
	fi
	@if [ -z "$(BRANCH)" ]; then \
		echo "Error: BRANCH parameter is required. Usage: make run-review TICKET=WIRE-1231 REPO=BambooHR/repo-name BRANCH=username/WIRE-1231"; \
		exit 1; \
	fi
	@go run ./cmd/agent --review --ticket=$(TICKET) --repo=$(REPO) --branch=$(BRANCH)

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

# Set up a repository for review
# Usage: make review REPO=username/repo-name PR-BRANCH=branch-name TICKET=WIRE-1231
review:
	@if [ -z "$(TICKET)" ]; then \
		echo "Error: TICKET parameter is required. Usage: make review REPO=username/repo-name PR-BRANCH=branch-name TICKET=WIRE-1231"; \
		exit 1; \
	fi
	@REPO_NAME=`echo $(REPO) | sed 's/.*\///'`; \
	if [ ! -d ".context/projects/$$REPO_NAME" ]; then \
		echo "Repository $$REPO_NAME not found, cloning first...";\
		$(MAKE) clone-repo REPO=$(REPO);\
	fi; \
	cd .context/projects/$$REPO_NAME && \
	(git checkout master 2>/dev/null || git checkout main 2>/dev/null || (echo "Error: Neither master nor main branch found" && exit 1)) && \
	git pull --rebase --depth 1 && \
	echo "Setting up PR branch $(PR-BRANCH)..." && \
	git fetch origin $(PR-BRANCH) --depth 100 && git checkout FETCH_HEAD -B review-$(TICKET) && \
	echo "Repository is ready for review with master and PR branch $(PR-BRANCH)." && \
	cd $(CURDIR) && \
	$(MAKE) diff-pr REPO=$(REPO) PR-BRANCH=$(PR-BRANCH) && \
	$(MAKE) list-changes REPO=$(REPO) PR-BRANCH=$(PR-BRANCH) && \
	$(MAKE) run-review TICKET=$(TICKET) REPO=$(REPO) BRANCH=review-$(TICKET)

# Generate a diff between main/master and PR branch
# Usage: make diff-pr REPO=username/repo-name PR-BRANCH=username/ticket-number
diff-pr:
	@if [ -z "$(REPO)" ]; then \
		echo "Error: REPO parameter is required. Usage: make diff-pr REPO=username/repo-name PR-BRANCH=username/ticket-number"; \
		exit 1; \
	fi
	@if [ -z "$(PR-BRANCH)" ]; then \
		echo "Error: PR-BRANCH parameter is required. Usage: make diff-pr REPO=username/repo-name PR-BRANCH=username/ticket-number"; \
		exit 1; \
	fi
	@REPO_NAME=`echo $(REPO) | sed 's/.*\///'`; \
	echo "Generating diff for PR branch $(PR-BRANCH) (Ticket: $(TICKET))...";\
	mkdir -p $(CURDIR)/.context/reviews;\
	cd .context/projects/$$REPO_NAME && \
	echo "Fetching branches with more history..." && \
	git fetch origin main --depth 100 2>/dev/null || git fetch origin master --depth 100 2>/dev/null && \
	git fetch origin $(PR-BRANCH) --depth 100 && \
	echo "Finding common ancestor between main/master and review-$(TICKET)..." && \
	DEFAULT_BRANCH=$$(git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@') && \
	MERGE_BASE=$$(git merge-base $$DEFAULT_BRANCH review-$(TICKET) 2>/dev/null || git merge-base main review-$(TICKET) 2>/dev/null || git merge-base master review-$(TICKET)) && \
	echo "Generating diff..." && \
	git diff $$MERGE_BASE..review-$(TICKET) > $(CURDIR)/.context/reviews/$(TICKET)-diff.md && \
	echo "Diff generated at .context/reviews/$(TICKET)-diff.md"

# Check status of integrations
# Usage: make status
status:
	@go run ./cmd/agent --status

# List changed files in a PR
# Usage: make list-changes REPO=username/repo-name PR-BRANCH=username/ticket-number
list-changes:
	@if [ -z "$(REPO)" ]; then \
		echo "Error: REPO parameter is required. Usage: make list-changes REPO=username/repo-name PR-BRANCH=username/ticket-number"; \
		exit 1; \
	fi
	@if [ -z "$(PR-BRANCH)" ]; then \
		echo "Error: PR-BRANCH parameter is required. Usage: make list-changes REPO=username/repo-name PR-BRANCH=username/ticket-number"; \
		exit 1; \
	fi
	@REPO_NAME=`echo $(REPO) | sed 's/.*\///'`; \
	echo "Listing changed files for PR branch $(PR-BRANCH) (Ticket: $(TICKET))...";\
	mkdir -p $(CURDIR)/.context/reviews;\
	cd .context/projects/$$REPO_NAME && \
	echo "# Changed Files for $(PR-BRANCH)" > $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "Fetching branches with more history..." && \
	git fetch origin main --depth 100 2>/dev/null || git fetch origin master --depth 100 2>/dev/null && \
	git fetch origin $(PR-BRANCH) --depth 100 && \
	echo "Finding common ancestor between main/master and review-$(TICKET)..." && \
	DEFAULT_BRANCH=$$(git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@') && \
	MERGE_BASE=$$(git merge-base $$DEFAULT_BRANCH review-$(TICKET) 2>/dev/null || git merge-base main review-$(TICKET) 2>/dev/null || git merge-base master review-$(TICKET)) && \
	echo "## Modified Files" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	git diff --name-status $$MERGE_BASE..review-$(TICKET) | grep "^M" | cut -f2 | sort >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "## Added Files" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	git diff --name-status $$MERGE_BASE..review-$(TICKET) | grep "^A" | cut -f2 | sort >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "## Deleted Files" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	git diff --name-status $$MERGE_BASE..review-$(TICKET) | grep "^D" | cut -f2 | sort >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "## File Statistics" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	git diff --numstat $$MERGE_BASE..review-$(TICKET) | sort -nr >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	git diff --stat $$MERGE_BASE..review-$(TICKET) >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "\`\`\`" >> $(CURDIR)/.context/reviews/$(TICKET)-files.md && \
	echo "File list generated at .context/reviews/$(TICKET)-files.md"

# Run revive linter
revive:
	@echo "Running revive linter..."
	@which revive > /dev/null || (echo "Error: revive not found. Install with: go install github.com/mgechev/revive@v1.3.2" && exit 1)
	@revive -formatter friendly ./...

# Help target
help:
	@echo "Available targets:"
	@echo "  build       - Build the application"
	@echo "  test        - Run all tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  run         - Run the application"
	@echo "  run-review  - Run the application in review mode (usage: make run-review TICKET=WIRE-1231)"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Run linter"
	@echo "  revive      - Run revive linter"
	@echo "  check       - Compile without producing executable"
	@echo "  setup-hooks - Configure Git to use project hooks"
	@echo "  clone-repo  - Clone a GitHub repository (usage: make clone-repo REPO=username/repo-name)"
	@echo "  pull-repo   - Pull latest changes from main/master branch (usage: make pull-repo REPO=username/repo-name)"
	@echo "  review      - Set up repo for review (usage: make review REPO=username/repo-name PR-BRANCH=branch-name)"
	@echo "  diff-pr     - Generate a diff between master and PR branch (usage: make diff-pr REPO=username/repo-name PR-BRANCH=username/ticket-number)"
	@echo "  list-changes - List changed files in a PR (usage: make list-changes REPO=username/repo-name PR-BRANCH=username/ticket-number)"
	@echo "  help        - Show this help message"
