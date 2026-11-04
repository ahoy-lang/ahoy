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

# Portable scratch dir for the compiled leak binaries. The old script used
# a hardcoded /tmp/opencode path which only exists on one dev machine, so
# fresh GitHub runners failed with:
#   ld: cannot open output file /tmp/opencode/memcheck_*: No such file or directory
# Use mktemp (with a $$ fallback) under $TMPDIR so this works anywhere.
MEMTMP="$(mktemp -d "${TMPDIR:-/tmp}/ahoy-memcheck.XXXXXX" 2>/dev/null || echo "${TMPDIR:-/tmp}/ahoy-memcheck-$$")"
mkdir -p "$OUT" "$MEMTMP" || { echo "cannot create scratch dirs"; exit 1; }
trap 'rm -rf "$MEMTMP"' EXIT

command -v valgrind >/dev/null 2>&1 || { echo "valgrind not found; install valgrind"; exit 1; }

cd "$SRC" || exit 1
go build -o "$BIN" . || { echo "compiler build failed"; exit 1; }

fail=0
for prog in "$TESTDIR"/input/leak_*.ahoy; do
    [ -e "$prog" ] || { echo "no leak_*.ahoy inputs found"; fail=1; break; }
    base="$(basename "$prog" .ahoy)"
    cfile="$OUT/$base.c"
    exe="$MEMTMP/memcheck_$base"
    echo "== $base =="
    "$BIN" -single-tu -f "$prog" >/dev/null 2>&1 || { echo "  ahoy compile FAILED"; fail=1; continue; }
    if [ ! -f "$cfile" ]; then echo "  missing $cfile"; fail=1; continue; fi
    rm -f "$exe"
    gcc -O1 -g "$cfile" -o "$exe" -lm 2>&1 | head -5
    if [ ! -x "$exe" ]; then echo "  gcc FAILED (no binary produced)"; fail=1; continue; fi
    out="$("$exe")" || { echo "  run FAILED"; fail=1; continue; }
    echo "  output tail: $(echo "$out" | tail -1)"
    # Note: the leak-kind option is --errors-for-leak-kinds, not
    # --errors-for-leaks. The old spelling made valgrind exit before the
    # run ("Unknown option"), and the empty report was scored as clean.
    # Keep the explicit verdict grep as a second line of defence.
    report="$(valgrind --leak-check=full --show-leak-kinds=definite,indirect \
        --errors-for-leak-kinds=definite,indirect --error-exitcode=99 \
        "$exe" 2>&1)"
    vg_status=$?
    echo "$report" | tail -20 | grep -E "total heap usage|in use at exit|definitely lost|indirectly lost|ERROR SUMMARY"
    if [ "$vg_status" -eq 99 ]; then
        echo "  LEAK DETECTED"
        fail=1
    elif [ "$vg_status" -ne 0 ]; then
        # A valgrind invocation failure must never look like a pass.
        echo "  valgrind FAILED (status $vg_status)"
        echo "$report" | tail -5
        fail=1
    elif echo "$report" | grep -qE "definitely lost: [1-9]|indirectly lost: [1-9]"; then
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
