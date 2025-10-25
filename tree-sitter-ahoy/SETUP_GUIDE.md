# Ahoy Tree-sitter Grammar - Complete Setup Guide

## Overview

This directory contains a complete tree-sitter grammar implementation for the Ahoy programming language. Tree-sitter provides fast, incremental parsing for syntax highlighting, code navigation, and language tooling.

## What's Included

```
tree-sitter/
├── grammar.js              # Main grammar definition
├── package.json            # Node.js dependencies
├── binding.gyp             # Native bindings configuration
├── Makefile                # Convenient build targets
├── README.md               # User documentation
├── SETUP_GUIDE.md          # This file
├── update_from_parser.sh   # Extract features from Go parser
├── auto_update.sh          # Automated update & validation
├── src/                    # Generated C parser (auto-generated)
├── test/                   # Test suite
│   └── corpus/            # Test cases
└── .gitignore              # Git ignore patterns
```

## Initial Setup (Done)

✅ Created grammar.js with complete Ahoy syntax
✅ Configured package.json with tree-sitter dependencies
✅ Generated parser successfully
✅ Created automation scripts
✅ Added test infrastructure

## How It Works

### 1. Grammar Definition (grammar.js)

The `grammar.js` file defines all syntactic rules using JavaScript:

- **Keywords**: if, then, else, loop, func, return, when, etc.
- **Operators**: Word operators (plus, minus, times, div, mod) and symbols (+, -, *, /, %)
- **Structures**: functions, arrays `<...>`, dictionaries `{...}`, loops, conditionals
- **Expressions**: binary operations, unary operations, function calls
- **Literals**: numbers, strings, booleans, characters

### 2. Parser Generation

When you run `tree-sitter generate`:
1. Reads grammar.js
2. Generates parser.c and associated files in src/
3. Creates language bindings
4. Compiles to binary for fast parsing

### 3. Automatic Synchronization

Two scripts help keep the grammar in sync with Ahoy's Go implementation:

#### `update_from_parser.sh`
- Extracts keywords from tokenizer.go
- Lists AST node types from parser.go
- Shows recent changes
- **When to use**: Before making grammar updates

#### `auto_update.sh`
- Checks keyword coverage
- Backs up current grammar
- Regenerates parser
- Runs tests
- Restores backup if generation fails
- **When to use**: After changing the Ahoy language

## Usage Guide

### Quick Commands

```bash
# Generate parser
make generate

# Run tests
make test

# Parse a specific file
make parse FILE=../input/simple.ahoy

# Check for language changes
make update

# Auto-update with validation
make auto-update

# Clean generated files
make clean
```

### Manual Workflow

1. **When Adding New Language Features**:
   ```bash
   cd tree-sitter
   ./update_from_parser.sh  # See what changed
   # Edit grammar.js to add new rules
   make generate
   make test
   ```

2. **When Modifying Existing Syntax**:
   ```bash
   # Edit grammar.js
   make generate
   # Add test cases in test/corpus/
   make test
   ```

3. **Automated Updates**:
   ```bash
   ./auto_update.sh  # Handles everything automatically
   ```

## Maintaining the Grammar

### When to Update

Update the grammar when:
- ✏️ New keywords are added to Ahoy
- 🔄 Syntax rules change
- ➕ New operators are introduced
- 🏗️ Language structures are modified
- 🐛 Bug fixes affect parsing

### Testing Strategy

1. **Corpus Tests**: Add examples in `test/corpus/basics.txt`
2. **Real Files**: Test with actual .ahoy files: `make parse FILE=...`
3. **Edge Cases**: Test error recovery and unusual syntax

### Common Issues

**Conflict Errors**:
- Tree-sitter may report conflicts when grammar rules overlap
- Resolve by adding precedence levels or explicit conflicts
- Current conflicts are documented in grammar.js

**Parsing Errors**:
- Run `tree-sitter parse` on sample files
- Look for (ERROR ...) nodes in output
- Adjust grammar rules to handle edge cases

## Integration with Other Tools

### VS Code Extension

```json
{
  "contributes": {
    "grammars": [{
      "language": "ahoy",
      "scopeName": "source.ahoy",
      "path": "./tree-sitter/src/parser.c"
    }]
  }
}
```

### Neovim

```lua
local parser_config = require('nvim-treesitter.parsers').get_parser_configs()
parser_config.ahoy = {
  install_info = {
    url = "/path/to/ahoy/tree-sitter",
    files = {"src/parser.c"}
  },
  filetype = "ahoy"
}
```

### Language Server

Tree-sitter parsers can power language servers for:
- Syntax highlighting
- Code folding
- Symbol navigation
- Error detection
- Code completion (with additional semantic analysis)

## Architecture

### Grammar Structure

```javascript
module.exports = grammar({
  name: 'ahoy',
  extras: [...],      // Whitespace & comments
  conflicts: [...],   // Known ambiguities
  rules: {
    source_file: ..., // Entry point
    _statement: ...,  // Statement types
    expression: ...,  // Expression types
    // ... more rules
  }
});
```

### Rule Types

- **Sequences**: `seq(a, b, c)` - matches in order
- **Choices**: `choice(a, b)` - matches one alternative
- **Repetition**: `repeat(a)` or `repeat1(a)`
- **Optional**: `optional(a)`
- **Precedence**: `prec.left(n, ...)` or `prec.right(n, ...)`

## Troubleshooting

### Generation Fails

```bash
# Check for syntax errors
tree-sitter generate

# View detailed errors
npm run generate
```

### Tests Fail

```bash
# Run with verbose output
tree-sitter test -d

# Test specific file
tree-sitter test test/corpus/basics.txt
```

### Parser Misparses Code

1. Run `tree-sitter parse problem.ahoy`
2. Look for ERROR nodes
3. Check if grammar rules cover the construct
4. Add/modify rules in grammar.js
5. Regenerate and test

## Resources

- **Tree-sitter Docs**: https://tree-sitter.github.io/tree-sitter/
- **Grammar Writing Guide**: https://tree-sitter.github.io/tree-sitter/creating-parsers
- **Example Grammars**: https://github.com/tree-sitter/
- **Ahoy Source**: ../source/*.go

## Future Enhancements

Potential improvements:
- [ ] Syntax highlighting queries (queries/highlights.scm)
- [ ] Indentation queries (queries/indents.scm)
- [ ] Code folding queries (queries/folds.scm)
- [ ] Injection queries for embedded languages
- [ ] VS Code extension integration
- [ ] Neovim plugin
- [ ] GitHub Linguist support
- [ ] CI/CD integration for auto-updates

## Contributing

When modifying the grammar:
1. Always backup (auto_update.sh does this)
2. Test thoroughly with real .ahoy files
3. Update test/corpus/ with new examples
4. Document changes in comments
5. Run full test suite before committing

## Maintenance Schedule

- **Weekly**: Run `./update_from_parser.sh` to check for changes
- **After Language Changes**: Run `./auto_update.sh`
- **Before Releases**: Full test suite + parse all example files
- **Monthly**: Review and clean .backups/ directory

---

**Setup Complete!** 🎉

The tree-sitter grammar is ready to use. Run `make help` for command reference.
