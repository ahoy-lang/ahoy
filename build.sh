#!/bin/bash
# Build the Ahoy compiler

echo "Building Ahoy compiler..."
cd source && go build -o ../ahoy-bin .

if [ $? -eq 0 ]; then
    echo "✓ Built ahoy-bin successfully"
    echo ""
    echo "Usage:"
    echo "  ./ahoy-bin -f input/test.ahoy -r"
    echo ""
    echo "Or from source directory:"
    echo "  cd source && go run . -f ../input/test.ahoy -r"
else
    echo "✗ Build failed"
    exit 1
fi
