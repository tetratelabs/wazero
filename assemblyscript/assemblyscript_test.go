package assemblyscript

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"
	"testing"
	"testing/iotest"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
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
		enabled  bool
		expected string
	}{
		{
			name:     "enabled",
			enabled:  true,
			expected: "message at filename:1:2\n",
		},
		{
			name:     "disabled",
			enabled:  false,
			expected: "",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			out := &bytes.Buffer{}

			if tc.enabled {
				_, err := Instantiate(testCtx, r)
				require.NoError(t, err)
			} else {
				_, err := NewBuilder(r).WithAbortMessageDisabled().Instantiate(testCtx, r)
				require.NoError(t, err)
			}

			abortWasm, err := watzero.Wat2Wasm(abortWat)
			require.NoError(t, err)

			code, err := r.CompileModule(testCtx, abortWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			mod, err := r.InstantiateModule(testCtx, code, wazero.NewModuleConfig().WithStderr(out))
			require.NoError(t, err)

			messageOff, filenameOff := writeAbortMessageAndFileName(t, mod.Memory(), encodeUTF16("message"), encodeUTF16("filename"))

			_, err = mod.ExportedFunction("abort").Call(testCtx, uint64(messageOff), uint64(filenameOff), 1, 2)
			require.Error(t, err)
			require.Equal(t, uint32(255), err.(*sys.ExitError).ExitCode())

			require.Equal(t, tc.expected, out.String())
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

// TestAbort_StopsExecution ensures code that follows an abort isn't invoked.
func TestAbort_StopsExecution(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	_, err := NewBuilder(r).WithAbortMessageDisabled().Instantiate(testCtx, r)
	require.NoError(t, err)

	abortWasm, err := watzero.Wat2Wasm(unreachableAfterAbort)
	require.NoError(t, err)

	_, err = r.InstantiateModuleFromBinary(testCtx, abortWasm)
	require.Error(t, err)
	require.Equal(t, uint32(255), err.(*sys.ExitError).ExitCode())
}

func TestSeed(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	seed := []byte{0, 1, 2, 3, 4, 5, 6, 7}

	_, err := Instantiate(testCtx, r)
	require.NoError(t, err)

	seedWasm, err := watzero.Wat2Wasm(seedWat)
	require.NoError(t, err)

	code, err := r.CompileModule(testCtx, seedWasm, wazero.NewCompileConfig())
	require.NoError(t, err)

	mod, err := r.InstantiateModule(testCtx, code, wazero.NewModuleConfig().WithRandSource(bytes.NewReader(seed)))
	require.NoError(t, err)

	seedFn := mod.ExportedFunction("seed")

	res, err := seedFn.Call(testCtx)
	require.NoError(t, err)

	// If this test doesn't break, the seed is deterministic.
	require.Equal(t, uint64(506097522914230528), res[0])
}

func TestTrace(t *testing.T) {
	noArgs := []uint64{4, 0, 0, 0, 0, 0, 0}
	tests := []struct {
		name     string
		mode     traceMode
		params   []uint64
		expected string
	}{
		{
			name:     "stderr",
			mode:     traceStderr,
			params:   noArgs,
			expected: "trace: hello\n",
		},
		{
			name:     "stdout",
			mode:     traceStdout,
			params:   noArgs,
			expected: "trace: hello\n",
		},
		{
			name:     "disabled",
			mode:     traceDisabled,
			params:   noArgs,
			expected: "",
		},
		{
			name:     "one",
			mode:     traceStdout,
			params:   []uint64{4, 1, api.EncodeF64(1), 0, 0, 0, 0},
			expected: "trace: hello 1\n",
		},
		{
			name:     "two",
			mode:     traceStdout,
			params:   []uint64{4, 2, api.EncodeF64(1), api.EncodeF64(2), 0, 0, 0},
			expected: "trace: hello 1,2\n",
		},
		{
			name: "five",
			mode: traceStdout,
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
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			out := &bytes.Buffer{}

			as := NewBuilder(r)
			modConfig := wazero.NewModuleConfig()
			switch tc.mode {
			case traceStderr:
				as = as.WithTraceToStderr()
				modConfig = modConfig.WithStderr(out)
			case traceStdout:
				as = as.WithTraceToStdout()
				modConfig = modConfig.WithStdout(out)
			case traceDisabled:
				// Set but not used
				modConfig = modConfig.WithStderr(out)
				modConfig = modConfig.WithStdout(out)
			}

			_, err := as.Instantiate(testCtx, r)
			require.NoError(t, err)

			traceWasm, err := watzero.Wat2Wasm(traceWat)
			require.NoError(t, err)

			code, err := r.CompileModule(testCtx, traceWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			mod, err := r.InstantiateModule(testCtx, code, modConfig)
			require.NoError(t, err)

			message := encodeUTF16("hello")
			ok := mod.Memory().WriteUint32Le(testCtx, 0, uint32(len(message)))
			require.True(t, ok)
			ok = mod.Memory().Write(testCtx, uint32(4), message)
			require.True(t, ok)

			_, err = mod.ExportedFunction("trace").Call(testCtx, tc.params...)
			require.NoError(t, err)
			require.Equal(t, tc.expected, out.String())
		})
	}
}

func TestReadAssemblyScriptString(t *testing.T) {
	tests := []struct {
		name                  string
		memory                func(api.Memory)
		offset                int
		expected, expectedErr string
	}{
		{
			name: "success",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 10)
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			offset:   4,
			expected: "hello",
		},
		{
			name: "can't read size",
			memory: func(memory api.Memory) {
				b := encodeUTF16("hello")
				memory.Write(testCtx, 0, b)
			},
			offset:      0, // will attempt to read size from offset -4
			expectedErr: "Memory.ReadUint32Le(4294967292) out of range",
		},
		{
			name: "odd size",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 9)
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			offset:      4,
			expectedErr: "read an odd number of bytes for utf-16 string: 9",
		},
		{
			name: "can't read string",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 10_000_000) // set size to too large value
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
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

			tc.memory(mod.Memory())

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

func TestAbort_error(t *testing.T) {
	tests := []struct {
		name          string
		messageUTF16  []byte
		fileNameUTF16 []byte
	}{
		{
			name:          "bad message",
			messageUTF16:  encodeUTF16("message")[:5],
			fileNameUTF16: encodeUTF16("filename"),
		},
		{
			name:          "bad filename",
			messageUTF16:  encodeUTF16("message"),
			fileNameUTF16: encodeUTF16("filename")[:5],
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			_, err := Instantiate(testCtx, r)
			require.NoError(t, err)

			abortWasm, err := watzero.Wat2Wasm(abortWat)
			require.NoError(t, err)

			compiled, err := r.CompileModule(testCtx, abortWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			out := &bytes.Buffer{}
			config := wazero.NewModuleConfig().WithName(t.Name()).WithStdout(out)
			mod, err := r.InstantiateModule(testCtx, compiled, config)
			require.NoError(t, err)

			messageOff, filenameOff := writeAbortMessageAndFileName(t, mod.Memory(), tc.messageUTF16, tc.fileNameUTF16)

			_, err = mod.ExportedFunction("abort").Call(testCtx, uint64(messageOff), uint64(filenameOff), 1, 2)
			require.NoError(t, err)
			require.Equal(t, "", out.String()) // nothing output if strings fail to read.
		})
	}
}

func writeAbortMessageAndFileName(t *testing.T, mem api.Memory, messageUTF16, fileNameUTF16 []byte) (int, int) {
	off := 0
	ok := mem.WriteUint32Le(testCtx, uint32(off), uint32(len(messageUTF16)))
	require.True(t, ok)
	off += 4
	messageOff := off
	ok = mem.Write(testCtx, uint32(off), messageUTF16)
	require.True(t, ok)
	off += len(messageUTF16)
	ok = mem.WriteUint32Le(testCtx, uint32(off), uint32(len(fileNameUTF16)))
	require.True(t, ok)
	off += 4
	filenameOff := off
	ok = mem.Write(testCtx, uint32(off), fileNameUTF16)
	require.True(t, ok)
	return messageOff, filenameOff
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
	env.seed() f64`,
		},
		{
			name:   "error reading",
			source: iotest.ErrReader(errors.New("ice cream")),
			expectedErr: `error reading random seed: ice cream (recovered by wazero)
wasm stack trace:
	env.seed() f64`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			_, err := Instantiate(testCtx, r)
			require.NoError(t, err)

			seedWasm, err := watzero.Wat2Wasm(seedWat)
			require.NoError(t, err)

			compiled, err := r.CompileModule(testCtx, seedWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			config := wazero.NewModuleConfig().WithName(t.Name()).WithRandSource(tc.source)
			mod, err := r.InstantiateModule(testCtx, compiled, config)
			require.NoError(t, err)

			_, err = mod.ExportedFunction("seed").Call(testCtx)
			require.EqualError(t, err, tc.expectedErr)
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
	env.trace(i32,i32,f64,f64,f64,f64,f64)`,
		},
		{
			name:    "error writing",
			message: encodeUTF16("hello"),
			out:     &errWriter{err: errors.New("ice cream")},
			expectedErr: `ice cream (recovered by wazero)
wasm stack trace:
	env.trace(i32,i32,f64,f64,f64,f64,f64)`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			_, err := NewBuilder(r).WithTraceToStdout().Instantiate(testCtx, r)
			require.NoError(t, err)

			traceWasm, err := watzero.Wat2Wasm(traceWat)
			require.NoError(t, err)

			compiled, err := r.CompileModule(testCtx, traceWasm, wazero.NewCompileConfig())
			require.NoError(t, err)

			config := wazero.NewModuleConfig().WithName(t.Name()).WithStdout(tc.out)
			mod, err := r.InstantiateModule(testCtx, compiled, config)
			require.NoError(t, err)

			ok := mod.Memory().WriteUint32Le(testCtx, 0, uint32(len(tc.message)))
			require.True(t, ok)
			ok = mod.Memory().Write(testCtx, uint32(4), tc.message)
			require.True(t, ok)

			_, err = mod.ExportedFunction("trace").Call(testCtx, 4, 0, 0, 0, 0, 0, 0)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
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
