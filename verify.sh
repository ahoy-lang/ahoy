#!/bin/bash

# Verification script for PyLang compiler
echo "=========================================="
echo "PyLang Compiler Verification"
echo "=========================================="
echo ""

# Run unit tests
echo "1. Running Go unit tests..."
go test -v
if [ $? -ne 0 ]; then
    echo "❌ Unit tests failed"
    exit 1
fi
echo "✅ Unit tests passed"
echo ""

# Test simple compilation
echo "2. Testing simple.py compilation..."
./compile.sh simple.py > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Simple compilation failed"
    exit 1
fi
echo "✅ Simple compilation succeeded"
echo ""

# Test full program
echo "3. Testing test.py (fibonacci, loops, conditionals)..."
./compile.sh test.py > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Full test compilation failed"
    exit 1
fi
echo "✅ Full test compilation succeeded"
echo ""

# Test raylib compilation (without running - requires display)
echo "4. Testing raylib_test.py compilation..."
go run . raylib_test.py > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Raylib test compilation failed"
    exit 1
fi
gcc -o output/raylib_test output/raylib_test.c \
    -I./repos/raylib/src -L./repos/raylib/src \
    -lraylib -lGL -lm -lpthread -ldl -lrt -lX11 > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Raylib C compilation failed"
    exit 1
fi
echo "✅ Raylib integration succeeded"
echo ""

echo "=========================================="
echo "All tests passed! ✅"
echo "=========================================="
echo ""
echo "PyLang features verified:"
echo "  ✓ Tokenizer/Lexer"
echo "  ✓ Parser with AST generation"
echo "  ✓ C code generation"
echo "  ✓ Type inference with :="
echo "  ✓ Python-like syntax (indentation, 'then', 'is', 'not', 'or', 'and')"
echo "  ✓ Control flow (if/else, while, for)"
echo "  ✓ Functions with recursion"
echo "  ✓ Boolean operations"
echo "  ✓ Arithmetic operations"
echo "  ✓ C library imports"
echo "  ✓ Raylib graphics library integration"
echo ""