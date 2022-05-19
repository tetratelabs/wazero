// Package assemblyscript contains Go-defined special functions imported by AssemblyScript under the module name "env".
//
// Note: Some code will only import "env.abort", but even that isn't imported when "import wasi" is used in the source.
// See https://www.assemblyscript.org/concepts.html#special-imports
package assemblyscript

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Instantiate instantiates a module implementing special functions defined by AssemblyScript:
// * "env.abort"
// * "env.trace"
// * "env.seed"
//abort, trace, and seed for use from AssemblyScript programs.
// The instantiated module will output abort messages to the io.Writer configured by wazero.ModuleConfig.WithStderr,
// not output trace messages, and use the io.Reader configured by wazero.ModuleConfig.WithRandSource as the source for
// seed values.
//
// To customize behavior, use NewModuleBuilder instead.
// Note: If the AssemblyScript program is configured to use WASI, by calling "import wasi" in any file, these
// functions will not be used.
// See wasi.InstantiateSnapshotPreview1
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	return NewModuleBuilder().Instantiate(ctx, r)
}

// ModuleBuilder allows configuring the module that will export functions used automatically by AssemblyScript.
type ModuleBuilder interface {
	// WithAbortDisabled configures the AssemblyScript abort function to be disabled. Any abort messages will
	// be discarded.
	WithAbortDisabled() ModuleBuilder

	// WithTraceToStdout configures the AssemblyScript trace function to output messages to Stdout, as configured by
	// wazero.ModuleConfig.WithStdout.
	WithTraceToStdout() ModuleBuilder

	// WithTraceToStderr configures the AssemblyScript trace function to output messages to Stderr, as configured by
	// wazero.ModuleConfig.WithStderr. Because of the potential volume of trace messages, it is often more appropriate
	// to use WithTraceToStdout instead.
	WithTraceToStderr() ModuleBuilder

	// Instantiate instantiates the module so that AssemblyScript can import from it.
	Instantiate(ctx context.Context, runtime wazero.Runtime) (api.Closer, error)
}

// NewModuleBuilder returns a ModuleBuilder for configuring a AssemblyScript host module.
func NewModuleBuilder() ModuleBuilder {
	return &moduleBuilder{
		abortEnabled: true,
		traceMode:    traceDisabled,
	}
}

type traceMode int

const (
	traceDisabled traceMode = 0
	traceStdout   traceMode = 1
	traceStderr   traceMode = 2
)

type moduleBuilder struct {
	abortEnabled bool
	traceMode    traceMode
}

// WithAbortDisabled implements ModuleBuilder.WithAbortWriter
func (m *moduleBuilder) WithAbortDisabled() ModuleBuilder {
	ret := *m // copy
	ret.abortEnabled = false
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
func (m *moduleBuilder) Instantiate(ctx context.Context, runtime wazero.Runtime) (api.Closer, error) {
	mod := runtime.NewModuleBuilder("env")

	if m.abortEnabled {
		mod.ExportFunction("abort", func(ctx context.Context, mod api.Module, message uint32, fileName uint32, lineNumber uint32, columnNumber uint32) {
			sys := sysCtx(mod)
			abort(ctx, mod, message, fileName, lineNumber, columnNumber, sys.Stderr())
		})
	} else {
		mod.ExportFunction("abort", func(ctx context.Context, mod api.Module, message uint32, fileName uint32, lineNumber uint32, columnNumber uint32) {
			// stub for no-op
		})
	}

	switch m.traceMode {
	case traceStderr:
		mod.ExportFunction("trace", func(ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0 float64, arg1 float64, arg2 float64, arg3 float64, arg4 float64) {
			sys := sysCtx(mod)
			trace(ctx, mod, message, nArgs, arg0, arg1, arg2, arg3, arg4, sys.Stderr())
		})
	case traceStdout:
		mod.ExportFunction("trace", func(ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0 float64, arg1 float64, arg2 float64, arg3 float64, arg4 float64) {
			sys := sysCtx(mod)
			trace(ctx, mod, message, nArgs, arg0, arg1, arg2, arg3, arg4, sys.Stdout())
		})
	case traceDisabled:
		mod.ExportFunction("trace", func(ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0 float64, arg1 float64, arg2 float64, arg3 float64, arg4 float64) {
			// stub for no-op
		})
	}

	mod.ExportFunction("seed", func(mod api.Module) float64 {
		sys := sysCtx(mod)
		return seed(sys.RandSource())
	})

	return mod.Instantiate(ctx)
}

// readAssemblyScriptString reads a UTF-16 string created by AssemblyScript.
func readAssemblyScriptString(ctx context.Context, m api.Module, pointer uint32) (string, error) {
	// Length is four bytes before pointer.
	size, ok := m.Memory().ReadUint32Le(ctx, pointer-4)
	if !ok {
		return "", fmt.Errorf("could not read size from memory")
	}
	if size%2 != 0 {
		return "", fmt.Errorf("odd number of bytes for utf-16 string")
	}
	buf, ok := m.Memory().Read(ctx, pointer, size)
	if !ok {
		return "", fmt.Errorf("could not read string from memory")
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

func abort(ctx context.Context, mod api.Module, message uint32, fileName uint32, lineNumber uint32, columnNumber uint32, writer io.Writer) {
	msg, err := readAssemblyScriptString(ctx, mod, message)
	if err != nil {
		return
	}
	fn, err := readAssemblyScriptString(ctx, mod, fileName)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(writer, "%s at %s:%d:%d\n", msg, fn, lineNumber, columnNumber)
}

func seed(source io.Reader) float64 {
	b := make([]byte, 8)
	n, err := source.Read(b)
	if n != 8 || err != nil {
		// AssemblyScript default JS bindings just use Date.now for a seed which is not a good seed at all.
		// We should almost always be able to read the seed, but if it fails for some reason we fallback to
		// current time as a simplest default.
		return float64(time.Now().UnixMilli())
	}
	bits := binary.LittleEndian.Uint64(b)
	return math.Float64frombits(bits)
}

func trace(ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0 float64, arg1 float64, arg2 float64, arg3 float64, arg4 float64, writer io.Writer) {
	msg, err := readAssemblyScriptString(ctx, mod, message)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(writer, "trace: %s", msg)
	if nArgs >= 1 {
		_, _ = fmt.Fprintf(writer, " %v", arg0)
	}
	if nArgs >= 2 {
		_, _ = fmt.Fprintf(writer, ",%v", arg1)
	}
	if nArgs >= 3 {
		_, _ = fmt.Fprintf(writer, ",%v", arg2)
	}
	if nArgs >= 4 {
		_, _ = fmt.Fprintf(writer, ",%v", arg3)
	}
	if nArgs >= 5 {
		_, _ = fmt.Fprintf(writer, ",%v", arg4)
	}
	_, _ = fmt.Fprintln(writer)
}

func sysCtx(m api.Module) *wasm.SysContext {
	if internal, ok := m.(*wasm.CallContext); !ok {
		panic(fmt.Errorf("unsupported wasm.Module implementation: %v", m))
	} else {
		return internal.Sys
	}
}
