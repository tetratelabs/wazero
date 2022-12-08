package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
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

	maskMemory(t, testCtx, mod, len(expectedMemory)+int(resultEnvironBuf))

	// Invoke environGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, environGetName, uint64(resultEnviron), uint64(resultEnvironBuf))
	require.Equal(t, `
--> proxy.environ_get(environ=26,environ_buf=16)
	==> wasi_snapshot_preview1.environ_get(environ=26,environ_buf=16)
	<== ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, resultEnvironBuf-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_environGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
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
			name:       "out-of-memory environ",
			environ:    memorySize,
			environBuf: validAddress,
			expectedLog: `
--> proxy.environ_get(environ=65536,environ_buf=0)
	==> wasi_snapshot_preview1.environ_get(environ=65536,environ_buf=0)
	<== EFAULT
<-- 21
`,
		},
		{
			name:       "out-of-memory environBuf",
			environ:    validAddress,
			environBuf: memorySize,
			expectedLog: `
--> proxy.environ_get(environ=0,environ_buf=65536)
	==> wasi_snapshot_preview1.environ_get(environ=0,environ_buf=65536)
	<== EFAULT
<-- 21
`,
		},
		{
			name: "environ exceeds the maximum valid address by 1",
			// 4*envCount is the expected length for environ, 4 is the size of uint32
			environ:    memorySize - 4*2 + 1,
			environBuf: validAddress,
			expectedLog: `
--> proxy.environ_get(environ=65529,environ_buf=0)
	==> wasi_snapshot_preview1.environ_get(environ=65529,environ_buf=0)
	<== EFAULT
<-- 21
`,
		},
		{
			name:    "environBuf exceeds the maximum valid address by 1",
			environ: validAddress,
			// "a=bc", "b=cd" size = size of "a=bc0b=cd0" = 10
			environBuf: memorySize - 10 + 1,
			expectedLog: `
--> proxy.environ_get(environ=0,environ_buf=65527)
	==> wasi_snapshot_preview1.environ_get(environ=0,environ_buf=65527)
	<== EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, environGetName, uint64(tc.environ), uint64(tc.environBuf))
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

	maskMemory(t, testCtx, mod, len(expectedMemory)+int(resultEnvironc))

	// Invoke environSizesGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, environSizesGetName, uint64(resultEnvironc), uint64(resultEnvironvLen))
	require.Equal(t, `
--> proxy.environ_sizes_get(result.environc=16,result.environv_len=21)
	--> wasi_snapshot_preview1.environ_sizes_get(result.environc=16,result.environv_len=21)
		==> wasi_snapshot_preview1.environSizesGet()
		<== (environc=2,environv_len=9,ESUCCESS)
	<-- ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, resultEnvironc-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_environSizesGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
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
--> proxy.environ_sizes_get(result.environc=65536,result.environv_len=0)
	--> wasi_snapshot_preview1.environ_sizes_get(result.environc=65536,result.environv_len=0)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:       "out-of-memory environLen",
			environc:   validAddress,
			environLen: memorySize,
			expectedLog: `
--> proxy.environ_sizes_get(result.environc=0,result.environv_len=65536)
	--> wasi_snapshot_preview1.environ_sizes_get(result.environc=0,result.environv_len=65536)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:       "environCount exceeds the maximum valid address by 1",
			environc:   memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of environ
			environLen: validAddress,
			expectedLog: `
--> proxy.environ_sizes_get(result.environc=65533,result.environv_len=0)
	--> wasi_snapshot_preview1.environ_sizes_get(result.environc=65533,result.environv_len=0)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:       "environLen exceeds the maximum valid size by 1",
			environc:   validAddress,
			environLen: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
			expectedLog: `
--> proxy.environ_sizes_get(result.environc=0,result.environv_len=65533)
	--> wasi_snapshot_preview1.environ_sizes_get(result.environc=0,result.environv_len=65533)
	<-- EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, environSizesGetName, uint64(tc.environc), uint64(tc.environLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
