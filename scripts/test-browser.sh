#!/bin/sh
set -eu
go build -trimpath -buildvcs=false -o bin/cliprag-browser ./cmd/cliprag-browser
python3 scripts/build-browser.py
exec node scripts/test-browser.mjs
