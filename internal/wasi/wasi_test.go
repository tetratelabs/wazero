package internalwasi

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
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

	mem, fn := instantiateModule(t, FunctionArgsGet, ImportArgsGet, moduleName, args)

	t.Run("SnapshotPreview1.ArgsGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke ArgsGet directly and check the memory side effects.
		errno := NewAPI(args).ArgsGet(ctx(mem), argv, argvBuf)
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

func ctx(mem publicwasm.Memory) *wasm.ModuleContext {
	ctx := (&wasm.ModuleContext{}).WithMemory(mem.(*wasm.MemoryInstance))
	return ctx
}

func TestSnapshotPreview1_ArgsGet_Errors(t *testing.T) {
	args, err := Args("a", "bc")
	require.NoError(t, err)
	mem, fn := instantiateModule(t, FunctionArgsGet, ImportArgsGet, moduleName, args)

	memorySize := mem.Size()
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

	mem, fn := instantiateModule(t, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, args)

	t.Run("SnapshotPreview1.ArgsSizesGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke ArgsSizesGet directly and check the memory side effects.
		errno := NewAPI(args).ArgsSizesGet(ctx(mem), resultArgc, resultArgvBufSize)
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

	mem, fn := instantiateModule(t, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, args)

	memorySize := mem.Size()
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

	mem, fn := instantiateModule(t, FunctionEnvironGet, ImportEnvironGet, moduleName, envOpt)

	t.Run("SnapshotPreview1.EnvironGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke EnvironGet directly and check the memory side effects.
		errno := NewAPI(envOpt).EnvironGet(ctx(mem), resultEnviron, resultEnvironBuf)
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

	mem, fn := instantiateModule(t, FunctionEnvironGet, ImportEnvironGet, moduleName, envOpt)

	memorySize := mem.Size()
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

	mem, fn := instantiateModule(t, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, envOpt)

	t.Run("SnapshotPreview1.EnvironSizesGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke EnvironSizesGet directly and check the memory side effects.
		errno := NewAPI(envOpt).EnvironSizesGet(ctx(mem), resultEnvironc, resultEnvironBufSize)
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

	mem, fn := instantiateModule(t, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, envOpt)

	memorySize := mem.Size()
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

// TestSnapshotPreview1_ClockResGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_ClockResGet(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionClockResGet, ImportClockResGet, moduleName)

	t.Run("SnapshotPreview1.ClockResGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().ClockResGet(ctx(mem), 0, 0))
	})

	t.Run(FunctionClockResGet, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

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
	mem, fn := instantiateModule(t, FunctionClockTimeGet, ImportClockTimeGet, moduleName, clockOpt)

	t.Run("SnapshotPreview1.ClockTimeGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// invoke ClockTimeGet directly and check the memory side effects!
		errno := NewAPI(clockOpt).ClockTimeGet(ctx(mem), 0 /* TODO: id */, 0 /* TODO: precision */, resultTimestamp)
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

	mem, fn := instantiateModule(t, FunctionClockTimeGet, ImportClockTimeGet, moduleName, func(api *wasiAPI) {
		api.timeNowUnixNano = func() uint64 { return epochNanos }
	})

	memorySize := mem.Size()

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

// TestSnapshotPreview1_FdAdvise only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdAdvise(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdAdvise, ImportFdAdvise, moduleName)

	t.Run("SnapshotPreview1.FdAdvise", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdAdvise(ctx(mem), 0, 0, 0, 0))
	})

	t.Run(FunctionFdAdvise, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdAllocate only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdAllocate(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdAllocate, ImportFdAllocate, moduleName)

	t.Run("SnapshotPreview1.FdAllocate", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdAllocate(ctx(mem), 0, 0, 0))
	})

	t.Run(FunctionFdAllocate, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdClose(t *testing.T) {
	fdToClose := uint32(3) // arbitrary fd
	fdToKeep := uint32(4)  // another arbitrary fd

	setupFD := func() (publicwasm.Memory, publicwasm.Function, *wasiAPI) {
		var api *wasiAPI
		mem, fn := instantiateModule(t, FunctionFdClose, ImportFdClose, moduleName, func(a *wasiAPI) {
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
		return mem, fn, api
	}

	t.Run("SnapshotPreview1.FdClose", func(t *testing.T) {
		mem, _, api := setupFD()

		errno := api.FdClose(ctx(mem), fdToClose)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.NotContains(t, api.opened, fdToClose) // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.opened, fdToKeep)
	})
	t.Run(FunctionFdClose, func(t *testing.T) {
		_, fn, api := setupFD()

		ret, err := fn.Call(context.Background(), uint64(fdToClose))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64
		require.NotContains(t, api.opened, fdToClose)           // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.opened, fdToKeep)
	})
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		mem, _, api := setupFD()

		errno := api.FdClose(ctx(mem), 42) // 42 is an arbitrary invalid FD
		require.Equal(t, wasi.ErrnoBadf, errno)
	})
}

// TestSnapshotPreview1_FdDatasync only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdDatasync(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdDatasync, ImportFdDatasync, moduleName)

	t.Run("SnapshotPreview1.FdDatasync", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdDatasync(ctx(mem), 0))
	})

	t.Run(FunctionFdDatasync, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TODO: TestSnapshotPreview1_FdFdstatGet TestSnapshotPreview1_FdFdstatGet_Errors

// TestSnapshotPreview1_FdFdstatSetFlags only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFdstatSetFlags(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdFdstatSetFlags, ImportFdFdstatSetFlags, moduleName)

	t.Run("SnapshotPreview1.FdFdstatSetFlags", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdFdstatSetFlags(ctx(mem), 0, 0))
	})

	t.Run(FunctionFdFdstatSetFlags, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFdstatSetRights only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFdstatSetRights(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdFdstatSetRights, ImportFdFdstatSetRights, moduleName)

	t.Run("SnapshotPreview1.FdFdstatSetRights", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdFdstatSetRights(ctx(mem), 0, 0, 0))
	})

	t.Run(FunctionFdFdstatSetRights, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFilestatGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFilestatGet(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdFilestatGet, ImportFdFilestatGet, moduleName)

	t.Run("SnapshotPreview1.FdFilestatGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdFilestatGet(ctx(mem), 0, 0))
	})

	t.Run(FunctionFdFilestatGet, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFilestatSetSize only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFilestatSetSize(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdFilestatSetSize, ImportFdFilestatSetSize, moduleName)

	t.Run("SnapshotPreview1.FdFilestatSetSize", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdFilestatSetSize(ctx(mem), 0, 0))
	})

	t.Run(FunctionFdFilestatSetSize, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFilestatSetTimes only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFilestatSetTimes(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdFilestatSetTimes, ImportFdFilestatSetTimes, moduleName)

	t.Run("SnapshotPreview1.FdFilestatSetTimes", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdFilestatSetTimes(ctx(mem), 0, 0, 0, 0))
	})

	t.Run(FunctionFdFilestatSetTimes, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdPread only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdPread(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdPread, ImportFdPread, moduleName)

	t.Run("SnapshotPreview1.FdPread", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdPread(ctx(mem), 0, 0, 0, 0))
	})

	t.Run(FunctionFdPread, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TODO: TestSnapshotPreview1_FdPrestatGet TestSnapshotPreview1_FdPrestatGet_Errors

func TestSnapshotPreview1_FdPrestatDirName(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	var api *wasiAPI
	mem, fn := instantiateModule(t, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, func(a *wasiAPI) {
		a.opened[fd] = fileEntry{
			path:    "/tmp",
			fileSys: &MemFS{},
		}
		api = a // for later tests
	})

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(3) // shorter than len("/tmp") to test the path is written for the length of pathLen
	expectedMemory := []byte{
		'?',
		'/', 't', 'm',
		'?', '?', '?',
	}

	t.Run("SnapshotPreview1.FdPrestatDirName", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		errno := api.FdPrestatDirName(ctx(mem), fd, path, pathLen)
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
	mem, fn := instantiateModule(t, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, opt)

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

// TestSnapshotPreview1_FdPwrite only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdPwrite(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdPwrite, ImportFdPwrite, moduleName)

	t.Run("SnapshotPreview1.FdPwrite", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdPwrite(ctx(mem), 0, 0, 0, 0))
	})

	t.Run(FunctionFdPwrite, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
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
	iovsCount := uint32(2)   // The count of iovs
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
	mem, fn := instantiateModule(t, FunctionFdRead, ImportFdRead, moduleName, func(a *wasiAPI) {
		api = a // for later tests
	})

	// TestSnapshotPreview1_FdRead uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdReadFn func(ctx publicwasm.ModuleContext, fd, iovs, iovsCount, resultSize uint32) wasi.Errno
	tests := []struct {
		name   string
		fdRead func() fdReadFn
	}{
		{"SnapshotPreview1.FdRead", func() fdReadFn {
			return api.FdRead
		}},
		{FunctionFdRead, func() fdReadFn {
			return func(ctx publicwasm.ModuleContext, fd, iovs, iovsCount, resultSize uint32) wasi.Errno {
				ret, err := fn.Call(context.Background(), uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultSize))
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

			errno := tc.fdRead()(ctx(mem), fd, iovs, iovsCount, resultSize)
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
	mem, fn := instantiateModule(t, FunctionFdRead, ImportFdRead, moduleName, func(a *wasiAPI) {
		a.opened[uint32(validFD)] = fileEntry{
			path:    "test_path",
			fileSys: memFS,
			file:    file,
		}
	})

	tests := []struct {
		name                            string
		fd, iovs, iovsCount, resultSize uint64
		memory                          []byte
		expectedErrno                   wasi.Errno
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
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   validFD,
			iovs: 1, iovsCount: 1,
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
			iovs: 1, iovsCount: 1,
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
			iovs: 1, iovsCount: 1,
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

			results, err := fn.Call(context.Background(), tc.fd, tc.iovs+offset, tc.iovsCount+offset, tc.resultSize+offset)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_FdReaddir only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdReaddir(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdReaddir, ImportFdReaddir, moduleName)

	t.Run("SnapshotPreview1.FdReaddir", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdReaddir(ctx(mem), 0, 0, 0, 0, 0))
	})

	t.Run(FunctionFdReaddir, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdRenumber only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdRenumber(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdRenumber, ImportFdRenumber, moduleName)

	t.Run("SnapshotPreview1.FdRenumber", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdRenumber(ctx(mem), 0, 0))
	})

	t.Run(FunctionFdRenumber, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdSeek only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdSeek(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdSeek, ImportFdSeek, moduleName)

	t.Run("SnapshotPreview1.FdSeek", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdSeek(ctx(mem), 0, 0, 0, 0))
	})

	t.Run(FunctionFdSeek, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdSync only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdSync(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdSync, ImportFdSync, moduleName)

	t.Run("SnapshotPreview1.FdSync", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdSync(ctx(mem), 0))
	})

	t.Run(FunctionFdSync, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdTell only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdTell(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionFdTell, ImportFdTell, moduleName)

	t.Run("SnapshotPreview1.FdTell", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().FdTell(ctx(mem), 0, 0))
	})

	t.Run(FunctionFdTell, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

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
	iovsCount := uint32(2)   // The count of iovs
	resultSize := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	var api *wasiAPI
	mem, fn := instantiateModule(t, FunctionFdWrite, ImportFdWrite, moduleName, func(a *wasiAPI) {
		api = a // for later tests
	})

	// TestSnapshotPreview1_FdWrite uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdWriteFn func(ctx publicwasm.ModuleContext, fd, iovs, iovsCount, resultSize uint32) wasi.Errno
	tests := []struct {
		name    string
		fdWrite func() fdWriteFn
	}{
		{"SnapshotPreview1.FdWrite", func() fdWriteFn {
			return api.FdWrite
		}},
		{FunctionFdWrite, func() fdWriteFn {
			return func(ctx publicwasm.ModuleContext, fd, iovs, iovsCount, resultSize uint32) wasi.Errno {
				ret, err := fn.Call(context.Background(), uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultSize))
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

			errno := tc.fdWrite()(ctx(mem), fd, iovs, iovsCount, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actualMemory, ok := mem.Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actualMemory)
			require.Equal(t, []byte("wazero"), file.buf.Bytes()) // verify the file was actually written
		})
	}
}

func TestSnapshotPreview1_FdWrite_Errors(t *testing.T) {
	validFD := uint64(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents
	mem, fn := instantiateModule(t, FunctionFdWrite, ImportFdWrite, moduleName, func(a *wasiAPI) {
		a.opened[uint32(validFD)] = fileEntry{
			path:    "test_path",
			fileSys: memFS,
			file:    file,
		}
	})

	tests := []struct {
		name                            string
		fd, iovs, iovsCount, resultSize uint64
		memory                          []byte
		expectedErrno                   wasi.Errno
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
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   validFD,
			iovs: 1, iovsCount: 1,
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
			iovs: 1, iovsCount: 1,
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
			iovs: 1, iovsCount: 1,
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

			results, err := fn.Call(context.Background(), tc.fd, tc.iovs+offset, tc.iovsCount+offset, tc.resultSize+offset)
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

// TestSnapshotPreview1_PathCreateDirectory only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathCreateDirectory(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathCreateDirectory, ImportPathCreateDirectory, moduleName)

	t.Run("SnapshotPreview1.PathCreateDirectory", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathCreateDirectory(ctx(mem), 0, 0, 0))
	})

	t.Run(FunctionPathCreateDirectory, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathFilestatGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathFilestatGet(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathFilestatGet, ImportPathFilestatGet, moduleName)

	t.Run("SnapshotPreview1.PathFilestatGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathFilestatGet(ctx(mem), 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathFilestatGet, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathFilestatSetTimes only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathFilestatSetTimes(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathFilestatSetTimes, ImportPathFilestatSetTimes, moduleName)

	t.Run("SnapshotPreview1.PathFilestatSetTimes", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathFilestatSetTimes(ctx(mem), 0, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathFilestatSetTimes, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathLink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathLink(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathLink, ImportPathLink, moduleName)

	t.Run("SnapshotPreview1.PathLink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathLink(ctx(mem), 0, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathLink, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_PathOpen(t *testing.T) {
	fd := uint32(3)                              // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	dirflags := uint32(0)                        // arbitrary dirflags
	path := uint32(1)                            // arbitrary offset
	pathLen := uint32(6)                         // The length of path
	oflags := uint32(0)                          // arbitrary oflags
	fsRightsBase := uint64(wasi.R_FD_READ)       // arbitrary right
	fsRightsInheriting := uint64(wasi.R_FD_READ) // arbitrary right
	fdFlags := uint32(0)
	resultOpenedFD := uint32(8)
	initialMemory := []byte{
		'?',                          // `path` is after this
		'w', 'a', 'z', 'e', 'r', 'o', // path
		'?', // `resultOpenedFD` is after this
	}
	expectedMemory := append(
		initialMemory,
		4, 0, 0, 0, // resultOpenedFD
		'?',
	)
	expectedFd := uint32(4) // arbitrary expected FD

	var api *wasiAPI
	mem, fn := instantiateModule(t, FunctionPathOpen, ImportPathOpen, moduleName, func(a *wasiAPI) {
		// randSouce is used to determine the new fd. Fix it to the expectedFD for testing.
		a.randSource = func(b []byte) error {
			binary.LittleEndian.PutUint32(b, expectedFd)
			return nil
		}
		api = a // for later tests
	})

	// TestSnapshotPreview1_PathOpen uses a matrix because setting up test files is complicated and has to be clean each time.
	type pathOpenFn func(ctx publicwasm.ModuleContext, fd, dirflags, pathString, pathLen, oflags uint32,
		fsRightsBase, fsRightsInheriting uint64,
		fdFlags, resultOpenedFD uint32) wasi.Errno
	tests := []struct {
		name     string
		pathOpen func() pathOpenFn
	}{
		{"SnapshotPreview1.PathOpen", func() pathOpenFn {
			return api.PathOpen
		}},
		{FunctionPathOpen, func() pathOpenFn {
			return func(ctx publicwasm.ModuleContext, fd, dirflags, path, pathLen, oflags uint32,
				fsRightsBase, fsRightsInheriting uint64,
				fdFlags, resultOpenedFD uint32) wasi.Errno {
				ret, err := fn.Call(context.Background(), uint64(fd), uint64(dirflags), uint64(path), uint64(pathLen), uint64(oflags), uint64(fsRightsBase), uint64(fsRightsInheriting), uint64(fdFlags), uint64(resultOpenedFD))
				require.NoError(t, err)
				return wasi.Errno(ret[0])
			}
		}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			// Create a memFS for testing that has "./wazero" file.
			memFS := &MemFS{
				Files: map[string][]byte{
					"wazero": []byte(""),
				},
			}
			api.opened = map[uint32]fileEntry{
				fd: {
					path:    ".",
					fileSys: memFS,
				},
			}

			maskMemory(t, mem, len(expectedMemory))
			ok := mem.Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.pathOpen()(ctx(mem), fd, dirflags, path, pathLen, oflags, fsRightsBase, fsRightsInheriting, fdFlags, resultOpenedFD)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actualMemory, ok := mem.Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actualMemory)
			require.Equal(t, "wazero", api.opened[expectedFd].path) // verify the file was actually opened
		})
	}
}

func TestSnapshotPreview1_PathOpen_Erros(t *testing.T) {
	validFD := uint64(3) // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	mem, fn := instantiateModule(t, FunctionPathOpen, ImportPathOpen, moduleName, func(a *wasiAPI) {
		// Create a memFS for testing that has "./wazero" file.
		memFS := &MemFS{
			Files: map[string][]byte{
				"wazero": []byte(""),
			},
		}
		a.opened = map[uint32]fileEntry{
			uint32(validFD): {
				path:    ".",
				fileSys: memFS,
			},
		}
	})

	tests := []struct {
		name                                                                                                 string
		fd, dirflags, pathString, pathLen, oflags, fsRightsBase, fsRightsInheriting, fdFlags, resultOpenedFD uint64
		pathName                                                                                             string
		expectedErrno                                                                                        wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory reading path",
			fd:            validFD,
			pathString:    uint64(mem.Size()),
			pathLen:       1, // arbitrary length
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            validFD,
			pathString:    0, // pathLen is out-of-memory for this offset
			pathLen:       uint64(mem.Size() + 1),
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "no such file exists",
			fd:            validFD,
			pathString:    0,      // abirtrary offset, pathname is here
			pathLen:       4,      // length of "none"
			pathName:      "none", // file that doesn't exist
			expectedErrno: wasi.ErrnoNoent,
		},
		{
			name:           "out-of-memory writing resultOpenedFD",
			fd:             validFD,
			pathString:     0, // abirtrary offset, pathName is here
			pathLen:        6, // length of "wazero"
			resultOpenedFD: uint64(mem.Size()),
			pathName:       "wazero", // the file that exists
			expectedErrno:  wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if tc.pathName != "" {
				mem.Write(uint32(tc.pathString), []byte(tc.pathName))
			}

			results, err := fn.Call(context.Background(), tc.fd, tc.dirflags, tc.pathString, tc.pathLen, tc.oflags, tc.fsRightsBase, tc.fsRightsInheriting, tc.fdFlags, tc.resultOpenedFD)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_PathReadlink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathReadlink(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathReadlink, ImportPathReadlink, moduleName)

	t.Run("SnapshotPreview1.PathLink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathReadlink(ctx(mem), 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathReadlink, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathRemoveDirectory only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathRemoveDirectory(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathRemoveDirectory, ImportPathRemoveDirectory, moduleName)

	t.Run("SnapshotPreview1.PathRemoveDirectory", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathRemoveDirectory(ctx(mem), 0, 0, 0))
	})

	t.Run(FunctionPathRemoveDirectory, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathRename only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathRename(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathRename, ImportPathRename, moduleName)

	t.Run("SnapshotPreview1.PathRename", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathRename(ctx(mem), 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathRename, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathSymlink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathSymlink(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathSymlink, ImportPathSymlink, moduleName)

	t.Run("SnapshotPreview1.PathSymlink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathSymlink(ctx(mem), 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathSymlink, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathUnlinkFile only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathUnlinkFile(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPathUnlinkFile, ImportPathUnlinkFile, moduleName)

	t.Run("SnapshotPreview1.PathUnlinkFile", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PathUnlinkFile(ctx(mem), 0, 0, 0))
	})

	t.Run(FunctionPathUnlinkFile, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PollOneoff only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PollOneoff(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionPollOneoff, ImportPollOneoff, moduleName)

	t.Run("SnapshotPreview1.PollOneoff", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().PollOneoff(ctx(mem), 0, 0, 0, 0))
	})

	t.Run(FunctionPollOneoff, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

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

	_, fn := instantiateModule(t, FunctionProcExit, ImportProcExit, moduleName)

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

// TestSnapshotPreview1_ProcRaise only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_ProcRaise(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionProcRaise, ImportProcRaise, moduleName)

	t.Run("SnapshotPreview1.ProcRaise", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().ProcRaise(ctx(mem), 0))
	})

	t.Run(FunctionProcRaise, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_SchedYield only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SchedYield(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionSchedYield, ImportSchedYield, moduleName)

	t.Run("SnapshotPreview1.SchedYield", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().SchedYield(ctx(mem)))
	})

	t.Run(FunctionSchedYield, func(t *testing.T) {
		results, err := fn.Call(context.Background())
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

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

	mem, fn := instantiateModule(t, FunctionRandomGet, ImportRandomGet, moduleName, randOpt)
	t.Run("SnapshotPreview1.RandomGet", func(t *testing.T) {
		maskMemory(t, mem, len(expectedMemory))

		// Invoke RandomGet directly and check the memory side effects!
		errno := NewAPI(randOpt).RandomGet(ctx(mem), offset, length)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mem.Read(0, offset+length+1)
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

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

	mem, fn := instantiateModule(t, FunctionRandomGet, ImportRandomGet, moduleName)
	memorySize := mem.Size()

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
	_, fn := instantiateModule(t, FunctionRandomGet, ImportRandomGet, moduleName, func(api *wasiAPI) {
		api.randSource = func(p []byte) error {
			return errors.New("random source error")
		}
	})

	results, err := fn.Call(context.Background(), uint64(1), uint64(5)) // arbitrary offset and length
	require.NoError(t, err)
	require.Equal(t, uint64(wasi.ErrnoIo), results[0]) // results[0] is the errno
}

// TestSnapshotPreview1_SockRecv only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockRecv(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionSockRecv, ImportSockRecv, moduleName)

	t.Run("SnapshotPreview1.SockRecv", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().SockRecv(ctx(mem), 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionSockRecv, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_SockSend only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockSend(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionSockSend, ImportSockSend, moduleName)

	t.Run("SnapshotPreview1.SockSend", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().SockSend(ctx(mem), 0, 0, 0, 0, 0))
	})

	t.Run(FunctionSockSend, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_SockShutdown only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockShutdown(t *testing.T) {
	mem, fn := instantiateModule(t, FunctionSockShutdown, ImportSockShutdown, moduleName)

	t.Run("SnapshotPreview1.SockShutdown", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI().SockShutdown(ctx(mem), 0, 0))
	})

	t.Run(FunctionSockShutdown, func(t *testing.T) {
		results, err := fn.Call(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

const testMemoryPageSize = 1

func instantiateModule(t *testing.T, wasiFunction, wasiImport, moduleName string, opts ...Option) (publicwasm.Memory, publicwasm.Function) {
	enabledFeatures := wasm.Features20191205
	mem, err := text.DecodeModule([]byte(fmt.Sprintf(`(module
  %[2]s
  (memory 1)  ;; just an arbitrary size big enough for tests
  (export "memory" (memory 0))
  (export "%[1]s" (func $wasi.%[1]s))
)`, wasiFunction, wasiImport)), enabledFeatures)
	require.NoError(t, err)

	// The package `wazero` has a simpler interface for adding host modules, but we can't use that as it would create an
	// import cycle. Instead, we export Store.NewHostModule and use it here.
	store := wasm.NewStore(context.Background(), interpreter.NewEngine(), enabledFeatures)
	_, err = store.NewHostModule(wasi.ModuleSnapshotPreview1, SnapshotPreview1Functions(opts...))
	require.NoError(t, err)

	instantiated, err := store.Instantiate(mem, moduleName)
	require.NoError(t, err)

	fn := instantiated.Function(wasiFunction)
	require.NotNil(t, fn)
	return instantiated.Memory("memory"), fn
}

// maskMemory sets the first memory in the store to '?' * size, so tests can see what's written.
//
func maskMemory(t *testing.T, mem publicwasm.Memory, size int) {
	for i := uint32(0); i < uint32(size); i++ {
		require.True(t, mem.WriteByte(i, '?'))
	}
}
