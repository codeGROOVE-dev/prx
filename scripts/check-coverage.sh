#!/bin/bash
set -e

if [ $# -lt 2 ]; then
    echo "Usage: $0 <coverage-file> <threshold>"
    exit 1
fi

COVERAGE_FILE="$1"
THRESHOLD="$2"

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "Error: Coverage file not found: $COVERAGE_FILE"
    exit 1
fi

echo "Analyzing package coverage..."
echo ""

# Get package-level coverage (not function-level), excluding cmd/ packages
packages=$(go tool cover -func="$COVERAGE_FILE" | grep -E '^github.com' | awk '{print $1}' | sed 's#/[^/]*\.go:.*##' | sort -u | grep -v '/cmd/')

failed_packages=""
passed_packages=""

for package in $packages; do
    # Calculate average coverage for this package
    # Get all lines for this package and calculate weighted average
    coverage=$(go tool cover -func="$COVERAGE_FILE" | \
        grep "^${package}/" | \
        awk '{
            # Extract coverage percentage from third column
            gsub(/%/, "", $3)
            sum += $3
            count++
        }
        END {
            if (count > 0) {
                printf "%.1f", sum/count
            } else {
                print "0.0"
            }
        }')

    if [ -z "$coverage" ]; then
        coverage="0.0"
    fi

    # Use awk for float comparison
    if awk "BEGIN {exit !($coverage < $THRESHOLD)}"; then
        failed_packages="${failed_packages}${package}: ${coverage}%\n"
        echo "❌ $package: ${coverage}% (below ${THRESHOLD}%)"
    else
        passed_packages="${passed_packages}${package}: ${coverage}%\n"
        echo "✅ $package: ${coverage}%"
    fi
done

# Get total coverage (excluding cmd/ packages)
total_line=$(go tool cover -func="$COVERAGE_FILE" | grep '^total:')
total_coverage=$(echo "$total_line" | awk '{print $3}' | sed 's/%//')

echo ""
echo "=================================================="
echo "Total coverage: ${total_coverage}% (cmd/ packages excluded)"
echo "Threshold: ${THRESHOLD}%"
echo "=================================================="

if [ -n "$failed_packages" ]; then
    echo ""
    echo "The following packages have coverage below ${THRESHOLD}%:"
    echo ""
    echo -e "$failed_packages"
    echo "Tests FAILED: Some packages are below coverage threshold"
    exit 1
fi

echo ""
echo "✅ All packages meet the ${THRESHOLD}% coverage threshold!"
exit 0
