package internalwasi

import (
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

func TestConfig_Args(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		config := NewConfig()
		err := config.Args("a", "bc")
		require.NoError(t, err)

		require.Equal(t, &nullTerminatedStrings{
			nullTerminatedValues: [][]byte{
				{'a', 0},
				{'b', 'c', 0},
			},
			totalBufSize: 5,
		}, config.args)
	})
	t.Run("error constructing args", func(t *testing.T) {
		err := NewConfig().Args("\xff\xfe\xfd", "foo", "bar")
		require.EqualError(t, err, "arg[0] is not a valid UTF-8 string")
	})
}

func TestWASIAPI_Config(t *testing.T) {
	t.Run("default when context empty", func(t *testing.T) {
		config := NewConfig()
		api := NewAPI(config)
		require.Same(t, config, api.config(context.Background()))
	})

	t.Run("overrides default", func(t *testing.T) {
		config := NewConfig()
		api := NewAPI(config)

		anotherConfig := NewConfig()
		require.NotSame(t, anotherConfig, config)

		overridingCtx := context.WithValue(context.Background(), ConfigContextKey{}, anotherConfig)
		require.Same(t, anotherConfig, api.config(overridingCtx))
	})
}

func TestSnapshotPreview1_ArgsGet(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()
	err := config.Args("a", "bc")
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

	mod, fn := instantiateModule(t, ctx, FunctionArgsGet, ImportArgsGet, moduleName, config)

	t.Run("SnapshotPreview1.ArgsGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke ArgsGet directly and check the memory side effects.
		errno := NewAPI(config).ArgsGet(mod, argv, argvBuf)
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
	config := NewConfig()
	err := config.Args("a", "bc")
	require.NoError(t, err)
	mod, fn := instantiateModule(t, ctx, FunctionArgsGet, ImportArgsGet, moduleName, config)

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
			results, err := fn.Call(mod, uint64(tc.argv), uint64(tc.argvBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_ArgsSizesGet(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()
	err := config.Args("a", "bc")
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

	mod, fn := instantiateModule(t, ctx, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, config)

	t.Run("SnapshotPreview1.ArgsSizesGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke ArgsSizesGet directly and check the memory side effects.
		errno := NewAPI(config).ArgsSizesGet(mod, resultArgc, resultArgvBufSize)
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
	config := NewConfig()
	err := config.Args("a", "bc")
	require.NoError(t, err)

	mod, fn := instantiateModule(t, ctx, FunctionArgsSizesGet, ImportArgsSizesGet, moduleName, config)

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
			results, err := fn.Call(mod, uint64(tc.argc), uint64(tc.argvBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestConfig_Environ(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		config := NewConfig()
		err := config.Environ("a=b", "b=cd")
		require.NoError(t, err)

		require.Equal(t, &nullTerminatedStrings{
			nullTerminatedValues: [][]byte{
				{'a', '=', 'b', 0},
				{'b', '=', 'c', 'd', 0},
			},
			totalBufSize: 9,
		}, config.environ)
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
			err := NewConfig().Environ(tc.environ)
			require.EqualError(t, err, tc.errorMessage)
		})
	}
}

func TestSnapshotPreview1_EnvironGet(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()
	err := config.Environ("a=b", "b=cd")
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

	mod, fn := instantiateModule(t, ctx, FunctionEnvironGet, ImportEnvironGet, moduleName, config)

	t.Run("SnapshotPreview1.EnvironGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke EnvironGet directly and check the memory side effects.
		errno := NewAPI(config).EnvironGet(mod, resultEnviron, resultEnvironBuf)
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
	config := NewConfig()
	err := config.Environ("a=bc", "b=cd")
	require.NoError(t, err)

	mod, fn := instantiateModule(t, ctx, FunctionEnvironGet, ImportEnvironGet, moduleName, config)

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
			results, err := fn.Call(mod, uint64(tc.environ), uint64(tc.environBuf))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_EnvironSizesGet(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()
	err := config.Environ("a=b", "b=cd")
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

	mod, fn := instantiateModule(t, ctx, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, config)

	t.Run("SnapshotPreview1.EnvironSizesGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke EnvironSizesGet directly and check the memory side effects.
		errno := NewAPI(config).EnvironSizesGet(mod, resultEnvironc, resultEnvironBufSize)
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
	config := NewConfig()
	err := config.Environ("a=b", "b=cd")
	require.NoError(t, err)

	mod, fn := instantiateModule(t, ctx, FunctionEnvironSizesGet, ImportEnvironSizesGet, moduleName, config)

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
			results, err := fn.Call(mod, uint64(tc.environc), uint64(tc.environBufSize))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_ClockResGet only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_ClockResGet(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionClockResGet, ImportClockResGet, moduleName, config)

	t.Run("SnapshotPreview1.ClockResGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).ClockResGet(mod, 0, 0))
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
	config := NewConfig()
	config.timeNowUnixNano = func() uint64 { return epochNanos }

	mod, fn := instantiateModule(t, ctx, FunctionClockTimeGet, ImportClockTimeGet, moduleName, config)

	t.Run("SnapshotPreview1.ClockTimeGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// invoke ClockTimeGet directly and check the memory side effects!
		errno := NewAPI(config).ClockTimeGet(mod, 0 /* TODO: id */, 0 /* TODO: precision */, resultTimestamp)
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
	config := NewConfig()
	config.timeNowUnixNano = func() uint64 { return epochNanos }

	mod, fn := instantiateModule(t, ctx, FunctionClockTimeGet, ImportClockTimeGet, moduleName, config)

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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdAdvise, ImportFdAdvise, moduleName, config)

	t.Run("SnapshotPreview1.FdAdvise", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdAdvise(mod, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdAllocate, ImportFdAllocate, moduleName, config)

	t.Run("SnapshotPreview1.FdAllocate", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdAllocate(mod, 0, 0, 0))
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
		config := NewConfig()

		memFs := &MemFS{}
		config.opened = map[uint32]fileEntry{
			fdToClose: {
				path:    "/tmp",
				fileSys: memFs,
			},
			fdToKeep: {
				path:    "path to keep",
				fileSys: memFs,
			},
		}

		mod, fn := instantiateModule(t, ctx, FunctionFdClose, ImportFdClose, moduleName, config)
		return mod, fn, NewAPI(config)
	}

	t.Run("SnapshotPreview1.FdClose", func(t *testing.T) {
		mod, _, api := setupFD()

		errno := api.FdClose(mod, fdToClose)
		require.Equal(t, wasi.ErrnoSuccess, errno)
		require.NotContains(t, api.cfg.opened, fdToClose) // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.cfg.opened, fdToKeep)
	})
	t.Run(FunctionFdClose, func(t *testing.T) {
		mod, fn, api := setupFD()

		ret, err := fn.Call(mod, uint64(fdToClose))
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoSuccess, wasi.Errno(ret[0])) // cast because results are always uint64
		require.NotContains(t, api.cfg.opened, fdToClose)       // Fd is closed and removed from the opened FDs.
		require.Contains(t, api.cfg.opened, fdToKeep)
	})
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		mod, _, api := setupFD()

		errno := api.FdClose(mod, 42) // 42 is an arbitrary invalid FD
		require.Equal(t, wasi.ErrnoBadf, errno)
	})
}

// TestSnapshotPreview1_FdDatasync only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdDatasync(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdDatasync, ImportFdDatasync, moduleName, config)

	t.Run("SnapshotPreview1.FdDatasync", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdDatasync(mod, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdFdstatSetFlags, ImportFdFdstatSetFlags, moduleName, config)

	t.Run("SnapshotPreview1.FdFdstatSetFlags", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdFdstatSetFlags(mod, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdFdstatSetRights, ImportFdFdstatSetRights, moduleName, config)

	t.Run("SnapshotPreview1.FdFdstatSetRights", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdFdstatSetRights(mod, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdFilestatGet, ImportFdFilestatGet, moduleName, config)

	t.Run("SnapshotPreview1.FdFilestatGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdFilestatGet(mod, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdFilestatSetSize, ImportFdFilestatSetSize, moduleName, config)

	t.Run("SnapshotPreview1.FdFilestatSetSize", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdFilestatSetSize(mod, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdFilestatSetTimes, ImportFdFilestatSetTimes, moduleName, config)

	t.Run("SnapshotPreview1.FdFilestatSetTimes", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdFilestatSetTimes(mod, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdPread, ImportFdPread, moduleName, config)

	t.Run("SnapshotPreview1.FdPread", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdPread(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionFdPread, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

// TODO: TestSnapshotPreview1_FdPrestatGet TestSnapshotPreview1_FdPrestatGet_Errors

func TestSnapshotPreview1_FdPrestatDirName(t *testing.T) {
	fd := uint32(3) // arbitrary fd after 0, 1, and 2, that are stdin/out/err

	ctx := context.Background()
	config := NewConfig()
	config.opened[fd] = fileEntry{
		path:    "/tmp",
		fileSys: &MemFS{},
	}

	mod, fn := instantiateModule(t, ctx, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, config)

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(3) // shorter than len("/tmp") to test the path is written for the length of pathLen
	expectedMemory := []byte{
		'?',
		'/', 't', 'm',
		'?', '?', '?',
	}

	t.Run("SnapshotPreview1.FdPrestatDirName", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		errno := NewAPI(config).FdPrestatDirName(mod, fd, path, pathLen)
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
	dirName := "/tmp"
	ctx := context.Background()
	config := NewConfig()
	config.Preopen(dirName, &MemFS{})

	mod, fn := instantiateModule(t, ctx, FunctionFdPrestatDirName, ImportFdPrestatDirName, moduleName, config)

	memorySize := mod.Memory().Size()
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
			results, err := fn.Call(mod, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_FdPwrite only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdPwrite(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdPwrite, ImportFdPwrite, moduleName, config)

	t.Run("SnapshotPreview1.FdPwrite", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdPwrite(mod, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionFdPwrite, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdRead(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

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

	mod, fn := instantiateModule(t, ctx, FunctionFdRead, ImportFdRead, moduleName, config)

	// TestSnapshotPreview1_FdRead uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdReadFn func(ctx publicwasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno
	tests := []struct {
		name   string
		fdRead func() fdReadFn
	}{
		{"SnapshotPreview1.FdRead", func() fdReadFn {
			return NewAPI(config).FdRead
		}},
		{FunctionFdRead, func() fdReadFn {
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
			config.opened[fd] = fileEntry{
				path:    "test_path",
				fileSys: memFS,
				file:    file,
			}
			maskMemory(t, mod, len(expectedMemory))

			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.fdRead()(mod, fd, iovs, iovsCount, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
		})
	}
}

func TestSnapshotPreview1_FdRead_Errors(t *testing.T) {
	validFD := uint64(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents

	ctx := context.Background()
	config := NewConfig()
	config.opened[uint32(validFD)] = fileEntry{
		path:    "test_path",
		fileSys: memFS,
		file:    file,
	}

	mod, fn := instantiateModule(t, ctx, FunctionFdRead, ImportFdRead, moduleName, config)

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
			offset := wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory))

			memoryWriteOK := mod.Memory().Write(uint32(offset), tc.memory)
			require.True(t, memoryWriteOK)

			results, err := fn.Call(mod, tc.fd, tc.iovs+offset, tc.iovsCount+offset, tc.resultSize+offset)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_FdReaddir only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdReaddir(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdReaddir, ImportFdReaddir, moduleName, config)

	t.Run("SnapshotPreview1.FdReaddir", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdReaddir(mod, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdRenumber, ImportFdRenumber, moduleName, config)

	t.Run("SnapshotPreview1.FdRenumber", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdRenumber(mod, 0, 0))
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
	config := NewConfig()
	config.opened[fd] = fileEntry{
		path:    "test_path",
		fileSys: memFS,
		file:    file,
	}

	mod, fn := instantiateModule(t, ctx, FunctionFdSeek, ImportFdSeek, moduleName, config)

	// TestSnapshotPreview1_FdSeek uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdSeekFn func(ctx publicwasm.Module, fd uint32, offset uint64, whence, resultNewOffset uint32) wasi.Errno
	seekFns := []struct {
		name   string
		fdSeek func() fdSeekFn
	}{
		{"SnapshotPreview1.FdSeek", func() fdSeekFn {
			return NewAPI(config).FdSeek
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
	validFD := uint64(3)                                        // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte("wazero")) // arbitrary valid file with non-empty contents
	ctx := context.Background()
	config := NewConfig()
	config.opened[uint32(validFD)] = fileEntry{
		path:    "test_path",
		fileSys: memFS,
		file:    file,
	}

	mod, fn := instantiateModule(t, ctx, FunctionFdSeek, ImportFdSeek, moduleName, config)
	memorySize := mod.Memory().Size()

	tests := []struct {
		name                                string
		fd, offset, whence, resultNewoffset uint64
		expectedErrno                       wasi.Errno
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
			resultNewoffset: uint64(memorySize),
			expectedErrno:   wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(mod, tc.fd, tc.offset, tc.whence, tc.resultNewoffset)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}

}

// TestSnapshotPreview1_FdSync only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_FdSync(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdSync, ImportFdSync, moduleName, config)

	t.Run("SnapshotPreview1.FdSync", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdSync(mod, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionFdTell, ImportFdTell, moduleName, config)

	t.Run("SnapshotPreview1.FdTell", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).FdTell(mod, 0, 0))
	})

	t.Run(FunctionFdTell, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_FdWrite(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

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

	mod, fn := instantiateModule(t, ctx, FunctionFdWrite, ImportFdWrite, moduleName, config)

	// TestSnapshotPreview1_FdWrite uses a matrix because setting up test files is complicated and has to be clean each time.
	type fdWriteFn func(ctx publicwasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno
	tests := []struct {
		name    string
		fdWrite func() fdWriteFn
	}{
		{"SnapshotPreview1.FdWrite", func() fdWriteFn {
			return NewAPI(config).FdWrite
		}},
		{FunctionFdWrite, func() fdWriteFn {
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
			config.opened[fd] = fileEntry{
				path:    "test_path",
				fileSys: memFS,
				file:    file,
			}
			maskMemory(t, mod, len(expectedMemory))
			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.fdWrite()(mod, fd, iovs, iovsCount, resultSize)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
			require.Equal(t, []byte("wazero"), file.buf) // verify the file was actually written
		})
	}
}

func TestSnapshotPreview1_FdWrite_Errors(t *testing.T) {
	validFD := uint64(3)                                // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	file, memFS := createFile(t, "test_path", []byte{}) // file with empty contents
	ctx := context.Background()
	config := NewConfig()
	config.opened[uint32(validFD)] = fileEntry{
		path:    "test_path",
		fileSys: memFS,
		file:    file,
	}

	mod, fn := instantiateModule(t, ctx, FunctionFdWrite, ImportFdWrite, moduleName, config)

	// Setup valid test memory
	iovs, iovsCount := uint64(0), uint64(1)
	memory := []byte{
		8, 0, 0, 0, // = iovs[0].offset (where the data "hi" begins)
		2, 0, 0, 0, // = iovs[0].length (how many bytes are in "hi")
		'h', 'i', // iovs[0].length bytes
	}

	tests := []struct {
		name           string
		fd, resultSize uint64
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
			resultSize:    uint64(len(memory)), // read was ok, but there wasn't enough memory to write the result.
			expectedErrno: wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			mod.Memory().(*wasm.MemoryInstance).Buffer = tc.memory

			results, err := fn.Call(mod, tc.fd, iovs, iovsCount, tc.resultSize)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

func createFile(t *testing.T, path string, contents []byte) (*memFile, *MemFS) {
	memFS := &MemFS{}
	f, err := memFS.OpenWASI(0, path, wasi.O_CREATE|wasi.O_TRUNC, wasi.R_FD_WRITE, 0, 0)
	require.NoError(t, err)

	memFile := f.(*memFile)
	memFile.buf = append([]byte{}, contents...)

	return memFile, memFS
}

// TestSnapshotPreview1_PathCreateDirectory only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathCreateDirectory(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathCreateDirectory, ImportPathCreateDirectory, moduleName, config)

	t.Run("SnapshotPreview1.PathCreateDirectory", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathCreateDirectory(mod, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathFilestatGet, ImportPathFilestatGet, moduleName, config)

	t.Run("SnapshotPreview1.PathFilestatGet", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathFilestatGet(mod, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathFilestatSetTimes, ImportPathFilestatSetTimes, moduleName, config)

	t.Run("SnapshotPreview1.PathFilestatSetTimes", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathFilestatSetTimes(mod, 0, 0, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathLink, ImportPathLink, moduleName, config)

	t.Run("SnapshotPreview1.PathLink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathLink(mod, 0, 0, 0, 0, 0, 0, 0))
	})

	t.Run(FunctionPathLink, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0, 0, 0, 0)
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
	resultOpenedFd := uint32(8)
	initialMemory := []byte{
		'?',                          // `path` is after this
		'w', 'a', 'z', 'e', 'r', 'o', // path
		'?', // `resultOpenedFd` is after this
	}
	expectedMemory := append(
		initialMemory,
		4, 0, 0, 0, // resultOpenedFd
		'?',
	)
	expectedFD := uint32(4) // arbitrary expected FD

	ctx := context.Background()
	config := NewConfig()
	// randSource is used to determine the new fd. Fix it to the expectedFD for testing.
	config.randSource = func(b []byte) error {
		binary.LittleEndian.PutUint32(b, expectedFD)
		return nil
	}

	mod, fn := instantiateModule(t, ctx, FunctionPathOpen, ImportPathOpen, moduleName, config)

	// TestSnapshotPreview1_PathOpen uses a matrix because setting up test files is complicated and has to be clean each time.
	type pathOpenFn func(ctx publicwasm.Module, fd, dirflags, path, pathLen, oflags uint32,
		fsRightsBase, fsRightsInheriting uint64,
		fdFlags, resultOpenedFd uint32) wasi.Errno
	tests := []struct {
		name     string
		pathOpen func() pathOpenFn
	}{
		{"SnapshotPreview1.PathOpen", func() pathOpenFn {
			return NewAPI(config).PathOpen
		}},
		{FunctionPathOpen, func() pathOpenFn {
			return func(ctx publicwasm.Module, fd, dirflags, path, pathLen, oflags uint32,
				fsRightsBase, fsRightsInheriting uint64,
				fdFlags, resultOpenedFd uint32) wasi.Errno {
				ret, err := fn.Call(mod, uint64(fd), uint64(dirflags), uint64(path), uint64(pathLen), uint64(oflags), uint64(fsRightsBase), uint64(fsRightsInheriting), uint64(fdFlags), uint64(resultOpenedFd))
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
			config.opened = map[uint32]fileEntry{
				fd: {
					path:    ".",
					fileSys: memFS,
				},
			}

			maskMemory(t, mod, len(expectedMemory))
			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			errno := tc.pathOpen()(mod, fd, dirflags, path, pathLen, oflags, fsRightsBase, fsRightsInheriting, fdFlags, resultOpenedFd)
			require.Equal(t, wasi.ErrnoSuccess, errno)

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
			require.Equal(t, "wazero", config.opened[expectedFD].path) // verify the file was actually opened
		})
	}
}

func TestSnapshotPreview1_PathOpen_Erros(t *testing.T) {
	validFD := uint64(3) // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	// Create a memFS for testing that has "./wazero" file.
	memFS := &MemFS{
		Files: map[string][]byte{
			"wazero": []byte(""),
		},
	}
	ctx := context.Background()
	config := NewConfig()
	config.opened = map[uint32]fileEntry{
		uint32(validFD): {
			path:    ".",
			fileSys: memFS,
		},
	}

	mod, fn := instantiateModule(t, ctx, FunctionPathOpen, ImportPathOpen, moduleName, config)

	validPath := uint64(0)    // arbitrary offset
	validPathLen := uint64(6) // the length of "wazero"
	mod.Memory().Write(uint32(validPath), []byte{
		'w', 'a', 'z', 'e', 'r', 'o', // write to offset 0 (= validPath)
	}) // wazero is the path to the file in the memFS

	tests := []struct {
		name                              string
		fd, path, pathLen, resultOpenedFd uint64
		expectedErrno                     wasi.Errno
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasi.ErrnoBadf,
		},
		{
			name:          "out-of-memory reading path",
			fd:            validFD,
			path:          uint64(mod.Memory().Size()),
			pathLen:       validPathLen,
			expectedErrno: wasi.ErrnoFault,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            validFD,
			path:          validPath,
			pathLen:       uint64(mod.Memory().Size() + 1), // path is in the valid memory range, but pathLen is out-of-memory for path
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
			resultOpenedFd: uint64(mod.Memory().Size()), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  wasi.ErrnoFault,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(mod, tc.fd, 0, tc.path, tc.pathLen, 0, 0, 0, 0, tc.resultOpenedFd)
			require.NoError(t, err)
			require.Equal(t, tc.expectedErrno, wasi.Errno(results[0])) // results[0] is the errno
		})
	}
}

// TestSnapshotPreview1_PathReadlink only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_PathReadlink(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathReadlink, ImportPathReadlink, moduleName, config)

	t.Run("SnapshotPreview1.PathLink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathReadlink(mod, 0, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathRemoveDirectory, ImportPathRemoveDirectory, moduleName, config)

	t.Run("SnapshotPreview1.PathRemoveDirectory", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathRemoveDirectory(mod, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathRename, ImportPathRename, moduleName, config)

	t.Run("SnapshotPreview1.PathRename", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathRename(mod, 0, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathSymlink, ImportPathSymlink, moduleName, config)

	t.Run("SnapshotPreview1.PathSymlink", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathSymlink(mod, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPathUnlinkFile, ImportPathUnlinkFile, moduleName, config)

	t.Run("SnapshotPreview1.PathUnlinkFile", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PathUnlinkFile(mod, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionPollOneoff, ImportPollOneoff, moduleName, config)

	t.Run("SnapshotPreview1.PollOneoff", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).PollOneoff(mod, 0, 0, 0, 0))
	})

	t.Run(FunctionPollOneoff, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

func TestSnapshotPreview1_ProcExit(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

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

	mod, fn := instantiateModule(t, ctx, FunctionProcExit, ImportProcExit, moduleName, config)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// When ProcExit is called, store.CallFunction returns immediately, returning the exit code as the error.
			_, err := fn.Call(mod, uint64(tc.exitCode))
			var code wasi.ExitCode
			require.ErrorAs(t, err, &code)
			require.Equal(t, code, wasi.ExitCode(tc.exitCode))
		})
	}
}

// TestSnapshotPreview1_ProcRaise only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_ProcRaise(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionProcRaise, ImportProcRaise, moduleName, config)

	t.Run("SnapshotPreview1.ProcRaise", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).ProcRaise(mod, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionSchedYield, ImportSchedYield, moduleName, config)

	t.Run("SnapshotPreview1.SchedYield", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).SchedYield(mod))
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
	config := NewConfig()
	config.randSource = func(p []byte) error {
		s := rand.NewSource(seed)
		rng := rand.New(s)
		_, err := rng.Read(p)

		return err
	}

	mod, fn := instantiateModule(t, ctx, FunctionRandomGet, ImportRandomGet, moduleName, config)

	t.Run("SnapshotPreview1.RandomGet", func(t *testing.T) {
		maskMemory(t, mod, len(expectedMemory))

		// Invoke RandomGet directly and check the memory side effects!
		errno := NewAPI(config).RandomGet(mod, offset, length)
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
	config := NewConfig()

	validAddress := uint32(0) // arbitrary valid address

	mod, fn := instantiateModule(t, ctx, FunctionRandomGet, ImportRandomGet, moduleName, config)
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
			results, err := fn.Call(mod, uint64(tc.offset), uint64(tc.length))
			require.NoError(t, err)
			require.Equal(t, uint64(wasi.ErrnoFault), results[0]) // results[0] is the errno
		})
	}
}

func TestSnapshotPreview1_RandomGet_SourceError(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()
	config.randSource = func(p []byte) error {
		return errors.New("random source error")
	}

	mod, fn := instantiateModule(t, ctx, FunctionRandomGet, ImportRandomGet, moduleName, config)

	results, err := fn.Call(mod, uint64(1), uint64(5)) // arbitrary offset and length
	require.NoError(t, err)
	require.Equal(t, uint64(wasi.ErrnoIo), results[0]) // results[0] is the errno
}

// TestSnapshotPreview1_SockRecv only tests it is stubbed for GrainLang per #271
func TestSnapshotPreview1_SockRecv(t *testing.T) {
	ctx := context.Background()
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionSockRecv, ImportSockRecv, moduleName, config)

	t.Run("SnapshotPreview1.SockRecv", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).SockRecv(mod, 0, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionSockSend, ImportSockSend, moduleName, config)

	t.Run("SnapshotPreview1.SockSend", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).SockSend(mod, 0, 0, 0, 0, 0))
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
	config := NewConfig()

	mod, fn := instantiateModule(t, ctx, FunctionSockShutdown, ImportSockShutdown, moduleName, config)

	t.Run("SnapshotPreview1.SockShutdown", func(t *testing.T) {
		require.Equal(t, wasi.ErrnoNosys, NewAPI(config).SockShutdown(mod, 0, 0))
	})

	t.Run(FunctionSockShutdown, func(t *testing.T) {
		results, err := fn.Call(mod, 0, 0)
		require.NoError(t, err)
		require.Equal(t, wasi.ErrnoNosys, wasi.Errno(results[0])) // cast because results are always uint64
	})
}

const testMemoryPageSize = 1

func instantiateModule(t *testing.T, ctx context.Context, wasiFunction, wasiImport, moduleName string, config *Config) (*wasm.ModuleContext, publicwasm.Function) {
	enabledFeatures := wasm.Features20191205
	store := wasm.NewStore(interpreter.NewEngine(), enabledFeatures)

	// The package `wazero` has a simpler interface for adding host modules, but we can't use that as it would create an
	// import cycle. Instead, we export internalwasm.NewHostModule and use it here.
	m, err := wasm.NewHostModule(wasi.ModuleSnapshotPreview1, SnapshotPreview1Functions(config))
	require.NoError(t, err)

	// Double-check what we created passes same validity as module-defined modules.
	require.NoError(t, m.Validate(enabledFeatures))

	_, err = store.Instantiate(ctx, m, m.NameSection.ModuleName)
	require.NoError(t, err)

	m, err = text.DecodeModule([]byte(fmt.Sprintf(`(module
  %[2]s
  (memory 1)  ;; just an arbitrary size big enough for tests
  (export "memory" (memory 0))
  (export "%[1]s" (func $wasi.%[1]s))
)`, wasiFunction, wasiImport)), enabledFeatures)
	require.NoError(t, err)

	mod, err := store.Instantiate(ctx, m, moduleName)
	require.NoError(t, err)

	fn := mod.ExportedFunction(wasiFunction)
	require.NotNil(t, fn)
	return mod, fn
}

// maskMemory sets the first memory in the store to '?' * size, so tests can see what's written.
func maskMemory(t *testing.T, mod publicwasm.Module, size int) {
	for i := uint32(0); i < uint32(size); i++ {
		require.True(t, mod.Memory().WriteByte(i, '?'))
	}
}
