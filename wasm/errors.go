package wasm

import "errors"

var (
	ErrCallStackOverflow = errors.New("callstack overflow")
)
