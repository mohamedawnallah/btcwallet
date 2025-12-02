#!/usr/bin/env python3
"""
analyze-benchmarks.py
Analyzes benchmark results using growth pattern consistency analysis.

Detects statistical significance by analyzing consistency of performance
changes across multiple input sizes. Real regressions show consistent
patterns, while noise is inconsistent.
"""

import sys
import re
import math
from collections import defaultdict


def parse_benchmarks(filename):
    """Parse benchmark file and return dict of {name: {metric: value}}"""
    results = {}
    with open(filename, 'r') as f:
        for line in f:
            if not line.startswith('Benchmark'):
                continue

            # Parse: BenchmarkName-N    count   ns/op   B/op   allocs/op
            parts = line.split()
            if len(parts) < 8:
                continue

            name = re.sub(r'-\d+$', '', parts[0])  # Remove trailing -N

            try:
                results[name] = {
                    'time': float(parts[2]),
                    'mem': float(parts[4]),
                    'allocs': float(parts[6])
                }
            except (ValueError, IndexError):
                continue

    return results


def extract_family(name):
    """Extract benchmark family from full name

    Supports both 2-level and 3-level structures:

    3-level (with variant - e.g., during refactoring with Before/After):
      BenchmarkListAccounts/05-Accounts-05-UTXOs/1-After
      -> family: BenchmarkListAccounts/1-After
      -> size: 05-Accounts-05-UTXOs

    2-level (without variant - e.g., after refactoring):
      BenchmarkListAccounts/05-Accounts-05-UTXOs
      -> family: BenchmarkListAccounts
      -> size: 05-Accounts-05-UTXOs
    """
    # Try 3-level structure first (Name/Size/Variant)
    match = re.match(r'^(Benchmark[^/]+)/([^/]+)/(.+)$', name)
    if match:
        family = f"{match.group(1)}/{match.group(3)}"
        size = match.group(2)
        return family, size

    # Try 2-level structure (Name/Size)
    match = re.match(r'^(Benchmark[^/]+)/([^/]+)$', name)
    if match:
        family = match.group(1)
        size = match.group(2)
        return family, size

    # No match - benchmark will be ignored
    return None, None


def calc_stats(values):
    """Calculate mean, std dev, and coefficient of variation"""
    if not values:
        return 0, 0, 999

    mean = sum(values) / len(values)
    if len(values) < 2:
        return mean, 0, 0

    variance = sum((x - mean) ** 2 for x in values) / len(values)
    std_dev = math.sqrt(variance)
    cv = abs(std_dev / mean) if mean != 0 else 999

    return mean, std_dev, cv


def main():
    if len(sys.argv) < 6:
        print("Usage: analyze-benchmarks.py BASE_FILE PR_FILE REGRESSION_THRESHOLD IMPROVEMENT_THRESHOLD CONSISTENCY_THRESHOLD")
        sys.exit(1)

    base_file = sys.argv[1]
    pr_file = sys.argv[2]
    regression_threshold = float(sys.argv[3])
    improvement_threshold = float(sys.argv[4])
    consistency_threshold = float(sys.argv[5])

    base_results = parse_benchmarks(base_file)
    pr_results = parse_benchmarks(pr_file)

    # Group deltas by family
    family_deltas = defaultdict(lambda: {'time': [], 'mem': [], 'allocs': []})
    detailed = []

    for name in base_results:
        if name not in pr_results:
            continue

        family, size = extract_family(name)
        if not family:
            continue

        # Calculate percentage deltas
        time_delta = ((pr_results[name]['time'] - base_results[name]['time']) /
                     base_results[name]['time'] * 100) if base_results[name]['time'] > 0 else 0
        mem_delta = ((pr_results[name]['mem'] - base_results[name]['mem']) /
                    base_results[name]['mem'] * 100) if base_results[name]['mem'] > 0 else 0
        allocs_delta = ((pr_results[name]['allocs'] - base_results[name]['allocs']) /
                       base_results[name]['allocs'] * 100) if base_results[name]['allocs'] > 0 else 0

        family_deltas[family]['time'].append(time_delta)
        family_deltas[family]['mem'].append(mem_delta)
        family_deltas[family]['allocs'].append(allocs_delta)

        detailed.append(f"{name}: time {time_delta:+.1f}% | mem {mem_delta:+.1f}% | allocs {allocs_delta:+.1f}%")

    # Analyze each family for significant regressions/improvements
    print("REGRESSIONS:")
    for family, deltas in sorted(family_deltas.items()):
        count = len(deltas['time'])
        if count < 3:  # Need at least 3 data points
            continue

        # Analyze metrics
        time_mean, time_std, time_cv = calc_stats(deltas['time'])
        mem_mean, mem_std, mem_cv = calc_stats(deltas['mem'])
        allocs_mean, allocs_std, allocs_cv = calc_stats(deltas['allocs'])

        significant = False
        details = []

        if time_mean >= regression_threshold and time_cv < consistency_threshold:
            significant = True
            details.append(f"time: +{time_mean:.1f}% (σ={time_std:.1f}%, CV={time_cv:.2f})")

        if mem_mean >= regression_threshold and mem_cv < consistency_threshold:
            significant = True
            details.append(f"mem: +{mem_mean:.1f}% (σ={mem_std:.1f}%, CV={mem_cv:.2f})")

        if allocs_mean >= regression_threshold and allocs_cv < consistency_threshold:
            significant = True
            details.append(f"allocs: +{allocs_mean:.1f}% (σ={allocs_std:.1f}%, CV={allocs_cv:.2f})")

        if significant:
            print(f"{family}|{' | '.join(details)}|{count}")

    print("\nIMPROVEMENTS:")
    for family, deltas in sorted(family_deltas.items()):
        count = len(deltas['time'])
        if count < 3:
            continue

        time_mean, time_std, time_cv = calc_stats(deltas['time'])
        mem_mean, mem_std, mem_cv = calc_stats(deltas['mem'])
        allocs_mean, allocs_std, allocs_cv = calc_stats(deltas['allocs'])

        significant = False
        details = []

        if time_mean <= -improvement_threshold and time_cv < consistency_threshold:
            significant = True
            details.append(f"time: {time_mean:.1f}% (σ={time_std:.1f}%, CV={time_cv:.2f})")

        if mem_mean <= -improvement_threshold and mem_cv < consistency_threshold:
            significant = True
            details.append(f"mem: {mem_mean:.1f}% (σ={mem_std:.1f}%, CV={mem_cv:.2f})")

        if allocs_mean <= -improvement_threshold and allocs_cv < consistency_threshold:
            significant = True
            details.append(f"allocs: {allocs_mean:.1f}% (σ={allocs_std:.1f}%, CV={allocs_cv:.2f})")

        if significant:
            print(f"{family}|{' | '.join(details)}|{count}")

    print("\nDETAILED:")
    for line in sorted(detailed):
        print(line)


if __name__ == '__main__':
    main()
