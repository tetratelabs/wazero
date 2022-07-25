// Package assemblyscript contains Go-defined special functions imported by
// AssemblyScript under the module name "env".
//
// # Special Functions
//
// AssemblyScript code import the below special functions when not using WASI.
// Note: Sometimes only "abort" is imported.
//
//   - "abort" - exits with 255 with an abort message written to
//     wazero.ModuleConfig WithStderr.
//   - "trace" - no output unless.
//   - "seed" - uses wazero.ModuleConfig WithRandSource as the source of seed
//     values.
//
// # Relationship to WASI
//
// A program compiled to use WASI, via "import wasi" in any file, won't import
// these functions.
//
// See wasi_snapshot_preview1.Instantiate and
//   - https://www.assemblyscript.org/concepts.html#special-imports
//   - https://www.assemblyscript.org/concepts.html#targeting-wasi
//   - https://www.assemblyscript.org/compiler.html#compiler-options
package assemblyscript

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

const (
	functionAbort = "abort"
	functionTrace = "trace"
	functionSeed  = "seed"
)

// Instantiate instantiates the "env" module used by AssemblyScript into the
// runtime default namespace.
//
// # Notes
//
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
//   - To instantiate into another wazero.Namespace, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewModuleBuilder("env")
	NewFunctionExporter().ExportFunctions(builder)
	return builder.Instantiate(ctx, r)
}

// FunctionExporter configures the functions in the "env" module used by
// AssemblyScript.
type FunctionExporter interface {
	// WithAbortMessageDisabled configures the AssemblyScript abort function to
	// discard any message.
	WithAbortMessageDisabled() FunctionExporter

	// WithTraceToStdout configures the AssemblyScript trace function to output
	// messages to Stdout, as configured by wazero.ModuleConfig WithStdout.
	WithTraceToStdout() FunctionExporter

	// WithTraceToStderr configures the AssemblyScript trace function to output
	// messages to Stderr, as configured by wazero.ModuleConfig WithStderr.
	//
	// Because of the potential volume of trace messages, it is often more
	// appropriate to use WithTraceToStdout instead.
	WithTraceToStderr() FunctionExporter

	// ExportFunctions builds functions to export with a wazero.ModuleBuilder
	// named "env".
	ExportFunctions(builder wazero.ModuleBuilder)
}

// NewFunctionExporter returns a FunctionExporter object with trace disabled.
func NewFunctionExporter() FunctionExporter {
	return &functionExporter{abortFn: abortMessageEnabled, traceFn: traceDisabled}
}

type functionExporter struct {
	abortFn, traceFn *wasm.HostFunc
}

// WithAbortMessageDisabled implements FunctionExporter.WithAbortMessageDisabled
func (e *functionExporter) WithAbortMessageDisabled() FunctionExporter {
	return &functionExporter{abortFn: abortMessageDisabled, traceFn: e.traceFn}
}

// WithTraceToStdout implements FunctionExporter.WithTraceToStdout
func (e *functionExporter) WithTraceToStdout() FunctionExporter {
	return &functionExporter{abortFn: e.abortFn, traceFn: traceStdout}
}

// WithTraceToStderr implements FunctionExporter.WithTraceToStderr
func (e *functionExporter) WithTraceToStderr() FunctionExporter {
	return &functionExporter{abortFn: e.abortFn, traceFn: traceStderr}
}

// ExportFunctions implements FunctionExporter.ExportFunctions
func (e *functionExporter) ExportFunctions(builder wazero.ModuleBuilder) {
	builder.ExportFunction(functionAbort, e.abortFn)
	builder.ExportFunction(functionTrace, e.traceFn)
	builder.ExportFunction(functionSeed, seed)
}

// abort is called on unrecoverable errors. This is typically present in Wasm
// compiled from AssemblyScript, if assertions are enabled or errors are
// thrown.
//
// The implementation writes the message to stderr, unless
// abortMessageDisabled, and closes the module with exit code 255.
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//
//	(import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L18
var abortMessageEnabled = wasm.NewGoFunc(
	"abort", "~lib/builtins/abort",
	[]string{"message", "fileName", "lineNumber", "columnNumber"},
	abortWithMessage,
)

var abortMessageDisabled = abortMessageEnabled.MustGoFunc(abort)

// abortWithMessage implements fnAbort
func abortWithMessage(
	ctx context.Context, mod api.Module, message, fileName, lineNumber, columnNumber uint32,
) {
	sysCtx := mod.(*wasm.CallContext).Sys
	mem := mod.Memory()
	// Don't panic if there was a problem reading the message
	if msg, msgOk := readAssemblyScriptString(ctx, mem, message); msgOk {
		if fn, fnOk := readAssemblyScriptString(ctx, mem, fileName); fnOk {
			_, _ = fmt.Fprintf(sysCtx.Stderr(), "%s at %s:%d:%d\n", msg, fn, lineNumber, columnNumber)
		}
	}
	abort(ctx, mod, message, fileName, lineNumber, columnNumber)
}

// abortWithMessage implements fnAbort ignoring the message.
func abort(
	ctx context.Context, mod api.Module, message, fileName, lineNumber, columnNumber uint32,
) {
	// AssemblyScript expects the exit code to be 255
	// See https://github.com/AssemblyScript/assemblyscript/blob/v0.20.13/tests/compiler/wasi/abort.js#L14
	exitCode := uint32(255)

	// Ensure other callers see the exit code.
	_ = mod.CloseWithExitCode(ctx, exitCode)

	// Prevent any code from executing after this function.
	panic(sys.NewExitError(mod.Name(), exitCode))
}

// traceDisabled ignores the input.
var traceDisabled = traceStdout.WithWasm([]byte{wasm.OpcodeEnd})

// traceStdout implements trace to the configured Stdout.
var traceStdout = wasm.NewGoFunc(functionTrace, "~lib/builtins/trace",
	[]string{"message", "nArgs", "arg0", "arg1", "arg2", "arg3", "arg4"},
	func(
		ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0, arg1, arg2, arg3, arg4 float64,
	) {
		traceTo(ctx, mod, message, nArgs, arg0, arg1, arg2, arg3, arg4, mod.(*wasm.CallContext).Sys.Stdout())
	},
)

// traceStderr implements trace to the configured Stderr.
var traceStderr = traceStdout.MustGoFunc(func(
	ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0, arg1, arg2, arg3, arg4 float64,
) {
	traceTo(ctx, mod, message, nArgs, arg0, arg1, arg2, arg3, arg4, mod.(*wasm.CallContext).Sys.Stderr())
})

// traceTo implements the function "trace" in AssemblyScript. Ex.
//
//	trace('Hello World!')
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//
//	(import "env" "trace" (func $~lib/builtins/trace (param i32 i32 f64 f64 f64 f64 f64)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L61
func traceTo(
	ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0, arg1, arg2, arg3, arg4 float64,
	writer io.Writer,
) {
	msg, ok := readAssemblyScriptString(ctx, mod.Memory(), message)
	if !ok {
		return // don't panic if unable to trace
	}
	var ret strings.Builder
	ret.WriteString("trace: ")
	ret.WriteString(msg)
	if nArgs >= 1 {
		ret.WriteString(" ")
		ret.WriteString(formatFloat(arg0))
	}
	if nArgs >= 2 {
		ret.WriteString(",")
		ret.WriteString(formatFloat(arg1))
	}
	if nArgs >= 3 {
		ret.WriteString(",")
		ret.WriteString(formatFloat(arg2))
	}
	if nArgs >= 4 {
		ret.WriteString(",")
		ret.WriteString(formatFloat(arg3))
	}
	if nArgs >= 5 {
		ret.WriteString(",")
		ret.WriteString(formatFloat(arg4))
	}
	ret.WriteByte('\n')
	_, _ = writer.Write([]byte(ret.String())) // don't crash if trace logging fails
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// seed is called when the AssemblyScript's random number generator needs to be
// seeded.
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//
//	(import "env" "seed" (func $~lib/builtins/seed (result f64)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L111
var seed = wasm.NewGoFunc(functionSeed, "~lib/builtins/seed", []string{},
	func(mod api.Module) float64 {
		randSource := mod.(*wasm.CallContext).Sys.RandSource()
		v, err := ieee754.DecodeFloat64(randSource)
		if err != nil {
			panic(fmt.Errorf("error reading random seed: %w", err))
		}
		return v
	},
)

// readAssemblyScriptString reads a UTF-16 string created by AssemblyScript.
func readAssemblyScriptString(ctx context.Context, mem api.Memory, offset uint32) (string, bool) {
	// Length is four bytes before pointer.
	byteCount, ok := mem.ReadUint32Le(ctx, offset-4)
	if !ok || byteCount%2 != 0 {
		return "", false
	}
	buf, ok := mem.Read(ctx, offset, byteCount)
	if !ok {
		return "", false
	}
	return decodeUTF16(buf), true
}

func decodeUTF16(b []byte) string {
	u16s := make([]uint16, len(b)/2)

	lb := len(b)
	for i := 0; i < lb; i += 2 {
		u16s[i/2] = uint16(b[i]) + (uint16(b[i+1]) << 8)
	}

	return string(utf16.Decode(u16s))
}
