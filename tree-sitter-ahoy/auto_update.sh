#!/bin/bash
# Automated grammar update and validation script
# Compares current source with grammar and regenerates if needed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="$SCRIPT_DIR/../source"
GRAMMAR_FILE="$SCRIPT_DIR/grammar.js"
BACKUP_DIR="$SCRIPT_DIR/.backups"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🚀 Ahoy Tree-sitter Auto-Update"
echo "================================"
echo

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup current grammar
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
if [ -f "$GRAMMAR_FILE" ]; then
  cp "$GRAMMAR_FILE" "$BACKUP_DIR/grammar_$TIMESTAMP.js"
  echo "✅ Backed up current grammar to .backups/"
fi

# Extract current keywords from tokenizer
echo "📊 Analyzing source files..."
KEYWORDS=$(grep -A 100 "keywords := map\[string\]TokenType{" "$SOURCE_DIR/tokenizer.go" | \
  grep '":' | \
  sed 's/.*"\(.*\)".*/\1/' | \
  grep -v "^$" | \
  sort -u)

# Check if all keywords are in grammar
echo "🔍 Checking grammar coverage..."
MISSING_KEYWORDS=""
for keyword in $KEYWORDS; do
  if ! grep -q "\"$keyword\"\\|'$keyword'" "$GRAMMAR_FILE"; then
    MISSING_KEYWORDS="$MISSING_KEYWORDS $keyword"
  fi
done

if [ -n "$MISSING_KEYWORDS" ]; then
  echo -e "${YELLOW}⚠️  Warning: Some keywords may not be in grammar:$NC"
  for kw in $MISSING_KEYWORDS; do
    echo "  - $kw"
  done
  echo
else
  echo -e "${GREEN}✅ All keywords appear to be covered$NC"
  echo
fi

# Regenerate parser
echo "🔨 Regenerating parser..."
cd "$SCRIPT_DIR"
if npm run generate --silent 2>&1 | grep -q "Error"; then
  echo -e "${RED}❌ Grammar generation failed!$NC"
  echo "Restoring backup..."
  cp "$BACKUP_DIR/grammar_$TIMESTAMP.js" "$GRAMMAR_FILE"
  exit 1
else
  echo -e "${GREEN}✅ Parser generated successfully$NC"
fi

# Run tests if they exist
if [ -d "$SCRIPT_DIR/test/corpus" ] && [ "$(ls -A $SCRIPT_DIR/test/corpus)" ]; then
  echo
  echo "🧪 Running tests..."
  if npm test --silent 2>&1 | grep -q "passed"; then
    echo -e "${GREEN}✅ Tests passed$NC"
  else
    echo -e "${YELLOW}⚠️  Some tests may need updating$NC"
  fi
fi

echo
echo "✨ Update complete!"
echo "📁 Grammar backed up to: .backups/grammar_$TIMESTAMP.js"
echo

# List recent backups
echo "Recent backups:"
ls -lt "$BACKUP_DIR" | head -6 | tail -5
