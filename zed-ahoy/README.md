# Ahoy Extension for Zed

This extension adds support for the Ahoy programming language to the [Zed editor](https://zed.dev).

## Features

- **Syntax Highlighting**: Full syntax highlighting for Ahoy code using Tree-sitter
- **Code Folding**: Fold functions, loops, conditionals, and other blocks
- **Bracket Matching**: Automatic bracket, brace, and parenthesis matching
- **Indentation**: Smart indentation support
- **Outline View**: Navigate your code structure easily

## Installation

### From Zed Extensions

1. Open Zed
2. Press `Cmd+Shift+X` (Mac) or `Ctrl+Shift+X` (Linux/Windows) to open extensions
3. Search for "Ahoy"
4. Click "Install"

### Manual Installation

1. Clone this repository
2. Run `cargo build --release` to build the extension
3. Install the extension using the Zed CLI:
   ```bash
   zed --install-extension /path/to/zed-ahoy
   ```

## Development

### Building

```bash
cargo build --release
```

### Testing

Create a test `.ahoy` file and open it in Zed to test syntax highlighting and other features.

## File Extension

The extension recognizes files with the `.ahoy` extension.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT
