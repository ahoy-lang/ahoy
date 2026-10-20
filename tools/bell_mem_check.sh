#!/bin/bash
# tools/bell_mem_check.sh — Bell game-loop + scene-load memory gate.
#
# Regenerates the balls game C with the working-tree compiler, builds it
# with AddressSanitizer, runs 90 frames headless (xvfb + DEMO_SHOT) and
# fails on any memory ERROR (use-after-free, double-free, overflow,
# SEGV). Exit-time *leaks* are reported but do not fail: they are
# load/boot-bounded teardown omissions tracked in
# docs/MEMORY_BASELINE.md (no site scales with frame count).
#
# Requires: go, gcc, valgrind-free ASan runtime, xvfb-run, the vendored
# raylib/SDL3/box2d/box3d archives. Takes several minutes (3.8MB of C).
#
# Usage: run from the ahoy repo:
#   bash tools/bell_mem_check.sh

set -u
AHOY="$(cd "$(dirname "$0")/.." && pwd)"
BELL="$(cd "$AHOY/../bell" && pwd)"
WORK="${BELL_MEM_WORK:-/tmp/opencode/bellmem}"
COMPILER="$WORK/ahoy-bin"
GAME_C="$WORK/main.c"
GAME_BIN="$WORK/balls_asan"
RUNLOG="$WORK/run.log"

RL="$AHOY/repos/raylib/src"
SDL="$BELL/repos/SDL3"
B2="$BELL/repos/box2d/include"
B3="$BELL/repos/box3d/include"

mkdir -p "$WORK"
echo "== build compiler =="
(cd "$AHOY/source" && go build -o "$COMPILER" .) || exit 1
echo "== generate game C =="
(cd "$WORK" && "$COMPILER" -f "$BELL/games/balls/main.ahoy" -D BACKEND_RAYLIB -D DEMO_SHOT) || exit 1
mv "$WORK/output/main.c" "$GAME_C" 2>/dev/null || cp "$WORK/output/main.c" "$GAME_C" || exit 1
echo "== build ASan game =="
gcc -g -fsanitize=address -o "$GAME_BIN" "$GAME_C" \
  -I"$RL" -I"$SDL/include" -I"$B2" -I"$B3" -I"$BELL/engine" \
  -DBACKEND_RAYLIB -DDEMO_SHOT \
  "$RL/libraylib.a" "$SDL/build/libSDL3.a" \
  "$BELL/repos/box2d/lib/linux/libbox2d.a" \
  "$BELL/repos/box3d/lib/linux/libbox3d.a" \
  -lGL -lm -lpthread -ldl -lrt -lX11 -lXext -lXrandr -lXcursor -lXi -lXinerama -lXxf86vm -lXfixes || exit 1
echo "== run 90 frames headless =="
(cd "$BELL" && timeout 100 xvfb-run -a "$GAME_BIN" > "$RUNLOG" 2>&1)
echo "== verdict =="
if grep -q "heap-use-after-free\|double-free\|heap-buffer-overflow\|SEGV\|AddressSanitizer: .*error" "$RUNLOG"; then
  grep -E "ERROR|SUMMARY" "$RUNLOG" | head -5
  echo "bell_mem_check: FAIL (memory errors)"
  exit 1
fi
grep -a "closed after\|SUMMARY" "$RUNLOG" | head -3
echo "bell_mem_check: PASS (no memory errors)"
