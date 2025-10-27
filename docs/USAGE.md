# Ahoy Language - Usage Guide

## Running the Compiler

There are three ways to run the Ahoy compiler:

### 1. Using the Pre-built Binary (Recommended)

First, build the binary:
```bash
./build.sh
```

Then use it:
```bash
# Compile and run
./ahoy-bin -f input/test.ahoy -r

# Just compile
./ahoy-bin -f input/test.ahoy

# Format source
./ahoy-bin -f input/test.ahoy -format

# Show help
./ahoy-bin -h
```

### 2. Using Go Run (From Source Directory)

```bash
cd source

# Compile and run
go run . -f ../input/test.ahoy -r

# Just compile
go run . -f ../input/test.ahoy

# Format source
go run . -f ../input/test.ahoy -format

# Show help
go run . -h
```

**Important**: Use `go run .` (with the dot) NOT `go run main.go`

The dot tells Go to compile all `.go` files in the package together.

### 3. Install System-Wide

```bash
./build.sh
sudo cp ahoy-bin /usr/local/bin/ahoy
# or
mkdir -p ~/bin
cp ahoy-bin ~/bin/ahoy
# (make sure ~/bin is in your PATH)
```

Then use from anywhere:
```bash
ahoy -f myprogram.ahoy -r
```

## Common Commands

```bash
# Compile a program
./ahoy-bin -f input/simple.ahoy

# Compile and run
./ahoy-bin -f input/simple.ahoy -r

# Format a source file
./ahoy-bin -f input/simple.ahoy -format

# Run tests
cd source && go test -v

# Run all examples
./ahoy-bin -f input/simple.ahoy -r
./ahoy-bin -f input/test.ahoy -r
./ahoy-bin -f input/features_test.ahoy -r
./ahoy-bin -f input/showcase.ahoy -r
```

## Troubleshooting

### Error: "undefined: formatSource"

This happens when you run `go run main.go` instead of `go run .`

**Solution**: Always use `go run .` when running from the source directory, or use the pre-built binary `./ahoy-bin`

### Error: "File not found"

Make sure you're running the command from the correct directory:
- `./ahoy-bin` should be run from the project root
- `go run .` should be run from the `source/` directory

### Error: "gcc: command not found"

You need GCC to compile the generated C code.

**Install GCC:**
```bash
# Ubuntu/Debian
sudo apt install build-essential

# macOS
xcode-select --install

# Fedora
sudo dnf install gcc
```

## Examples

See the `input/` directory for example programs:
- `simple.ahoy` - Basic features with semicolons
- `test.ahoy` - Fibonacci and loops
- `features_test.ahoy` - All language features
- `showcase.ahoy` - Comprehensive demo

Ahoy! 🏴‍☠️
