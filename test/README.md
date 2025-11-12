# Ahoy Formatter Tests

This directory contains tests for the Ahoy code formatter.

## Directory Structure

```
test/
├── fmt_input/          # Input files (unformatted or badly formatted)
├── fmt_output/         # Expected output files (properly formatted)
└── fmt_test.go         # Test runner
```

## Running Tests

```bash
# Run all formatter tests
cd /home/lee/Documents/ahoy-lang/ahoy/test
go test -v

# Run specific test
go test -v -run TestFormatter/basic

# Run with benchmarks
go test -v -bench=.

# Run only the end-to-end test
go test -v -run TestFormatterEndToEnd
```

## Test Cases

Current test cases:

1. **basic** - Basic formatting (functions, variables, if statements)
2. **loops** - Loop formatting (range loops, till loops, array loops)
3. **function_double_colon** - Function declarations with `::` syntax
4. **switch** - Switch statement formatting
5. **comments** - Comment handling (inline and full-line)

## Adding New Tests

1. Create input file in `fmt_input/`:
   ```bash
   echo "? Test case" > fmt_input/my_test.ahoy
   ```

2. Create expected output in `fmt_output/`:
   ```bash
   echo "? Test case" > fmt_output/my_test.ahoy
   ```

3. Add test case to `fmt_test.go`:
   ```go
   {
       Name:     "my_test",
       Input:    "fmt_input/my_test.ahoy",
       Expected: "fmt_output/my_test.ahoy",
   },
   ```

4. Run the test:
   ```bash
   go test -v -run TestFormatter/my_test
   ```

## Test Types

### TestFormatter
- Main test suite
- Tests each case individually
- Shows detailed diffs on failure
- Writes temporary output files for inspection

### TestFormatterEndToEnd
- Runs all tests with summary
- Shows pass/fail counts
- Good for quick overview

### TestFormatterIdempotent
- Tests that formatting is idempotent
- Ensures that formatting an already-formatted file doesn't change it
- Important for editor integration

### BenchmarkFormatter
- Performance benchmarking
- Measures formatter speed

## Current Issues

The formatter currently has limitations:

1. **Indentation**: Doesn't handle indentation correctly
   - Bodies of functions, loops, if statements not indented
   - Needs AST-based indentation logic

2. **Spacing**: Inconsistent spacing around operators
   - `x:5` should be `x: 5`

3. **Comments**: Leading whitespace on comments not handled

## Expected Behavior

Formatted code should have:

- 4 spaces per indentation level
- Space after `:` in assignments (`x: 5` not `x:5`)
- Consistent indentation in all blocks
- Trailing whitespace removed
- File ends with newline
- Comments properly aligned

## Temporary Output Files

When tests fail, temporary output files are written to `fmt_output/temp_*.ahoy`
for inspection. These show what the formatter actually produced.

## Integration

This test suite can be:
- Run in CI/CD pipelines
- Used for regression testing
- Benchmarked for performance tracking
- Extended with more edge cases
