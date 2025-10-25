# Tree-sitter Grammar for Ahoy - Quick Start

## What is This?

A tree-sitter grammar that parses Ahoy language files into abstract syntax trees. This enables:
- **Syntax highlighting** in editors (VS Code, Vim, Neovim, etc.)
- **Code navigation** (go to definition, find references)
- **Structural editing** (smart selection, folding)
- **Language tooling** (linters, formatters, language servers)

## Quick Start

### 1. Prerequisites

```bash
# Install Node.js (if not already installed)
# Then install tree-sitter CLI globally:
npm install -g tree-sitter-cli
```

### 2. Generate Parser

```bash
cd tree-sitter
make generate
```

This creates the parser in `src/` directory.

### 3. Test It

```bash
# Parse a file
make parse FILE=../input/simple.ahoy

# Run test suite
make test

# Test all example files
./test_integration.sh
```

## Maintenance

### When You Change the Ahoy Language

After modifying the language (adding keywords, changing syntax, etc.):

```bash
cd tree-sitter
./auto_update.sh
```

This automatically:
- ✅ Backs up current grammar
- ✅ Checks for missing keywords
- ✅ Regenerates parser
- ✅ Runs tests
- ✅ Restores backup if anything fails

### Manual Grammar Updates

```bash
# Check what changed in the language
./update_from_parser.sh

# Edit grammar.js manually if needed
# Then regenerate:
make generate
make test
```

## Available Commands

```bash
make generate      # Generate parser
make test          # Run tests
make clean         # Remove generated files
make update        # Check for language changes
make auto-update   # Fully automated update
make parse FILE=<path>  # Parse a specific file
make help          # Show all commands
```

## Directory Structure

```
tree-sitter/
├── grammar.js              ← Main grammar definition (edit this)
├── src/                    ← Generated parser (don't edit)
├── test/corpus/           ← Test cases
├── Makefile               ← Build commands
├── auto_update.sh         ← Automation script
├── update_from_parser.sh  ← Feature extraction
└── README.md              ← Full documentation
```

## How Automatic Updates Work

The grammar stays synchronized with your Go parser implementation:

1. **Extract Features**: `update_from_parser.sh` reads `source/tokenizer.go` and `source/parser.go`
2. **Check Coverage**: Compares extracted keywords with grammar.js
3. **Regenerate**: Compiles new parser if needed
4. **Validate**: Runs tests to ensure nothing broke
5. **Backup**: Keeps backups in `.backups/` directory

## Integration Examples

### Parse in Your Code

```bash
tree-sitter parse myfile.ahoy
```

### Use in Editor

The generated parser can be used with:
- **VS Code**: Tree-sitter extension
- **Neovim**: nvim-treesitter plugin
- **Emacs**: tree-sitter-mode
- **Atom**: tree-sitter package

### Syntax Highlighting

Add highlight queries in `queries/highlights.scm` (to be added) to enable semantic syntax highlighting.

## Troubleshooting

**Parse Errors?**
- Run: `tree-sitter parse problematic_file.ahoy`
- Look for `(ERROR ...)` nodes
- Update grammar.js rules to handle the construct
- Regenerate with `make generate`

**Generation Fails?**
- Check `grammar.js` syntax
- Look for unresolved conflicts in output
- Add conflicts to `conflicts: [...]` section
- See `SETUP_GUIDE.md` for details

**Tests Fail?**
- Update test cases in `test/corpus/`
- Tests may need adjustment after grammar changes
- Run `tree-sitter test -d` for detailed output

## Learn More

- **Full Documentation**: See `README.md` and `SETUP_GUIDE.md`
- **Tree-sitter Docs**: https://tree-sitter.github.io/tree-sitter/
- **Ahoy Language**: See `../README.md`

## Status

✅ **Grammar**: Complete and functional
✅ **Parser**: Generated successfully  
✅ **Tests**: Basic test suite created
✅ **Automation**: Update scripts working
⏳ **Highlight queries**: To be added
⏳ **Editor integration**: To be configured

---

**Quick Tip**: Run `./auto_update.sh` after any language changes to keep everything synchronized automatically!
