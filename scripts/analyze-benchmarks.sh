#!/usr/bin/env bash
set -e

# analyze-benchmarks.sh
# Analyzes benchmark results and generates a summary

BASE_FILE="${1:-base-bench.txt}"
PR_FILE="${2:-pr-bench.txt}"
OUTPUT_FILE="${3:-summary.txt}"
REGRESSION_THRESHOLD="${4:-10}"
IMPROVEMENT_THRESHOLD="${5:-30}"

# Run benchstat
benchstat "$BASE_FILE" "$PR_FILE" > benchstat-output.txt

echo "## 📊 Benchmark Results" > "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Check for regressions beyond threshold - SHOW FIRST
echo "### 🔴 Regressions Beyond ${REGRESSION_THRESHOLD}% Threshold" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

REGRESSIONS_FOUND=false

while IFS= read -r line; do
  if [[ $line =~ \+([0-9]+\.[0-9]+)% ]]; then
    delta="${BASH_REMATCH[1]}"
    if (( $(echo "$delta > $REGRESSION_THRESHOLD" | bc -l) )); then
      echo "- $line" >> "$OUTPUT_FILE"
      REGRESSIONS_FOUND=true
    fi
  fi
done < benchstat-output.txt

if [ "$REGRESSIONS_FOUND" = false ]; then
  echo "✅ None" >> "$OUTPUT_FILE"
fi
echo "" >> "$OUTPUT_FILE"

# Show significant improvements - SHOW SECOND
echo "### ✅ Significant Improvements (≥${IMPROVEMENT_THRESHOLD}%)" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

IMPROVEMENTS_FOUND=false

while IFS= read -r line; do
  if [[ $line =~ \-([0-9]+\.[0-9]+)% ]]; then
    delta="${BASH_REMATCH[1]}"
    if (( $(echo "$delta >= $IMPROVEMENT_THRESHOLD" | bc -l) )); then
      echo "- $line" >> "$OUTPUT_FILE"
      IMPROVEMENTS_FOUND=true
    fi
  fi
done < benchstat-output.txt

if [ "$IMPROVEMENTS_FOUND" = false ]; then
  echo "None significant (≥${IMPROVEMENT_THRESHOLD}%)" >> "$OUTPUT_FILE"
fi
echo "" >> "$OUTPUT_FILE"

# Show benchstat results in collapsible section - SHOW LAST
echo "<details>" >> "$OUTPUT_FILE"
echo "<summary>📋 Detailed Comparison</summary>" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo '```' >> "$OUTPUT_FILE"
cat benchstat-output.txt >> "$OUTPUT_FILE"
echo '```' >> "$OUTPUT_FILE"
echo "</details>" >> "$OUTPUT_FILE"

# Show summary
cat "$OUTPUT_FILE"
