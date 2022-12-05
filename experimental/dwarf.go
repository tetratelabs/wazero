package experimental

import (
	"context"
)

type enableDWARFBasedStackTraceKey struct{}

// WithDWARFBasedStackTrace enables the DWARF based stack traces in the face of runtime errors.
// This only takes into effect when the original Wasm binary has the DWARF "custom sections"
// that are often stripped depending on the optimization options of the compilers.
//
// For example, when this is not enabled, the stack trace message looks like:
//
//	wasm stack trace:
//		.runtime._panic(i32)
//		.c()
//		.main.main()
//		.runtime.run()
//		._start()
//
// and when it is enabled:
//
//	wasm stack trace:
//		.runtime._panic(i32)
//		  0x16e2: /opt/homebrew/Cellar/tinygo/0.26.0/src/runtime/runtime_tinygowasm.go:73:6
//		.c()
//		  0x190b: /Users/XXXXX/wazero/internal/testing/dwarftestdata/testdata/main.go:19:7
//		.b()
//		  0x1901: /Users/XXXXX/wazero/internal/testing/dwarftestdata/testdata/main.go:14:3
//		.a()
//		  0x18f7: /Users/XXXXX/wazero/internal/testing/dwarftestdata/testdata/main.go:9:3
//		.main.main()
//		  0x18ed: /Users/XXXXX/wazero/internal/testing/dwarftestdata/testdata/main.go:4:3
//		.runtime.run()
//		  0x18cc: /opt/homebrew/Cellar/tinygo/0.26.0/src/runtime/scheduler_none.go:26:10
//		._start()
//		  0x18b6: /opt/homebrew/Cellar/tinygo/0.26.0/src/runtime/runtime_wasm_wasi.go:22:5
//
// which contains the source code information.
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
