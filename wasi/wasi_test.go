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
			argsOpt, err := Args(tc.args)
			require.NoError(t, err)
			wasiEnv := NewEnvironment(argsOpt)
			hostFunctionCallContext := buildMockHostFunctionCallContext()

			// Serialize the expected result of args_size_get
			argCountPtr := uint32(0)
			expectedArgCount := make([]byte, 4)
			binary.LittleEndian.PutUint32(expectedArgCount, uint32(len(tc.args)))
			bufSizePtr := uint32(4)
			expectedBufSize := make([]byte, 4)
			binary.LittleEndian.PutUint32(expectedBufSize, tc.expectedBufSize)

			// Compare them
			errno := wasiEnv.args_sizes_get(hostFunctionCallContext, argCountPtr, bufSizePtr)
			require.Equal(t, ESUCCESS, errno)
			require.Equal(t, expectedArgCount, hostFunctionCallContext.Memory.Buffer[argCountPtr:argCountPtr+4])
			require.Equal(t, expectedBufSize, hostFunctionCallContext.Memory.Buffer[bufSizePtr:bufSizePtr+4])

			// Serialize the expected result of args_get
			expectedArgs := make([]byte, 4*len(tc.args))
			argsPtr := uint32(0)
			expectedArgv := make([]byte, tc.expectedBufSize)
			argvPtr := uint32(0x100)
			argvWritten := uint32(0)
			for i, arg := range tc.expectedArgs {
				binary.LittleEndian.PutUint32(expectedArgs[argsPtr+uint32(i*4):], argvPtr+argvWritten)
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

func TestArgsAPIReturnError(t *testing.T) {
	dummyArgs := []string{"foo", "bar", "baz"}
	argsOpt, err := Args(dummyArgs)
	require.NoError(t, err)
	wasiEnv := NewEnvironment(argsOpt)
	hostFunctionCallContext := buildMockHostFunctionCallContext()

	outOfBounds := uint32(len(hostFunctionCallContext.Memory.Buffer))

	argsArray, err := newWASIStringArray(dummyArgs)
	require.NoError(t, err)

	errno := wasiEnv.args_sizes_get(hostFunctionCallContext, outOfBounds, 0)
	require.Equal(t, EINVAL, errno)
	errno = wasiEnv.args_sizes_get(hostFunctionCallContext, 0, outOfBounds)
	require.Equal(t, EINVAL, errno)
	maxValidArgsCountPtr := outOfBounds - 4
	errno = wasiEnv.args_sizes_get(hostFunctionCallContext, maxValidArgsCountPtr+1, 0)
	require.Equal(t, EINVAL, errno)
	maxValidBufSizeCountPtr := outOfBounds - 4
	errno = wasiEnv.args_sizes_get(hostFunctionCallContext, 0, maxValidBufSizeCountPtr+1)
	require.Equal(t, EINVAL, errno)

	errno = wasiEnv.args_get(hostFunctionCallContext, outOfBounds, 0)
	require.Equal(t, EINVAL, errno)
	errno = wasiEnv.args_get(hostFunctionCallContext, 0, outOfBounds)
	require.Equal(t, EINVAL, errno)
	maxValidArgsPtr := outOfBounds - 4*uint32(len(argsArray.strings))
	errno = wasiEnv.args_get(hostFunctionCallContext, maxValidArgsPtr+1, 0)
	require.Equal(t, EINVAL, errno)
	maxValidArgsBufPtr := outOfBounds - argsArray.totalBufSize
	errno = wasiEnv.args_get(hostFunctionCallContext, 0, maxValidArgsBufPtr+1)
	require.Equal(t, EINVAL, errno)
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

			_, _, err = store.CallFunction("test", "_start")
			require.NoError(t, err)

			require.Equal(t, tc.expectedArgs, strings.TrimSpace(stdoutBuf.String()))
		})
	}
}
