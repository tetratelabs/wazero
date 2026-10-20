package filecache

import "errors"

// These errors are exposed by wazero's public compilation cache API.
var (
	ErrMiss        = errors.New("compilation cache: required entry missing")
	ErrStale       = errors.New("compilation cache: stale entry")
	ErrCorrupt     = errors.New("compilation cache: corrupt entry")
	ErrIO          = errors.New("compilation cache: I/O error")
	ErrUnsupported = errors.New("compilation cache: engine does not support required persistent hits")
	ErrReadOnly    = errors.New("compilation cache: read-only")
)
