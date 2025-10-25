# Ahoy Quick Start Guide

## TL;DR

```bash
# Build the compiler
./build.sh

# Run an example
./ahoy-bin -f input/test.ahoy -r
```

Done! 🏴‍☠️

## Common Commands

```bash
# Build once
./build.sh

# Then use the binary for everything:
./ahoy-bin -f input/simple.ahoy -r       # Run simple example
./ahoy-bin -f input/test.ahoy -r         # Run fibonacci test
./ahoy-bin -f input/features_test.ahoy -r # Run features demo
./ahoy-bin -f input/showcase.ahoy -r     # Run complete showcase
```

## Alternative: Run Without Building

If you don't want to build, run directly from source:

```bash
cd source
go run . -f ../input/test.ahoy -r
```

⚠️ **IMPORTANT**: Use `go run .` (with the dot), NOT `go run main.go`

## Why Use the Dot?

`go run .` tells Go to compile all `.go` files in the package together.

`go run main.go` only compiles main.go and will fail with "undefined" errors.

## Error: "undefined: formatSource"?

You ran `go run main.go` instead of `go run .`

**Fix**: Use the binary (`./ahoy-bin`) or run `go run .` from source directory.

## Test Everything Works

```bash
cd source
go test -v
```

All 5 tests should pass.

## Create Your Own Program

1. Create a file: `input/myprogram.ahoy`
2. Write your code (see examples in `input/`)
3. Run it: `./ahoy-bin -f input/myprogram.ahoy -r`

## Example Program

```python
# myprogram.ahoy
import "stdio.h"

x: 42; name: "Ahoy"

ahoy|"Hello from %s!\n", name|
ahoy|"The answer is %d\n", x|
```

Run it:
```bash
./ahoy-bin -f input/myprogram.ahoy -r
```

## See Also

- `README.md` - Full language documentation
- `USAGE.md` - Detailed usage guide  
- `input/` - Example programs

Ahoy! 🏴‍☠️
