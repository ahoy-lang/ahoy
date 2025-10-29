# Output Testing Feature - Quick Start

## What Is This?

A testing framework that compiles Ahoy programs, runs them, captures their output, and compares it against expected results. This enables automated regression testing to ensure programs produce correct output.

## Quick Example

### 1. Create a test program

Create `test/input/hello.ahoy`:
```ahoy
name: "World"
ahoy|"Hello, %s!", name|
ahoy "Done"
```

### 2. Generate the test

```bash
cd ahoy
make generate-test FILE=test/input/hello.ahoy
```

Output:
```go
{
    Name:      "Hello",
    InputFile: "../test/input/hello.ahoy",
    ExpectedOutput: `Hello, World!
Done
`,
}
```

### 3. Add to `source/main_test.go`

Paste the generated code into the `TestProgramOutput` tests array.

### 4. Run the test

```bash
cd source
go test -v -run TestProgramOutput/Hello
```

## Commands

```bash
# Generate test for a file
make generate-test FILE=test/input/yourfile.ahoy

# Run all tests
cd source && go test -v

# Run specific test type
go test -v -run TestProgramOutput
go test -v -run TestProgramOutputFlexible
go test -v -run TestProgramCompilation
```

## Test Types

### Exact Output (for deterministic programs)
```go
{
    Name:      "Math Test",
    InputFile: "../test/input/math.ahoy",
    ExpectedOutput: `Result: 42
`,
}
```

### Flexible Output (for programs with random elements)
```go
{
    Name:      "Random Test",
    InputFile: "../test/input/random.ahoy",
    ExpectedLines: []string{
        "Test started",
        "All tests passed",
    },
}
```

## How It Works

1. **Compile**: Tokenizes, parses, and generates C code from `.ahoy` file
2. **Build**: Compiles C code with `gcc`
3. **Run**: Executes the binary and captures stdout
4. **Compare**: Checks output matches expectations

## Current Test Results

✅ **array_methods_test.ahoy** - Array operations (27 lines output)
✅ **conditionals_test.ahoy** - If/else/switch statements  
⚠️  Some tests have compilation issues (being fixed)

## Use Cases

- **Regression Testing**: Ensure changes don't break existing functionality
- **Feature Verification**: Confirm new features work as expected
- **Documentation**: Tests serve as working examples
- **CI/CD Integration**: Automated testing in build pipelines

## Full Documentation

See `TESTING_FRAMEWORK.md` for complete details on:
- Test file structure
- Advanced configuration
- Best practices
- Troubleshooting
- CI/CD integration
