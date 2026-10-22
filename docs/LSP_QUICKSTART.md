# Ahoy LSP Quick Start Guide

Get the Ahoy Language Server running in your editor in 5 minutes!

## Step 1: Install the LSP Server (2 minutes)

```bash
cd /path/to/ahoy-lang/ahoy-lsp
make install
```

This creates `~/.local/bin/ahoy-lsp` as a symlink to the repository binary, so
future builds update the command automatically. Do not copy the binary; a copy
goes stale on the next build.

Verify installation:
```bash
which ahoy-lsp
readlink -f "$(which ahoy-lsp)"
ahoy-lsp --version
```

## Step 2: Set Up Your Editor (1 minute)

### Zed Editor

1. Rebuild the extension:
   ```bash
   cd /path/to/zed-ahoy
   cargo build --release
   ```

2. Restart Zed

3. Open any `.ahoy` file - LSP should activate automatically!

### VS Code

1. Install the Ahoy extension (being developed)
2. Restart VS Code
3. Open any `.ahoy` file

### Neovim

Add to `~/.config/nvim/init.lua`:

```lua
require('lspconfig').ahoy_lsp.setup{
  cmd = { 'ahoy-lsp' },
  filetypes = { 'ahoy' },
}
```

Create `~/.config/nvim/ftdetect/ahoy.vim`:

```vim
au BufRead,BufNewFile *.ahoy set filetype=ahoy
```

Restart Neovim.

## Step 3: Test It! (2 minutes)

Create a test file `test.ahoy`:

```ahoy
? This has an error - missing 'do'
func broken_function
    ahoy "This should show an error!"
end

? This is correct
func working_function do
    x: 42
    ahoy "x is: " x
end

? Test autocomplete - type "fu" then press Ctrl+Space
? Test go-to-definition - Ctrl+Click on 'x' below
result: x

? Test hover - hover your mouse over 'working_function'
```

Open this file in your editor:

### What You Should See:

1. **Red underline** on line 2 (missing 'do' error)
2. Type `fu` + **Ctrl+Space** → autocomplete shows "func"
3. **Ctrl+Click** on `x` on line 12 → jumps to line 9
4. **Hover** over `working_function` → shows "Function working_function, Defined at line 7"
5. **Ctrl+.** on error line → suggests "Add 'do' keyword"

## Features Available

✅ **Diagnostics** - Real-time syntax error checking
✅ **Autocomplete** - Keywords, operators, types
✅ **Go-to-Definition** - Jump to variable/function definitions
✅ **Hover Info** - Type information and documentation
✅ **Outline View** - See all functions/variables (Ctrl+Shift+O in Zed/VS Code)
✅ **Quick Fixes** - Automatic error corrections (Ctrl+.)

## Troubleshooting

### LSP Not Starting?

```bash
# Check binary exists
which ahoy-lsp

# Test manually (Ctrl+C to exit)
ahoy-lsp

# Check editor logs
# Zed: ~/.config/zed/logs/
# Neovim: :lua print(vim.lsp.get_log_path())
```

### No Diagnostics?

1. Make sure file extension is `.ahoy`
2. Save the file
3. Check status bar shows "ahoy-lsp" is connected

### Autocomplete Not Working?

1. Type at least 1-2 characters
2. Press **Ctrl+Space** to trigger
3. Try typing "fu" - should suggest "func"

## More Information

- **Full Setup Guide:** `LSP_SETUP.md`
- **Testing Guide:** `lsp/TESTING.md`
- **LSP README:** `lsp/README.md`
- **Implementation Details:** `LSP_IMPLEMENTATION_SUMMARY.md`

## What's Next?

You're all set! The LSP provides:

- 🔴 **Errors as you type** - catch syntax mistakes immediately
- 💡 **Smart suggestions** - autocomplete keywords and operators
- 🔍 **Navigate easily** - jump to definitions with one click
- 📖 **Inline docs** - hover to see type info
- 🔧 **Quick fixes** - automatic error corrections
- 📋 **Code outline** - see document structure at a glance

Start writing Ahoy code and enjoy your enhanced IDE experience! 🚢

## Keyboard Shortcuts

| Action | Zed/VS Code | Neovim |
|--------|-------------|--------|
| Autocomplete | Ctrl+Space | Ctrl+X Ctrl+O |
| Go-to-Definition | Ctrl+Click or F12 | gd |
| Hover Info | Hover mouse | K |
| Quick Fix | Ctrl+. | <leader>ca |
| Outline View | Ctrl+Shift+O | :SymbolsOutline |

Happy coding! 🎉