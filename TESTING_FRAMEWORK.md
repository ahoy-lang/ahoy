# Testing Framework Documentation

## Overview

The Ahoy compiler now includes a comprehensive testing framework that compiles `.ahoy` files to C, runs them, and compares their output against expected results. This allows for automated regression testing and verification that programs produce correct output.

## Test Types

### 1. Exact Output Matching (`TestProgramOutput`)

Tests that require exact output matching. Best for deterministic programs without random elements.

```go
{
    Name:      "Test Name",
    InputFile: "../test/input/your_file.ahoy",
    ExpectedOutput: `exact expected output here
including newlines
`,
}
```

### 2. Flexible Output Matching (`TestProgramOutputFlexible`)

Tests that check for specific lines or patterns in output. Best for programs with non-deterministic elements (random numbers, etc.).

```go
{
    Name:      "Test Name",
    InputFile: "../test/input/your_file.ahoy",
    ExpectedLines: []string{
        "Line that must appear",
        "Another required line",
    },
    ForbiddenLines: []string{
        "Line that should NOT appear",
    },
    ExpectedLineCount: 10, // Optional: check number of output lines
}
```

### 3. Compilation Tests (`TestProgramCompilation`)

Tests that just verify files compile and run without errors, without checking output.

## Usage

### Running Tests

```bash
# Run all tests
cd ahoy/source && go test -v

# Run specific test suite
go test -v -run TestProgramOutput
go test -v -run TestProgramOutputFlexible
go test -v -run TestProgramCompilation

# Run a specific test case
go test -v -run TestProgramOutput/ArrayMethods
```

### Generating Test Cases

Use the `generate_tests.go` tool to automatically capture output from an Ahoy program and generate test code:

```bash
# Method 1: Using Makefile
cd ahoy
make generate-test FILE=test/input/your_file.ahoy

# Method 2: Direct command
cd ahoy/source
go run generate_tests.go tokenizer.go parser.go codegen.go formatter.go ../test/input/your_file.ahoy
```

This will:
1. Compile the `.ahoy` file to C
2. Compile the C code with gcc
3. Run the executable
4. Capture the output
5. Generate Go test code you can paste into `main_test.go`

### Example Workflow

1. **Create an Ahoy program**: `test/input/my_feature.ahoy`

2. **Generate test code**:
   ```bash
   make generate-test FILE=test/input/my_feature.ahoy
   ```

3. **Copy the generated code** into `TestProgramOutput` in `main_test.go`

4. **Run the test**:
   ```bash
   cd source && go test -v -run TestProgramOutput/MyFeature
   ```

## Test File Structure

```
ahoy/
├── source/
│   ├── main_test.go          # Main test file
│   ├── generate_tests.go     # Test generation tool
│   └── ...
└── test/
    ├── input/                # Ahoy source files
    │   ├── array_methods_test.ahoy
    │   ├── conditionals_test.ahoy
    │   └── ...
    └── output/               # Generated C files and executables (auto-created)
        ├── array_methods_test.c
        ├── array_methods_test (executable)
        └── ...
```

## Example Test Cases

### Exact Match Test
```go
{
    Name:      "Simple Math",
    InputFile: "../test/input/simple_math.ahoy",
    ExpectedOutput: `Result: 42
Done
`,
}
```

### Flexible Match Test  
```go
{
    Name:      "Random Numbers",
    InputFile: "../test/input/random_test.ahoy",
    ExpectedLines: []string{
        "Test started",
        "Test completed",
    },
    ExpectedLineCount: 10,
}
```

## Current Test Status

As of implementation:
- ✅ **Array Methods**: Tests array operations (with random-tolerant matching)
- ✅ **Conditionals**: Tests if/else/switch statements
- ⚠️  **Loops**: Some compilation issues with array implementation
- ⚠️  **Break/Skip**: Some compilation issues with array implementation

## Continuous Integration

Tests can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Run Ahoy Tests
  run: |
    cd ahoy/source
    go test -v
```

## Adding New Tests

1. Create your `.ahoy` file in `test/input/`
2. Run `make generate-test FILE=test/input/yourfile.ahoy`
3. Add the generated code to the appropriate test function in `main_test.go`
4. Commit both the `.ahoy` file and updated test

## Best Practices

1. **Use descriptive test names** that explain what's being tested
2. **Keep test files focused** on specific features
3. **Use flexible matching** for programs with random or system-dependent output
4. **Add comments** explaining why certain lines are expected or forbidden
5. **Test edge cases** and error conditions
6. **Keep expected output up-to-date** when language features change

## Troubleshooting

### Test fails with "compilation failed"
- Check that the `.ahoy` file compiles with `./ahoy-bin -f test/input/yourfile.ahoy`
- Review C compilation errors in the test output
- May indicate codegen bug or unsupported feature

### Test fails with output mismatch
- Check for trailing whitespace or newline differences
- Consider using flexible matching if output has variable parts
- Regenerate expected output with `make generate-test`

### Random output causes failures
- Use `TestProgramOutputFlexible` instead
- Add only non-random lines to `ExpectedLines`
- Avoid exact output matching for random elements

## Future Enhancements

Planned improvements to the testing framework:
- JSON output comparison for structured data
- Performance benchmarking tests
- Memory leak detection
- Code coverage reporting
- Parallel test execution
- Test result caching
