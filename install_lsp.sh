#!/bin/bash

# Build and install the Ahoy language server.
#
# The installed command is a symlink to ../ahoy-lsp/ahoy-lsp, so rebuilding the
# server in the repository updates the command editors run. Restart the editor
# (or its language server) after a rebuild: a running process keeps the binary
# image it was started with.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LSP_DIR="$SCRIPT_DIR/../ahoy-lsp"

if [ ! -d "$LSP_DIR" ]; then
    echo "Error: ahoy-lsp directory not found at $LSP_DIR" >&2
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "Error: Go is not installed" >&2
    exit 1
fi

echo "Building and installing ahoy-lsp..."
make -C "$LSP_DIR" install

echo ""
echo "Installed command:"
command -v ahoy-lsp || true
ahoy-lsp --version || true
