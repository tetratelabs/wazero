package wasi

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"testing"

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

func TestAPI_ArgsGet(t *testing.T) {
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
	store, wasiAPI := instantiateWasmStore(t, FunctionArgsGet, ImportArgsGet, "test", args)

	t.Run("API.ArgsGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// provide a host context we call directly
		hContext := wasm.NewHostFunctionCallContext(context.Background(), store.Memories[0])

		// invoke ArgsGet directly and check the memory side-effects!
		errno := wasiAPI.ArgsGet(hContext, argv, argvBuf)
		require.Equal(t, ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
	t.Run(FunctionArgsGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		ret, _, err := store.CallFunction(context.Background(), "test", FunctionArgsGet, uint64(argv), uint64(argvBuf))
		require.NoError(t, err)
		require.Equal(t, ErrnoSuccess, Errno(ret[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_ArgsGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	store, wasiAPI := instantiateWasmStore(t, FunctionArgsGet, ImportArgsGet, "test", args)

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
			ret, _, err := store.CallFunction(context.Background(), "test", FunctionArgsGet, uint64(tc.argv), uint64(tc.argvBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(ErrnoInval), ret[0]) // ret[0] is returned errno
		})
	}
}

func TestAPI_ArgsSizesGet(t *testing.T) {
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
	store, wasiAPI := instantiateWasmStore(t, FunctionArgsSizesGet, ImportArgsSizesGet, "test", args)

	t.Run("API.ArgsSizesGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// provide a host context we call directly
		hContext := wasm.NewHostFunctionCallContext(context.Background(), store.Memories[0])

		// invoke ArgsSizesGet directly and check the memory side effects!
		errno := wasiAPI.ArgsSizesGet(hContext, resultArgc, resultArgvBufSize)
		require.Equal(t, ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
	t.Run(FunctionArgsSizesGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		ret, _, err := store.CallFunction(context.Background(), "test", FunctionArgsSizesGet, uint64(resultArgc), uint64(resultArgvBufSize))
		require.NoError(t, err)
		require.Equal(t, ErrnoSuccess, Errno(ret[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_ArgsSizesGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	store, _ := instantiateWasmStore(t, FunctionArgsSizesGet, ImportArgsSizesGet, "test", args)

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
			ret, _, err := store.CallFunction(context.Background(), "test", FunctionArgsSizesGet, uint64(tc.argc), uint64(tc.argvBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(ErrnoInval), ret[0]) // ret[0] is returned errno
		})
	}
}

// TODO TestAPI_EnvironGet TestAPI_EnvironGet_Errors
// TODO TestAPI_EnvironSizesGet TestAPI_EnvironSizesGet_Errors
// TODO TestAPI_ClockResGet TestAPI_ClockResGet_Errors

func TestAPI_ClockTimeGet(t *testing.T) {
	epochNanos := uint64(1640995200000000000) // midnight UTC 2022-01-01
	resultTimestamp := uint32(1)              // arbitrary offset
	maskLength := 10                          // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',                                          // resultTimestamp is after this
		0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // little endian-encoded epochNanos
		'?', // stopped after encoding
	} // tr

	store, wasiAPI := instantiateWasmStore(t, FunctionClockTimeGet, ImportClockTimeGet, "test")
	wasiAPI.(*api).timeNowUnixNano = func() uint64 { return epochNanos }

	t.Run("API.ClockTimeGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// provide a host context we call directly
		hContext := wasm.NewHostFunctionCallContext(context.Background(), store.Memories[0])

		// invoke ClockTimeGet directly and check the memory side effects!
		errno := wasiAPI.ClockTimeGet(hContext, 0 /* TODO: id */, 0 /* TODO: precision */, resultTimestamp)
		require.Equal(t, ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
	t.Run(FunctionClockTimeGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		ret, _, err := store.CallFunction(context.Background(), "test", FunctionClockTimeGet, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(resultTimestamp))
		require.NoError(t, err)
		require.Equal(t, ErrnoSuccess, Errno(ret[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_ClockTimeGet_Errors(t *testing.T) {
	epochNanos := uint64(1640995200000000000) // midnight UTC 2022-01-01
	store, wasiAPI := instantiateWasmStore(t, FunctionClockTimeGet, ImportClockTimeGet, "test")
	wasiAPI.(*api).timeNowUnixNano = func() uint64 { return epochNanos }

	memorySize := uint32(len(store.Memories[0].Buffer))

	tests := []struct {
		name            string
		resultTimestamp uint32
		argvBufSize     uint32
	}{
		{
			name:            "resultTimestamp out-of-memory",
			resultTimestamp: memorySize,
		},

		{
			name:            "resultTimestamp exceeds the maximum valid address by 1",
			resultTimestamp: memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ret, _, err := store.CallFunction(context.Background(), "test", FunctionClockTimeGet, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(tc.resultTimestamp))
			require.NoError(t, err)
			require.Equal(t, uint64(ErrnoInval), ret[0]) // ret[0] is returned errno
		})
	}
}

// TODO: TestAPI_FdAdvise TestAPI_FdAdvise_Errors
// TODO: TestAPI_FdAllocate TestAPI_FdAllocate_Errors
// TODO: TestAPI_FdClose TestAPI_FdClose_Errors
// TODO: TestAPI_FdDataSync TestAPI_FdDataSync_Errors
// TODO: TestAPI_FdFdstatGet TestAPI_FdFdstatGet_Errors
// TODO: TestAPI_FdFdstatSetFlags TestAPI_FdFdstatSetFlags_Errors
// TODO: TestAPI_FdFdstatSetRights TestAPI_FdFdstatSetRights_Errors
// TODO: TestAPI_FdFilestatGet TestAPI_FdFilestatGet_Errors
// TODO: TestAPI_FdFilestatSetSize TestAPI_FdFilestatSetSize_Errors
// TODO: TestAPI_FdFilestatSetTimes TestAPI_FdFilestatSetTimes_Errors
// TODO: TestAPI_FdPread TestAPI_FdPread_Errors
// TODO: TestAPI_FdPrestatGet TestAPI_FdPrestatGet_Errors
// TODO: TestAPI_FdPrestatDirName TestAPI_FdPrestatDirName_Errors
// TODO: TestAPI_FdPwrite TestAPI_FdPwrite_Errors
// TODO: TestAPI_FdRead TestAPI_FdRead_Errors
// TODO: TestAPI_FdReaddir TestAPI_FdReaddir_Errors
// TODO: TestAPI_FdRenumber TestAPI_FdRenumber_Errors
// TODO: TestAPI_FdSeek TestAPI_FdSeek_Errors
// TODO: TestAPI_FdSync TestAPI_FdSync_Errors
// TODO: TestAPI_FdTell TestAPI_FdTell_Errors
// TODO: TestAPI_FdWrite TestAPI_FdWrite_Errors
// TODO: TestAPI_PathCreateDirectory TestAPI_PathCreateDirectory_Errors
// TODO: TestAPI_PathFilestatGet TestAPI_PathFilestatGet_Errors
// TODO: TestAPI_PathFilestatSetTimes TestAPI_PathFilestatSetTimes_Errors
// TODO: TestAPI_PathLink TestAPI_PathLink_Errors
// TODO: TestAPI_PathOpen TestAPI_PathOpen_Errors
// TODO: TestAPI_PathReadlink TestAPI_PathReadlink_Errors
// TODO: TestAPI_PathRemoveDirectory TestAPI_PathRemoveDirectory_Errors
// TODO: TestAPI_PathRename TestAPI_PathRename_Errors
// TODO: TestAPI_PathSymlink TestAPI_PathSymlink_Errors
// TODO: TestAPI_PathUnlinkFile TestAPI_PathUnlinkFile_Errors
// TODO: TestAPI_PollOneoff TestAPI_PollOneoff_Errors
// TODO: TestAPI_ProcExit TestAPI_ProcExit_Errors
// TODO: TestAPI_ProcRaise TestAPI_ProcRaise_Errors
// TODO: TestAPI_SchedYield TestAPI_SchedYield_Errors


func TestAPI_RandomGet(t *testing.T) {
	store, wasiAPI := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, "test")
	maskLength := 7 // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',                          // random bytes in `buf` is after this
		0x53, 0x8c, 0x7f, 0x96, 0xb1, // random data from seed value of 42
		'?', // stopped after encoding
	} // tr

	var bufLen = uint32(5) // arbitrary buffer size,
	var buf = uint32(1)    // offset,
	var seed = int64(42)   // and seed value

	wasiAPI.(*api).randSource = func(p []byte) error {
		s := rand.NewSource(seed)
		rng := rand.New(s)
		_, err := rng.Read(p)

		return err
	}

	t.Run("API.RandomGet", func(t *testing.T) {
		maskMemory(store, maskLength)
		// provide a host context with a seed value for random generator
		hContext := wasm.NewHostFunctionCallContext(context.Background(), store.Memories[0])

		errno := wasiAPI.RandomGet(hContext, buf, bufLen)
		require.Equal(t, ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_RandomGet_Errors(t *testing.T) {
	store, _ := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, "test")

	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to args_sizes_get. We chose 0 here.
	tests := []struct {
		name   string
		buf    uint32
		bufLen uint32
	}{
		{
			name:   "random buffer out-of-memory",
			buf:    memorySize,
			bufLen: 1,
		},

		{
			name:   "random buffer size exceeds the maximum valid address by 1",
			buf:    validAddress,
			bufLen: memorySize + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ret, _, err := store.CallFunction(context.Background(), "test", FunctionRandomGet, uint64(tc.buf), uint64(tc.bufLen))
			require.NoError(t, err)
			require.Equal(t, uint64(ErrnoInval), ret[0]) // ret[0] is returned errno
		})
	}
}

// TODO: TestAPI_SockRecv TestAPI_SockRecv_Errors
// TODO: TestAPI_SockSend TestAPI_SockSend_Errors
// TODO: TestAPI_SockShutdown TestAPI_SockShutdown_Errors

func instantiateWasmStore(t *testing.T, wasiFunction, wasiImport string, moduleName string, opts ...Option) (*wasm.Store, API) {
	mod, err := text.DecodeModule([]byte(fmt.Sprintf(`(module
  %[2]s
  (memory 1)  ;; just an arbitrary size big enough for tests
  (export "memory" (memory 0))
  (export "%[1]s" (func $wasi.%[1]s))
)`, wasiFunction, wasiImport)))
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
