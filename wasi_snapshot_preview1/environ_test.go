package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_environGet(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

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

	maskMemory(t, testCtx, mod, len(expectedMemory))

	// Invoke environGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, functionEnvironGet, uint64(resultEnviron), uint64(resultEnvironBuf))
	require.Equal(t, `
==> wasi_snapshot_preview1.environ_get(environ=11,environ_buf=1)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_environGet_Errors(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig().
		WithEnv("a", "bc").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
	validAddress := uint32(0) // arbitrary valid address as arguments to environ_get. We chose 0 here.

	tests := []struct {
		name                string
		environ, environBuf uint32
		expectedLog         string
	}{
		{
			name:       "out-of-memory environPtr",
			environ:    memorySize,
			environBuf: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=65536,environ_buf=0)
<== EFAULT
`,
		},
		{
			name:       "out-of-memory environBufPtr",
			environ:    validAddress,
			environBuf: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=0,environ_buf=65536)
<== EFAULT
`,
		},
		{
			name: "environPtr exceeds the maximum valid address by 1",
			// 4*envCount is the expected length for environPtr, 4 is the size of uint32
			environ:    memorySize - 4*2 + 1,
			environBuf: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=65529,environ_buf=0)
<== EFAULT
`,
		},
		{
			name:    "environBufPtr exceeds the maximum valid address by 1",
			environ: validAddress,
			// "a=bc", "b=cd" size = size of "a=bc0b=cd0" = 10
			environBuf: memorySize - 10 + 1,
			expectedLog: `
==> wasi_snapshot_preview1.environ_get(environ=0,environ_buf=65527)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, functionEnvironGet, uint64(tc.environ), uint64(tc.environBuf))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_environSizesGet(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	resultEnvironc := uint32(1)       // arbitrary offset
	resultEnvironBufSize := uint32(6) // arbitrary offset
	expectedMemory := []byte{
		'?',                // resultEnvironc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded environment variable count
		'?',                // resultEnvironBufSize is after this
		0x9, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	}

	maskMemory(t, testCtx, mod, len(expectedMemory))

	// Invoke environSizesGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, functionEnvironSizesGet, uint64(resultEnvironc), uint64(resultEnvironBufSize))
	require.Equal(t, `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=1,result.environBufSize=6)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_environSizesGet_Errors(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
	validAddress := uint32(0) // arbitrary valid address as arguments to environ_sizes_get. We chose 0 here.

	tests := []struct {
		name                     string
		environc, environBufSize uint32
		expectedLog              string
	}{
		{
			name:           "out-of-memory environCountPtr",
			environc:       memorySize,
			environBufSize: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=65536,result.environBufSize=0)
<== EFAULT
`,
		},
		{
			name:           "out-of-memory environBufSizePtr",
			environc:       validAddress,
			environBufSize: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=0,result.environBufSize=65536)
<== EFAULT
`,
		},
		{
			name:           "environCountPtr exceeds the maximum valid address by 1",
			environc:       memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of environ
			environBufSize: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=65533,result.environBufSize=0)
<== EFAULT
`,
		},
		{
			name:           "environBufSizePtr exceeds the maximum valid size by 1",
			environc:       validAddress,
			environBufSize: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
			expectedLog: `
==> wasi_snapshot_preview1.environ_sizes_get(result.environc=0,result.environBufSize=65533)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, functionEnvironSizesGet, uint64(tc.environc), uint64(tc.environBufSize))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
