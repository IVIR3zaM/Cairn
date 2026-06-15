#!/usr/bin/env bash
# Placeholder local verification entrypoint. Cairn will eventually run *itself*
# here (`cairn verify`); until then it runs the Go toolchain directly.
set -euo pipefail

echo "==> go build"
go build ./...

echo "==> go test"
go test ./...

if command -v golangci-lint >/dev/null 2>&1; then
	echo "==> golangci-lint"
	golangci-lint run
else
	echo "==> golangci-lint (skipped: not installed)"
fi
