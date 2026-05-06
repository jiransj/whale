#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
echo "[smoke/local] go test ./..."
go test ./...
