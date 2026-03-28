#!/bin/bash
# Build stealth_complement.js from modular JS files.
# Files are concatenated in alphabetical order, wrapped in IIFE.
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
OUT="$DIR/../stealth_complement.js"

echo "(() => {" > "$OUT"
for f in "$DIR"/[0-9]*.js; do
  echo "" >> "$OUT"
  echo "  // === $(basename "$f") ===" >> "$OUT"
  sed 's/^/  /' "$f" >> "$OUT"
done
echo "" >> "$OUT"
echo "})();" >> "$OUT"

echo "Built $(wc -l < "$OUT") lines → $OUT"
