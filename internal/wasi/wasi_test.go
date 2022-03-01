package internalwasi

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/wasi"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

const moduleName = "test"

func TestNewSnapshotPreview1_Args(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		o, err := Args("a", "bc")
		require.NoError(t, err)
		a := NewAPI(o)
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

func TestSnapshotPreview1_ArgsGet(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	argv := uint32(7)    // arbitrary offset
	argvBuf := uint32(1) // arbitrary offset
	expectedMemory := []byte{
		'?',                 // argvBuf is after this
		'a', 0, 'b', 'c', 0, // null terminated "a", "bc"
		'?',        // argv is after this
		1, 0, 0, 0, // little endian-encoded offset of "a"
		3, 0, 0, 0, // little endian-encoded offset of "bc"
		'?', // stopped after encoding
	}

	_, ctx := instantiateWasmStore(t, FunctionArgsGet, ImportArgsGet, moduleName, args)

	fn := ctx.Function(FunctionArgsGet)
	require.NotNil(t, fn)

	mem := ctx.Memory()

	t.Run("SnapshotPreview1.ArgsGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke ArgsGet directly and check the memory side effects.
		errno := NewAPI(args).ArgsGet(ctx, argv, argvBuf)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionArgsGet, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		results, err := fn.Call(context.Background(), uint64(argv), uint64(argvBuf))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_ArgsGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	_, ctx := instantiateWasmStore(t, FunctionArgsGet, ImportArgsGet, moduleName, args)

	fn := ctx.Function(FunctionArgsGet)
	require.NotNil(t, fn)

	memorySize := ctx.Memory().Size()
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
			results, err := fn.Call(context.Background(), uint64(tc.argv), uint64(tc.argvBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_ArgsSizesGet(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	resultArgc := uint32(1)        // arbitrary offset
	resultArgvBufSize := uint32(6) // arbitrary offset
	expectedMemory := []byte{
		'?',                // resultArgc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded arg count
		'?',                // resultArgvBufSize is after this
		0x5, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	}

	_, ctx := instantiateWasmStore(t, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, args)

	fn := ctx.Function(FunctionArgsSizesGet)
	require.NotNil(t, fn)

	mem := ctx.Memory()

	t.Run("SnapshotPreview1.ArgsSizesGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke ArgsSizesGet directly and check the memory side effects.
		errno := NewAPI(args).ArgsSizesGet(ctx, resultArgc, resultArgvBufSize)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionArgsSizesGet, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		results, err := fn.Call(context.Background(), uint64(resultArgc), uint64(resultArgvBufSize))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_ArgsSizesGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)

	_, ctx := instantiateWasmStore(t, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, args)

	fn := ctx.Function(FunctionArgsSizesGet)
	require.NotNil(t, fn)

	memorySize := ctx.Memory().Size()
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
			argvBufSize: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(context.Background(), uint64(tc.argc), uint64(tc.argvBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestNewSnapshotPreview1_Environ(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		o, err := Environ("a=b", "b=cd")
		require.NoError(t, err)
		a := NewAPI(o)
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

func TestSnapshotPreview1_EnvironGet(t *testing.T) {
	envOpt, err := Environ("a=b", "b=cd")
	require.NoError(t, err)
	resultEnviron := uint32(11)   // arbitrary offset
	resultEnvironBuf := uint32(1) // arbitrary offset
	expectedMemory := []byte{
		'?',              // environBuf is after this
		'a', '=', 'b', 0, // null terminated "a=b",
		'b', '=', 'c', 'd', 0, // null terminated "b=cd"
		'?',        // environ is after this
		1, 0, 0, 0, // little endian-encoded offset of "a=b"
		5, 0, 0, 0, // little endian-encoded offset of "b=cd"
		'?', // stopped after encoding
	}

	_, ctx := instantiateWasmStore(t, FunctionEnvironGet, ImportEnvironGet, moduleName, envOpt)

	fn := ctx.Function(FunctionEnvironGet)
	require.NotNil(t, fn)

	mem := ctx.Memory()

	t.Run("SnapshotPreview1.EnvironGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke EnvironGet directly and check the memory side effects.
		errno := NewAPI(envOpt).EnvironGet(ctx, resultEnviron, resultEnvironBuf)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionEnvironGet, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		results, err := fn.Call(context.Background(), uint64(resultEnviron), uint64(resultEnvironBuf))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_EnvironGet_Errors(t *testing.T) {
	envOpt, err := Environ("a=bc", "b=cd")
	require.NoError(t, err)

	_, ctx := instantiateWasmStore(t, FunctionEnvironGet, ImportEnvironGet, moduleName, envOpt)

	fn := ctx.Function(FunctionEnvironGet)
	require.NotNil(t, fn)

	memorySize := ctx.Memory().Size()
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
			// 4*envCount is the expected length for environPtr, 4 is the size of uint32
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
			results, err := fn.Call(context.Background(), uint64(tc.environ), uint64(tc.environBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_EnvironSizesGet(t *testing.T) {
	envOpt, err := Environ("a=b", "b=cd")
	require.NoError(t, err)
	resultEnvironc := uint32(1)       // arbitrary offset
	resultEnvironBufSize := uint32(6) // arbitrary offset
	expectedMemory := []byte{
		'?',                // resultEnvironc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded environment variable count
		'?',                // resultEnvironBufSize is after this
		0x9, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	}

	_, ctx := instantiateWasmStore(t, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, envOpt)

	fn := ctx.Function(FunctionEnvironSizesGet)
	require.NotNil(t, fn)

	mem := ctx.Memory()

	t.Run("SnapshotPreview1.EnvironSizesGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke EnvironSizesGet directly and check the memory side effects.
		errno := NewAPI(envOpt).EnvironSizesGet(ctx, resultEnvironc, resultEnvironBufSize)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionEnvironSizesGet, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		results, err := fn.Call(context.Background(), uint64(resultEnvironc), uint64(resultEnvironBufSize))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_EnvironSizesGet_Errors(t *testing.T) {
	envOpt, err := Environ("a=b", "b=cd")
	require.NoError(t, err)

	_, ctx := instantiateWasmStore(t, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, envOpt)

	fn := ctx.Function(FunctionEnvironSizesGet)
	require.NotNil(t, fn)

	memorySize := ctx.Memory().Size()
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
			environBufSize: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(context.Background(), uint64(tc.environc), uint64(tc.environBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

// TODO TestSnapshotPreview1_ClockResGet TestSnapshotPreview1_ClockResGet_Errors

func TestSnapshotPreview1_ClockTimeGet(t *testing.T) {
	epochNanos := uint64(1640995200000000000) // midnight UTC 2022-01-01
	resultTimestamp := uint32(1)              // arbitrary offset
	expectedMemory := []byte{
		'?',                                          // resultTimestamp is after this
		0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // little endian-encoded epochNanos
		'?', // stopped after encoding
	}

	clockOpt := func(api *wasiAPI) {
		api.timeNowUnixNano = func() uint64 { return epochNanos }
	}
	_, ctx := instantiateWasmStore(t, FunctionClockTimeGet, ImportClockTimeGet, moduleName, clockOpt)
	fn := ctx.Function(FunctionClockTimeGet)
	require.NotNil(t, fn)

	mem := ctx.Memory()

	t.Run("SnapshotPreview1.ClockTimeGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// invoke ClockTimeGet directly and check the memory side effects!
		errno := NewAPI(clockOpt).ClockTimeGet(ctx, 0 /* TODO: id */, 0 /* TODO: precision */, resultTimestamp)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionClockTimeGet, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		results, err := fn.Call(context.Background(), 0 /* TODO: id */, 0 /* TODO: precision */, uint64(resultTimestamp))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_ClockTimeGet_Errors(t *testing.T) {
	epochNanos := uint64(1640995200000000000) // midnight UTC 2022-01-01

	_, ctx := instantiateWasmStore(t, FunctionClockTimeGet, ImportClockTimeGet, moduleName, func(api *wasiAPI) {
		api.timeNowUnixNano = func() uint64 { return epochNanos }
	})

	fn := ctx.Function(FunctionClockTimeGet)
	require.NotNil(t, fn)

	memorySize := ctx.Memory().Size()

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
			results, err := fn.Call(context.Background(), 0 /* TODO: id */, 0 /* TODO: precision */, uint64(tc.resultTimestamp))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

// TODO: TestSnapshotPreview1_FdAdvise TestSnapshotPreview1_FdAdvise_Errors
// TODO: TestSnapshotPreview1_FdAllocate TestSnapshotPreview1_FdAllocate_Errors

func TestSnapshotPreview1_FdClose(t *testing.T) {
	fdToClose := uint32(3) // arbitrary fd
	fdToKeep := uint32(4)  // another arbitrary fd

	setupFD := func() (*wasm.ModuleContext, *wasiAPI) {
		var api *wasiAPI
		_, ctx := instantiateWasmStore(t, FunctionFdClose, ImportFdClose, moduleName, func(a *wasiAPI) {
			memFs := &MemFS{}
			a.opened = map[uint32]fileEntry{
				fdToClose: {
					path:    "/tmp",
					fileSys: memFs,
				},
				fdToKeep: {
					path:    "path to keep",
					fileSys: memFs,
				},
			}
			api = a // for later tests
		})
		return ctx, api
	}

	t.Run("SnapshotPreview1.FdClose", func(t *testing.T) {
		ctx, api := setupFD()
		fn := ctx.Function(FunctionFdClose)
		require.NotNil(t, fn)

		errno := api.FdClose(ctx, fdToClose)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.NotContains(t, api.opened, fdToClose) // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.opened, fdToKeep)
	})
	t.Run(FunctionFdClose, func(t *testing.T) {
		ctx, api := setupFD()
		fn := ctx.Function(FunctionFdClose)
		require.NotNil(t, fn)

		ret, err := fn.Call(context.Background(), uint64(fdToClose))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64
		require.NotContains(t, api.opened, fdToClose)           // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.opened, fdToKeep)
	})
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		ctx, api := setupFD()
		errno := api.FdClose(ctx, 42) // 42 is an arbitrary invalid FD
		require.Equal(t, wasi.ErrnoBadf, errno)
	})
}

// TODO: TestSnapshotPreview1_FdDataSync TestSnapshotPreview1_FdDataSync_Errors
// TODO: TestSnapshotPreview1_FdFdstatGet TestSnapshotPreview1_FdFdstatGet_Errors
// TODO: TestSnapshotPreview1_FdFdstatSetFlags TestSnapshotPreview1_FdFdstatSetFlags_Errors
// TODO: TestSnapshotPreview1_FdFdstatSetRights TestSnapshotPreview1_FdFdstatSetRights_Errors
// TODO: TestSnapshotPreview1_FdFilestatGet TestSnapshotPreview1_FdFilestatGet_Errors
// TODO: TestSnapshotPreview1_FdFilestatSetSize TestSnapshotPreview1_FdFilestatSetSize_Errors
// TODO: TestSnapshotPreview1_FdFilestatSetTimes TestSnapshotPreview1_FdFilestatSetTimes_Errors
// TODO: TestSnapshotPreview1_FdPread TestSnapshotPreview1_FdPread_Errors
// TODO: TestSnapshotPreview1_FdPrestatGet TestSnapshotPreview1_FdPrestatGet_Errors

func TestSnapshotPreview1_FdPrestatDirName(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	var api *wasiAPI
	_, ctx := instantiateWasmStore(t, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, func(a *wasiAPI) {
		a.opened[fd] = fileEntry{
			path:    "/tmp",
			fileSys: &MemFS{},
		}
		api = a // for later tests
	})

	mem := ctx.Memory()
	fn := ctx.Function(FunctionFdPrestatDirName)
	require.NotNil(t, fn)

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(3) // shorter than len("/tmp") to test the path is written for the length of pathLen
	expectedMemory := []byte{
		'?',
		'/', 't', 'm',
		'?', '?', '?',
	}

	t.Run("SnapshotPreview1.FdPrestatDirName", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		errno := api.FdPrestatDirName(ctx, fd, path, pathLen)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionFdPrestatDirName, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		ret, err := fn.Call(context.Background(), uint64(fd), uint64(path), uint64(pathLen))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_FdPrestatDirName_Errors(t *testing.T) {
	dirName := "/tmp"
	opt := Preopen(dirName, &MemFS{})
	_, ctx := instantiateWasmStore(t, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, opt)

	mem := ctx.Memory()
	fn := ctx.Function(FunctionFdPrestatDirName)
	require.NotNil(t, fn)

	memorySize := mem.Size()
	validAddress := uint32(0) // Arbitrary valid address as arguments to fd_prestat_dir_name. We chose 0 here.
	fd := uint32(3)           // fd 3 will be opened for the "/tmp" directory after 0, 1, and 2, that are stdin/out/err

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
			pathLen:       uint32(len(dirName)),
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            fd,
			path:          memorySize - uint32(len(dirName)) + 1,
			pathLen:       uint32(len(dirName)),
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "pathLen exceeds the length of the dir name",
			fd:            fd,
			path:          validAddress,
			pathLen:       uint32(len(dirName)) + 1,
			expectedErrno: wasi.ErrnoNametoolong,
		},
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       uint32(len(dirName)) + 1,
			expectedErrno: wasi.ErrnoBadf,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(context.Background(), uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_FdRead(t *testing.T) {
	fd := uint32(3)   // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
	}
	iovsLen := uint32(2)     // The length of iovs
	resultSize := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',        // resultSize is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	var api *wasiAPI
	_, ctx := instantiateWasmStore(t, FunctionFdRead, ImportFdRead, moduleName, func(a *wasiAPI) {
		api = a // for later tests
	})

	mem := ctx.Memory()
	fn := ctx.Function(FunctionFdRead)
	require.NotNil(t, fn)

	// TestSnapshotPreview1_FdRead uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdReadFn func(ctx publicwasm.ModuleContext, fd, iovs, iovsLen, resultSize uint32) wasi.Errno
	tests := []struct {
		name   string
		fdRead func() fdReadFn
	}{
		{"SnapshotPreview1.FdRead", func() fdReadFn {
			return api.FdRead
		}},
		{FunctionFdRead, func() fdReadFn {
			return func(ctx publicwasm.ModuleContext, fd, iovs, iovsLen, resultSize uint32) wasi.Errno {
				ret, err := fn.Call(context.Background(), uint64(fd), uint64(iovs), uint64(iovsLen), uint64(resultSize))
				require.NoError(t, err)
				return wasi.Errno(ret[0])
			}
		}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh file to read the contents from
			file, memFS := createFile(t, "test_path", []byte("wazero"))
			api.opened[fd] = fileEntry{
				path:    "test_path",
				fileSys: memFS,
				file:    file,
			}
			maskMemory(t, mem, len(expectedMemory))

			ok := mem.Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.fdRead()(ctx, fd, iovs, iovsLen, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mem.Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
		})
	}
}

func TestSnapshotPreview1_FdRead_Errors(t *testing.T) {
	validFD := uint64(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents
	_, ctx := instantiateWasmStore(t, FunctionFdRead, ImportFdRead, moduleName, func(a *wasiAPI) {
		a.opened[uint32(validFD)] = fileEntry{
			path:    "test_path",
			fileSys: memFS,
			file:    file,
		}
	})

	mem := ctx.Memory()
	fn := ctx.Function(FunctionFdRead)
	require.NotNil(t, fn)

	tests := []struct {
		name                          string
		fd, iovs, iovsLen, resultSize uint64
		memory                        []byte
		expectedErrno                 wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            validFD,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "out-of-memory reading iovs[0].length",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			memory: []byte{
				'?',          // `iovs` is after this
				0, 0, 0x1, 0, // = iovs[0].offset on the secod page
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "length to read exceeds memory by 1",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				0, 0, 0x1, 0, // = iovs[0].length on the secod page
				'?',
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "resultSize offset is outside memory",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			resultSize: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			offset := uint64(wasm.MemoryPagesToBytesNum(testMemoryPageSize)) - uint64(len(tc.memory))

			memoryWriteOK := mem.Write(uint32(offset), tc.memory)
			require.True(t, memoryWriteOK)

			results, err := fn.Call(context.Background(), tc.fd, tc.iovs+offset, tc.iovsLen+offset, tc.resultSize+offset)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

// // TODO: TestSnapshotPreview1_FdReaddir TestSnapshotPreview1_FdReaddir_Errors
// // TODO: TestSnapshotPreview1_FdRenumber TestSnapshotPreview1_FdRenumber_Errors
// // TODO: TestSnapshotPreview1_FdSeek TestSnapshotPreview1_FdSeek_Errors
// // TODO: TestSnapshotPreview1_FdSync TestSnapshotPreview1_FdSync_Errors
// // TODO: TestSnapshotPreview1_FdTell TestSnapshotPreview1_FdTell_Errors

func TestSnapshotPreview1_FdWrite(t *testing.T) {
	fd := uint32(3)   // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',                // iovs[0].offset is after this
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',
	}
	iovsLen := uint32(2)     // The length of iovs
	resultSize := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	var api *wasiAPI
	_, ctx := instantiateWasmStore(t, FunctionFdWrite, ImportFdWrite, moduleName, func(a *wasiAPI) {
		api = a // for later tests
	})

	mem := ctx.Memory()
	fn := ctx.Function(FunctionFdWrite)
	require.NotNil(t, fn)

	// TestSnapshotPreview1_FdWrite uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdWriteFn func(ctx publicwasm.ModuleContext, fd, iovs, iovsLen, resultSize uint32) wasi.Errno
	tests := []struct {
		name    string
		fdWrite func() fdWriteFn
	}{
		{"SnapshotPreview1.FdWrite", func() fdWriteFn {
			return api.FdWrite
		}},
		{FunctionFdWrite, func() fdWriteFn {
			return func(ctx publicwasm.ModuleContext, fd, iovs, iovsLen, resultSize uint32) wasi.Errno {
				ret, err := fn.Call(context.Background(), uint64(fd), uint64(iovs), uint64(iovsLen), uint64(resultSize))
				require.NoError(t, err)
				return wasi.Errno(ret[0])
			}
		}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh file to write the contents to
			file, memFS := createFile(t, "test_path", []byte{})
			api.opened[fd] = fileEntry{
				path:    "test_path",
				fileSys: memFS,
				file:    file,
			}
			maskMemory(t, mem, len(expectedMemory))
			ok := mem.Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.fdWrite()(ctx, fd, iovs, iovsLen, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mem.Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
			require.Equal(t, []byte("wazero"), file.buf.Bytes()) // verify the file was actually written
		})
	}
}

func TestSnapshotPreview1_FdWrite_Errors(t *testing.T) {
	validFD := uint64(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents
	_, ctx := instantiateWasmStore(t, FunctionFdWrite, ImportFdWrite, moduleName, func(a *wasiAPI) {
		a.opened[uint32(validFD)] = fileEntry{
			path:    "test_path",
			fileSys: memFS,
			file:    file,
		}
	})

	mem := ctx.Memory()
	fn := ctx.Function(FunctionFdWrite)
	require.NotNil(t, fn)

	tests := []struct {
		name                          string
		fd, iovs, iovsLen, resultSize uint64
		memory                        []byte
		expectedErrno                 wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            validFD,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "out-of-memory reading iovs[0].length",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset = one past the size of this memory
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "length to write exceeds memory by 1",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				2, 0, 0, 0, // = iovs[0].length = one past the size of this memory
				'?',
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "resultSize offset is outside memory",
			fd:   validFD,
			iovs: 1, iovsLen: 1,
			resultSize: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			offset := uint64(wasm.MemoryPagesToBytesNum(testMemoryPageSize)) - uint64(len(tc.memory))

			memoryWriteOK := mem.Write(uint32(offset), tc.memory)
			require.True(t, memoryWriteOK)

			results, err := fn.Call(context.Background(), tc.fd, tc.iovs+offset, tc.iovsLen+offset, tc.resultSize+offset)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

func createFile(t *testing.T, path string, contents []byte) (*memFile, *MemFS) {
	memFS := &MemFS{}
	f, err := memFS.OpenWASI(0, path, wasi.O_CREATE|wasi.O_TRUNC, wasi.R_FD_WRITE, 0, 0)
	require.NoError(t, err)

	if _, err := io.Copy(f, bytes.NewBuffer(contents)); err != nil {
		require.NoError(t, err)
	}

	return f.(*memFile), memFS
}

// // TODO: TestSnapshotPreview1_PathCreateDirectory TestSnapshotPreview1_PathCreateDirectory_Errors
// // TODO: TestSnapshotPreview1_PathFilestatGet TestSnapshotPreview1_PathFilestatGet_Errors
// // TODO: TestSnapshotPreview1_PathFilestatSetTimes TestSnapshotPreview1_PathFilestatSetTimes_Errors
// // TODO: TestSnapshotPreview1_PathLink TestSnapshotPreview1_PathLink_Errors
// // TODO: TestSnapshotPreview1_PathOpen TestSnapshotPreview1_PathOpen_Errors
// // TODO: TestSnapshotPreview1_PathReadlink TestSnapshotPreview1_PathReadlink_Errors
// // TODO: TestSnapshotPreview1_PathRemoveDirectory TestSnapshotPreview1_PathRemoveDirectory_Errors
// // TODO: TestSnapshotPreview1_PathRename TestSnapshotPreview1_PathRename_Errors
// // TODO: TestSnapshotPreview1_PathSymlink TestSnapshotPreview1_PathSymlink_Errors
// // TODO: TestSnapshotPreview1_PathUnlinkFile TestSnapshotPreview1_PathUnlinkFile_Errors
// // TODO: TestSnapshotPreview1_PollOneoff TestSnapshotPreview1_PollOneoff_Errors

func TestSnapshotPreview1_ProcExit(t *testing.T) {
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

	_, ctx := instantiateWasmStore(t, FunctionProcExit, ImportProcExit, moduleName)
	fn := ctx.Function(FunctionProcExit)
	require.NotNil(t, fn)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// When ProcExit is called, store.CallFunction returns immediately, returning the exit code as the error.
			_, err := fn.Call(context.Background(), uint64(tc.exitCode))
			var code wasi.ExitCode
			require.ErrorAs(t, err, &code)
			require.Equal(t, code, wasi.ExitCode(tc.exitCode))
		})
	}
}

// // TODO: TestSnapshotPreview1_ProcRaise TestSnapshotPreview1_ProcRaise_Errors
// // TODO: TestSnapshotPreview1_SchedYield TestSnapshotPreview1_SchedYield_Errors

func TestSnapshotPreview1_RandomGet(t *testing.T) {
	expectedMemory := []byte{
		'?',                          // `offset` is after this
		0x53, 0x8c, 0x7f, 0x96, 0xb1, // random data from seed value of 42
		'?', // stopped after encoding
	}

	var length = uint32(5) // arbitrary length,
	var offset = uint32(1) // offset,
	var seed = int64(42)   // and seed value

	randOpt := func(api *wasiAPI) {
		api.randSource = func(p []byte) error {
			s := rand.NewSource(seed)
			rng := rand.New(s)
			_, err := rng.Read(p)

			return err
		}
	}

	_, ctx := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, moduleName, randOpt)
	mem := ctx.Memory()
	t.Run("SnapshotPreview1.RandomGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke RandomGet directly and check the memory side effects!
		errno := NewAPI(randOpt).RandomGet(ctx, offset, length)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, offset+length+1)
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	fn := ctx.Function(FunctionRandomGet)
	require.NotNil(t, fn)

	t.Run(FunctionRandomGet, func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		results, err := fn.Call(context.Background(), uint64(offset), uint64(length))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mem.Read(0, offset+length+1)
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_RandomGet_Errors(t *testing.T) {
	validAddress := uint32(0) // arbitrary valid address

	_, ctx := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, moduleName)
	memorySize := ctx.Memory().Size()
	fn := ctx.Function(FunctionRandomGet)
	require.NotNil(t, fn)

	tests := []struct {
		name   string
		offset uint32
		length uint32
	}{
		{
			name:   "out-of-memory",
			offset: memorySize,
			length: 1,
		},

		{
			name:   "random length exceeds maximum valid address by 1",
			offset: validAddress,
			length: memorySize + 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(context.Background(), uint64(tc.offset), uint64(tc.length))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_RandomGet_SourceError(t *testing.T) {
	_, ctx := instantiateWasmStore(t, FunctionRandomGet, ImportRandomGet, moduleName, func(api *wasiAPI) {
		api.randSource = func(p []byte) error {
			return errors.New("random source error")
		}
	})

	fn := ctx.Function(FunctionRandomGet)
	require.NotNil(t, fn)

	results, err := fn.Call(context.Background(), uint64(1), uint64(5)) // arbitrary offset and length
	require.NoError(t, err)
	require.Equal(t, uint64(wasi.ErrnoIo), results[0]) // results[0] is the errno
}

// TODO: TestSnapshotPreview1_SockRecv TestSnapshotPreview1_SockRecv_Errors
// TODO: TestSnapshotPreview1_SockSend TestSnapshotPreview1_SockSend_Errors
// TODO: TestSnapshotPreview1_SockShutdown TestSnapshotPreview1_SockShutdown_Errors

const testMemoryPageSize = 1

func instantiateWasmStore(t *testing.T, wasiFunction, wasiImport, moduleName string, opts ...Option) (*wasm.Store, *wasm.ModuleContext) {
	mod, err := text.DecodeModule([]byte(fmt.Sprintf(`(module
  %[2]s
  (memory %[3]d)
  (export "memory" (memory 0))
  (export "%[1]s" (func $wasi.%[1]s))
)`, wasiFunction, wasiImport, testMemoryPageSize)))
	require.NoError(t, err)

	store := wasm.NewStore(context.Background(), interpreter.NewEngine())

	snapshotPreview1Functions := SnapshotPreview1Functions(opts...)
	_, err = store.ExportHostFunctions(wasi.ModuleSnapshotPreview1, snapshotPreview1Functions)
	require.NoError(t, err)

	instantiated, err := store.Instantiate(mod, moduleName)
	require.NoError(t, err)
	return store, instantiated.Context
}

// maskMemory sets the first memory in the store to '?' * size, so tests can see what's written.
func maskMemory(t *testing.T, mem publicwasm.Memory, size int) {
	for i := uint32(0); i < uint32(size); i++ {
		require.True(t, mem.WriteByte(i, '?'))
	}
}
