#!/bin/sh
set -eu
mkdir -p bin
go build -trimpath -buildvcs=false -o bin/cliprag-shell ./cmd/cliprag-shell
exec python3 scripts/test-shell.py "$@"
