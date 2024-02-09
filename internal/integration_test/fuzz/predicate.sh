#!/usr/bin/env bash

# The Wasm file is given as the first and only argument to the script.
WASM=$1

echo "Testing $WASM"

export WASM_BINARY_PATH=$WASM

# Run the test and reverse the exit code so that a non-zero exit code indicates interesting case.
./nodiff.test -test.run=TestReRunFailedRequireNoDiffCase
exit $((! $?))
