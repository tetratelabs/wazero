package assemblyscript

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/watzero"
	"github.com/tetratelabs/wazero/sys"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestAbort(t *testing.T) {
	var stderr bytes.Buffer
	mod, r := requireModule(t, wazero.NewModuleConfig().WithStderr(&stderr))
	defer r.Close(testCtx)

	tests := []struct {
		name     string
		abortFn  fnAbort
		exporter FunctionExporter
		expected string
	}{
		{
			name:     "enabled",
			abortFn:  abortWithMessage,
			exporter: NewFunctionExporter(),
			expected: "message at filename:1:2\n",
		},
		{
			name:     "disabled",
			abortFn:  abort,
			exporter: NewFunctionExporter().WithAbortMessageDisabled(),
			expected: "",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer stderr.Reset()

			messageOff, filenameOff := writeAbortMessageAndFileName(t, mod.Memory(), encodeUTF16("message"), encodeUTF16("filename"))

			err := require.CapturePanic(func() {
				tc.abortFn(testCtx, mod, messageOff, filenameOff, 1, 2)
			})
			require.Error(t, err)
			require.Equal(t, uint32(255), err.(*sys.ExitError).ExitCode())

			require.Equal(t, tc.expected, stderr.String())
		})
	}
}

func TestAbort_Error(t *testing.T) {
	var stderr bytes.Buffer
	mod, r := requireModule(t, wazero.NewModuleConfig().WithStderr(&stderr))
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		messageUTF16  []byte
		fileNameUTF16 []byte
		expectedLog   string
	}{
		{
			name:          "bad message",
			messageUTF16:  encodeUTF16("message")[:5],
			fileNameUTF16: encodeUTF16("filename"),
			expectedLog: `==> env.~lib/builtins/abort(message=4,fileName=13,lineNumber=1,columnNumber=2)
<== ()
`,
		},
		{
			name:          "bad filename",
			messageUTF16:  encodeUTF16("message"),
			fileNameUTF16: encodeUTF16("filename")[:5],
			expectedLog: `==> env.~lib/builtins/abort(message=4,fileName=22,lineNumber=1,columnNumber=2)
<== ()
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer stderr.Reset()

			messageOff, filenameOff := writeAbortMessageAndFileName(t, mod.Memory(), tc.messageUTF16, tc.fileNameUTF16)

			// Since abort panics, any opcodes afterwards cannot be reached.
			_ = require.CapturePanic(func() {
				abortWithMessage(testCtx, mod, messageOff, filenameOff, 1, 2)
			})
			require.Equal(t, "", stderr.String()) // nothing output if strings fail to read.
		})
	}
}

func TestSeed(t *testing.T) {
	mod, r := requireModule(t, wazero.NewModuleConfig().
		WithRandSource(bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7})))
	defer r.Close(testCtx)

	require.Equal(t, 7.949928895127363e-275, seed(mod))
}

func TestSeed_error(t *testing.T) {
	tests := []struct {
		name        string
		source      io.Reader
		expectedErr string
	}{
		{
			name:        "not 8 bytes",
			source:      bytes.NewReader([]byte{0, 1}),
			expectedErr: `error reading random seed: unexpected EOF`,
		},
		{
			name:        "error reading",
			source:      iotest.ErrReader(errors.New("ice cream")),
			expectedErr: `error reading random seed: ice cream`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r := requireModule(t, wazero.NewModuleConfig().WithRandSource(tc.source))
			defer r.Close(testCtx)

			err := require.CapturePanic(func() { seed(mod) })
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

// TestFunctionExporter_Trace ensures the trace output is according to configuration.
func TestFunctionExporter_Trace(t *testing.T) {
	noArgs := []uint64{4, 0, 0, 0, 0, 0, 0}
	noArgsLog := `==> env.~lib/builtins/trace(message=4,nArgs=0,arg0=0,arg1=0,arg2=0,arg3=0,arg4=0)
<== ()
`
	tests := []struct {
		name                  string
		exporter              FunctionExporter
		params                []uint64
		expected, expectedLog string
	}{
		{
			name:     "disabled",
			exporter: NewFunctionExporter(),
			params:   noArgs,
			expected: "",
			// expect no host call since it is disabled. ==> is host and --> is wasm.
			expectedLog: strings.ReplaceAll(noArgsLog, "==", "--"),
		},
		{
			name:        "ToStderr",
			exporter:    NewFunctionExporter().WithTraceToStderr(),
			params:      noArgs,
			expected:    "trace: hello\n",
			expectedLog: noArgsLog,
		},
		{
			name:        "ToStdout - no args",
			exporter:    NewFunctionExporter().WithTraceToStdout(),
			params:      noArgs,
			expected:    "trace: hello\n",
			expectedLog: noArgsLog,
		},
		{
			name:     "ToStdout - one arg",
			exporter: NewFunctionExporter().WithTraceToStdout(),
			params:   []uint64{4, 1, api.EncodeF64(1), 0, 0, 0, 0},
			expected: "trace: hello 1\n",
			expectedLog: `==> env.~lib/builtins/trace(message=4,nArgs=1,arg0=1,arg1=0,arg2=0,arg3=0,arg4=0)
<== ()
`,
		},
		{
			name:     "ToStdout - two args",
			exporter: NewFunctionExporter().WithTraceToStdout(),
			params:   []uint64{4, 2, api.EncodeF64(1), api.EncodeF64(2), 0, 0, 0},
			expected: "trace: hello 1,2\n",
			expectedLog: `==> env.~lib/builtins/trace(message=4,nArgs=2,arg0=1,arg1=2,arg2=0,arg3=0,arg4=0)
<== ()
`,
		},
		{
			name:     "ToStdout - five args",
			exporter: NewFunctionExporter().WithTraceToStdout(),
			params: []uint64{
				4,
				5,
				api.EncodeF64(1),
				api.EncodeF64(2),
				api.EncodeF64(3.3),
				api.EncodeF64(4.4),
				api.EncodeF64(5),
			},
			expected: "trace: hello 1,2,3.3,4.4,5\n",
			expectedLog: `==> env.~lib/builtins/trace(message=4,nArgs=5,arg0=1,arg1=2,arg2=3.3,arg3=4.4,arg4=5)
<== ()
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var stderr, functionLog bytes.Buffer

			// Set context to one that has an experimental listener
			ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&functionLog))

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
			defer r.Close(ctx)

			envBuilder := r.NewModuleBuilder("env")
			tc.exporter.ExportFunctions(envBuilder)
			_, err := envBuilder.Instantiate(ctx, r)
			require.NoError(t, err)

			traceWasm, err := watzero.Wat2Wasm(`(module
  (import "env" "trace" (func $~lib/builtins/trace (param i32 i32 f64 f64 f64 f64 f64)))
  (memory 1 1)
  (export "trace" (func 0))
)`)
			require.NoError(t, err)

			code, err := r.CompileModule(ctx, traceWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			config := wazero.NewModuleConfig()
			if strings.Contains("ToStderr", tc.name) {
				config = config.WithStderr(&stderr)
			} else {
				config = config.WithStdout(&stderr)
			}

			mod, err := r.InstantiateModule(ctx, code, config)
			require.NoError(t, err)

			message := encodeUTF16("hello")
			ok := mod.Memory().WriteUint32Le(ctx, 0, uint32(len(message)))
			require.True(t, ok)
			ok = mod.Memory().Write(ctx, uint32(4), message)
			require.True(t, ok)

			_, err = mod.ExportedFunction("trace").Call(ctx, tc.params...)
			require.NoError(t, err)
			require.Equal(t, tc.expected, stderr.String())
			require.Equal(t, tc.expectedLog, functionLog.String())
		})
	}
}

func TestTrace_error(t *testing.T) {
	tests := []struct {
		name        string
		message     []byte
		stderr      io.Writer
		expectedErr string
	}{
		{
			name:        "not 8 bytes",
			message:     encodeUTF16("hello")[:5],
			stderr:      &bytes.Buffer{},
			expectedErr: `invalid UTF-16 reading message`,
		},
		{
			name:        "error writing",
			message:     encodeUTF16("hello"),
			stderr:      &errWriter{err: errors.New("ice cream")},
			expectedErr: `ice cream`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r := requireModule(t, wazero.NewModuleConfig().WithStderr(tc.stderr))
			defer r.Close(testCtx)

			ok := mod.Memory().WriteUint32Le(testCtx, 0, uint32(len(tc.message)))
			require.True(t, ok)
			ok = mod.Memory().Write(testCtx, uint32(4), tc.message)
			require.True(t, ok)

			err := require.CapturePanic(func() {
				traceToStderr(testCtx, mod, 4, 0, 0, 0, 0, 0, 0)
			})
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func Test_requireAssemblyScriptString(t *testing.T) {
	var stderr bytes.Buffer
	mod, r := requireModule(t, wazero.NewModuleConfig().WithStderr(&stderr))
	defer r.Close(testCtx)

	tests := []struct {
		name                  string
		memory                func(context.Context, api.Memory)
		offset                int
		expected, expectedErr string
	}{
		{
			name: "success",
			memory: func(testCtx context.Context, memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 10)
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			offset:   4,
			expected: "hello",
		},
		{
			name: "can't read size",
			memory: func(testCtx context.Context, memory api.Memory) {
				b := encodeUTF16("hello")
				memory.Write(testCtx, 0, b)
			},
			offset:      0, // will attempt to read size from offset -4
			expectedErr: "out of memory reading message",
		},
		{
			name: "odd size",
			memory: func(testCtx context.Context, memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 9)
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			offset:      4,
			expectedErr: "invalid UTF-16 reading message",
		},
		{
			name: "can't read string",
			memory: func(testCtx context.Context, memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 10_000_000) // set size to too large value
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			offset:      4,
			expectedErr: "out of memory reading message",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			tc.memory(testCtx, mod.Memory())

			if tc.expectedErr != "" {
				err := require.CapturePanic(func() {
					_ = requireAssemblyScriptString(testCtx, mod, "message", uint32(tc.offset))
				})
				require.EqualError(t, err, tc.expectedErr)
			} else {
				s := requireAssemblyScriptString(testCtx, mod, "message", uint32(tc.offset))
				require.Equal(t, tc.expected, s)
			}
		})
	}
}

func writeAbortMessageAndFileName(t *testing.T, mem api.Memory, messageUTF16, fileNameUTF16 []byte) (uint32, uint32) {
	off := uint32(0)
	ok := mem.WriteUint32Le(testCtx, off, uint32(len(messageUTF16)))
	require.True(t, ok)
	off += 4
	messageOff := off
	ok = mem.Write(testCtx, off, messageUTF16)
	require.True(t, ok)
	off += uint32(len(messageUTF16))
	ok = mem.WriteUint32Le(testCtx, off, uint32(len(fileNameUTF16)))
	require.True(t, ok)
	off += 4
	filenameOff := off
	ok = mem.Write(testCtx, off, fileNameUTF16)
	require.True(t, ok)
	return messageOff, filenameOff
}

func encodeUTF16(s string) []byte {
	runes := utf16.Encode([]rune(s))
	b := make([]byte, len(runes)*2)
	for i, r := range runes {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return b
}

type errWriter struct {
	err error
}

func (w *errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func requireModule(t *testing.T, config wazero.ModuleConfig) (api.Module, api.Closer) {
	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())

	compiled, err := r.NewModuleBuilder(t.Name()).
		ExportMemoryWithMax("memory", 1, 1).
		Compile(testCtx, wazero.NewCompileConfig())
	require.NoError(t, err)

	mod, err := r.InstantiateModule(testCtx, compiled, config)
	require.NoError(t, err)
	return mod, r
}
