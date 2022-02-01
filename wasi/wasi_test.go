package wasi

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
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
