// Package assemblyscript contains Go-defined special functions imported by AssemblyScript under the module name "env".
//
// Note: Some code will only import "env.abort", but even that isn't imported when "import wasi" is used in the source.
//
// See https://www.assemblyscript.org/concepts.html#special-imports
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
)

// Instantiate instantiates a module implementing special functions defined by AssemblyScript:
//	* "env.abort" - exits with 255 with an abort message written to wazero.ModuleConfig WithStderr.
//	* "env.trace" - no output unless.
//	* "env.seed" - uses wazero.ModuleConfig WithRandSource as the source of seed values.
//
// Notes
//
//	* To customize behavior, use NewModuleBuilder instead.
//	* A program compiled to use WASI, via "import wasi" in any file, won't import these functions.
//
// See NewModuleBuilder and wasi.InstantiateSnapshotPreview1
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	return NewModuleBuilder(r).Instantiate(ctx)
}

// ModuleBuilder allows configuring the module that will export functions used automatically by AssemblyScript.
type ModuleBuilder interface {
	// WithAbortMessageDisabled configures the AssemblyScript abort function to discard any message.
	WithAbortMessageDisabled() ModuleBuilder

	// WithTraceToStdout configures the AssemblyScript trace function to output messages to Stdout, as configured by
	// wazero.ModuleConfig WithStdout.
	WithTraceToStdout() ModuleBuilder

	// WithTraceToStderr configures the AssemblyScript trace function to output messages to Stderr, as configured by
	// wazero.ModuleConfig WithStderr. Because of the potential volume of trace messages, it is often more appropriate
	// to use WithTraceToStdout instead.
	WithTraceToStderr() ModuleBuilder

	// Instantiate instantiates the module so that AssemblyScript can import from it.
	Instantiate(context.Context) (api.Closer, error)
}

// NewModuleBuilder is an alternative to Instantiate which allows customization via ModuleBuilder.
func NewModuleBuilder(r wazero.Runtime) ModuleBuilder {
	return &moduleBuilder{r: r, traceMode: traceDisabled}
}

type traceMode int

const (
	traceDisabled traceMode = 0
	traceStdout   traceMode = 1
	traceStderr   traceMode = 2
)

type moduleBuilder struct {
	r                    wazero.Runtime
	abortMessageDisabled bool
	traceMode            traceMode
}

// WithAbortMessageDisabled implements ModuleBuilder.WithAbortMessageDisabled
func (m *moduleBuilder) WithAbortMessageDisabled() ModuleBuilder {
	ret := *m // copy
	ret.abortMessageDisabled = true
	return &ret
}

// WithTraceToStdout implements ModuleBuilder.WithTraceToStdout
func (m *moduleBuilder) WithTraceToStdout() ModuleBuilder {
	ret := *m // copy
	ret.traceMode = traceStdout
	return &ret
}

// WithTraceToStderr implements ModuleBuilder.WithTraceToStderr
func (m *moduleBuilder) WithTraceToStderr() ModuleBuilder {
	ret := *m // copy
	ret.traceMode = traceStderr
	return &ret
}

// Instantiate implements ModuleBuilder.Instantiate
func (m *moduleBuilder) Instantiate(ctx context.Context) (api.Closer, error) {
	env := &assemblyscript{abortMessageDisabled: m.abortMessageDisabled, traceMode: m.traceMode}
	return m.r.NewModuleBuilder("env").
		ExportFunction("abort", env.abort).
		ExportFunction("trace", env.trace).
		ExportFunction("seed", env.seed).
		Instantiate(ctx)
}

// assemblyScript includes "Special imports" only used In AssemblyScript when a user didn't add `import "wasi"` to their
// entry file.
//
// See https://www.assemblyscript.org/concepts.html#special-imports
// See https://www.assemblyscript.org/concepts.html#targeting-wasi
// See https://www.assemblyscript.org/compiler.html#compiler-options
// See https://github.com/AssemblyScript/assemblyscript/issues/1562
type assemblyscript struct {
	abortMessageDisabled bool
	traceMode            traceMode
}

// abort is called on unrecoverable errors. This is typically present in Wasm compiled from AssemblyScript, if
// assertions are enabled or errors are thrown.
//
// The implementation writes the message to stderr, unless abortMessageDisabled, and closes the module with exit code
// 255.
//
// Here's the import in a user's module that ends up using this, in WebAssembly 1.0 (MVP) Text Format:
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
		sys := sysCtx(mod)
		msg, err := readAssemblyScriptString(ctx, mod, message)
		if err != nil {
			return
		}
		fn, err := readAssemblyScriptString(ctx, mod, fileName)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(sys.Stderr(), "%s at %s:%d:%d\n", msg, fn, lineNumber, columnNumber)
	}
	_ = mod.CloseWithExitCode(ctx, 255)
}

// trace implements the same named function in AssemblyScript (ex. trace('Hello World!'))
//
// Here's the import in a user's module that ends up using this, in WebAssembly 1.0 (MVP) Text Format:
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
		writer = sysCtx(mod).Stdout()
	case traceStderr:
		writer = sysCtx(mod).Stderr()
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

// seed is called when the AssemblyScript's random number generator needs to be seeded
//
// Here's the import in a user's module that ends up using this, in WebAssembly 1.0 (MVP) Text Format:
//	(import "env" "seed" (func $~lib/builtins/seed (result f64)))
//
// See https://github.com/AssemblyScript/assemblyscript/blob/fa14b3b03bd4607efa52aaff3132bea0c03a7989/std/assembly/wasi/index.ts#L111
func (a *assemblyscript) seed(mod api.Module) float64 {
	source := sysCtx(mod).RandSource()
	v, err := ieee754.DecodeFloat64(source)
	if err != nil {
		panic(fmt.Errorf("error reading Module.RandSource: %w", err))
	}
	return v
}

// readAssemblyScriptString reads a UTF-16 string created by AssemblyScript.
func readAssemblyScriptString(ctx context.Context, m api.Module, offset uint32) (string, error) {
	// Length is four bytes before pointer.
	byteCount, ok := m.Memory().ReadUint32Le(ctx, offset-4)
	if !ok {
		return "", fmt.Errorf("Memory.ReadUint32Le(%d) out of range", offset-4)
	}
	if byteCount%2 != 0 {
		return "", fmt.Errorf("read an odd number of bytes for utf-16 string: %d", byteCount)
	}
	buf, ok := m.Memory().Read(ctx, offset, byteCount)
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

func sysCtx(m api.Module) *wasm.SysContext {
	if internal, ok := m.(*wasm.CallContext); !ok {
		panic(fmt.Errorf("unsupported wasm.Module implementation: %v", m))
	} else {
		return internal.Sys
	}
}
