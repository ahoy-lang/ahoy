#!/bin/bash
# Integration test - parses all example .ahoy files and reports results

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INPUT_DIR="$SCRIPT_DIR/../input"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🧪 Tree-sitter Integration Test"
echo "================================"
echo

# Counter
TOTAL=0
SUCCESS=0
FAILED=0

# Find all .ahoy files
for file in "$INPUT_DIR"/*.ahoy; do
  if [ -f "$file" ]; then
    TOTAL=$((TOTAL + 1))
    filename=$(basename "$file")
    
    echo -n "Testing $filename ... "
    
    # Parse and check for errors
    if tree-sitter parse "$file" 2>&1 | grep -q "(ERROR"; then
      echo -e "${RED}❌ Has parse errors$NC"
      FAILED=$((FAILED + 1))
    else
      echo -e "${GREEN}✅ Parsed successfully$NC"
      SUCCESS=$((SUCCESS + 1))
    fi
  fi
done

echo
echo "================================"
echo "Results:"
echo "  Total files: $TOTAL"
echo -e "  ${GREEN}Successful: $SUCCESS$NC"
echo -e "  ${RED}Failed: $FAILED$NC"

if [ $FAILED -eq 0 ]; then
  echo -e "\n${GREEN}✨ All files parsed successfully!$NC"
  exit 0
else
  echo -e "\n${YELLOW}⚠️  Some files have parse errors (this may be expected for WIP files)$NC"
  exit 1
fi
