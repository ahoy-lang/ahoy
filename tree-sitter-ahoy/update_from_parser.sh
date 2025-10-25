#!/bin/bash
# Script to help update tree-sitter grammar when Ahoy language changes

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="$SCRIPT_DIR/../source"

echo "🔍 Extracting language features from Ahoy source..."
echo

# Extract keywords from tokenizer.go
echo "📋 Keywords and Tokens:"
echo "======================"
grep -A 100 "keywords := map\[string\]TokenType{" "$SOURCE_DIR/tokenizer.go" | \
  grep '":' | \
  sed 's/.*"\(.*\)".*/  - \1/' | \
  head -40
echo

# Extract node types from parser.go
echo "🌳 AST Node Types:"
echo "=================="
grep "NODE_" "$SOURCE_DIR/parser.go" | \
  grep "const\|NODE_.*=" | \
  head -30 | \
  sed 's/.*NODE_/  - NODE_/' | \
  sed 's/ .*//'
echo

# Check for recent changes in parser files
echo "📝 Recent Changes:"
echo "=================="
if [ -d "$SCRIPT_DIR/../.git" ]; then
  echo "Last 5 commits affecting parser:"
  cd "$SCRIPT_DIR/.." && git log --oneline -5 -- source/parser.go source/tokenizer.go 2>/dev/null || echo "  (No git history found)"
fi
echo

echo "✅ Feature extraction complete!"
echo
echo "Next steps:"
echo "1. Review grammar.js to ensure it covers all keywords and node types"
echo "2. Run: npm run generate"
echo "3. Run: npm test"
echo "4. Add test cases in test/corpus/ for new features"
echo
