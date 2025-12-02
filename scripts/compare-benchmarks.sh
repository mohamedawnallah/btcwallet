#!/usr/bin/env bash
set -e

# compare-benchmarks.sh
# Compare benchmark results using growth pattern consistency analysis
#
# Usage: compare-benchmarks.sh [BASE_FILE] [PR_FILE] [OUTPUT_FILE] [REGRESSION_THRESHOLD] [IMPROVEMENT_THRESHOLD]
#   BASE_FILE (default: base-bench.txt) - baseline benchmark results
#   PR_FILE (default: pr-bench.txt) - PR benchmark results
#   OUTPUT_FILE (default: summary.txt) - output summary file
#   REGRESSION_THRESHOLD (default: 30) - performance regression percentage threshold
#   IMPROVEMENT_THRESHOLD (default: 30) - performance improvement percentage threshold
#
# This script detects statistical significance by analyzing consistency of
# performance changes across multiple input sizes. Real regressions show
# consistent patterns across all input sizes, while noise is inconsistent.

BASE_FILE="${1:-base-bench.txt}"
PR_FILE="${2:-pr-bench.txt}"
OUTPUT_FILE="${3:-summary.txt}"
REGRESSION_THRESHOLD="${4:-30}"
IMPROVEMENT_THRESHOLD="${5:-30}"

# Consistency threshold: coefficient of variation (CV = std_dev / mean)
# Lower CV means more consistent changes across input sizes
CONSISTENCY_THRESHOLD="0.5"

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Run analysis using Python script
analysis_output=$(python3 "$SCRIPT_DIR/analyze-benchmarks.py" \
  "$BASE_FILE" "$PR_FILE" \
  "$REGRESSION_THRESHOLD" "$IMPROVEMENT_THRESHOLD" "$CONSISTENCY_THRESHOLD")

# Generate summary output
echo "## 📊 Benchmark Results (Growth Pattern Analysis)" > "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo "Detection method: Consistency analysis across multiple input sizes" >> "$OUTPUT_FILE"
echo "- **Significant** = Mean delta ≥${REGRESSION_THRESHOLD}% AND consistent (CV <${CONSISTENCY_THRESHOLD}) AND ≥3 input sizes" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Extract regressions
echo "### 🔴 Significant Performance Regressions" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

REGRESSIONS_FOUND=false
regressions=$(echo "$analysis_output" | sed -n '/^REGRESSIONS:/,/^IMPROVEMENTS:/p' | grep -v "^REGRESSIONS:" | grep -v "^IMPROVEMENTS:" | grep -v "^$" || true)

if [ -n "$regressions" ]; then
  while IFS='|' read -r family details count; do
    if [ -n "$family" ]; then
      echo "- **${family}**: ${details} [${count} input sizes]" >> "$OUTPUT_FILE"
      REGRESSIONS_FOUND=true
    fi
  done <<< "$regressions"
fi

if [ "$REGRESSIONS_FOUND" = false ]; then
  echo "👍 None" >> "$OUTPUT_FILE"
fi
echo "" >> "$OUTPUT_FILE"

# Extract improvements
echo "### ✅ Significant Performance Improvements" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

IMPROVEMENTS_FOUND=false
improvements=$(echo "$analysis_output" | sed -n '/^IMPROVEMENTS:/,/^AGGREGATED:/p' | grep -v "^IMPROVEMENTS:" | grep -v "^AGGREGATED:" | grep -v "^$" || true)

if [ -n "$improvements" ]; then
  while IFS='|' read -r family details count; do
    if [ -n "$family" ]; then
      echo "- **${family}**: ${details} [${count} input sizes]" >> "$OUTPUT_FILE"
      IMPROVEMENTS_FOUND=true
    fi
  done <<< "$improvements"
fi

if [ "$IMPROVEMENTS_FOUND" = false ]; then
  echo "ℹ️ None" >> "$OUTPUT_FILE"
fi
echo "" >> "$OUTPUT_FILE"

# Add notes about methodology
echo "---" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo "📝 **Methodology:**" >> "$OUTPUT_FILE"
echo "- Uses growth pattern consistency analysis instead of multiple runs" >> "$OUTPUT_FILE"
echo "- Real regressions show consistent % change across ALL input sizes" >> "$OUTPUT_FILE"
echo "- Random CI noise produces inconsistent, oscillating deltas" >> "$OUTPUT_FILE"
echo "- CV (Coefficient of Variation) = σ/mean measures consistency (<${CONSISTENCY_THRESHOLD} = significant)" >> "$OUTPUT_FILE"
echo "- Requires ≥3 input sizes to establish a pattern" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Show aggregated metrics in collapsible section
echo "<details>" >> "$OUTPUT_FILE"
echo "<summary>📊 Aggregated Metrics (Mean ± StdDev per Benchmark)</summary>" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo '```' >> "$OUTPUT_FILE"
echo "$analysis_output" | sed -n '/^AGGREGATED:/,/^DETAILED:/p' | grep -v "^AGGREGATED:" | grep -v "^DETAILED:" >> "$OUTPUT_FILE"
echo '```' >> "$OUTPUT_FILE"
echo "</details>" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Show detailed comparison in collapsible section
echo "<details>" >> "$OUTPUT_FILE"
echo "<summary>📋 Detailed Per-Size Comparison</summary>" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo '```' >> "$OUTPUT_FILE"
echo "$analysis_output" | sed -n '/^DETAILED:/,$p' | grep -v "^DETAILED:" >> "$OUTPUT_FILE"
echo '```' >> "$OUTPUT_FILE"
echo "</details>" >> "$OUTPUT_FILE"

# Show summary
cat "$OUTPUT_FILE"

# Exit with error code if regressions found
if [ "$REGRESSIONS_FOUND" = true ]; then
  exit 1
fi
