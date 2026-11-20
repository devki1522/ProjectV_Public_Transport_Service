#!/usr/bin/env bash
set -e
NODE_PATH="$(dirname "$0")/scripts/test-all.mjs"
node "$NODE_PATH" "$@"
