# Ahoy LSP Setup Guide

This guide explains how to set up the Ahoy Language Server Protocol (LSP) for your editor.

## What Was Changed

The Ahoy compiler has been restructured to support LSP:

### Structure Changes

**Before:**
```
ahoy/
├── source/
│   ├── main.go
│   ├── parser.go      # Was here
│   ├── tokenizer.go   # Was here
│   └── codegen.go
```

**After:**
```
ahoy/
├── parser.go          # Moved up - shared library
├── tokenizer.go       # Moved up - shared library
├── go.mod             # New - root module
├── source/            # Compiler CLI
│   ├── main.go
│   ├── codegen.go
│   └── formatter.go
└── lsp/               # NEW - Language Server
    ├── main.go
    ├── server.go
    ├── diagnostics.go
    ├── completion.go
    ├── definition.go
    ├── hover.go
    └── symbols.go
```

### Why This Structure?

Both the compiler (`source/`) and the LSP server (`lsp/`) need to parse Ahoy code. Instead of duplicating the parser and tokenizer, they're now shared at the root level. Both import from the `ahoy` package.

## Installation

### Step 1: Build the LSP Server

```bash
cd /path/to/ahoy/lsp
./build.sh
```

Or manually:
```bash
cd /path/to/ahoy/lsp
go build -o ahoy-lsp .
```

### Step 2: Install the Binary

**Option A: System-wide (recommended)**
```bash
sudo cp ahoy-lsp /usr/local/bin/
```

**Option B: User installation**
```bash
mkdir -p ~/.local/bin
cp ahoy-lsp ~/.local/bin/
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Step 3: Verify Installation

```bash
which ahoy-lsp
# Should output: /usr/local/bin/ahoy-lsp (or ~/.local/bin/ahoy-lsp)
```

## Editor Setup

### Zed (Primary Support)

The Zed extension has been updated to automatically use `ahoy-lsp`.

1. Make sure `ahoy-lsp` is in your PATH (see Step 2 above)
2. Rebuild the Zed extension:
   ```bash
   cd /path/to/zed-ahoy
   cargo build --release
   ```
3. Restart Zed
4. Open any `.ahoy` file - you should see diagnostics!

### VS Code

A separate Claude instance is working on the VS Code extension. Once complete, it should automatically detect and use `ahoy-lsp`.

### Neovim

Add to your `~/.config/nvim/init.lua`:

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- Register ahoy-lsp
if not configs.ahoy_lsp then
  configs.ahoy_lsp = {
    default_config = {
      cmd = { 'ahoy-lsp' },
      filetypes = { 'ahoy' },
      root_dir = lspconfig.util.root_pattern('.git', 'ahoy.toml'),
      settings = {},
    },
  }
end

-- Setup ahoy-lsp
lspconfig.ahoy_lsp.setup{
  on_attach = function(client, bufnr)
    -- Enable completion triggered by <c-x><c-o>
    vim.api.nvim_buf_set_option(bufnr, 'omnifunc', 'v:lua.vim.lsp.omnifunc')
    
    -- Keybindings
    local opts = { noremap=true, silent=true, buffer=bufnr }
    vim.keymap.set('n', 'gd', vim.lsp.buf.definition, opts)
    vim.keymap.set('n', 'K', vim.lsp.buf.hover, opts)
    vim.keymap.set('n', '<leader>rn', vim.lsp.buf.rename, opts)
  end
}
```

You'll also need to set the filetype for `.ahoy` files. Create `~/.config/nvim/ftdetect/ahoy.vim`:

```vim
au BufRead,BufNewFile *.ahoy set filetype=ahoy
```

### Vim (with vim-lsp)

Add to your `~/.vimrc`:

```vim
if executable('ahoy-lsp')
  au User lsp_setup call lsp#register_server({
    \ 'name': 'ahoy-lsp',
    \ 'cmd': {server_info->['ahoy-lsp']},
    \ 'allowlist': ['ahoy'],
    \ })
endif

" Set filetype for .ahoy files
au BufRead,BufNewFile *.ahoy set filetype=ahoy
```

### Emacs (with lsp-mode)

Add to your `~/.emacs.d/init.el`:

```elisp
(require 'lsp-mode)

(add-to-list 'lsp-language-id-configuration '(ahoy-mode . "ahoy"))

(lsp-register-client
 (make-lsp-client :new-connection (lsp-stdio-connection "ahoy-lsp")
                  :activation-fn (lsp-activate-on "ahoy")
                  :server-id 'ahoy-lsp))

;; Auto-start LSP for .ahoy files
(add-to-list 'auto-mode-alist '("\\.ahoy\\'" . ahoy-mode))
(add-hook 'ahoy-mode-hook #'lsp)
```

## Features

### ✅ Currently Working

- **Real-time Diagnostics** - Syntax errors appear as you type
- **Autocomplete** - Keywords and operators (triggered by typing)
- **Document Sync** - Changes are tracked automatically

### 🚧 Coming Soon

- **Go-to-Definition** - Jump to variable/function definitions
- **Hover Information** - See type info and docs on hover
- **Document Outline** - Tree view of functions and variables
- **Find References** - Find all usages of a symbol
- **Rename Refactoring** - Rename across entire file

## Testing the LSP

### Test 1: Syntax Error Detection

1. Open any `.ahoy` file in your editor
2. Type some invalid syntax, e.g.:
   ```ahoy
   func test
       ? This is missing 'do'
       ahoy "test"
   ```
3. You should see a red squiggly underline with an error message

### Test 2: Autocomplete

1. Start typing a keyword:
   ```ahoy
   fu
   ```
2. Press your editor's autocomplete key (usually Ctrl+Space)
3. You should see suggestions like `func`, `for`, etc.

### Test 3: Manual LSP Test

You can test the LSP server directly:

```bash
# Start the server
./ahoy-lsp

# In another terminal, send a test request
cat << 'EOF' | nc localhost 9999  # (if using TCP)
Content-Length: 123

{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":"file:///tmp","capabilities":{}}}
EOF
```

Or use an LSP testing tool:

```bash
# Install lsp-devtools
pip install lsp-devtools

# Test the server
lsp-devtools client --server-command "ahoy-lsp"
```

## Troubleshooting

### LSP not starting

**Check if binary is in PATH:**
```bash
which ahoy-lsp
```

**Check Zed logs:**
```bash
tail -f ~/.config/zed/logs/*.log
```

**Check Neovim LSP logs:**
```vim
:lua vim.lsp.set_log_level("debug")
:lua print(vim.lsp.get_log_path())
```
Then open the log file.

### No diagnostics appearing

1. Verify the file is recognized as Ahoy (check status bar)
2. Check that LSP is running (in Neovim: `:LspInfo`)
3. Try saving the file to trigger a refresh
4. Check LSP logs for errors

### Autocomplete not working

1. Make sure you're triggering it correctly:
   - VS Code: Ctrl+Space
   - Zed: Ctrl+Space
   - Neovim: Ctrl+X Ctrl+O (or Ctrl+Space with nvim-cmp)
2. Try typing a few letters of a keyword first
3. Check that LSP is connected (see above)

### Building LSP fails

Make sure you have Go 1.20+ installed:
```bash
go version
```

If you get import errors, try:
```bash
cd /path/to/ahoy/lsp
rm -rf go.mod go.sum
go mod init ahoy/lsp
echo 'replace ahoy => ../' >> go.mod
go mod tidy
go build -o ahoy-lsp .
```

## Debugging

### Enable verbose logging in Zed

1. Open Zed settings (Cmd+,)
2. Add to settings.json:
   ```json
   {
     "lsp": {
       "ahoy-lsp": {
         "log_level": "debug"
       }
     }
   }
   ```

### Test LSP communication

The LSP uses JSON-RPC over stdio. You can capture the communication:

```bash
# Wrap ahoy-lsp to log all I/O
cat > /tmp/ahoy-lsp-wrapper.sh << 'EOF'
#!/bin/bash
tee -a /tmp/lsp-input.log | ahoy-lsp | tee -a /tmp/lsp-output.log
EOF

chmod +x /tmp/ahoy-lsp-wrapper.sh

# Update your editor config to use /tmp/ahoy-lsp-wrapper.sh instead of ahoy-lsp
# Then check the log files
tail -f /tmp/lsp-input.log /tmp/lsp-output.log
```

## Development

Want to add features to the LSP? See:
- `lsp/README.md` - Architecture and development guide
- `parser.go` - AST structure for traversing code
- `tokenizer.go` - Token types for syntax analysis

The main areas to implement:
1. **Symbol Table** - Track all definitions in scope
2. **Go-to-Definition** - Use symbol table to find definitions
3. **Hover** - Show type information
4. **Semantic Tokens** - Enhanced syntax highlighting
5. **Code Actions** - Quick fixes

## Further Reading

- [LSP Specification](https://microsoft.github.io/language-server-protocol/)
- [go.lsp.dev documentation](https://pkg.go.dev/go.lsp.dev)
- [gopls source](https://github.com/golang/tools/tree/master/gopls) - Reference implementation