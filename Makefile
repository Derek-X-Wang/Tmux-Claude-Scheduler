.PHONY: build test run clean lint fmt vet help install-deps

# Variables
BINARY_NAME=tcs
BINARY_PATH=./bin/$(BINARY_NAME)
GO_FILES=$(shell find . -name "*.go" -type f -not -path "./vendor/*")

# Default target
all: lint test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	go build -o $(BINARY_PATH) ./main.go

# Run tests
test:
	@echo "Running tests..."
	go test ./... -v -race -coverprofile=coverage.out

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	$(BINARY_PATH)

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -rf bin/
	rm -f coverage.out
	go clean

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run linter (requires golangci-lint)
lint:
	@echo "╔════════════════════════════════════════════════════════════════╗"
	@echo "║                    Running Code Quality Checks                  ║"
	@echo "╚════════════════════════════════════════════════════════════════╝"
	@echo ""
	@if [ -f "$(HOME)/go/bin/golangci-lint" ]; then \
		GOLANGCI_LINT="$(HOME)/go/bin/golangci-lint"; \
	elif which golangci-lint > /dev/null 2>&1; then \
		GOLANGCI_LINT="golangci-lint"; \
	else \
		echo "❌ ERROR: golangci-lint not installed"; \
		echo ""; \
		echo "To install, run:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo ""; \
		exit 1; \
	fi; \
	echo "🔧 Linter Details:"; \
	echo "  • Binary: $$GOLANGCI_LINT"; \
	echo "  • Version: $$($$GOLANGCI_LINT version --format short 2>/dev/null || echo 'unknown')"; \
	echo "  • Config: .golangci.yml"; \
	echo ""; \
	echo "📋 Active Linters:"; \
	echo "  • errcheck    - Checks for unchecked errors"; \
	echo "  • govet       - Reports suspicious constructs"; \
	echo "  • gofmt       - Checks formatting"; \
	echo "  • goimports   - Checks import formatting"; \
	echo "  • staticcheck - Advanced static analysis"; \
	echo "  • misspell    - Finds misspelled words"; \
	echo "  • ineffassign - Detects ineffectual assignments"; \
	echo "  • unused      - Finds unused code"; \
	echo "  • unparam     - Reports unused function parameters"; \
	echo "  • unconvert   - Detects unnecessary type conversions"; \
	echo "  • and more..."; \
	echo ""; \
	echo "🔍 Analyzing code..."; \
	echo ""; \
	TEMP_FILE=$$(mktemp); \
	if $$GOLANGCI_LINT run --out-format=line-number --print-linter-name 2>&1 | tee $$TEMP_FILE | grep -E "^[^:]+:[0-9]+:[0-9]+:"; then \
		echo ""; \
		ISSUE_COUNT=$$(grep -c "^[^:]+:[0-9]+:[0-9]+:" $$TEMP_FILE || echo "0"); \
		echo "❌ Found $$ISSUE_COUNT issue(s) that need attention"; \
		echo ""; \
		echo "💡 To see more details, run:"; \
		echo "   golangci-lint run --verbose"; \
		echo ""; \
		rm -f $$TEMP_FILE; \
		exit 1; \
	else \
		if grep -q "no such file or directory" $$TEMP_FILE 2>/dev/null; then \
			echo "❌ Error: Unable to find source files"; \
			rm -f $$TEMP_FILE; \
			exit 1; \
		fi; \
		echo "✅ Excellent! All quality checks passed"; \
		echo ""; \
		echo "📊 Summary:"; \
		echo "  • Files analyzed: $$(find . -name '*.go' -not -path './vendor/*' -not -path './bin/*' | wc -l | tr -d ' ')"; \
		echo "  • Packages checked: $$(go list ./... 2>/dev/null | grep -v vendor | wc -l | tr -d ' ')"; \
		echo "  • Issues found: 0"; \
		echo ""; \
		rm -f $$TEMP_FILE; \
	fi

# Install dependencies
install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run TUI
tui: build
	@echo "Starting TUI..."
	$(BINARY_PATH) tui

# Development workflow
dev: fmt vet lint test build

# Show coverage
coverage: test
	@echo "Showing test coverage..."
	go tool cover -html=coverage.out

# Install system-wide
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BINARY_PATH) /usr/local/bin/$(BINARY_NAME)
	@echo "$(BINARY_NAME) installed successfully!"

# Help
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  test        - Run tests"
	@echo "  run         - Build and run the application"
	@echo "  clean       - Clean build artifacts"
	@echo "  fmt         - Format code"
	@echo "  vet         - Run go vet"
	@echo "  lint        - Run golangci-lint"
	@echo "  install-deps - Install Go dependencies"
	@echo "  install-tools - Install development tools"
	@echo "  tui         - Build and start TUI"
	@echo "  dev         - Run full development workflow (fmt, vet, lint, test, build)"
	@echo "  coverage    - Show test coverage"
	@echo "  install     - Install binary to /usr/local/bin"
	@echo "  help        - Show this help"