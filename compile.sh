#!/bin/bash

# PyLang Compiler Script
# Usage: ./compile.sh <source_file.py>

if [ $# -eq 0 ]; then
    echo "Usage: $0 <source_file.py>"
    exit 1
fi

SOURCE_FILE="$1"
BASE_NAME=$(basename "${SOURCE_FILE%.py}")
C_FILE="output/${BASE_NAME}.c"
EXECUTABLE="output/${BASE_NAME}"

echo "Compiling ${SOURCE_FILE} to C..."
go run . "${SOURCE_FILE}"

if [ $? -eq 0 ]; then
    echo "Compiling C code to executable..."
    gcc -o "${EXECUTABLE}" "${C_FILE}" -lm
    
    if [ $? -eq 0 ]; then
        echo "Successfully compiled to ${EXECUTABLE}"
        echo "Running program:"
        echo "=================="
        ./"${EXECUTABLE}"
        echo "=================="
        echo "Program finished with exit code: $?"
    else
        echo "Failed to compile C code"
        exit 1
    fi
else
    echo "Failed to compile ${SOURCE_FILE}"
    exit 1
fi