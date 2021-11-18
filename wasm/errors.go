package wasm

import "errors"

var (
	ErrCallStackOverflow     = errors.New("callstack overflow")
	ErrInvalidByte           = errors.New("invalid byte")
	ErrInvalidMagicNumber    = errors.New("invalid magic number")
	ErrInvalidVersion        = errors.New("invalid version header")
	ErrInvalidSectionID      = errors.New("invalid section id")
	ErrCustomSectionNotFound = errors.New("custom section not found")
)
