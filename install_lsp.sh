#!/bin/bash

# Ahoy LSP Installation Script
# This script builds and installs the Ahoy Language Server

set -e

echo "========================================"
echo "  Ahoy LSP Installation Script"
echo "========================================"
echo ""

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LSP_DIR="$SCRIPT_DIR/lsp"

# Check if lsp directory exists
if [ ! -d "$LSP_DIR" ]; then
    echo "❌ Error: LSP directory not found at $LSP_DIR"
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed"
    echo "Please install Go 1.20 or later from https://go.dev/dl/"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.20"

if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "❌ Error: Go version $GO_VERSION is too old"
    echo "Please install Go $REQUIRED_VERSION or later"
    exit 1
fi

echo "✓ Go version $GO_VERSION detected"
echo ""

# Build the LSP server
echo "Building Ahoy LSP server..."
cd "$LSP_DIR"

if go build -o ahoy-lsp .; then
    echo "✓ Build successful!"
else
    echo "❌ Build failed"
    exit 1
fi

echo ""
echo "LSP server binary created: $LSP_DIR/ahoy-lsp"
echo "Size: $(du -h ahoy-lsp | cut -f1)"
echo ""

# Ask for installation preference
echo "Where would you like to install ahoy-lsp?"
echo ""
echo "1) System-wide (/usr/local/bin) - requires sudo"
echo "2) User directory (~/.local/bin) - no sudo needed"
echo "3) Skip installation (just build)"
echo ""

read -p "Enter choice [1-3]: " choice

case $choice in
    1)
        echo ""
        echo "Installing to /usr/local/bin (requires sudo)..."
        if sudo cp ahoy-lsp /usr/local/bin/; then
            echo "✓ Installed to /usr/local/bin/ahoy-lsp"
            INSTALL_PATH="/usr/local/bin/ahoy-lsp"
        else
            echo "❌ Installation failed"
            exit 1
        fi
        ;;
    2)
        echo ""
        echo "Installing to ~/.local/bin..."
        mkdir -p ~/.local/bin
        if cp ahoy-lsp ~/.local/bin/; then
            echo "✓ Installed to ~/.local/bin/ahoy-lsp"
            INSTALL_PATH="$HOME/.local/bin/ahoy-lsp"

            # Check if ~/.local/bin is in PATH
            if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
                echo ""
                echo "⚠️  Warning: ~/.local/bin is not in your PATH"
                echo ""
                echo "Add the following line to your ~/.bashrc or ~/.zshrc:"
                echo ""
                echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
                echo ""
                echo "Then run: source ~/.bashrc (or source ~/.zshrc)"
            fi
        else
            echo "❌ Installation failed"
            exit 1
        fi
        ;;
    3)
        echo ""
        echo "Skipping installation. Binary is at: $LSP_DIR/ahoy-lsp"
        echo ""
        echo "To use it, either:"
        echo "1) Add $LSP_DIR to your PATH"
        echo "2) Copy it manually to a directory in your PATH"
        exit 0
        ;;
    *)
        echo "Invalid choice. Exiting."
        exit 1
        ;;
esac

echo ""
echo "========================================"
echo "  Installation Complete!"
echo "========================================"
echo ""

# Verify installation
if command -v ahoy-lsp &> /dev/null; then
    echo "✓ ahoy-lsp is now in your PATH"
    echo "  Location: $(which ahoy-lsp)"
else
    echo "⚠️  ahoy-lsp may not be in your PATH yet"
    if [ -n "$INSTALL_PATH" ]; then
        echo "  Installed to: $INSTALL_PATH"
    fi
fi

echo ""
echo "Next Steps:"
echo ""
echo "1. Editor Integration:"
echo "   - Zed:     Rebuild zed-ahoy extension, restart Zed"
echo "   - VS Code: Install vscode-ahoy extension (in development)"
echo "   - Neovim:  See ahoy/LSP_SETUP.md for configuration"
echo ""
echo "2. Test the LSP:"
echo "   Open any .ahoy file in your editor"
echo "   You should see:"
echo "   - Real-time syntax error highlighting"
echo "   - Autocomplete when typing (Ctrl+Space)"
echo "   - Hover information over symbols"
echo "   - Go-to-definition (Ctrl+Click or F12)"
echo ""
echo "3. Documentation:"
echo "   - Setup guide:  $SCRIPT_DIR/LSP_SETUP.md"
echo "   - Testing guide: $LSP_DIR/TESTING.md"
echo "   - LSP README:   $LSP_DIR/README.md"
echo ""
echo "4. Troubleshooting:"
echo "   If LSP doesn't work:"
echo "   - Check: which ahoy-lsp"
echo "   - Check editor logs (see LSP_SETUP.md)"
echo "   - Try: ahoy-lsp (it will wait for stdin - press Ctrl+C to exit)"
echo ""
echo "Happy coding with Ahoy! 🚢"
