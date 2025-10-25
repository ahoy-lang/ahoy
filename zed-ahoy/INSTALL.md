# Quick Installation Guide

## For Development/Testing

### Prerequisites
- Zed editor installed
- Rust toolchain (for building)

### Install the Extension

1. **Build the extension:**
   ```bash
   cd zed-ahoy
   cargo build --release
   ```

2. **Install in Zed:**
   
   **Option A - Using Zed UI:**
   - Open Zed
   - Press `Cmd+Shift+P` (Mac) or `Ctrl+Shift+P` (Linux/Windows)
   - Type "zed: install dev extension"
   - Select the `zed-ahoy` directory

   **Option B - Manual symlink:**
   ```bash
   mkdir -p ~/.config/zed/extensions/installed
   ln -sf $(pwd) ~/.config/zed/extensions/installed/ahoy
   ```

3. **Test it:**
   - Open the included `test.ahoy` file in Zed
   - You should see syntax highlighting!

## Verifying Installation

Create a new file with `.ahoy` extension and paste this code:

```ahoy
# Hello World in Ahoy
func main() {
    print("Hello from Ahoy!")
}

main()
```

You should see:
- Keywords highlighted (func, print)
- Strings in string color
- Comments in comment color
- Function names highlighted

## Troubleshooting

### Extension not loading
- Check Zed's log: Open Command Palette → "zed: open log"
- Ensure the tree-sitter grammar is built in `../tree-sitter-ahoy`

### No syntax highlighting
- Verify the file has `.ahoy` extension
- Reload Zed: Command Palette → "zed: reload"
- Check the language indicator in bottom-right should show "Ahoy"

### Build errors
- Ensure Rust is installed: `rustc --version`
- Update Rust: `rustup update`
- Clean and rebuild: `cargo clean && cargo build --release`

## Next Steps

Once working locally, you can:
1. Publish to Zed extensions registry (update extension.toml with git repo first)
2. Add more features like language server support
3. Improve syntax highlighting queries

See DEVELOPMENT.md for more details.
