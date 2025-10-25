.PHONY: all generate test clean update check help

# Default target
all: generate

# Generate the parser from grammar.js
generate:
	@echo "Generating parser..."
	@tree-sitter generate
	@echo "✅ Parser generated"

# Run tests
test:
	@echo "Running tests..."
	@tree-sitter test

# Clean generated files
clean:
	@echo "Cleaning generated files..."
	@rm -rf src/ bindings/ build/
	@echo "✅ Cleaned"

# Update grammar from source and regenerate
update:
	@echo "Updating from Ahoy source files..."
	@./update_from_parser.sh

# Auto-update with validation
auto-update:
	@./auto_update.sh

# Check grammar for issues
check:
	@echo "Checking grammar..."
	@tree-sitter generate --no-bindings 2>&1 | grep -i "error\|warning" || echo "✅ No issues found"

# Parse a test file
parse:
	@if [ -z "$(FILE)" ]; then \
		echo "Usage: make parse FILE=../input/test.ahoy"; \
	else \
		tree-sitter parse $(FILE); \
	fi

# Show help
help:
	@echo "Ahoy Tree-sitter Grammar Makefile"
	@echo "=================================="
	@echo ""
	@echo "Targets:"
	@echo "  make generate      - Generate parser from grammar.js"
	@echo "  make test          - Run test suite"
	@echo "  make clean         - Remove generated files"
	@echo "  make update        - Check source files for changes"
	@echo "  make auto-update   - Automatically update and validate"
	@echo "  make check         - Check grammar for issues"
	@echo "  make parse FILE=<path> - Parse a specific file"
	@echo "  make help          - Show this help"
	@echo ""
	@echo "Examples:"
	@echo "  make parse FILE=../input/simple.ahoy"
	@echo "  make auto-update"
