# tree-sitter-ahoy

Ahoy grammar for [tree-sitter](https://github.com/tree-sitter/tree-sitter).

## Features

This grammar provides syntax parsing for the Ahoy programming language, including:

- Variables and assignments
- Functions with typed parameters  
- Control flow (`if`, `else`, `elseif`, `anif`)
- Multiple loop types (`loop:start to end`, `loop element in array`, `loop key,val in dict`)
- Arrays (`<1, 2, 3>`)
- Dictionaries (`{"key":"value"}`)
- Word operators (`plus`, `minus`, `times`, `div`, `mod`)
- Comparison operators (`greater_than`, `lesser_than`, `is`)
- Comments (`? comment`)
- Imports (`import "lib.h"`)
- Compile-time conditionals (`when DEBUG then ...`)

## Installation

### npm

```bash
npm install tree-sitter-ahoy
```

### From source

```bash
git clone https://github.com/yourusername/tree-sitter-ahoy
cd tree-sitter-ahoy
npm install
npm run generate
```

## Usage

### Node.js

```javascript
const Parser = require('tree-sitter');
const Ahoy = require('tree-sitter-ahoy');

const parser = new Parser();
parser.setLanguage(Ahoy);

const sourceCode = `
x: 42
ahoy|"Hello, World!\\n"|
`;

const tree = parser.parse(sourceCode);
console.log(tree.rootNode.toString());
```

### CLI

```bash
# Parse a file
tree-sitter parse example.ahoy

# Test the grammar
tree-sitter test

# Generate the parser
tree-sitter generate
```

## Development

### Testing

```bash
npm test
```

### Regenerating the parser

After modifying `grammar.js`:

```bash
npm run generate
```

### Structure

```
tree-sitter-ahoy/
├── grammar.js          # Grammar definition
├── tree-sitter.json    # Configuration
├── src/                # Generated parser (C)
├── bindings/           # Language bindings
│   └── node/           # Node.js bindings
├── queries/            # Syntax queries
│   └── highlights.scm  # Syntax highlighting
└── test/               # Test cases
    └── corpus/         # Test corpus
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT

## Related

- [Ahoy Language](../README.md)
- [Tree-sitter Documentation](https://tree-sitter.github.io/tree-sitter/)

