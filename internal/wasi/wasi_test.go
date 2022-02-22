package internalwasi

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/wasi"
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
	}

	store, ctx, fn := instantiateWasmStore(t, FunctionArgsGet, ImportArgsGet, "test", args)

	t.Run("ArgsGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// Invoke ArgsGet directly and check the memory side effects.
		errno := newAPI(args).ArgsGet(ctx, argv, argvBuf)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})

	t.Run(FunctionArgsGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		results, err := store.Engine.Call(ctx, fn, uint64(argv), uint64(argvBuf))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_ArgsGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	store, ctx, fn := instantiateWasmStore(t, FunctionArgsGet, ImportArgsGet, "test", args)

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
			// 4*argCount is the size of the result of the pointers to args, 4 is the size of uint32
			argv:    memorySize - 4*2 + 1,
			argvBuf: validAddress,
		},
		{
			name: "argvBuf exceeds the maximum valid address by 1",
			argv: validAddress,
			// "a", "bc" size = size of "a0bc0" = 5
			argvBuf: memorySize - 5 + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := store.Engine.Call(ctx, fn, uint64(tc.argv), uint64(tc.argvBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
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
	}

	store, ctx, fn := instantiateWasmStore(t, FunctionArgsSizesGet, ImportArgsSizesGet, "test", args)

	t.Run("ArgsSizesGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// Invoke ArgsSizesGet directly and check the memory side effects.
		errno := newAPI(args).ArgsSizesGet(ctx, resultArgc, resultArgvBufSize)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})

	t.Run(FunctionArgsSizesGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		results, err := store.Engine.Call(ctx, fn, uint64(resultArgc), uint64(resultArgvBufSize))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_ArgsSizesGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)

	store, ctx, fn := instantiateWasmStore(t, FunctionArgsSizesGet, ImportArgsSizesGet, "test", args)
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
			results, err := store.Engine.Call(ctx, fn, uint64(tc.argc), uint64(tc.argvBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestNewAPI_Environ(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		o, err := Environ("a=b", "b=cd")
		require.NoError(t, err)
		a := newAPI(o)
		require.Equal(t, &nullTerminatedStrings{
			nullTerminatedValues: [][]byte{
				{'a', '=', 'b', 0},
				{'b', '=', 'c', 'd', 0},
			},
			totalBufSize: 9,
		}, a.environ)
	})

	errorTests := []struct {
		name         string
		environ      string
		errorMessage string
	}{
		{name: "error invalid utf-8",
			environ:      "non_utf8=\xff\xfe\xfd",
			errorMessage: "environ[0] is not a valid UTF-8 string"},
		{name: "error not '='-joined pair",
			environ:      "no_equal_pair",
			errorMessage: "environ[0] is not joined with '='"},
	}
	for _, tt := range errorTests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := Environ(tc.environ)
			require.EqualError(t, err, tc.errorMessage)
		})
	}
}

func TestAPI_EnvironGet(t *testing.T) {
	envOpt, err := Environ("a=b", "b=cd")
	require.NoError(t, err)
	resultEnviron := uint32(11)   // arbitrary offset
	resultEnvironBuf := uint32(1) // arbitrary offset
	maskLength := 20              // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',              // environBuf is after this
		'a', '=', 'b', 0, // null terminated "a=b",
		'b', '=', 'c', 'd', 0, // null terminated "b=cd"
		'?',        // environ is after this
		1, 0, 0, 0, // little endian-encoded offset of "a=b"
		5, 0, 0, 0, // little endian-encoded offset of "b=cd"
		'?', // stopped after encoding
	}

	store, ctx, fn := instantiateWasmStore(t, FunctionEnvironGet, ImportEnvironGet, "test", envOpt)

	t.Run("EnvironGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// Invoke EnvironGet directly and check the memory side effects.
		errno := newAPI(envOpt).EnvironGet(ctx, resultEnviron, resultEnvironBuf)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})

	t.Run(FunctionEnvironGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		results, err := store.Engine.Call(ctx, fn, uint64(resultEnviron), uint64(resultEnvironBuf))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_EnvironGet_Errors(t *testing.T) {
	envOpt, err := Environ("a=bc", "b=cd")
	require.NoError(t, err)

	store, ctx, fn := instantiateWasmStore(t, FunctionEnvironGet, ImportEnvironGet, "test", envOpt)
	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to environ_get. We chose 0 here.

	tests := []struct {
		name       string
		environ    uint32
		environBuf uint32
	}{
		{
			name:       "out-of-memory environPtr",
			environ:    memorySize,
			environBuf: validAddress,
		},
		{
			name:       "out-of-memory environBufPtr",
			environ:    validAddress,
			environBuf: memorySize,
		},
		{
			name: "environPtr exceeds the maximum valid address by 1",
			// 4*envCount is the expected buffer size for environPtr, 4 is the size of uint32
			environ:    memorySize - 4*2 + 1,
			environBuf: validAddress,
		},
		{
			name:    "environBufPtr exceeds the maximum valid address by 1",
			environ: validAddress,
			// "a=bc", "b=cd" size = size of "a=bc0b=cd0" = 10
			environBuf: memorySize - 10 + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := store.Engine.Call(ctx, fn, uint64(tc.environ), uint64(tc.environBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestAPI_EnvironSizesGet(t *testing.T) {
	envOpt, err := Environ("a=b", "b=cd")
	require.NoError(t, err)
	resultEnvironc := uint32(1)       // arbitrary offset
	resultEnvironBufSize := uint32(6) // arbitrary offset
	maskLength := 11                  // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',                // resultEnvironc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded environment variable count
		'?',                // resultEnvironBufSize is after this
		0x9, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	}

	store, ctx, fn := instantiateWasmStore(t, FunctionEnvironSizesGet, ImportEnvironSizesGet, "test", envOpt)

	t.Run("EnvironSizesGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// Invoke EnvironSizesGet directly and check the memory side effects.
		errno := newAPI(envOpt).EnvironSizesGet(ctx, resultEnvironc, resultEnvironBufSize)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})

	t.Run(FunctionEnvironSizesGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		results, err := store.Engine.Call(ctx, fn, uint64(resultEnvironc), uint64(resultEnvironBufSize))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_EnvironSizesGet_Errors(t *testing.T) {
	envOpt, err := Environ("a=b", "b=cd")
	require.NoError(t, err)

	store, ctx, fn := instantiateWasmStore(t, FunctionEnvironSizesGet, ImportEnvironSizesGet, "test", envOpt)
	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0) // arbitrary valid address as arguments to environ_sizes_get. We chose 0 here.

	tests := []struct {
		name           string
		environc       uint32
		environBufSize uint32
	}{
		{
			name:           "out-of-memory environCountPtr",
			environc:       memorySize,
			environBufSize: validAddress,
		},
		{
			name:           "out-of-memory environBufSizePtr",
			environc:       validAddress,
			environBufSize: memorySize,
		},
		{
			name:           "environCountPtr exceeds the maximum valid address by 1",
			environc:       memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of environ
			environBufSize: validAddress,
		},
		{
			name:           "environBufSizePtr exceeds the maximum valid size by 1",
			environc:       validAddress,
			environBufSize: memorySize - 4 + 1, // 4 is the size of uint32, the type of the buffer size
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := store.Engine.Call(ctx, fn, uint64(tc.environc), uint64(tc.environBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

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

	clockOpt := func(api *wasiAPI) {
		api.timeNowUnixNano = func() uint64 { return epochNanos }
	}
	store, ctx, fn := instantiateWasmStore(t, FunctionClockTimeGet, ImportClockTimeGet, "test", clockOpt)

	t.Run("ClockTimeGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// invoke ClockTimeGet directly and check the memory side effects!
		errno := newAPI(clockOpt).ClockTimeGet(ctx, 0 /* TODO: id */, 0 /* TODO: precision */, resultTimestamp)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})

	t.Run(FunctionClockTimeGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		results, err := store.Engine.Call(ctx, fn, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(resultTimestamp))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_ClockTimeGet_Errors(t *testing.T) {
	epochNanos := uint64(1640995200000000000) // midnight UTC 2022-01-01

	store, ctx, fn := instantiateWasmStore(t, FunctionClockTimeGet, ImportClockTimeGet, "test", func(api *wasiAPI) {
		api.timeNowUnixNano = func() uint64 { return epochNanos }
	})
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
			results, err := store.Engine.Call(ctx, fn, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(tc.resultTimestamp))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

// TODO: TestAPI_FdAdvise TestAPI_FdAdvise_Errors
// TODO: TestAPI_FdAllocate TestAPI_FdAllocate_Errors

func TestAPI_FdClose(t *testing.T) {
	fdToClose := uint32(3) // arbitrary fd
	fdToKeep := uint32(4)  // another arbitrary fd
	setupFD := func() (*wasm.Store, *wasm.ModuleContext, *wasm.FunctionInstance, *wasiAPI) {
		var api *wasiAPI
		store, ctx, fn := instantiateWasmStore(t, FunctionFdClose, ImportFdClose, "test", func(a *wasiAPI) {
			memFs := &MemFS{}
			a.opened = map[uint32]fileEntry{
				fdToClose: {
					path:    "test",
					fileSys: memFs,
				},
				fdToKeep: {
					path:    "path to keep",
					fileSys: memFs,
				},
			}
			api = a // for later tests
		})
		return store, ctx, fn, api
	}

	t.Run("SnapshotPreview1.FdClose", func(t *testing.T) {
		_, ctx, _, api := setupFD()
		errno := api.FdClose(ctx, fdToClose)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.NotContains(t, api.opened, fdToClose) // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.opened, fdToKeep)
	})
	t.Run(FunctionFdClose, func(t *testing.T) {
		store, ctx, fn, api := setupFD()
		ret, err := store.Engine.Call(ctx, fn, uint64(fdToClose), uint64(fdToClose))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64
		require.NotContains(t, api.opened, fdToClose)           // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.opened, fdToKeep)
	})
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		_, ctx, _, api := setupFD()
		errno := api.FdClose(ctx, 42) // 42 is an arbitrary invalid FD
		require.Equal(t, wasi.ErrnoBadf, errno)
	})
}

// TODO: TestAPI_FdDataSync TestAPI_FdDataSync_Errors
// TODO: TestAPI_FdFdstatGet TestAPI_FdFdstatGet_Errors
// TODO: TestAPI_FdFdstatSetFlags TestAPI_FdFdstatSetFlags_Errors
// TODO: TestAPI_FdFdstatSetRights TestAPI_FdFdstatSetRights_Errors
// TODO: TestAPI_FdFilestatGet TestAPI_FdFilestatGet_Errors
// TODO: TestAPI_FdFilestatSetSize TestAPI_FdFilestatSetSize_Errors
// TODO: TestAPI_FdFilestatSetTimes TestAPI_FdFilestatSetTimes_Errors
// TODO: TestAPI_FdPread TestAPI_FdPread_Errors
// TODO: TestAPI_FdPrestatGet TestAPI_FdPrestatGet_Errors

func TestAPI_FdPrestatDirName(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	var api *wasiAPI
	store, ctx, fn := instantiateWasmStore(t, FunctionFdPrestatDirName, ImportFdPrestatDirName, "test", func(a *wasiAPI) {
		a.opened[fd] = fileEntry{
			path:    "test",
			fileSys: &MemFS{},
		}
		api = a // for later tests
	})

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(3) // shorter than len("test") to test the path is written for the length of pathLen
	maskLength := 7      // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',
		't', 'e', 's',
		'?', '?', '?',
	}

	t.Run("SnapshotPreview1.FdPrestatDirName", func(t *testing.T) {
		maskMemory(store, maskLength)

		errno := api.FdPrestatDirName(ctx, fd, path, pathLen)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
	t.Run(FunctionFdPrestatDirName, func(t *testing.T) {
		maskMemory(store, maskLength)

		ret, err := store.Engine.Call(ctx, fn, uint64(fd), uint64(path), uint64(pathLen))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_FdPrestatDirName_Errors(t *testing.T) {
	dirName := "test"
	opt := Preopen(dirName, &MemFS{})
	store, ctx, fn := instantiateWasmStore(t, FunctionFdPrestatDirName, ImportFdPrestatDirName, "test", opt)

	memorySize := uint32(len(store.Memories[0].Buffer))
	validAddress := uint32(0)     // Arbitrary valid address as arguments to fd_prestat_dir_name. We chose 0 here.
	actualPathLen := len(dirName) // Actual length of the dirName as a valid pathLen.
	fd := uint32(3)               // fd 3 will be opened for the "test" directory after 0, 1, and 2, that are stdin/out/err

	tests := []struct {
		name          string
		fd            uint32
		path          uint32
		pathLen       uint32
		expectedErrno wasi.Errno
	}{
		{
			name:          "out-of-memory path",
			fd:            fd,
			path:          memorySize,
			pathLen:       uint32(actualPathLen),
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            fd,
			path:          memorySize - uint32(actualPathLen) + 1,
			pathLen:       uint32(actualPathLen),
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "pathLen exceeds the actual length of the dir name",
			fd:            fd,
			path:          validAddress,
			pathLen:       uint32(actualPathLen) + 1,
			expectedErrno: wasi.ErrnoNametoolong,
		},
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       uint32(actualPathLen) + 1,
			expectedErrno: wasi.ErrnoBadf,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := store.Engine.Call(ctx, fn, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

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

func TestAPI_ProcExit(t *testing.T) {
	tests := []struct {
		name     string
		exitCode uint32
	}{
		{
			name:     "success (exitcode 0)",
			exitCode: 0,
		},

		{
			name:     "arbitrary non-zero exitcode",
			exitCode: 42,
		},
	}

	store, ctx, fn := instantiateWasmStore(t, FunctionProcExit, ImportProcExit, "test")

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// When ProcExit is called, store.CallFunction returns immediately, returning the exit code as the error.
			_, err := store.Engine.Call(ctx, fn, uint64(tc.exitCode))
			var code wasi.ExitCode
			require.ErrorAs(t, err, &code)
			require.Equal(t, code, wasi.ExitCode(tc.exitCode))
		})
	}
}

// TODO: TestAPI_ProcRaise TestAPI_ProcRaise_Errors
// TODO: TestAPI_SchedYield TestAPI_SchedYield_Errors

func TestAPI_RandomGet(t *testing.T) {
	maskLength := 7 // number of bytes to write '?' to tell what we've written
	expectedMemory := []byte{
		'?',                          // random bytes in `buf` is after this
		0x53, 0x8c, 0x7f, 0x96, 0xb1, // random data from seed value of 42
		'?', // stopped after encoding
	} // tr

	var bufLen = uint32(5) // arbitrary buffer size,
	var buf = uint32(1)    // offset,
	var seed = int64(42)   // and seed value

	randOpt := func(api *wasiAPI) {
		api.randSource = func(p []byte) error {
			s := rand.NewSource(seed)
			rng := rand.New(s)
			_, err := rng.Read(p)

			return err
		}
	}

	store, ctx, fn := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, "test", randOpt)

	t.Run("RandomGet", func(t *testing.T) {
		maskMemory(store, maskLength)

		// invoke RandomGet directly and check the memory side effects!
		errno := newAPI(randOpt).RandomGet(ctx, buf, bufLen)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})

	t.Run(FunctionRandomGet, func(t *testing.T) {
		maskMemory(store, maskLength)

		results, err := store.Engine.Call(ctx, fn, uint64(buf), uint64(bufLen))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64
		require.Equal(t, expectedMemory, store.Memories[0].Buffer[0:maskLength])
	})
}

func TestAPI_RandomGet_Errors(t *testing.T) {
	validAddress := uint32(0) // arbitrary valid address

	store, ctx, fn := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, "test")
	memorySize := uint32(len(store.Memories[0].Buffer))

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
			name:   "random buffer size exceeds maximum valid address by 1",
			buf:    validAddress,
			bufLen: memorySize + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := store.Engine.Call(ctx, fn, uint64(tc.buf), uint64(tc.bufLen))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestAPI_RandomGet_SourceError(t *testing.T) {
	store, ctx, fn := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, "test", func(api *wasiAPI) {
		api.randSource = func(p []byte) error {
			return errors.New("random source error")
		}
	})

	results, err := store.Engine.Call(ctx, fn, uint64(1), uint64(5)) // arbitrary offset and buffer size
	require.NoError(t, err)
	require.Equal(t, uint64(wasi.ErrnoIo), results[0]) // results[0] is the errno
}

// TODO: TestAPI_SockRecv TestAPI_SockRecv_Errors
// TODO: TestAPI_SockSend TestAPI_SockSend_Errors
// TODO: TestAPI_SockShutdown TestAPI_SockShutdown_Errors

func instantiateWasmStore(t *testing.T, wasiFunction, wasiImport, moduleName string, opts ...Option) (*wasm.Store, *wasm.ModuleContext, *wasm.FunctionInstance) {
	mod, err := text.DecodeModule([]byte(fmt.Sprintf(`(module
  %[2]s
  (memory 1)  ;; just an arbitrary size big enough for tests
  (export "memory" (memory 0))
  (export "%[1]s" (func $wasi.%[1]s))
)`, wasiFunction, wasiImport)))
	require.NoError(t, err)

	store := wasm.NewStore(context.Background(), interpreter.NewEngine())

	snapshotPreview1Functions := SnapshotPreview1Functions(opts...)
	goFunc := snapshotPreview1Functions[wasiFunction]
	fn, err := wasm.NewGoFunc(wasiFunction, goFunc)
	require.NoError(t, err)

	// Add the host module
	hostModule := &wasm.ModuleInstance{Name: wasi.ModuleSnapshotPreview1, Exports: map[string]*wasm.ExportInstance{}}
	store.ModuleInstances[hostModule.Name] = hostModule

	wasiFn, err := store.AddHostFunction(hostModule, fn)
	require.NoError(t, err)

	instantiated, err := store.Instantiate(mod, moduleName)
	require.NoError(t, err)

	return store, instantiated.Context, wasiFn
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
