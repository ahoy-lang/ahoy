# Tree-sitter Parser for Ahoy

## Overview

A complete tree-sitter grammar implementation has been added to enable modern editor support and language tooling for Ahoy.

## Location

All tree-sitter files are in the `tree-sitter/` directory.

## What You Get

✅ **Fast, Incremental Parsing** - Parse Ahoy files efficiently
✅ **Syntax Highlighting** - Rich, semantic syntax highlighting support
✅ **Code Navigation** - Go to definition, find references, symbol search
✅ **Structural Editing** - Smart selection, code folding
✅ **Editor Integration** - Ready for VS Code, Neovim, Emacs, Atom
✅ **Automatic Synchronization** - Scripts to keep grammar updated with language changes

## Quick Start

```bash
cd tree-sitter

# Generate the parser
make generate

# Test it
make parse FILE=../input/simple.ahoy

# Run all tests
make test
```

## For Language Developers

### When You Change Ahoy's Syntax

After modifying keywords, operators, or grammar in the Go source:

```bash
cd tree-sitter-ahoy
./auto_update.sh
```

This automatically:
- Checks for new keywords from `source/tokenizer.go`
- Validates grammar coverage
- Regenerates the parser
- Runs tests
- Creates backups (in `.backups/`)

### Manual Updates

```bash
# See what changed
./update_from_parser.sh

# Edit grammar.js manually
vim grammar.js

# Regenerate
make generate
```

## Available Commands

```bash
make generate      # Generate parser from grammar.js
make test          # Run test suite
make clean         # Remove generated files
make parse FILE=<path>  # Parse and visualize a file
make help          # Show all commands
```

## Integration

### VS Code

Install the tree-sitter extension and point it to the generated parser.

### Neovim

```lua
require('nvim-treesitter.parsers').get_parser_configs().ahoy = {
  install_info = {
    url = "/path/to/ahoy/tree-sitter",
    files = {"src/parser.c"}
  }
}
```

### Command Line

```bash
tree-sitter parse myfile.ahoy
```

## Files

```
tree-sitter-ahoy/
├── QUICKSTART.md          ← Start here
├── README.md              ← Full documentation
├── SETUP_GUIDE.md         ← Detailed setup guide
├── grammar.js             ← Grammar definition (edit this)
├── package.json           ← Dependencies
├── Makefile               ← Build commands
├── auto_update.sh         ← Automatic update script
├── update_from_parser.sh  ← Feature extraction script
├── test_integration.sh    ← Integration tests
├── src/                   ← Generated parser
└── test/corpus/           ← Test cases
```

## Documentation

- **Quick Start**: `tree-sitter-ahoy/QUICKSTART.md`
- **Full Guide**: `tree-sitter-ahoy/README.md`
- **Setup Details**: `tree-sitter-ahoy/SETUP_GUIDE.md`

## Grammar Features

The grammar supports all Ahoy language features:

- ✅ Variables and assignments (`x: 42`)
- ✅ Functions (`func name|args| type then ...`)
- ✅ Control flow (`if`, `else`, `elseif`, `anif`)
- ✅ Loops (`loop:start to end`, `loop item in array`)
- ✅ Arrays (`<1, 2, 3>`)
- ✅ Dictionaries (`{"key":"value"}`)
- ✅ Word operators (`plus`, `minus`, `times`, `div`, `mod`)
- ✅ Comparisons (`greater_than`, `lesser_than`, `is`)
- ✅ Comments (`? comment`)
- ✅ Imports (`import "lib.h"`)
- ✅ Compile-time conditionals (`when DEBUG then ...`)
- ✅ Semicolon statement separators

## Maintenance

### Regular Updates

```bash
# Weekly check (recommended)
cd tree-sitter-ahoy && ./update_from_parser.sh

# After language changes (required)
cd tree-sitter-ahoy && ./auto_update.sh
```

### Testing

```bash
# Test all example files
cd tree-sitter-ahoy && ./test_integration.sh

# Test specific file
make parse FILE=../input/test.ahoy
```

## Implementation Details

The grammar now follows official tree-sitter conventions as documented at https://tree-sitter.github.io/tree-sitter/ and matches the structure of tree-sitter-go (https://github.com/tree-sitter/tree-sitter-go).

### Standards Compliance

✅ **tree-sitter.json** - Configuration matching official format
✅ **bindings/node/** - Node.js bindings with node-addon-api
✅ **queries/highlights.scm** - Syntax highlighting queries  
✅ **LICENSE** - MIT license
✅ **Proper package.json** - Ready for npm publishing

See `tree-sitter-ahoy/STATUS.md` for detailed comparison with tree-sitter-go.

## Status

✅ Grammar: Complete and functional
✅ Parser: Generated successfully
✅ Automation: Working
✅ Tests: Basic suite created
⏳ Syntax highlighting queries: To be added
⏳ Editor plugins: To be configured

## Learn More

- **Tree-sitter**: https://tree-sitter.github.io/tree-sitter/
- **Creating Parsers**: https://tree-sitter.github.io/tree-sitter/creating-parsers
- **Ahoy Language**: See main README.md

---

**Ready to use!** Start with `tree-sitter-ahoy/QUICKSTART.md` for a step-by-step guide.
