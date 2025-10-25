# Zed Ahoy Extension Development Guide

## Structure

The extension follows the standard Zed extension layout:

```
zed-ahoy/
├── Cargo.toml              # Rust project configuration
├── extension.toml          # Extension metadata
├── src/
│   └── ahoy.rs            # Extension implementation
└── languages/
    └── ahoy/
        ├── config.toml     # Language configuration
        ├── highlights.scm  # Syntax highlighting queries
        ├── brackets.scm    # Bracket matching
        ├── folds.scm       # Code folding
        ├── indents.scm     # Indentation rules
        └── outline.scm     # Symbol outline
```

## Files Explained

### `extension.toml`
Defines the extension metadata and points to the tree-sitter grammar. Currently uses a local path (`../tree-sitter-ahoy`) for development.

### `Cargo.toml`
Standard Rust crate definition for a Zed extension with:
- `crate-type = ["cdylib"]` to produce a dynamic library
- `zed_extension_api` dependency

### `src/ahoy.rs`
Minimal extension implementation. Since Ahoy doesn't have a language server yet, this just registers the extension with Zed. Language servers can be added later.

### `languages/ahoy/config.toml`
Language configuration including:
- File extension (`.ahoy`)
- Comment syntax (`#`)
- Bracket pairs
- Autoclose behavior

### `languages/ahoy/highlights.scm`
Tree-sitter query file that maps grammar nodes to syntax highlighting categories. Copied from your tree-sitter grammar queries.

### Other `.scm` files
Additional Tree-sitter queries for editor features like folding, indentation, and outline view.

## Building

```bash
cd zed-ahoy
cargo build --release
```

The compiled extension will be in `target/wasm32-wasi/release/` (Zed extensions are compiled to WASM).

## Installing for Development

### Option 1: Install from local path
```bash
zed: install dev extension
# Then select the zed-ahoy directory
```

### Option 2: Symlink into Zed extensions directory
```bash
ln -s $(pwd) ~/.config/zed/extensions/installed/ahoy
```

### Option 3: Use Zed's extension development mode
Open Zed, go to Extensions panel, and use "Install Dev Extension" pointing to this directory.

## Testing

1. Create a test file `test.ahoy`:
```ahoy
# Sample Ahoy code
func greet(name: string) {
    print("Hello, " plus name)
}

greet("World")
```

2. Open it in Zed to see syntax highlighting in action

## Publishing

Before publishing to the Zed extension registry:

1. Update `extension.toml` to use a git repository URL instead of local path:
```toml
[grammars.ahoy]
repository = "https://github.com/yourusername/tree-sitter-ahoy"
commit = "abcdef123456"  # Use specific commit hash
```

2. Test thoroughly with the remote grammar

3. Submit to Zed extensions registry following their documentation

## Adding Language Server Support

When you have a language server for Ahoy, update `src/ahoy.rs` to:
1. Download/locate the language server binary
2. Start it with appropriate arguments
3. Configure initialization options

See the Odin extension example for reference.
