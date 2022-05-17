package assemblyscript

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"time"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// Instantiate instantiates a module implementing abort, trace, and seed for use from AssemblyScript programs.
// The instantiated module will output abort messages to os.Stderr, not output trace messages, and use
// crypto/rand as the source for seed values. If the AssemblyScript program is configured to use WASI, by
// calling "import wasi" in any file, these functions will not be used.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	return NewModuleBuilder().Instantiate(ctx, r)
}

// ModuleBuilder allows configuring the module that will export functions used automatically by AssemblyScript.
type ModuleBuilder interface {
	// WithAbortWriter sets the io.Writer that should be used to write abort messages to.
	// Abort messages are written when an error is thrown within an AssemblyScript program.
	//
	// Defaults to os.Stderr.
	WithAbortWriter(writer io.Writer) ModuleBuilder

	// WithTraceWriter sets the io.Writer that should be used to write trace messages to.
	// Trace messages are written when the built-in trace function is called within an
	// AssemblyScript program.
	//
	// Defaults to io.Discard meaning trace messages are not written.
	WithTraceWriter(writer io.Writer) ModuleBuilder

	// WithSeedSource sets the io.Reader to read bytes from for seeding random number generation.
	//
	// Defaults to crypto/rand.Reader to seed using cryptographically random bytes.
	WithSeedSource(reader io.Reader) ModuleBuilder

	// Instantiate instantiates the module so that AssemblyScript can import from it.
	Instantiate(ctx context.Context, runtime wazero.Runtime) (api.Closer, error)
}

// NewModuleBuilder returns a ModuleBuilder for configuring a AssemblyScript host module.
func NewModuleBuilder() ModuleBuilder {
	return &moduleBuilder{
		abortWriter: os.Stderr,
		traceWriter: io.Discard,
		seedSource:  rand.Reader,
	}
}

type moduleBuilder struct {
	abortWriter io.Writer
	traceWriter io.Writer
	seedSource  io.Reader
}

// WithAbortWriter implements ModuleBuilder.WithAbortWriter
func (m *moduleBuilder) WithAbortWriter(writer io.Writer) ModuleBuilder {
	m.abortWriter = writer
	return m
}

// WithTraceWriter implements ModuleBuilder.WithTraceWriter
func (m *moduleBuilder) WithTraceWriter(writer io.Writer) ModuleBuilder {
	m.traceWriter = writer
	return m
}

// WithSeedSource implements ModuleBuilder.WithSeedSource
func (m *moduleBuilder) WithSeedSource(reader io.Reader) ModuleBuilder {
	m.seedSource = reader
	return m
}

// Instantiate implements ModuleBuilder.Instantiate
func (m *moduleBuilder) Instantiate(ctx context.Context, runtime wazero.Runtime) (api.Closer, error) {
	return runtime.NewModuleBuilder("env").
		ExportFunction("abort", func(ctx context.Context, mod api.Module, message uint32, fileName uint32, lineNumber uint32, columnNumber uint32) {
			msg, err := readAssemblyScriptString(ctx, mod, message)
			if err != nil {
				return
			}
			fn, err := readAssemblyScriptString(ctx, mod, fileName)
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(m.abortWriter, "%s at %s:%d:%d\n", msg, fn, lineNumber, columnNumber)
		}).
		ExportFunction("trace", func(ctx context.Context, mod api.Module, message uint32, nArgs uint32, arg0 float64, arg1 float64, arg2 float64, arg3 float64, arg4 float64) {
			msg, err := readAssemblyScriptString(ctx, mod, message)
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(m.traceWriter, "trace: %s", msg)
			if nArgs >= 1 {
				_, _ = fmt.Fprintf(m.traceWriter, " %v", arg0)
			}
			if nArgs >= 2 {
				_, _ = fmt.Fprintf(m.traceWriter, ",%v", arg1)
			}
			if nArgs >= 3 {
				_, _ = fmt.Fprintf(m.traceWriter, ",%v", arg2)
			}
			if nArgs >= 4 {
				_, _ = fmt.Fprintf(m.traceWriter, ",%v", arg3)
			}
			if nArgs >= 5 {
				_, _ = fmt.Fprintf(m.traceWriter, ",%v", arg4)
			}
			_, _ = fmt.Fprintln(m.traceWriter)
		}).
		ExportFunction("seed", func() float64 {
			b := make([]byte, 8)
			n, err := m.seedSource.Read(b)
			if n != 8 || err != nil {
				// AssemblyScript default JS bindings just use Date.now for a seed which is not a good seed at all.
				// We should almost always be able to read the seed, but if it fails for some reason we fallback to
				// current time as a simplest default.
				return float64(time.Now().UnixMilli())
			}
			bits := binary.LittleEndian.Uint64(b)
			return math.Float64frombits(bits)
		}).
		Instantiate(ctx)
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
	end := pointer + size
	start := pointer
	buf, ok := m.Memory().Read(ctx, start, end-start)
	if !ok {
		panic("Could not read memory")
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
