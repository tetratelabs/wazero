package experimental

import (
	"context"
)

type enableDWARFBasedStackTraceKey struct{}

// WithDWARFBasedStackTrace enables the DWARF based stack traces in the face of runtime errors.
// This only takes into effect when the original Wasm binary has the DWARF "custom sections"
// that are often stripped depending on the optimization options of the compilers.
//
// See https://github.com/tetratelabs/wazero/pull/881 for more context.
func WithDWARFBasedStackTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, enableDWARFBasedStackTraceKey{}, struct{}{})
}

// DWARFBasedStackTraceEnabled returns true if the given context has the option enabling the DWARF
// based stack trace, and false otherwise.
func DWARFBasedStackTraceEnabled(ctx context.Context) bool {
	return ctx.Value(enableDWARFBasedStackTraceKey{}) != nil
}
