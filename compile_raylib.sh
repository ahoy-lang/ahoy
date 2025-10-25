#!/bin/bash

# PyLang Compiler Script with Raylib Support
# Usage: ./compile_raylib.sh <source_file.py>

if [ $# -eq 0 ]; then
    echo "Usage: $0 <source_file.py>"
    exit 1
fi

SOURCE_FILE="$1"
BASE_NAME=$(basename "${SOURCE_FILE%.py}")
C_FILE="output/${BASE_NAME}.c"
EXECUTABLE="output/${BASE_NAME}"

# Raylib paths
RAYLIB_SRC="./repos/raylib/src"

echo "Compiling ${SOURCE_FILE} to C..."
go run . "${SOURCE_FILE}"

if [ $? -eq 0 ]; then
    echo "Compiling C code with Raylib to executable..."
    
    # Copy raylib.h to a temporary location or use direct path
    gcc -o "${EXECUTABLE}" "${C_FILE}" \
        -I"${RAYLIB_SRC}" \
        "${RAYLIB_SRC}/libraylib.a" \
        -lGL -lm -lpthread -ldl -lrt -lX11
    
    if [ $? -eq 0 ]; then
        echo "Successfully compiled to ${EXECUTABLE}"
        echo "Run with: ${EXECUTABLE}"
    else
        echo "Failed to compile C code"
        exit 1
    fi
else
    echo "Failed to compile ${SOURCE_FILE}"
    exit 1
fi