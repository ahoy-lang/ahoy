#!/bin/bash
# tools/mem_check.sh — unified memory plan, Phase 7 (part 1).
#
# Builds every ahoy/test/input/leak_*.ahoy program with gcc and runs it
# under valgrind, failing on any definitely/indirectly lost byte.
# Functional correctness is covered by `go test ./...` (the leak inputs
# assert their own output); this script covers ownership.
#
# Usage: run from the ahoy repo:
#   bash tools/mem_check.sh
# Requires: go, gcc, valgrind.

set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/source"
TESTDIR="$ROOT/test"
OUT="$TESTDIR/output"
BIN="$OUT/ahoy-memcheck-bin"

cd "$SRC" || exit 1
go build -o "$BIN" . || { echo "compiler build failed"; exit 1; }

fail=0
for prog in "$TESTDIR"/input/leak_*.ahoy; do
    base="$(basename "$prog" .ahoy)"
    cfile="$OUT/$base.c"
    exe="/tmp/opencode/memcheck_$base"
    echo "== $base =="
    "$BIN" -f "$prog" >/dev/null 2>&1 || { echo "  ahoy compile FAILED"; fail=1; continue; }
    gcc -O1 -g "$cfile" -o "$exe" -lm 2>&1 | head -5
    out="$("$exe")" || { echo "  run FAILED"; fail=1; continue; }
    echo "  output tail: $(echo "$out" | tail -1)"
    report="$(valgrind --leak-check=full --errors-for-leaks=yes "$exe" 2>&1 | tail -20)"
    echo "$report" | grep -E "total heap usage|in use at exit|definitely lost|indirectly lost|ERROR SUMMARY"
    if echo "$report" | grep -qE "definitely lost: [1-9]|indirectly lost: [1-9]"; then
        echo "  LEAK DETECTED"
        fail=1
    else
        echo "  clean"
    fi
    rm -f "$exe"
done

if [ "$fail" -ne 0 ]; then
    echo "mem_check: FAIL"
    exit 1
fi
echo "mem_check: PASS"
