#!/bin/bash
# Build the Ahoy compiler

echo "Building Ahoy compiler..."
cd source && go build -o ../test/ahoy-bin .

if [ $? -eq 0 ]; then
    echo "✓ Built test/ahoy-bin successfully"
    echo ""
    echo "Usage:"
    echo "  ./test/ahoy-bin -f input/test.ahoy -r"
    echo ""
    echo "Or from source directory:"
    echo "  cd source && go run . -f ../test/input/test.ahoy -r"
else
    echo "✗ Build failed"
    exit 1
fi
