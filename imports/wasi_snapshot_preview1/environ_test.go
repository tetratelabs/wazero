package wasi_snapshot_preview1_test

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
)

func Test_environGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	resultEnvironBuf := uint32(16) // arbitrary offset
	resultEnviron := uint32(26)    // arbitrary offset
	expectedMemory := []byte{
		'?',              // environBuf is after this
		'a', '=', 'b', 0, // null terminated "a=b",
		'b', '=', 'c', 'd', 0, // null terminated "b=cd"
		'?',         // environ is after this
		16, 0, 0, 0, // little endian-encoded offset of "a=b"
		20, 0, 0, 0, // little endian-encoded offset of "b=cd"
		'?', // stopped after encoding
	}

	maskMemory(t, mod, len(expectedMemory)+int(resultEnvironBuf))

	// Invoke environGet and check the memory side effects.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.EnvironGetName, uint64(resultEnviron), uint64(resultEnvironBuf))
	require.Equal(t, `
==> wasi_snapshot_preview1.environ_get(environ=26,environ_buf=16)
<== errno=ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(resultEnvironBuf-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_environGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithEnv("a", "bc").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
	validAddress := uint32(0) // arbitrary valid address as arguments to environ_get. We chose 0 here.

	tests := []struct {
		name                string
		environ, environBuf uint32
		expectedLog         string
	}{
		{
			name:       "out-of-memory environ",
			environ:    memorySize,
			environBuf: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=65536,environ_buf=0)
<== errno=EFAULT
`,
		},
		{
			name:       "out-of-memory environBuf",
			environ:    validAddress,
			environBuf: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=0,environ_buf=65536)
<== errno=EFAULT
`,
		},
		{
			name: "environ exceeds the maximum valid address by 1",
			// 4*envCount is the expected length for environ, 4 is the size of uint32
			environ:    memorySize - 4*2 + 1,
			environBuf: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=65529,environ_buf=0)
<== errno=EFAULT
`,
		},
		{
			name:    "environBuf exceeds the maximum valid address by 1",
			environ: validAddress,
			// "a=bc", "b=cd" size = size of "a=bc0b=cd0" = 10
			environBuf: memorySize - 10 + 1,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=0,environ_buf=65527)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, wasip1.ErrnoFault, mod, wasip1.EnvironGetName, uint64(tc.environ), uint64(tc.environBuf))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_environSizesGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	resultEnvironc := uint32(16)    // arbitrary offset
	resultEnvironvLen := uint32(21) // arbitrary offset
	expectedMemory := []byte{
		'?',                // resultEnvironc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded environment variable count
		'?',                // resultEnvironvLen is after this
		0x9, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	}

	maskMemory(t, mod, len(expectedMemory)+int(resultEnvironc))

	// Invoke environSizesGet and check the memory side effects.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.EnvironSizesGetName, uint64(resultEnvironc), uint64(resultEnvironvLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=16,result.environv_len=21)
<== errno=ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(resultEnvironc-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_environSizesGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
	validAddress := uint32(0) // arbitrary

	tests := []struct {
		name                 string
		environc, environLen uint32
		expectedLog          string
	}{
		{
			name:       "out-of-memory environCount",
			environc:   memorySize,
			environLen: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=65536,result.environv_len=0)
<== errno=EFAULT
`,
		},
		{
			name:       "out-of-memory environLen",
			environc:   validAddress,
			environLen: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=0,result.environv_len=65536)
<== errno=EFAULT
`,
		},
		{
			name:       "environCount exceeds the maximum valid address by 1",
			environc:   memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of environ
			environLen: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=65533,result.environv_len=0)
<== errno=EFAULT
`,
		},
		{
			name:       "environLen exceeds the maximum valid size by 1",
			environc:   validAddress,
			environLen: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=0,result.environv_len=65533)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, wasip1.ErrnoFault, mod, wasip1.EnvironSizesGetName, uint64(tc.environc), uint64(tc.environLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
