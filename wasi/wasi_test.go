package wasi

import (
	"context"
	_ "embed"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/text"
)

func TestNewAPI_Args(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		o, err := Args("a", "bc")
		require.NoError(t, err)
		a := newAPI(o)
		require.Equal(t, &nullTerminatedStrings{
			nullTerminatedValues: [][]byte{
				{'a', 0},
				{'b', 'c', 0},
			},
			totalBufSize: 5,
		}, a.args)
	})
	t.Run("error constructing args", func(t *testing.T) {
		_, err := Args("\xff\xfe\xfd", "foo", "bar")
		require.EqualError(t, err, "arg[0] is not a valid UTF-8 string")
	})
}

// argsWat is a wasm module to call args_get and args_sizes_get.
//go:embed testdata/args.wat
var argsWat []byte

func TestApi_ArgsGet(t *testing.T) {
	ctx := context.Background()
	args, err := Args("a", "bc")
	require.NoError(t, err)

	argv := uint32(7)    // arbitrary offset
	argvBuf := uint32(1) // arbitrary offset
	maskLength := 16     // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',                 // argvBuf is after this
		'a', 0, 'b', 'c', 0, // null terminated "a", "bc"
		'?',        // argv is after this
		1, 0, 0, 0, // little endian-encoded offset of "a"
		3, 0, 0, 0, // little endian-encoded offset of "bc"
		'?', // stopped after encoding
	} // tr
	store, wasiAPI := instantiateWasmStore(t, argsWat, "test", args)

	t.Run("API.ArgsGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// provide a host context we call directly
		hContext := wasm.NewHostFunctionCallContext(ctx, store.Memories[0])

		// invoke ArgsGet directly and check the memory side-effects!
		errno := wasiAPI.ArgsGet(hContext, argv, argvBuf)
		require.Equal(t, ESUCCESS, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
	t.Run(FunctionArgsGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		ret, _, err := store.CallFunction(ctx, "test", FunctionArgsGet, uint64(argv), uint64(argvBuf))
		require.NoError(t, err)
		require.Equal(t, ESUCCESS, Errno(ret[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestArgsGet_Errors(t *testing.T) {
	ctx := context.Background()
	args, err := Args("a", "bc")
	require.NoError(t, err)
	store, wasiAPI := instantiateWasmStore(t, argsWat, "test", args)

	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_get. We chose 0 here.

	tests := []struct {
		name    string
		argv    uint32
		argvBuf uint32
	}{
		{
			name:    "out-of-memory argv",
			argv:    memorySize,
			argvBuf: validAddress,
		},
		{
			name:    "out-of-memory argvBuf",
			argv:    validAddress,
			argvBuf: memorySize,
		},
		{
			name: "argv exceeds the maximum valid address by 1",
			// 4*uint32(len(argsArray.nullTerminatedValues)) is the size of the result of the pointers to args, 4 is the size of uint32
			argv:    memorySize - 4*uint32(len(wasiAPI.(*api).args.nullTerminatedValues)) + 1,
			argvBuf: validAddress,
		},
		{
			name:    "argvBuf exceeds the maximum valid address by 1",
			argv:    validAddress,
			argvBuf: memorySize - wasiAPI.(*api).args.totalBufSize + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ret, _, err := store.CallFunction(ctx, "test", FunctionArgsGet, uint64(tc.argv), uint64(tc.argvBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(EINVAL), ret[0]) // ret[0] is returned errno
		})
	}
}

func TestApi_ArgsSizesGet(t *testing.T) {
	ctx := context.Background()
	args, err := Args("a", "bc")
	require.NoError(t, err)
	resultArgc := uint32(1)        // arbitrary offset
	resultArgvBufSize := uint32(6) // arbitrary offset
	maskLength := 11               // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',                // resultArgc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded arg count
		'?',                // resultArgvBufSize is after this
		0x5, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	} // tr
	store, wasiAPI := instantiateWasmStore(t, argsWat, "test", args)

	t.Run("API.ArgsSizesGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// provide a host context we call directly
		hContext := wasm.NewHostFunctionCallContext(ctx, store.Memories[0])

		// invoke ArgsSizesGet directly and check the memory side effects!
		errno := wasiAPI.ArgsSizesGet(hContext, resultArgc, resultArgvBufSize)
		require.Equal(t, ESUCCESS, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
	t.Run(FunctionArgsSizesGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		ret, _, err := store.CallFunction(ctx, "test", FunctionArgsSizesGet, uint64(resultArgc), uint64(resultArgvBufSize))
		require.NoError(t, err)
		require.Equal(t, ESUCCESS, Errno(ret[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestArgsSizesGet_Errors(t *testing.T) {
	ctx := context.Background()
	args, err := Args("a", "bc")
	require.NoError(t, err)
	store, _ := instantiateWasmStore(t, argsWat, "test", args)

	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_sizes_get. We chose 0 here.

	tests := []struct {
		name        string
		argc        uint32
		argvBufSize uint32
	}{
		{
			name:        "out-of-memory argc",
			argc:        memorySize,
			argvBufSize: validAddress,
		},
		{
			name:        "out-of-memory argvBufSize",
			argc:        validAddress,
			argvBufSize: memorySize,
		},
		{
			name:        "argc exceeds the maximum valid address by 1",
			argc:        memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			argvBufSize: validAddress,
		},
		{
			name:        "argvBufSize exceeds the maximum valid size by 1",
			argc:        validAddress,
			argvBufSize: memorySize - 4 + 1, // 4 is the size of uint32, the type of the buffer size
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ret, _, err := store.CallFunction(ctx, "test", FunctionArgsSizesGet, uint64(tc.argc), uint64(tc.argvBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(EINVAL), ret[0]) // ret[0] is returned errno
		})
	}
}

// TODO TestEnvironGet TestEnvironGet_Errors
// TODO TestEnvironSizesGet TestEnvironSizesGet_Errors
// TODO TestClockResGet TestClockResGet_Errors

// clockWat is a wasm module to call clock_time_get.
//go:embed testdata/clock.wat
var clockWat []byte

// TODO: rewrite
func TestClockGetTime(t *testing.T) {
	ctx := context.Background()
	store, wasiAPI := instantiateWasmStore(t, clockWat, "test")
	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_get. We chose 0 here.

	tests := []struct {
		name         string
		timestampVal uint64
		timestampPtr uint32
		result       Errno
	}{
		{
			name:         "zero uint64 value",
			timestampVal: 0,
			timestampPtr: validAddress,
			result:       ESUCCESS,
		},
		{
			name:         "low uint64 value",
			timestampVal: 12345,
			timestampPtr: validAddress,
			result:       ESUCCESS,
		},
		{
			name:         "high uint64 value - no truncation",
			timestampVal: math.MaxUint64,
			timestampPtr: validAddress,
			result:       ESUCCESS,
		},
		{
			name:         "with an endian-sensitive uint64 val - no truncation",
			timestampVal: math.MaxUint64 - 1,
			timestampPtr: validAddress,
			result:       ESUCCESS,
		},
		{
			name:         "timestampPtr exceeds the maximum valid address by 1",
			timestampVal: math.MaxUint64,
			timestampPtr: memorySize - 8 + 1,
			result:       EINVAL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wasiAPI.(*api).getTimeNanosFn = func() uint64 { return tt.timestampVal }
			ret, _, err := store.CallFunction(ctx, "test", FunctionClockTimeGet, uint64(0), uint64(0), uint64(tt.timestampPtr))
			require.NoError(t, err)
			errno := Errno(ret[0])
			require.Equal(t, tt.result, errno) // ret[0] is returned errno
			if errno == ESUCCESS {
				nanos := binary.LittleEndian.Uint64(store.Memories[0].Buffer)
				assert.Equal(t, tt.timestampVal, nanos)
			}
		})
	}
}

// TODO: TestFDAdvise TestFDAdvise_Errors
// TODO: TestFDAllocate TestFDAllocate_Errors
// TODO: TestFDClose TestFDClose_Errors
// TODO: TestFDDataSync TestFDDataSync_Errors
// TODO: TestFDFDStatGet TestFDFDStatGet_Errors
// TODO: TestFDFDStatSetFlags TestFDFDStatSetFlags_Errors
// TODO: TestFDFDStatSetRights TestFDFDStatSetRights_Errors
// TODO: TestFDFilestatGet TestFDFilestatGet_Errors
// TODO: TestFDFilestatSetSize TestFDFilestatSetSize_Errors
// TODO: TestFDFilestatSetTimes TestFDFilestatSetTimes_Errors
// TODO: TestFDPread TestFDPread_Errors
// TODO: TestFDPrestatGet TestFDPrestatGet_Errors
// TODO: TestFDPrestatDirName TestFDPrestatDirName_Errors
// TODO: TestFDPwrite TestFDPwrite_Errors
// TODO: TestFDRead TestFDRead_Errors
// TODO: TestFDReaddir TestFDReaddir_Errors
// TODO: TestFDRenumber TestFDRenumber_Errors
// TODO: TestFDSeek TestFDSeek_Errors
// TODO: TestFDSync TestFDSync_Errors
// TODO: TestFDTell TestFDTell_Errors
// TODO: TestFDWrite TestFDWrite_Errors
// TODO: TestPathCreateDirectory TestPathCreateDirectory_Errors
// TODO: TestPathFilestatGet TestPathFilestatGet_Errors
// TODO: TestPathFilestatSetTimes TestPathFilestatSetTimes_Errors
// TODO: TestPathLink TestPathLink_Errors
// TODO: TestPathOpen TestPathOpen_Errors
// TODO: TestPathReadlink TestPathReadlink_Errors
// TODO: TestPathRemoveDirectory TestPathRemoveDirectory_Errors
// TODO: TestPathRename TestPathRename_Errors
// TODO: TestPathSymlink TestPathSymlink_Errors
// TODO: TestPathUnlinkFile TestPathUnlinkFile_Errors
// TODO: TestPollOneoff TestPollOneoff_Errors
// TODO: TestProcExit TestProcExit_Errors
// TODO: TestProcRaise TestProcRaise_Errors
// TODO: TestSchedYield TestSchedYield_Errors
// TODO: TestRandomGet TestRandomGet_Errors
// TODO: TestSockRecv TestSockRecv_Errors
// TODO: TestSockSend TestSockSend_Errors
// TODO: TestSockShutdown TestSockShutdown_Errors

func instantiateWasmStore(t *testing.T, wat []byte, moduleName string, opts ...Option) (*wasm.Store, API) {
	mod, err := text.DecodeModule(wat)
	require.NoError(t, err)

	store := wasm.NewStore(interpreter.NewEngine())
	wasiAPI, err := registerAPI(store, opts...)
	require.NoError(t, err)

	err = store.Instantiate(mod, moduleName)
	require.NoError(t, err)

	return store, wasiAPI
}

// maskMemory overwrites the first memory in the store with '?', so tests can see what's written.
// As the memory can be very large, this only masks up to the given length.
//
// Note: WebAssembly 1.0 (MVP) can have only up to one memory, so which is unambiguous.
func maskMemory(store *wasm.Store, maskLength int) {
	for i := 0; i < maskLength; i++ {
		store.Memories[0].Buffer[i] = '?'
	}
}
