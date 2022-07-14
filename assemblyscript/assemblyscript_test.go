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

var abortWat = `(module
  (import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))
  (memory 1 1)
  (export "abort" (func 0))
)`

var seedWat = `(module
  (import "env" "seed" (func $~lib/builtins/seed (result f64)))
  (memory 1 1)
  (export "seed" (func 0))
)`

var traceWat = `(module
  (import "env" "trace" (func $~lib/builtins/trace (param i32 i32 f64 f64 f64 f64 f64)))
  (memory 1 1)
  (export "trace" (func 0))
)`

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestAbort(t *testing.T) {
	tests := []struct {
		name     string
		exporter FunctionExporter
		expected string
	}{
		{
			name:     "enabled",
			exporter: NewFunctionExporter(),
			expected: "message at filename:1:2\n",
		},
		{
			name:     "disabled",
			exporter: NewFunctionExporter().WithAbortMessageDisabled(),
			expected: "",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var out, log bytes.Buffer

			// Set context to one that has an experimental listener
			ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
			defer r.Close(ctx)

			envBuilder := r.NewModuleBuilder("env")
			tc.exporter.ExportFunctions(envBuilder)
			_, err := envBuilder.Instantiate(ctx, r)
			require.NoError(t, err)

			abortWasm, err := watzero.Wat2Wasm(abortWat)
			require.NoError(t, err)

			code, err := r.CompileModule(ctx, abortWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			mod, err := r.InstantiateModule(ctx, code, wazero.NewModuleConfig().WithStderr(&out))
			require.NoError(t, err)

			messageOff, filenameOff := writeAbortMessageAndFileName(ctx, t, mod.Memory(), encodeUTF16("message"), encodeUTF16("filename"))

			_, err = mod.ExportedFunction("abort").Call(ctx, uint64(messageOff), uint64(filenameOff), 1, 2)
			require.Error(t, err)
			require.Equal(t, uint32(255), err.(*sys.ExitError).ExitCode())

			require.Equal(t, tc.expected, out.String())
			require.Equal(t, `==> env.~lib/builtins/abort(message=4,fileName=22,lineNumber=1,columnNumber=2)
`, log.String())
		})
	}
}

func TestAbort_Error(t *testing.T) {
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
			var out, log bytes.Buffer

			// Set context to one that has an experimental listener
			ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
			defer r.Close(ctx)

			_, err := Instantiate(ctx, r)
			require.NoError(t, err)

			abortWasm, err := watzero.Wat2Wasm(abortWat)
			require.NoError(t, err)

			compiled, err := r.CompileModule(ctx, abortWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			exporter := wazero.NewModuleConfig().WithName(t.Name()).WithStdout(&out)
			mod, err := r.InstantiateModule(ctx, compiled, exporter)
			require.NoError(t, err)

			messageOff, filenameOff := writeAbortMessageAndFileName(ctx, t, mod.Memory(), tc.messageUTF16, tc.fileNameUTF16)

			_, err = mod.ExportedFunction("abort").Call(ctx, uint64(messageOff), uint64(filenameOff), 1, 2)
			require.NoError(t, err)
			require.Equal(t, "", out.String()) // nothing output if strings fail to read.
			require.Equal(t, tc.expectedLog, log.String())
		})
	}
}

var unreachableAfterAbort = `(module
  (import "env" "abort" (func $~lib/builtins/abort (param i32 i32 i32 i32)))
  (func $main
    i32.const 0
    i32.const 0
    i32.const 0
    i32.const 0
    call $~lib/builtins/abort
	unreachable ;; If abort doesn't panic, this code is reached.
  )
  (start $main)
)`

// TestAbort_UnreachableAfter ensures code that follows an abort isn't invoked.
func TestAbort_UnreachableAfter(t *testing.T) {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx)

	envBuilder := r.NewModuleBuilder("env")
	// Disable the abort message as we are passing invalid memory offsets.
	NewFunctionExporter().WithAbortMessageDisabled().ExportFunctions(envBuilder)
	_, err := envBuilder.Instantiate(ctx, r)
	require.NoError(t, err)

	abortWasm, err := watzero.Wat2Wasm(unreachableAfterAbort)
	require.NoError(t, err)

	_, err = r.InstantiateModuleFromBinary(ctx, abortWasm)
	require.Error(t, err)
	require.Equal(t, uint32(255), err.(*sys.ExitError).ExitCode())
	require.Equal(t, `--> .main()
	==> env.~lib/builtins/abort(message=0,fileName=0,lineNumber=0,columnNumber=0)
`, log.String())
}

func TestSeed(t *testing.T) {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx)

	seed := []byte{0, 1, 2, 3, 4, 5, 6, 7}

	_, err := Instantiate(ctx, r)
	require.NoError(t, err)

	seedWasm, err := watzero.Wat2Wasm(seedWat)
	require.NoError(t, err)

	code, err := r.CompileModule(ctx, seedWasm, wazero.NewCompileConfig())
	require.NoError(t, err)

	mod, err := r.InstantiateModule(ctx, code, wazero.NewModuleConfig().WithRandSource(bytes.NewReader(seed)))
	require.NoError(t, err)

	seedFn := mod.ExportedFunction("seed")

	_, err = seedFn.Call(ctx)
	require.NoError(t, err)

	// If this test doesn't break, the seed is deterministic.
	require.Equal(t, `==> env.~lib/builtins/seed()
<== (7.949928895127363e-275)
`, log.String())
}

func TestSeed_error(t *testing.T) {
	tests := []struct {
		name        string
		source      io.Reader
		expectedErr string
	}{
		{
			name:   "not 8 bytes",
			source: bytes.NewReader([]byte{0, 1}),
			expectedErr: `error reading random seed: unexpected EOF (recovered by wazero)
wasm stack trace:
	env.~lib/builtins/seed() f64`,
		},
		{
			name:   "error reading",
			source: iotest.ErrReader(errors.New("ice cream")),
			expectedErr: `error reading random seed: ice cream (recovered by wazero)
wasm stack trace:
	env.~lib/builtins/seed() f64`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer

			// Set context to one that has an experimental listener
			ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
			defer r.Close(ctx)

			_, err := Instantiate(ctx, r)
			require.NoError(t, err)

			seedWasm, err := watzero.Wat2Wasm(seedWat)
			require.NoError(t, err)

			compiled, err := r.CompileModule(ctx, seedWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			exporter := wazero.NewModuleConfig().WithName(t.Name()).WithRandSource(tc.source)
			mod, err := r.InstantiateModule(ctx, compiled, exporter)
			require.NoError(t, err)

			_, err = mod.ExportedFunction("seed").Call(ctx)
			require.EqualError(t, err, tc.expectedErr)
			require.Equal(t, `==> env.~lib/builtins/seed()
`, log.String())
		})
	}
}

func TestTrace(t *testing.T) {
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
			name:        "disabled",
			exporter:    NewFunctionExporter(),
			params:      noArgs,
			expected:    "",
			expectedLog: noArgsLog,
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
			var out, log bytes.Buffer

			// Set context to one that has an experimental listener
			ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
			defer r.Close(ctx)

			envBuilder := r.NewModuleBuilder("env")
			tc.exporter.ExportFunctions(envBuilder)
			_, err := envBuilder.Instantiate(ctx, r)
			require.NoError(t, err)

			traceWasm, err := watzero.Wat2Wasm(traceWat)
			require.NoError(t, err)

			code, err := r.CompileModule(ctx, traceWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			config := wazero.NewModuleConfig()
			if strings.Contains("ToStderr", tc.name) {
				config = config.WithStderr(&out)
			} else {
				config = config.WithStdout(&out)
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
			require.Equal(t, tc.expected, out.String())
			require.Equal(t, tc.expectedLog, log.String())
		})
	}
}

func TestTrace_error(t *testing.T) {
	tests := []struct {
		name        string
		message     []byte
		out         io.Writer
		expectedErr string
	}{
		{
			name:    "not 8 bytes",
			message: encodeUTF16("hello")[:5],
			out:     &bytes.Buffer{},
			expectedErr: `read an odd number of bytes for utf-16 string: 5 (recovered by wazero)
wasm stack trace:
	env.~lib/builtins/trace(i32,i32,f64,f64,f64,f64,f64)`,
		},
		{
			name:    "error writing",
			message: encodeUTF16("hello"),
			out:     &errWriter{err: errors.New("ice cream")},
			expectedErr: `ice cream (recovered by wazero)
wasm stack trace:
	env.~lib/builtins/trace(i32,i32,f64,f64,f64,f64,f64)`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer

			// Set context to one that has an experimental listener
			ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, NewLoggingListenerFactory(&log))

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
			defer r.Close(ctx)

			envBuilder := r.NewModuleBuilder("env")
			NewFunctionExporter().WithTraceToStdout().ExportFunctions(envBuilder)
			_, err := envBuilder.Instantiate(ctx, r)
			require.NoError(t, err)

			traceWasm, err := watzero.Wat2Wasm(traceWat)
			require.NoError(t, err)

			compiled, err := r.CompileModule(ctx, traceWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			exporter := wazero.NewModuleConfig().WithName(t.Name()).WithStdout(tc.out)
			mod, err := r.InstantiateModule(ctx, compiled, exporter)
			require.NoError(t, err)

			ok := mod.Memory().WriteUint32Le(ctx, 0, uint32(len(tc.message)))
			require.True(t, ok)
			ok = mod.Memory().Write(ctx, uint32(4), tc.message)
			require.True(t, ok)

			_, err = mod.ExportedFunction("trace").Call(ctx, 4, 0, 0, 0, 0, 0, 0)
			require.EqualError(t, err, tc.expectedErr)
			require.Equal(t, `==> env.~lib/builtins/trace(message=4,nArgs=0,arg0=0,arg1=0,arg2=0,arg3=0,arg4=0)
`, log.String())
		})
	}
}

func Test_readAssemblyScriptString(t *testing.T) {
	tests := []struct {
		name                  string
		memory                func(context.Context, api.Memory)
		offset                int
		expected, expectedErr string
	}{
		{
			name: "success",
			memory: func(ctx context.Context, memory api.Memory) {
				memory.WriteUint32Le(ctx, 0, 10)
				b := encodeUTF16("hello")
				memory.Write(ctx, 4, b)
			},
			offset:   4,
			expected: "hello",
		},
		{
			name: "can't read size",
			memory: func(ctx context.Context, memory api.Memory) {
				b := encodeUTF16("hello")
				memory.Write(ctx, 0, b)
			},
			offset:      0, // will attempt to read size from offset -4
			expectedErr: "Memory.ReadUint32Le(4294967292) out of range",
		},
		{
			name: "odd size",
			memory: func(ctx context.Context, memory api.Memory) {
				memory.WriteUint32Le(ctx, 0, 9)
				b := encodeUTF16("hello")
				memory.Write(ctx, 4, b)
			},
			offset:      4,
			expectedErr: "read an odd number of bytes for utf-16 string: 9",
		},
		{
			name: "can't read string",
			memory: func(ctx context.Context, memory api.Memory) {
				memory.WriteUint32Le(ctx, 0, 10_000_000) // set size to too large value
				b := encodeUTF16("hello")
				memory.Write(ctx, 4, b)
			},
			offset:      4,
			expectedErr: "Memory.Read(4, 10000000) out of range",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			mod, err := r.NewModuleBuilder("mod").
				ExportMemory("memory", 1).
				Instantiate(testCtx, r)
			require.NoError(t, err)

			tc.memory(testCtx, mod.Memory())

			s, err := readAssemblyScriptString(testCtx, mod, uint32(tc.offset))
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, s)
			}
		})
	}
}

func writeAbortMessageAndFileName(ctx context.Context, t *testing.T, mem api.Memory, messageUTF16, fileNameUTF16 []byte) (int, int) {
	off := 0
	ok := mem.WriteUint32Le(ctx, uint32(off), uint32(len(messageUTF16)))
	require.True(t, ok)
	off += 4
	messageOff := off
	ok = mem.Write(ctx, uint32(off), messageUTF16)
	require.True(t, ok)
	off += len(messageUTF16)
	ok = mem.WriteUint32Le(ctx, uint32(off), uint32(len(fileNameUTF16)))
	require.True(t, ok)
	off += 4
	filenameOff := off
	ok = mem.Write(ctx, uint32(off), fileNameUTF16)
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
