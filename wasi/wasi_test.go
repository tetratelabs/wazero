package wasi

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	wbinary "github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

func TestArgsAPISucceed(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedArgs    [][]byte
		expectedBufSize uint32
	}{
		{
			name:            "no args",
			args:            nil,
			expectedArgs:    [][]byte{},
			expectedBufSize: 0,
		},
		{
			name:            "empty",
			args:            []string{},
			expectedArgs:    [][]byte{},
			expectedBufSize: 0,
		},
		{
			name: "simple",
			args: []string{"foo", "bar", "foobar", "", "baz"},
			expectedArgs: [][]byte{
				[]byte("foo\x00"),
				[]byte("bar\x00"),
				[]byte("foobar\x00"),
				[]byte("\x00"),
				[]byte("baz\x00"),
			},
			expectedBufSize: 20,
		},
		{
			name: "utf-8 string",
			// "😨", "🤣", and "️🏃‍♀️" have 4, 4, and 13 bytes respectively
			args: []string{"😨🤣🏃\u200d♀️", "foo", "bar"},
			expectedArgs: [][]byte{
				[]byte("😨🤣🏃\u200d♀️\x00"),
				[]byte("foo\x00"),
				[]byte("bar\x00"),
			},
			expectedBufSize: 30,
		},
		{
			name: "invalid utf-8 string",
			args: []string{"\xff\xfe\xfd", "foo", "bar"},
			expectedArgs: [][]byte{
				[]byte("\xff\xfe\xfd\x00"),
				[]byte("foo\x00"),
				[]byte("bar\x00"),
			},
			expectedBufSize: 12,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			opts := []Option{}
			if tc.args != nil {
				argsOpt, err := Args(tc.args)
				require.NoError(t, err)
				opts = append(opts, argsOpt)
			}
			wasiEnv := NewEnvironment(opts...)
			hostFunctionCallContext := buildMockHostFunctionCallContext()

			// Serialize the expected result of args_size_get
			argCountPtr := uint32(0) // arbitrary valid address
			expectedArgCount := make([]byte, SIZE_UINT32)
			binary.LittleEndian.PutUint32(expectedArgCount, uint32(len(tc.args)))
			bufSizePtr := uint32(0x100) // arbitrary valid address that doesn't overwrap with argCountPtr
			expectedBufSize := make([]byte, SIZE_UINT32)
			binary.LittleEndian.PutUint32(expectedBufSize, tc.expectedBufSize)

			// Compare them
			errno := wasiEnv.args_sizes_get(hostFunctionCallContext, argCountPtr, bufSizePtr)
			require.Equal(t, ESUCCESS, errno)
			require.Equal(t, expectedArgCount, hostFunctionCallContext.Memory.Buffer[argCountPtr:argCountPtr+4])
			require.Equal(t, expectedBufSize, hostFunctionCallContext.Memory.Buffer[bufSizePtr:bufSizePtr+4])

			// Serialize the expected result of args_get
			expectedArgs := make([]byte, SIZE_UINT32*len(tc.args))
			argsPtr := uint32(0) // arbitrary valid address
			expectedArgv := make([]byte, tc.expectedBufSize)
			argvPtr := uint32(0x100) // arbitrary valid address that doesn't overwrap with argsPtr
			argvWritten := uint32(0)
			for i, arg := range tc.expectedArgs {
				binary.LittleEndian.PutUint32(expectedArgs[argsPtr+uint32(i*SIZE_UINT32):], argvPtr+argvWritten)
				copy(expectedArgv[argvWritten:], arg)
				argvWritten += uint32(len(arg))
			}

			// Compare them
			errno = wasiEnv.args_get(hostFunctionCallContext, argsPtr, argvPtr)
			require.Equal(t, ESUCCESS, errno)
			require.Equal(t, expectedArgs, hostFunctionCallContext.Memory.Buffer[argsPtr:argsPtr+uint32(len(expectedArgs))])
			require.Equal(t, expectedArgv, hostFunctionCallContext.Memory.Buffer[argvPtr:argvPtr+uint32(len(expectedArgv))])
		})
	}
}

func TestArgsSizesGetReturnError(t *testing.T) {
	dummyArgs := []string{"foo", "bar", "baz"}
	argsOpt, err := Args(dummyArgs)
	require.NoError(t, err)
	wasiEnv := NewEnvironment(argsOpt)
	hostFunctionCallContext := buildMockHostFunctionCallContext()

	outOfBounds := uint32(len(hostFunctionCallContext.Memory.Buffer))

	tests := []struct {
		name           string
		argsCountPtr   uint32
		argsBufSizePtr uint32
	}{
		{
			name:           "out-of-bound argsCountPtr",
			argsCountPtr:   outOfBounds,
			argsBufSizePtr: 0,
		},
		{
			name:           "out-of-bound argsBufSizePtr",
			argsCountPtr:   0,
			argsBufSizePtr: outOfBounds,
		},
		{
			name:           "out-of-bound boundary argsCountPtr",
			argsCountPtr:   outOfBounds - SIZE_UINT32 + 1,
			argsBufSizePtr: 0,
		},
		{
			name:           "out-of-bound boundary argsBufSizePtr",
			argsCountPtr:   0,
			argsBufSizePtr: outOfBounds - SIZE_UINT32 + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			errno := wasiEnv.args_sizes_get(hostFunctionCallContext, tc.argsCountPtr, tc.argsBufSizePtr)
			require.Equal(t, EINVAL, errno)
		})
	}
}

func TestArgsGetAPIReturnError(t *testing.T) {
	dummyArgs := []string{"foo", "bar", "baz"}
	argsOpt, err := Args(dummyArgs)
	require.NoError(t, err)
	wasiEnv := NewEnvironment(argsOpt)
	hostFunctionCallContext := buildMockHostFunctionCallContext()

	outOfBounds := uint32(len(hostFunctionCallContext.Memory.Buffer))
	argsArray, err := newWASIStringArray(dummyArgs)
	require.NoError(t, err)

	tests := []struct {
		name       string
		argsPtr    uint32
		argsBufPtr uint32
	}{
		{
			name:       "out-of-bound argsPtr",
			argsPtr:    outOfBounds,
			argsBufPtr: 0,
		},
		{
			name:       "out-of-bound argsBufPtr",
			argsPtr:    0,
			argsBufPtr: outOfBounds,
		},
		{
			name:       "out-of-bound boundary argsPtr",
			argsPtr:    outOfBounds - SIZE_UINT32*uint32(len(argsArray.strings)) + 1,
			argsBufPtr: 0,
		},
		{
			name:       "out-of-bound boundary argsBufPtr",
			argsPtr:    0,
			argsBufPtr: outOfBounds - argsArray.totalBufSize + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			errno := wasiEnv.args_get(hostFunctionCallContext, tc.argsPtr, tc.argsBufPtr)
			require.Equal(t, EINVAL, errno)
		})
	}
}

func buildMockHostFunctionCallContext() *wasm.HostFunctionCallContext {
	return &wasm.HostFunctionCallContext{
		Memory: &wasm.MemoryInstance{Buffer: make([]byte, wasm.PageSize), Min: 1},
	}
}

// Test whether the specs implemented by the args API match those understood by a language runtime.
func TestArgsAPICompatiblity(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedArgs string
	}{
		{
			name:         "empty",
			args:         []string{},
			expectedArgs: "os.Args: []",
		},
		{
			name:         "simple",
			args:         []string{"foo", "bar", "foobar", "", "baz"},
			expectedArgs: "os.Args: [foo bar foobar  baz]",
		},
	}

	buf, err := os.ReadFile("testdata/args.wasm")
	require.NoError(t, err)

	mod, err := wbinary.DecodeModule(buf)
	require.NoError(t, err)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			store := wasm.NewStore(interpreter.NewEngine())
			require.NoError(t, err)

			stdoutBuf := bytes.NewBuffer(nil)
			args, err := Args(tc.args)
			require.NoError(t, err)
			wasiEnv := NewEnvironment(args, Stdout(stdoutBuf))

			err = wasiEnv.Register(store)
			require.NoError(t, err)

			err = store.Instantiate(mod, "test")
			require.NoError(t, err)

			// XXX Strictly speaking, this test code violates the WASI specification.
			// The WASI specification does not guarantee that a function exported from a WASI command
			// can be called outside the context of `_start`.
			//   > Command instances may assume that none of their exports are accessed outside the duraction of that call.
			// Link: https://github.com/WebAssembly/WASI/blob/db4e3a12dadbe3e7e41dddd04888db3bf1cf7a96/design/application-abi.md
			// In fact, calling a WASI function from a normal exported function without calling `_start` first in TinyGo crashes.
			//
			// However, once `_start` is called, it appears that WASI functions can be called from exported functions.
			// We believe it's unlikely TinyGo wil break this behavior in the future.
			// So, we call the test helper functions directly after calling `_start` once for more concise testing.

			// Let TinyGo runtime initialize the WASI environment by calling _start
			_, _, err = store.CallFunction("test", "_start")
			require.NoError(t, err)

			// Call a test function directly
			_, _, err = store.CallFunction("test", "PrintArgs")
			require.NoError(t, err)

			require.Equal(t, tc.expectedArgs, strings.TrimSpace(stdoutBuf.String()))
		})
	}
}
