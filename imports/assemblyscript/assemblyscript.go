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
// See https://www.assemblyscript.org/concepts.html#special-imports
//
// # Relationship to WASI
//
// AssemblyScript supports compiling JavaScript functions that use I/O, such
// as `console.log("hello")`. However, WASI is not built-in to AssemblyScript.
// Use the `wasi-shim` to compile if you get import errors.
//
// See https://github.com/AssemblyScript/wasi-shim#usage and
// wasi_snapshot_preview1.Instantiate for more.
package assemblyscript

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

const (
	i32, f64 = wasm.ValueTypeI32, wasm.ValueTypeF64

	functionAbort = "abort"
	functionTrace = "trace"
	functionSeed  = "seed"
)

// MustInstantiate calls Instantiate or panics on error.
//
// This is a simpler function for those who know the module "env" is not
// already instantiated, and don't need to unload it.
func MustInstantiate(ctx context.Context, r wazero.Runtime) {
	if _, err := Instantiate(ctx, r); err != nil {
		panic(err)
	}
}

// Instantiate instantiates the "env" module used by AssemblyScript into the
// runtime default namespace.
//
// # Notes
//
//   - Failure cases are documented on wazero.Namespace InstantiateModule.
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
//   - To instantiate into another wazero.Namespace, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewHostModuleBuilder("env")
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

	// ExportFunctions builds functions to export with a wazero.HostModuleBuilder
	// named "env".
	ExportFunctions(wazero.HostModuleBuilder)
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
func (e *functionExporter) ExportFunctions(builder wazero.HostModuleBuilder) {
	exporter := builder.(wasm.HostFuncExporter)
	exporter.ExportHostFunc(e.abortFn)
	exporter.ExportHostFunc(e.traceFn)
	exporter.ExportHostFunc(seed)
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
var abortMessageEnabled = &wasm.HostFunc{
	ExportNames: []string{functionAbort},
	Name:        "~lib/builtins/abort",
	ParamTypes:  []api.ValueType{i32, i32, i32, i32},
	ParamNames:  []string{"message", "fileName", "lineNumber", "columnNumber"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(abortWithMessage),
	},
}

var abortMessageDisabled = abortMessageEnabled.WithGoModuleFunc(abort)

// abortWithMessage implements functionAbort
func abortWithMessage(ctx context.Context, mod api.Module, stack []uint64) {
	fsc := mod.(*wasm.CallContext).Sys.FS()
	mem := mod.Memory()

	message := uint32(stack[0])
	fileName := uint32(stack[1])
	lineNumber := uint32(stack[2])
	columnNumber := uint32(stack[3])

	// Don't panic if there was a problem reading the message
	stderr := fsc.FdWriter(internalsys.FdStderr)
	if msg, msgOk := readAssemblyScriptString(ctx, mem, message); msgOk {
		if fn, fnOk := readAssemblyScriptString(ctx, mem, fileName); fnOk {
			_, _ = fmt.Fprintf(stderr, "%s at %s:%d:%d\n", msg, fn, lineNumber, columnNumber)
		}
	}
	abort(ctx, mod, stack)
}

// abortWithMessage implements functionAbort ignoring the message.
func abort(ctx context.Context, mod api.Module, _ []uint64) {
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
var traceStdout = &wasm.HostFunc{
	ExportNames: []string{functionTrace},
	Name:        "~lib/builtins/trace",
	ParamTypes:  []api.ValueType{i32, i32, f64, f64, f64, f64, f64},
	ParamNames:  []string{"message", "nArgs", "arg0", "arg1", "arg2", "arg3", "arg4"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc: api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			fsc := mod.(*wasm.CallContext).Sys.FS()
			stdout := fsc.FdWriter(internalsys.FdStdout)
			traceTo(ctx, mod, stack, stdout)
		}),
	},
}

// traceStderr implements trace to the configured Stderr.
var traceStderr = traceStdout.WithGoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
	fsc := mod.(*wasm.CallContext).Sys.FS()
	stderr := fsc.FdWriter(internalsys.FdStderr)
	traceTo(ctx, mod, stack, stderr)
})

// traceTo implements the function "trace" in AssemblyScript. e.g.
//
//	trace('Hello World!')
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//
//	(import "env" "trace" (func $~lib/builtins/trace (param i32 i32 f64 f64 f64 f64 f64)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L61
func traceTo(ctx context.Context, mod api.Module, params []uint64, writer io.Writer) {
	message := uint32(params[0])
	nArgs := uint32(params[1])
	arg0 := api.DecodeF64(params[2])
	arg1 := api.DecodeF64(params[3])
	arg2 := api.DecodeF64(params[4])
	arg3 := api.DecodeF64(params[5])
	arg4 := api.DecodeF64(params[6])

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
var seed = &wasm.HostFunc{
	ExportNames: []string{functionSeed},
	Name:        "~lib/builtins/seed",
	ResultTypes: []api.ValueType{f64},
	ResultNames: []string{"rand"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc: api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			r := mod.(*wasm.CallContext).Sys.RandSource()
			buf := make([]byte, 8)
			_, err := io.ReadFull(r, buf)
			if err != nil {
				panic(fmt.Errorf("error reading random seed: %w", err))
			}

			// the caller interprets the result as a float64
			stack[0] = binary.LittleEndian.Uint64(buf)
		}),
	},
}

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
