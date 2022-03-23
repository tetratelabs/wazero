package internalwasi

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math"
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

func TestSnapshotPreview1_ArgsGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext([]string{"a", "bc"}, nil, nil)
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

	a, mod, fn := instantiateModule(t, ctx, FunctionArgsGet, ImportArgsGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.ArgsGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke ArgsGet directly and check the memory side effects.
		errno := a.ArgsGet(mod, argv, argvBuf)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionArgsGet, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		results, err := fn.Call(mod, uint64(argv), uint64(argvBuf))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_ArgsGet_Errors(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext([]string{"a", "bc"}, nil, nil)
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionArgsGet, ImportArgsGet, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()
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
			errno := a.ArgsGet(mod, tc.argv, tc.argvBuf)
			require.NoError(t, err)
			require.Equal(t, wasi.ErrnoFault, errno)
		})
	}
}

func TestSnapshotPreview1_ArgsSizesGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext([]string{"a", "bc"}, nil, nil)
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

	a, mod, fn := instantiateModule(t, ctx, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.ArgsSizesGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke ArgsSizesGet directly and check the memory side effects.
		errno := a.ArgsSizesGet(mod, resultArgc, resultArgvBufSize)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionArgsSizesGet, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		results, err := fn.Call(mod, uint64(resultArgc), uint64(resultArgvBufSize))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_ArgsSizesGet_Errors(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext([]string{"a", "bc"}, nil, nil)
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()
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
			errno := a.ArgsSizesGet(mod, tc.argc, tc.argvBufSize)
			require.NoError(t, err)
			require.Equal(t, wasi.ErrnoFault, errno)
		})
	}
}

func TestSnapshotPreview1_EnvironGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, []string{"a=b", "b=cd"}, nil)
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

	a, mod, fn := instantiateModule(t, ctx, FunctionEnvironGet, ImportEnvironGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.EnvironGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke EnvironGet directly and check the memory side effects.
		errno := a.EnvironGet(mod, resultEnviron, resultEnvironBuf)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionEnvironGet, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		results, err := fn.Call(mod, uint64(resultEnviron), uint64(resultEnvironBuf))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_EnvironGet_Errors(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, []string{"a=bc", "b=cd"}, nil)
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionEnvironGet, ImportEnvironGet, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()
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
			errno := a.EnvironGet(mod, tc.environ, tc.environBuf)
			require.NoError(t, err)
			require.Equal(t, wasi.ErrnoFault, errno)
		})
	}
}

func TestSnapshotPreview1_EnvironSizesGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, []string{"a=b", "b=cd"}, nil)
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

	a, mod, fn := instantiateModule(t, ctx, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.EnvironSizesGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke EnvironSizesGet directly and check the memory side effects.
		errno := a.EnvironSizesGet(mod, resultEnvironc, resultEnvironBufSize)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionEnvironSizesGet, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		results, err := fn.Call(mod, uint64(resultEnvironc), uint64(resultEnvironBufSize))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_EnvironSizesGet_Errors(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, []string{"a=b", "b=cd"}, nil)
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()
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
			errno := a.EnvironSizesGet(mod, tc.environc, tc.environBufSize)
			require.NoError(t, err)
			require.Equal(t, wasi.ErrnoFault, errno)
		})
	}
}

// TestSnapshotPreview1_ClockResGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_ClockResGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionClockResGet, ImportClockResGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.ClockResGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.ClockResGet(mod, 0, 0))
	})

	t.Run(FunctionClockResGet, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
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

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionClockTimeGet, ImportClockTimeGet, moduleName, sys)
	defer mod.Close()

	a.timeNowUnixNano = func() uint64 { return epochNanos }

	t.Run("SnapshotPreview1.ClockTimeGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// invoke ClockTimeGet directly and check the memory side effects!
		errno := a.ClockTimeGet(mod, 0 /* TODO: id */, 0 /* TODO: precision */, resultTimestamp)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionClockTimeGet, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		results, err := fn.Call(mod, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(resultTimestamp))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_ClockTimeGet_Errors(t *testing.T) {
	epochNanos := uint64(1640995200000000000) // midnight UTC 2022-01-01

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionClockTimeGet, ImportClockTimeGet, moduleName, sys)
	defer mod.Close()

	a.timeNowUnixNano = func() uint64 { return epochNanos }

	memorySize := mod.Memory().Size()

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
			results, err := fn.Call(mod, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(tc.resultTimestamp))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_FdAdvise only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdAdvise(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdAdvise, ImportFdAdvise, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdAdvise", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdAdvise(mod, 0, 0, 0, 0))
	})

	t.Run(FunctionFdAdvise, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdAllocate only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdAllocate(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdAllocate, ImportFdAllocate, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdAllocate", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdAllocate(mod, 0, 0, 0))
	})

	t.Run(FunctionFdAllocate, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdClose(t *testing.T) {
	fdToClose := uint32(3) // arbitrary fd
	fdToKeep := uint32(4)  // another arbitrary fd

	setupFD := func() (publicwasm.Module, publicwasm.Function, *wasiAPI) {
		ctx := context.Background()
		memFs := &MemFS{}
		sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
			fdToClose: {
				Path: "/tmp",
				FS:   memFs,
			},
			fdToKeep: {
				Path: "path to keep",
				FS:   memFs,
			},
		})
		require.NoError(t, err)

		a, mod, fn := instantiateModule(t, ctx, FunctionFdClose, ImportFdClose, moduleName, sys)
		return mod, fn, a
	}

	verify := func(mod publicwasm.Module) {
		// Verify fdToClose is closed and removed from the opened FDs.
		_, ok := sysContext(mod).OpenedFile(fdToClose)
		require.False(t, ok)

		// Verify fdToKeep is not closed
		_, ok = sysContext(mod).OpenedFile(fdToKeep)
		require.True(t, ok)
	}

	t.Run("SnapshotPreview1.FdClose", func(t *testing.T) {
		mod, _, api := setupFD()
		defer mod.Close()

		errno := api.FdClose(mod, fdToClose)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		verify(mod)
	})
	t.Run(FunctionFdClose, func(t *testing.T) {
		mod, fn, _ := setupFD()
		defer mod.Close()

		ret, err := fn.Call(mod, uint64(fdToClose))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64

		verify(mod)
	})
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		mod, _, api := setupFD()
		defer mod.Close()

		errno := api.FdClose(mod, 42) // 42 is an arbitrary invalid FD
		require.Equal(t, wasi.ErrnoBadf, errno)
	})
}

// TestSnapshotPreview1_FdDatasync only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdDatasync(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdDatasync, ImportFdDatasync, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdDatasync", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdDatasync(mod, 0))
	})

	t.Run(FunctionFdDatasync, func(t *testing.T) {
		results, err := fn.Call(mod, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TODO: TestSnapshotPreview1_FdFdstatGet TestSnapshotPreview1_FdFdstatGet_Errors

// TestSnapshotPreview1_FdFdstatSetFlags only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFdstatSetFlags(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdFdstatSetFlags, ImportFdFdstatSetFlags, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdFdstatSetFlags", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdFdstatSetFlags(mod, 0, 0))
	})

	t.Run(FunctionFdFdstatSetFlags, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFdstatSetRights only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFdstatSetRights(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdFdstatSetRights, ImportFdFdstatSetRights, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdFdstatSetRights", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdFdstatSetRights(mod, 0, 0, 0))
	})

	t.Run(FunctionFdFdstatSetRights, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFilestatGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFilestatGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdFilestatGet, ImportFdFilestatGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdFilestatGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdFilestatGet(mod, 0, 0))
	})

	t.Run(FunctionFdFilestatGet, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFilestatSetSize only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFilestatSetSize(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdFilestatSetSize, ImportFdFilestatSetSize, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdFilestatSetSize", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdFilestatSetSize(mod, 0, 0))
	})

	t.Run(FunctionFdFilestatSetSize, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdFilestatSetTimes only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdFilestatSetTimes(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdFilestatSetTimes, ImportFdFilestatSetTimes, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdFilestatSetTimes", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdFilestatSetTimes(mod, 0, 0, 0, 0))
	})

	t.Run(FunctionFdFilestatSetTimes, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdPread only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdPread(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdPread, ImportFdPread, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdPread", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdPread(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionFdPread, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdPrestatGet(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{fd: {Path: "/tmp"}})
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdPrestatGet, ImportFdPrestatGet, moduleName, sys)
	defer mod.Close()

	resultPrestat := uint32(1) // arbitrary offset
	expectedMemory := []byte{
		'?',     // resultPrstat after this
		0,       // 8-bit tag indicating `prestat_dir`, the only available tag
		0, 0, 0, // 3-byte padding
		// the result path length field after this
		4, 0, 0, 0, // = 4, which is len("/tmp")
		'?',
	}

	t.Run("SnapshotPreview1.FdPrestatGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		errno := a.FdPrestatGet(mod, fd, resultPrestat)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionFdPrestatDirName, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		ret, err := fn.Call(mod, uint64(fd), uint64(resultPrestat))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_FdPrestatGet_Errors(t *testing.T) {
	fd := uint32(3)           // fd 3 will be opened for the "/tmp" directory after 0, 1, and 2, that are stdin/out/err
	validAddress := uint32(0) // Arbitrary valid address as arguments to fd_prestat_get. We chose 0 here.

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{fd: {Path: "/tmp"}})
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionFdPrestatGet, ImportFdPrestatGet, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()

	tests := []struct {
		name          string
		fd            uint32
		resultPrestat uint32
		expectedErrno wasi.Errno
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid FD
			resultPrestat: validAddress,
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory resultPrestat",
			fd:            fd,
			resultPrestat: memorySize,
			expectedErrno: wasi.ErrnoFault,
		},
		// TODO: non pre-opened file == wasi.ErrnoBadf
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			errno := a.FdPrestatGet(mod, tc.fd, tc.resultPrestat)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

func TestSnapshotPreview1_FdPrestatDirName(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{fd: {Path: "/tmp"}})
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, sys)
	defer mod.Close()

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(3) // shorter than len("/tmp") to test the path is written for the length of pathLen
	expectedMemory := []byte{
		'?',
		'/', 't', 'm',
		'?', '?', '?',
	}

	t.Run("SnapshotPreview1.FdPrestatDirName", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		errno := a.FdPrestatDirName(mod, fd, path, pathLen)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionFdPrestatDirName, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		ret, err := fn.Call(mod, uint64(fd), uint64(path), uint64(pathLen))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_FdPrestatDirName_Errors(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{fd: {Path: "/tmp"}})
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()
	validAddress := uint32(0) // Arbitrary valid address as arguments to fd_prestat_dir_name. We chose 0 here.
	pathLen := uint32(len("/tmp"))

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
			pathLen:       pathLen,
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            fd,
			path:          memorySize - pathLen + 1,
			pathLen:       pathLen,
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "pathLen exceeds the length of the dir name",
			fd:            fd,
			path:          validAddress,
			pathLen:       pathLen + 1,
			expectedErrno: wasi.ErrnoNametoolong,
		},
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: wasi.ErrnoBadf,
		},
		// TODO: non pre-opened file == wasi.ErrnoBadf
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			errno := a.FdPrestatDirName(mod, tc.fd, tc.path, tc.pathLen)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

// TestSnapshotPreview1_FdPwrite only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdPwrite(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdPwrite, ImportFdPwrite, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdPwrite", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdPwrite(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionFdPwrite, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdRead(t *testing.T) {
	ctx := context.Background()

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

	// TestSnapshotPreview1_FdRead uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdReadFn func(ctx publicwasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno
	tests := []struct {
		name   string
		fdRead func(*wasiAPI, *wasm.ModuleContext, publicwasm.Function) fdReadFn
	}{
		{"SnapshotPreview1.FdRead", func(a *wasiAPI, _ *wasm.ModuleContext, _ publicwasm.Function) fdReadFn {
			return a.FdRead
		}},
		{FunctionFdRead, func(_ *wasiAPI, mod *wasm.ModuleContext, fn publicwasm.Function) fdReadFn {
			return func(ctx publicwasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno {
				ret, err := fn.Call(mod, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultSize))
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
			sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
				fd: {Path: "test_path", FS: memFS, File: file},
			})
			require.NoError(t, err)

			a, mod, fn := instantiateModule(t, ctx, FunctionFdRead, ImportFdRead, moduleName, sys)
			defer mod.Close()

			maskMemory(t, mod, len(expectedMemory))

			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.fdRead(a, mod, fn)(mod, fd, iovs, iovsCount, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
		})
	}
}

func TestSnapshotPreview1_FdRead_Errors(t *testing.T) {
	validFD := uint32(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
		validFD: {Path: "test_path", FS: memFS, File: file},
	})
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionFdRead, ImportFdRead, moduleName, sys)
	defer mod.Close()

	tests := []struct {
		name                            string
		fd, iovs, iovsCount, resultSize uint32
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
				0, 0, 0x1, 0, // = iovs[0].offset on the second page
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
				0, 0, 0x1, 0, // = iovs[0].length on the second page
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
			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(offset, tc.memory)
			require.True(t, memoryWriteOK)

			errno := a.FdRead(mod, tc.fd, tc.iovs+offset, tc.iovsCount+offset, tc.resultSize+offset)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

// TestSnapshotPreview1_FdReaddir only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdReaddir(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdReaddir, ImportFdReaddir, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdReaddir", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdReaddir(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionFdReaddir, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdRenumber only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdRenumber(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdRenumber, ImportFdRenumber, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdRenumber", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdRenumber(mod, 0, 0))
	})

	t.Run(FunctionFdRenumber, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdSeek(t *testing.T) {
	fd := uint32(3)                                             // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	resultNewoffset := uint32(1)                                // arbitrary offset in `ctx.Memory` for the new offset value
	file, memFS := createFile(t, "test_path", []byte("wazero")) // arbitrary non-empty contents

	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
		fd: {Path: "test_path", FS: memFS, File: file},
	})
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdSeek, ImportFdSeek, moduleName, sys)
	defer mod.Close()

	// TestSnapshotPreview1_FdSeek uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdSeekFn func(ctx publicwasm.Module, fd uint32, offset uint64, whence, resultNewOffset uint32) wasi.Errno
	seekFns := []struct {
		name   string
		fdSeek func() fdSeekFn
	}{
		{"SnapshotPreview1.FdSeek", func() fdSeekFn {
			return a.FdSeek
		}},
		{FunctionFdSeek, func() fdSeekFn {
			return func(ctx publicwasm.Module, fd uint32, offset uint64, whence, resultNewoffset uint32) wasi.Errno {
				ret, err := fn.Call(mod, uint64(fd), offset, uint64(whence), uint64(resultNewoffset))
				require.NoError(t, err)
				return wasi.Errno(ret[0])
			}
		}},
	}

	tests := []struct {
		name           string
		offset         int64
		whence         int
		expectedOffset int64
		expectedMemory []byte
	}{
		{
			name:           "SeekStart",
			offset:         4, // arbitrary offset
			whence:         io.SeekStart,
			expectedOffset: 4, // = offset
			expectedMemory: []byte{
				'?',        // resultNewoffset is after this
				4, 0, 0, 0, // = expectedOffset
				'?',
			},
		},
		{
			name:           "SeekCurrent",
			offset:         1, // arbitrary offset
			whence:         io.SeekCurrent,
			expectedOffset: 2, // = 1 (the initial offset of the test file) + 1 (offset)
			expectedMemory: []byte{
				'?',        // resultNewoffset is after this
				2, 0, 0, 0, // = expectedOffset
				'?',
			},
		},
		{
			name:           "SeekEnd",
			offset:         -1, // arbitrary offset, note that offset can be negative
			whence:         io.SeekEnd,
			expectedOffset: 5, // = 6 (the size of the test file with content "wazero") + -1 (offset)
			expectedMemory: []byte{
				'?',        // resultNewoffset is after this
				5, 0, 0, 0, // = expectedOffset
				'?',
			},
		},
	}

	for _, seekFn := range seekFns {
		sf := seekFn
		t.Run(sf.name, func(t *testing.T) {
			for _, tt := range tests {
				tc := tt
				t.Run(tc.name, func(t *testing.T) {
					maskMemory(t, mod, len(tc.expectedMemory))
					file.offset = 1 // set the initial offset of the file to 1

					errno := sf.fdSeek()(mod, fd, uint64(tc.offset), uint32(tc.whence), resultNewoffset)
					require.Equal(t, wasi.ErrnoSuccess, errno)

					actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
					require.True(t, ok)
					require.Equal(t, tc.expectedMemory, actual)

					require.Equal(t, tc.expectedOffset, file.offset) // test that the offset of file is actually updated.
				})
			}
		})
	}
}

func TestSnapshotPreview1_FdSeek_Errors(t *testing.T) {
	validFD := uint32(3)                                        // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte("wazero")) // arbitrary valid file with non-empty contents
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
		validFD: {Path: "test_path", FS: memFS, File: file},
	})
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionFdSeek, ImportFdSeek, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()

	tests := []struct {
		name                    string
		fd                      uint32
		offset                  uint64
		whence, resultNewoffset uint32
		expectedErrno           wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "invalid whence",
			fd:            validFD,
			whence:        3, // invalid whence, the largest whence io.SeekEnd(2) + 1
			expectedErrno: wasi.ErrnoInval,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              validFD,
			resultNewoffset: memorySize,
			expectedErrno:   wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			errno := a.FdSeek(mod, tc.fd, tc.offset, tc.whence, tc.resultNewoffset)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}

}

// TestSnapshotPreview1_FdSync only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdSync(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdSync, ImportFdSync, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdSync", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdSync(mod, 0))
	})

	t.Run(FunctionFdSync, func(t *testing.T) {
		results, err := fn.Call(mod, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_FdTell only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdTell(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionFdTell, ImportFdTell, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.FdTell", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.FdTell(mod, 0, 0))
	})

	t.Run(FunctionFdTell, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdWrite(t *testing.T) {
	ctx := context.Background()

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

	// TestSnapshotPreview1_FdWrite uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdWriteFn func(ctx publicwasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno
	tests := []struct {
		name    string
		fdWrite func(*wasiAPI, *wasm.ModuleContext, publicwasm.Function) fdWriteFn
	}{
		{"SnapshotPreview1.FdWrite", func(a *wasiAPI, _ *wasm.ModuleContext, _ publicwasm.Function) fdWriteFn {
			return a.FdWrite
		}},
		{FunctionFdWrite, func(_ *wasiAPI, mod *wasm.ModuleContext, fn publicwasm.Function) fdWriteFn {
			return func(ctx publicwasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno {
				ret, err := fn.Call(mod, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultSize))
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
			sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
				fd: {Path: "test_path", FS: memFS, File: file},
			})
			require.NoError(t, err)

			a, mod, fn := instantiateModule(t, ctx, FunctionFdWrite, ImportFdWrite, moduleName, sys)
			defer mod.Close()

			maskMemory(t, mod, len(expectedMemory))
			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.fdWrite(a, mod, fn)(mod, fd, iovs, iovsCount, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
			require.Equal(t, []byte("wazero"), file.buf) // verify the file was actually written
		})
	}
}

func TestSnapshotPreview1_FdWrite_Errors(t *testing.T) {
	validFD := uint32(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
		validFD: {Path: "test_path", FS: memFS, File: file},
	})
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionFdWrite, ImportFdWrite, moduleName, sys)
	defer mod.Close()

	// Setup valid test memory
	iovs, iovsCount := uint32(0), uint32(1)
	memory := []byte{
		8, 0, 0, 0, // = iovs[0].offset (where the data "hi" begins)
		2, 0, 0, 0, // = iovs[0].length (how many bytes are in "hi")
		'h', 'i', // iovs[0].length bytes
	}

	tests := []struct {
		name           string
		fd, resultSize uint32
		memory         []byte
		expectedErrno  wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            validFD,
			memory:        []byte{},
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "out-of-memory reading iovs[0].length",
			fd:            validFD,
			memory:        memory[0:4], // iovs[0].offset was 4 bytes and iovs[0].length next, but not enough mod.Memory()!
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "iovs[0].offset is outside memory",
			fd:            validFD,
			memory:        memory[0:8], // iovs[0].offset (where to read "hi") is outside memory.
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "length to read exceeds memory by 1",
			fd:            validFD,
			memory:        memory[0:9], // iovs[0].offset (where to read "hi") is in memory, but truncated.
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "resultSize offset is outside memory",
			fd:            validFD,
			memory:        memory,
			resultSize:    uint32(len(memory)), // read was ok, but there wasn't enough memory to write the result.
			expectedErrno: wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			mod.Memory().(*wasm.MemoryInstance).Buffer = tc.memory

			errno := a.FdWrite(mod, tc.fd, iovs, iovsCount, tc.resultSize)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

func createFile(t *testing.T, path string, contents []byte) (*memFile, *MemFS) {
	memFS := &MemFS{}
	f, err := memFS.OpenWASI(0, path, wasi.O_CREATE|wasi.O_TRUNC, wasi.R_FD_WRITE, 0, 0)
	require.NoError(t, err)

	mf := f.(*memFile)
	mf.buf = append([]byte{}, contents...)

	return mf, memFS
}

// TestSnapshotPreview1_PathCreateDirectory only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathCreateDirectory(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathCreateDirectory, ImportPathCreateDirectory, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathCreateDirectory", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathCreateDirectory(mod, 0, 0, 0))
	})

	t.Run(FunctionPathCreateDirectory, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathFilestatGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathFilestatGet(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathFilestatGet, ImportPathFilestatGet, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathFilestatGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathFilestatGet(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathFilestatGet, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathFilestatSetTimes only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathFilestatSetTimes(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathFilestatSetTimes, ImportPathFilestatSetTimes, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathFilestatSetTimes", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathFilestatSetTimes(mod, 0, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathFilestatSetTimes, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathLink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathLink(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathLink, ImportPathLink, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathLink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathLink(mod, 0, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathLink, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_PathOpen(t *testing.T) {
	workdirFD := uint32(3)                    // arbitrary fd after 0, 1, and 2, that are stdin/out/err
	dirflags := uint32(0)                     // arbitrary dirflags
	path := uint32(1)                         // arbitrary offset
	pathLen := uint32(6)                      // The length of path
	oflags := uint32(0)                       // arbitrary oflags
	fsRightsBase := uint64(rightFDRead)       // arbitrary right
	fsRightsInheriting := uint64(rightFDRead) // arbitrary right
	fdFlags := uint32(0)
	resultOpenedFd := uint32(8)
	initialMemory := []byte{
		'?',                          // `path` is after this
		'w', 'a', 'z', 'e', 'r', 'o', // path
	}
	expectedFD := byte(workdirFD + 1)
	expectedMemory := append(
		initialMemory,
		'?', // `resultOpenedFd` is after this
		expectedFD, 0, 0, 0,
		'?',
	)

	ctx := context.Background()

	// TestSnapshotPreview1_PathOpen uses a matrix because setting up test files is complicated and has to be clean each time.
	type pathOpenFn func(ctx publicwasm.Module, fd, dirflags, path, pathLen, oflags uint32,
		fsRightsBase, fsRightsInheriting uint64,
		fdFlags, resultOpenedFd uint32) wasi.Errno
	pathOpenFns := []struct {
		name     string
		pathOpen func(*wasiAPI, *wasm.ModuleContext, publicwasm.Function) pathOpenFn
	}{
		{"SnapshotPreview1.PathOpen", func(a *wasiAPI, _ *wasm.ModuleContext, _ publicwasm.Function) pathOpenFn {
			return a.PathOpen
		}},
		{FunctionPathOpen, func(_ *wasiAPI, mod *wasm.ModuleContext, fn publicwasm.Function) pathOpenFn {
			return func(ctx publicwasm.Module, fd, dirflags, path, pathLen, oflags uint32,
				fsRightsBase, fsRightsInheriting uint64,
				fdFlags, resultOpenedFd uint32) wasi.Errno {
				ret, err := fn.Call(mod, uint64(fd), uint64(dirflags), uint64(path), uint64(pathLen), uint64(oflags), uint64(fsRightsBase), uint64(fsRightsInheriting), uint64(fdFlags), uint64(resultOpenedFd))
				require.NoError(t, err)
				return wasi.Errno(ret[0])
			}
		}},
	}

	tests := []struct {
		name         string
		fd           uint32
		expectedPath string
	}{
		{
			name:         "simple file open",
			fd:           workdirFD,
			expectedPath: "wazero",
		},
	}

	for _, pathOpenFn := range pathOpenFns {
		pf := pathOpenFn
		t.Run(pf.name, func(t *testing.T) {
			for _, tt := range tests {
				tc := tt
				t.Run(tc.name, func(t *testing.T) {
					// Create a memFS for testing that has "./wazero" file.
					memFS := &MemFS{Files: map[string][]byte{"wazero": {}}}
					sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
						workdirFD: {Path: ".", FS: memFS},
					})
					require.NoError(t, err)

					a, mod, fn := instantiateModule(t, ctx, FunctionPathOpen, ImportPathOpen, moduleName, sys)
					defer mod.Close()

					maskMemory(t, mod, len(expectedMemory))
					ok := mod.Memory().Write(0, initialMemory)
					require.True(t, ok)

					errno := pf.pathOpen(a, mod, fn)(mod, tc.fd, dirflags, path, pathLen, oflags, fsRightsBase, fsRightsInheriting, fdFlags, resultOpenedFd)
					require.Equal(t, wasi.ErrnoSuccess, errno)

					actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
					require.True(t, ok)
					require.Equal(t, expectedMemory, actual)

					// verify the file was actually opened
					f, ok := sys.OpenedFile(uint32(expectedFD))
					require.True(t, ok)
					require.Equal(t, tc.expectedPath, f.Path)
				})
			}
		})
	}
}

func TestSnapshotPreview1_PathOpen_Errors(t *testing.T) {
	validFD := uint32(3) // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	// Create a memFS for testing that has "./wazero" file.
	memFS := &MemFS{
		Files: map[string][]byte{
			"wazero": []byte(""),
		},
	}
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, map[uint32]*wasm.FileEntry{
		validFD: {Path: ".", FS: memFS},
	})
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionPathOpen, ImportPathOpen, moduleName, sys)
	defer mod.Close()

	validPath := uint32(0)    // arbitrary offset
	validPathLen := uint32(6) // the length of "wazero"
	mod.Memory().Write(validPath, []byte{
		'w', 'a', 'z', 'e', 'r', 'o', // write to offset 0 (= validPath)
	}) // wazero is the path to the file in the memFS

	tests := []struct {
		name                                      string
		fd, path, pathLen, oflags, resultOpenedFd uint32
		expectedErrno                             wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory reading path",
			fd:            validFD,
			path:          mod.Memory().Size(),
			pathLen:       validPathLen,
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            validFD,
			path:          validPath,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is out-of-memory for path
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "no such file exists",
			fd:            validFD,
			path:          validPath,
			pathLen:       validPathLen - 1, // this make the path "wazer", which doesn't exit
			expectedErrno: wasi.ErrnoNoent,
		},
		{
			name:           "out-of-memory writing resultOpenedFd",
			fd:             validFD,
			path:           validPath,
			pathLen:        validPathLen,
			resultOpenedFd: mod.Memory().Size(), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			errno := a.PathOpen(mod, tc.fd, 0, tc.path, tc.pathLen, tc.oflags, 0, 0, 0, tc.resultOpenedFd)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

// TestSnapshotPreview1_PathReadlink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathReadlink(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathReadlink, ImportPathReadlink, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathLink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathReadlink(mod, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathReadlink, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathRemoveDirectory only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathRemoveDirectory(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathRemoveDirectory, ImportPathRemoveDirectory, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathRemoveDirectory", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathRemoveDirectory(mod, 0, 0, 0))
	})

	t.Run(FunctionPathRemoveDirectory, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathRename only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathRename(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathRename, ImportPathRename, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathRename", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathRename(mod, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathRename, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathSymlink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathSymlink(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathSymlink, ImportPathSymlink, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathSymlink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathSymlink(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathSymlink, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PathUnlinkFile only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathUnlinkFile(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPathUnlinkFile, ImportPathUnlinkFile, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PathUnlinkFile", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PathUnlinkFile(mod, 0, 0, 0))
	})

	t.Run(FunctionPathUnlinkFile, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_PollOneoff only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PollOneoff(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionPollOneoff, ImportPollOneoff, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.PollOneoff", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.PollOneoff(mod, 0, 0, 0, 0))
	})

	t.Run(FunctionPollOneoff, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_ProcExit(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

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

	// Note: Unlike most tests, this uses fn, not the 'a' result parameter. This is because currently, this function
	// body panics, and we expect Call to unwrap the panic.
	_, mod, fn := instantiateModule(t, ctx, FunctionProcExit, ImportProcExit, moduleName, sys)
	defer mod.Close()

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// When ProcExit is called, store.CallFunction returns immediately, returning the exit code as the error.
			_, err = fn.Call(mod, uint64(tc.exitCode))
			var code wasi.ExitCode
			require.ErrorAs(t, err, &code)
			require.Equal(t, code, wasi.ExitCode(tc.exitCode))
		})
	}
}

// TestSnapshotPreview1_ProcRaise only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_ProcRaise(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionProcRaise, ImportProcRaise, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.ProcRaise", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.ProcRaise(mod, 0))
	})

	t.Run(FunctionProcRaise, func(t *testing.T) {
		results, err := fn.Call(mod, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_SchedYield only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SchedYield(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionSchedYield, ImportSchedYield, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.SchedYield", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.SchedYield(mod))
	})

	t.Run(FunctionSchedYield, func(t *testing.T) {
		results, err := fn.Call(mod)
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

	length := uint32(5) // arbitrary length,
	offset := uint32(1) // offset,
	seed := int64(42)   // and seed value
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionRandomGet, ImportRandomGet, moduleName, sys)
	defer mod.Close()

	a.randSource = func(p []byte) error {
		s := rand.NewSource(seed)
		rng := rand.New(s)
		_, err := rng.Read(p)

		return err
	}

	t.Run("SnapshotPreview1.RandomGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke RandomGet directly and check the memory side effects!
		errno := a.RandomGet(mod, offset, length)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		actual, ok := mod.Memory().Read(0, offset+length+1)
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})

	t.Run(FunctionRandomGet, func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		results, err := fn.Call(mod, uint64(offset), uint64(length))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(results[0])) // cast because results are always uint64

		actual, ok := mod.Memory().Read(0, offset+length+1)
		require.True(t, ok)
		require.Equal(t, expectedMemory, actual)
	})
}

func TestSnapshotPreview1_RandomGet_Errors(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	validAddress := uint32(0) // arbitrary valid address

	a, mod, _ := instantiateModule(t, ctx, FunctionRandomGet, ImportRandomGet, moduleName, sys)
	defer mod.Close()

	memorySize := mod.Memory().Size()

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
			errno := a.RandomGet(mod, tc.offset, tc.length)
			require.Equal(t, wasi.ErrnoFault, errno)
		})
	}
}

func TestSnapshotPreview1_RandomGet_SourceError(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, _ := instantiateModule(t, ctx, FunctionRandomGet, ImportRandomGet, moduleName, sys)
	defer mod.Close()

	a.randSource = func(p []byte) error {
		return errors.New("random source error")
	}

	errno := a.RandomGet(mod, uint32(1), uint32(5)) // arbitrary offset and length
	require.Equal(t, wasi.ErrnoIo, errno)
}

// TestSnapshotPreview1_SockRecv only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockRecv(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionSockRecv, ImportSockRecv, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.SockRecv", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.SockRecv(mod, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionSockRecv, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_SockSend only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockSend(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionSockSend, ImportSockSend, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.SockSend", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.SockSend(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionSockSend, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TestSnapshotPreview1_SockShutdown only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockShutdown(t *testing.T) {
	ctx := context.Background()
	sys, err := newSysContext(nil, nil, nil)
	require.NoError(t, err)

	a, mod, fn := instantiateModule(t, ctx, FunctionSockShutdown, ImportSockShutdown, moduleName, sys)
	defer mod.Close()

	t.Run("SnapshotPreview1.SockShutdown", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, a.SockShutdown(mod, 0, 0))
	})

	t.Run(FunctionSockShutdown, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

const testMemoryPageSize = 1

func instantiateModule(t *testing.T, ctx context.Context, wasiFunction, wasiImport, moduleName string, sys *wasm.SysContext) (*wasiAPI, *wasm.ModuleContext, publicwasm.Function) {
	enabledFeatures := wasm.Features20191205
	store := wasm.NewStore(interpreter.NewEngine(), enabledFeatures)

	// The package `wazero` has a simpler interface for adding host modules, but we can't use that as it would create an
	// import cycle. Instead, we export internalwasm.NewHostModule and use it here.
	a, fns := SnapshotPreview1Functions()
	m, err := wasm.NewHostModule(wasi.ModuleSnapshotPreview1, fns)
	require.NoError(t, err)

	// Double-check what we created passes same validity as module-defined modules.
	require.NoError(t, m.Validate(enabledFeatures))

	_, err = store.Instantiate(ctx, m, m.NameSection.ModuleName, nil) // TODO: close
	require.NoError(t, err)

	m, err = text.DecodeModule([]byte(fmt.Sprintf(`(module
  %[2]s
  (memory 1)  ;; just an arbitrary size big enough for tests
  (export "memory" (memory 0))
  (export "%[1]s" (func $wasi.%[1]s))
)`, wasiFunction, wasiImport)), enabledFeatures)
	require.NoError(t, err)

	mod, err := store.Instantiate(ctx, m, moduleName, sys)
	require.NoError(t, err)

	fn := mod.ExportedFunction(wasiFunction)
	require.NotNil(t, fn)
	return a, mod, fn
}

// maskMemory sets the first memory in the store to '?' * size, so tests can see what's written.
func maskMemory(t *testing.T, mod publicwasm.Module, size int) {
	for i := uint32(0); i < uint32(size); i++ {
		require.True(t, mod.Memory().WriteByte(i, '?'))
	}
}

func newSysContext(args, environ []string, openedFiles map[uint32]*wasm.FileEntry) (sys *wasm.SysContext, err error) {
	return wasm.NewSysContext(math.MaxUint32, args, environ, new(bytes.Buffer), nil, nil, openedFiles)
}
