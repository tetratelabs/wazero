// Package assemblyscript contains Go-defined special functions imported by
// AssemblyScript under the module name "env".
//
// Special Functions
//
// AssemblyScript code import the below special functions when not using WASI.
// Note: Sometimes only "abort" is imported.
//
//	* "abort" - exits with 255 with an abort message written to
//	  wazero.ModuleConfig WithStderr.
//	* "trace" - no output unless.
//	* "seed" - uses wazero.ModuleConfig WithRandSource as the source of seed
//	  values.
//
// Relationship to WASI
//
// A program compiled to use WASI, via "import wasi" in any file, won't import
// these functions.
//
// See wasi_snapshot_preview1.Instantiate and
// https://www.assemblyscript.org/concepts.html#special-imports
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

// Instantiate instantiates the "env" module used by AssemblyScript into the
// runtime default namespace.
//
// Notes
//
//	* Closing the wazero.Runtime has the same effect as closing the result.
//	* To add more functions to the "env" module, use FunctionExporter.
//	* To instantiate into another wazero.Namespace, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	return r.NewModuleBuilder("env").
		ExportFunctions(NewFunctionExporter().ExportFunctions()).
		Instantiate(ctx, r)
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
	ExportFunctions() (nameToGoFunc map[string]interface{})
}

// NewFunctionExporter returns a FunctionExporter object with trace disabled.
func NewFunctionExporter() FunctionExporter {
	return &functionExporter{traceMode: traceDisabled}
}

type traceMode int

const (
	traceDisabled traceMode = 0
	traceStdout   traceMode = 1
	traceStderr   traceMode = 2
)

type functionExporter struct {
	abortMessageDisabled bool
	traceMode            traceMode
}

// WithAbortMessageDisabled implements FunctionExporter.WithAbortMessageDisabled
func (e *functionExporter) WithAbortMessageDisabled() FunctionExporter {
	ret := *e // copy
	ret.abortMessageDisabled = true
	return &ret
}

// WithTraceToStdout implements FunctionExporter.WithTraceToStdout
func (e *functionExporter) WithTraceToStdout() FunctionExporter {
	ret := *e // copy
	ret.traceMode = traceStdout
	return &ret
}

// WithTraceToStderr implements FunctionExporter.WithTraceToStderr
func (e *functionExporter) WithTraceToStderr() FunctionExporter {
	ret := *e // copy
	ret.traceMode = traceStderr
	return &ret
}

// ExportFunctions implements FunctionExporter.ExportFunctions
func (e *functionExporter) ExportFunctions() (nameToGoFunc map[string]interface{}) {
	env := &assemblyscript{abortMessageDisabled: e.abortMessageDisabled, traceMode: e.traceMode}
	return map[string]interface{}{
		"abort": env.abort,
		"trace": env.trace,
		"seed":  env.seed,
	}
}

// assemblyScript includes "Special imports" only used In AssemblyScript when a
// user didn't add `import "wasi"` to their entry file.
//
// See https://www.assemblyscript.org/concepts.html#special-imports
// See https://www.assemblyscript.org/concepts.html#targeting-wasi
// See https://www.assemblyscript.org/compiler.html#compiler-options
// See https://github.com/AssemblyScript/assemblyscript/issues/1562
type assemblyscript struct {
	abortMessageDisabled bool
	traceMode            traceMode
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
//	(import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L18
func (a *assemblyscript) abort(
	ctx context.Context,
	mod api.Module,
	message uint32,
	fileName uint32,
	lineNumber uint32,
	columnNumber uint32,
) {
	if !a.abortMessageDisabled {
		sysCtx := mod.(*wasm.CallContext).Sys
		msg, err := readAssemblyScriptString(ctx, mod, message)
		if err != nil {
			return
		}
		fn, err := readAssemblyScriptString(ctx, mod, fileName)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(sysCtx.Stderr(), "%s at %s:%d:%d\n", msg, fn, lineNumber, columnNumber)
	}

	// AssemblyScript expects the exit code to be 255
	// See https://github.com/AssemblyScript/assemblyscript/blob/v0.20.13/tests/compiler/wasi/abort.js#L14
	exitCode := uint32(255)

	// Ensure other callers see the exit code.
	_ = mod.CloseWithExitCode(ctx, exitCode)

	// Prevent any code from executing after this function.
	panic(sys.NewExitError(mod.Name(), exitCode))
}

// trace implements the same named function in AssemblyScript. Ex.
//	trace('Hello World!')
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//	(import "env" "trace" (func $~lib/builtins/trace (param i32 i32 f64 f64 f64 f64 f64)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L61
func (a *assemblyscript) trace(
	ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0, arg1, arg2, arg3, arg4 float64,
) {
	var writer io.Writer
	switch a.traceMode {
	case traceDisabled:
		return
	case traceStdout:
		writer = mod.(*wasm.CallContext).Sys.Stdout()
	case traceStderr:
		writer = mod.(*wasm.CallContext).Sys.Stderr()
	}

	msg, err := readAssemblyScriptString(ctx, mod, message)
	if err != nil {
		panic(err)
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
	_, err = writer.Write([]byte(ret.String()))
	if err != nil {
		panic(err)
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// seed is called when the AssemblyScript's random number generator needs to be
// seeded.
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//	(import "env" "seed" (func $~lib/builtins/seed (result f64)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L111
func (a *assemblyscript) seed(mod api.Module) float64 {
	randSource := mod.(*wasm.CallContext).Sys.RandSource()
	v, err := ieee754.DecodeFloat64(randSource)
	if err != nil {
		panic(fmt.Errorf("error reading random seed: %w", err))
	}
	return v
}

// readAssemblyScriptString reads a UTF-16 string created by AssemblyScript.
func readAssemblyScriptString(ctx context.Context, mod api.Module, offset uint32) (string, error) {
	// Length is four bytes before pointer.
	byteCount, ok := mod.Memory().ReadUint32Le(ctx, offset-4)
	if !ok {
		return "", fmt.Errorf("Memory.ReadUint32Le(%d) out of range", offset-4)
	}
	if byteCount%2 != 0 {
		return "", fmt.Errorf("read an odd number of bytes for utf-16 string: %d", byteCount)
	}
	buf, ok := mod.Memory().Read(ctx, offset, byteCount)
	if !ok {
		return "", fmt.Errorf("Memory.Read(%d, %d) out of range", offset, byteCount)
	}
	return decodeUTF16(buf), nil
}

func decodeUTF16(b []byte) string {
	u16s := make([]uint16, len(b)/2)

	lb := len(b)
	for i := 0; i < lb; i += 2 {
		u16s[i/2] = uint16(b[i]) + (uint16(b[i+1]) << 8)
	}

	return string(utf16.Decode(u16s))
}
