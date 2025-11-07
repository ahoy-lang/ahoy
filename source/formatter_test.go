package main

import (
	"os"
	"strings"
	"testing"
)

func TestFormatterBasicIndentation(t *testing.T) {
	input := `greet :: |name:string|:
ahoy|"Hello"|
end`

	expected := `greet :: |name:string|:
    ahoy|"Hello"|
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Basic indentation failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterIfStatement(t *testing.T) {
	input := `check :: |num:int|:
if num > 0 then
ahoy|"Positive"|
end
end`

	expected := `check :: |num:int|:
    if num > 0 then
        ahoy|"Positive"|
    end
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("If statement formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterIfElseIfElse(t *testing.T) {
	input := `classify :: |num:int|:
if num > 0 then
ahoy|"Positive"|
elseif num < 0 then
ahoy|"Negative"|
else
ahoy|"Zero"|
end
end`

	expected := `classify :: |num:int|:
    if num > 0 then
        ahoy|"Positive"|
    elseif num < 0 then
        ahoy|"Negative"|
    else
        ahoy|"Zero"|
    end
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("If/elseif/else formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterLoop(t *testing.T) {
	input := `count :: |max:int|:
loop i to max do
ahoy|i|
end
end`

	expected := `count :: |max:int|:
    loop i to max do
        ahoy|i|
    end
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Loop formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterSwitch(t *testing.T) {
	input := `test_switch :: |value:int|:
switch value on
1: ahoy|"One"|
2: ahoy|"Two"|
_: ahoy|"Other"|
end
end`

	expected := `test_switch :: |value:int|:
    switch value on
        1: ahoy|"One"|
        2: ahoy|"Two"|
        _: ahoy|"Other"|
    end
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Switch formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterNested(t *testing.T) {
	input := `nested :: |x:int|:
if x > 0 then
loop i to x do
ahoy|i|
end
end
end`

	expected := `nested :: |x:int|:
    if x > 0 then
        loop i to x do
            ahoy|i|
        end
    end
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Nested blocks formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterSingleLineIf(t *testing.T) {
	input := `quick :: |value:int|:
if value > 0 then ahoy|"Positive"|
end`

	expected := `quick :: |value:int|:
    if value > 0 then ahoy|"Positive"|
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Single-line if formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterSingleLineLoop(t *testing.T) {
	input := `quick :: |max:int|:
loop i to max do ahoy|i|
end`

	expected := `quick :: |max:int|:
    loop i to max do ahoy|i|
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Single-line loop formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterEnum(t *testing.T) {
	input := `enum Color:
RED
GREEN
BLUE
end`

	expected := `enum Color:
    RED
    GREEN
    BLUE
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Enum formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterStruct(t *testing.T) {
	input := `struct Point:
x: float
y: float
end`

	expected := `struct Point:
    x: float
    y: float
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Struct formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterStructWithType(t *testing.T) {
	input := `struct Particle:
pos: Vector2
vel: Vector2
type smoke:
alpha: float
size: float
end`

	expected := `struct Particle:
    pos: Vector2
    vel: Vector2
    type smoke:
        alpha: float
        size: float
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Struct with type variant formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterWhen(t *testing.T) {
	input := `when DEBUG then
ahoy|"Debug mode"|
end`

	expected := `when DEBUG then
    ahoy|"Debug mode"|
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("When statement formatting failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterPreservesComments(t *testing.T) {
	input := `# This is a comment
greet :: |name:string|:
? This is also a comment
ahoy|"Hello"|
end`

	expected := `# This is a comment
greet :: |name:string|:
    ? This is also a comment
    ahoy|"Hello"|
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Comment preservation failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterPreservesEmptyLines(t *testing.T) {
	input := `greet :: |name:string|:
ahoy|"Hello"|
end

add :: |a:int, b:int| int:
return a + b
end`

	expected := `greet :: |name:string|:
    ahoy|"Hello"|
end

add :: |a:int, b:int| int:
    return a + b
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Empty line preservation failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestFormatterComplexFile(t *testing.T) {
	// Read the unindented test file
	input, err := os.ReadFile("testdata/formatter/test_unindented.ahoy")
	if err != nil {
		t.Skipf("Skipping test - test file not found: %v", err)
		return
	}

	// Read the expected formatted output
	expectedBytes, err := os.ReadFile("testdata/formatter/test_expected.ahoy")
	if err != nil {
		t.Skipf("Skipping test - expected file not found: %v", err)
		return
	}

	expected := string(expectedBytes)
	result := formatSource(string(input))

	if result != expected {
		// Show line-by-line diff for easier debugging
		expectedLines := strings.Split(expected, "\n")
		resultLines := strings.Split(result, "\n")

		maxLines := len(expectedLines)
		if len(resultLines) > maxLines {
			maxLines = len(resultLines)
		}

		t.Errorf("Complex file formatting failed. Line-by-line diff:")
		for i := 0; i < maxLines; i++ {
			var expLine, resLine string
			if i < len(expectedLines) {
				expLine = expectedLines[i]
			}
			if i < len(resultLines) {
				resLine = resultLines[i]
			}

			if expLine != resLine {
				t.Errorf("Line %d differs:\nExpected: %q\nGot:      %q", i+1, expLine, resLine)
			}
		}
	}
}

func TestFormatterIdempotent(t *testing.T) {
	input := `greet :: |name:string|:
    ahoy|"Hello"|
end
`

	// Formatting already-formatted code should not change it
	result := formatSource(input)
	if result != input {
		t.Errorf("Formatter is not idempotent.\nInput:\n%s\nOutput:\n%s", input, result)
	}
}

func TestFormatterTabsToSpaces(t *testing.T) {
	input := "greet :: |name:string|:\n\tahoy|\"Hello\"|\nend"
	expected := `greet :: |name:string|:
    ahoy|"Hello"|
end
`

	result := formatSource(input)
	if result != expected {
		t.Errorf("Tab conversion failed.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}
