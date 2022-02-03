package wasi

import (
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/text"
)

func TestNewWasiStringArray(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedBufSize uint32
	}{
		{
			name:            "nil args",
			args:            nil,
			expectedBufSize: 0,
		},
		{
			name:            "empty",
			args:            []string{},
			expectedBufSize: 0,
		},
		{
			name:            "simple",
			args:            []string{"foo", "bar", "foobar", "", "baz"},
			expectedBufSize: 20,
		},
		{
			name: "utf-8 string",
			// "😨", "🤣", and "️🏃‍♀️" have 4, 4, and 13 bytes respectively
			args:            []string{"😨🤣🏃\u200d♀️", "foo", "bar"},
			expectedBufSize: 30,
		},
		{
			name:            "invalid utf-8 string",
			args:            []string{"\xff\xfe\xfd", "foo", "bar"},
			expectedBufSize: 12,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			wasiStringsArray, err := newWASIStringArray(tc.args)
			require.NoError(t, err)

			require.Equal(t, tc.expectedBufSize, wasiStringsArray.totalBufSize)
			require.Equal(t, len(wasiStringsArray.nullTerminatedValues), len(tc.args))
			for i, arg := range tc.args {
				wasiString := wasiStringsArray.nullTerminatedValues[i]
				require.Equal(t, wasiString[0:len(wasiString)-1], []byte(arg))
				require.Equal(t, wasiString[len(wasiString)-1], byte(0))
			}
		})
	}
}

// argsWat is a wasm module to call args_get and args_sizes_get.
//go:embed testdata/args.wat
var argsWat []byte

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
			store := instantiateWasmStore(t, argsWat, "test", wasiEnv)

			// Serialize the expected result of args_size_get
			argCountPtr := uint32(0)            // arbitrary valid address
			expectedArgCount := make([]byte, 4) // size of uint32
			binary.LittleEndian.PutUint32(expectedArgCount, uint32(len(tc.args)))
			bufSizePtr := uint32(0x100)        // arbitrary valid address that doesn't overwrap with argCountPtr
			expectedBufSize := make([]byte, 4) // size of uint32
			binary.LittleEndian.PutUint32(expectedBufSize, tc.expectedBufSize)

			// Compare them
			ret, _, err := store.CallFunction("test", "args_sizes_get", uint64(argCountPtr), uint64(bufSizePtr))
			require.NoError(t, err)
			require.Equal(t, uint64(ESUCCESS), ret[0]) // ret[0] is errno
			require.Equal(t, expectedArgCount, store.Memories[0].Buffer[argCountPtr:argCountPtr+4])
			require.Equal(t, expectedBufSize, store.Memories[0].Buffer[bufSizePtr:bufSizePtr+4])

			// Serialize the expected result of args_get
			expectedArgs := make([]byte, 4*len(tc.args)) // expected size of the pointers to the args. 4 is the size of uint32
			argsPtr := uint32(0)                         // arbitrary valid address
			expectedArgv := make([]byte, tc.expectedBufSize)
			argvPtr := uint32(0x100) // arbitrary valid address that doesn't overwrap with argsPtr
			argvWritten := uint32(0)
			for i, arg := range tc.expectedArgs {
				binary.LittleEndian.PutUint32(expectedArgs[argsPtr+uint32(i*4):], argvPtr+argvWritten) // 4 is the size of uint32
				copy(expectedArgv[argvWritten:], arg)
				argvWritten += uint32(len(arg))
			}

			// Compare them
			ret, _, err = store.CallFunction("test", "args_get", uint64(argsPtr), uint64(argvPtr))
			require.NoError(t, err)
			require.Equal(t, uint64(ESUCCESS), ret[0]) // ret[0] is the returned errno
			require.Equal(t, expectedArgs, store.Memories[0].Buffer[argsPtr:argsPtr+uint32(len(expectedArgs))])
			require.Equal(t, expectedArgv, store.Memories[0].Buffer[argvPtr:argvPtr+uint32(len(expectedArgv))])
		})
	}
}

func TestArgsSizesGetReturnError(t *testing.T) {
	dummyArgs := []string{"foo", "bar", "baz"}
	argsOpt, err := Args(dummyArgs)
	require.NoError(t, err)
	wasiEnv := NewEnvironment(argsOpt)
	store := instantiateWasmStore(t, argsWat, "test", wasiEnv)

	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_sizes_get. We chose 0 here.

	tests := []struct {
		name           string
		argsCountPtr   uint32
		argsBufSizePtr uint32
	}{
		{
			name:           "out-of-memory argsCountPtr",
			argsCountPtr:   memorySize,
			argsBufSizePtr: validAddress,
		},
		{
			name:           "out-of-memory argsBufSizePtr",
			argsCountPtr:   validAddress,
			argsBufSizePtr: memorySize,
		},
		{
			name:           "argsCountPtr exceeds the maximum valid address by 1",
			argsCountPtr:   memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			argsBufSizePtr: validAddress,
		},
		{
			name:           "argsBufSizePtr exceeds the maximum valid size by 1",
			argsCountPtr:   validAddress,
			argsBufSizePtr: memorySize - 4 + 1, // 4 is the size of uint32, the type of the buffer size
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ret, _, err := store.CallFunction("test", "args_sizes_get", uint64(tc.argsCountPtr), uint64(tc.argsBufSizePtr))
			require.NoError(t, err)
			require.Equal(t, uint64(EINVAL), ret[0]) // ret[0] is returned errno
		})
	}
}

func TestArgsGetAPIReturnError(t *testing.T) {
	dummyArgs := []string{"foo", "bar", "baz"}
	argsOpt, err := Args(dummyArgs)
	require.NoError(t, err)
	wasiEnv := NewEnvironment(argsOpt)
	store := instantiateWasmStore(t, argsWat, "test", wasiEnv)

	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_get. We chose 0 here.
	argsArray, err := newWASIStringArray(dummyArgs)
	require.NoError(t, err)

	tests := []struct {
		name       string
		argsPtr    uint32
		argsBufPtr uint32
	}{
		{
			name:       "out-of-memory argsPtr",
			argsPtr:    memorySize,
			argsBufPtr: validAddress,
		},
		{
			name:       "out-of-memory argsBufPtr",
			argsPtr:    validAddress,
			argsBufPtr: memorySize,
		},
		{
			name: "argsPtr exceeds the maximum valid address by 1",
			// 4*uint32(len(argsArray.nullTerminatedValues)) is the size of the result of the pointers to args, 4 is the size of uint32
			argsPtr:    memorySize - 4*uint32(len(argsArray.nullTerminatedValues)) + 1,
			argsBufPtr: validAddress,
		},
		{
			name:       "argsBufPtr exceeds the maximum valid address by 1",
			argsPtr:    validAddress,
			argsBufPtr: memorySize - argsArray.totalBufSize + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ret, _, err := store.CallFunction("test", "args_get", uint64(tc.argsPtr), uint64(tc.argsBufPtr))
			require.NoError(t, err)
			require.Equal(t, uint64(EINVAL), ret[0]) // ret[0] is returned errno
		})
	}
}

func instantiateWasmStore(t *testing.T, wat []byte, moduleName string, wasiEnv *WASIEnvironment) *wasm.Store {
	mod, err := text.DecodeModule(wat)
	require.NoError(t, err)

	store := wasm.NewStore(interpreter.NewEngine())
	err = wasiEnv.Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, moduleName)
	require.NoError(t, err)

	return store
}

func TestClockGetTime(t *testing.T) {
	wasiEnv := NewEnvironment()
	expected := uint64(0xffffffffffffffff)
	wasiEnv.getTimeNanosFn = func() uint64 { return expected }
	store := instantiateWasmStore(t, argsWat, "test", wasiEnv)
	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_get. We chose 0 here.

	tests := []struct {
		name         string
		timestampVal uint64
		timestampPtr uint32
		result       Errno
	}{
		{
			name:         "low uint64 value",
			timestampVal: 12345,
			timestampPtr: validAddress,
			result:       ESUCCESS,
		},
		{
			name:         "high int64 value - no truncation",
			timestampVal: 0xffffffffffffffff,
			timestampPtr: validAddress,
			result:       ESUCCESS,
		},
		{
			name:         "timestampPtr exceeds the maximum valid address by 1",
			timestampVal: 0xffffffffffffffff,
			timestampPtr: memorySize - 8 + 1,
			result:       EINVAL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected = tt.timestampVal
			ret, _, err := store.CallFunction("test", "clock_time_get", uint64(0), uint64(0), uint64(tt.timestampPtr))
			require.NoError(t, err)
			errno := Errno(ret[0])
			require.Equal(t, tt.result, errno) // ret[0] is returned errno
			if errno == ESUCCESS {
				nanos := binary.LittleEndian.Uint64(store.Memories[0].Buffer)
				assert.Equal(t, expected, nanos)
			}
		})
	}
}
