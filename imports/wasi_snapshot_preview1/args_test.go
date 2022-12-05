package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_argsGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithArgs("a", "bc"))
	defer r.Close(testCtx)

	argvBuf := uint32(16) // arbitrary offset
	argv := uint32(22)    // arbitrary offset
	expectedMemory := []byte{
		'?',                 // argvBuf is after this
		'a', 0, 'b', 'c', 0, // null terminated "a", "bc"
		'?',         // argv is after this
		16, 0, 0, 0, // little endian-encoded offset of "a"
		18, 0, 0, 0, // little endian-encoded offset of "bc"
		'?', // stopped after encoding
	}

	maskMemory(t, testCtx, mod, len(expectedMemory)+int(argvBuf))

	// Invoke argsGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, argsGetName, uint64(argv), uint64(argvBuf))
	require.Equal(t, `
--> proxy.args_get(argv=22,argv_buf=16)
	==> wasi_snapshot_preview1.args_get(argv=22,argv_buf=16)
	<== ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, argvBuf-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_argsGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithArgs("a", "bc"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
	validAddress := uint32(0) // arbitrary

	tests := []struct {
		name          string
		argv, argvBuf uint32
		expectedLog   string
	}{
		{
			name:    "out-of-memory argv",
			argv:    memorySize,
			argvBuf: validAddress,
			expectedLog: `
--> proxy.args_get(argv=65536,argv_buf=0)
	==> wasi_snapshot_preview1.args_get(argv=65536,argv_buf=0)
	<== EFAULT
<-- 21
`,
		},
		{
			name:    "out-of-memory argvBuf",
			argv:    validAddress,
			argvBuf: memorySize,
			expectedLog: `
--> proxy.args_get(argv=0,argv_buf=65536)
	==> wasi_snapshot_preview1.args_get(argv=0,argv_buf=65536)
	<== EFAULT
<-- 21
`,
		},
		{
			name: "argv exceeds the maximum valid address by 1",
			// 4*argCount is the size of the result of the pointers to args, 4 is the size of uint32
			argv:    memorySize - 4*2 + 1,
			argvBuf: validAddress,
			expectedLog: `
--> proxy.args_get(argv=65529,argv_buf=0)
	==> wasi_snapshot_preview1.args_get(argv=65529,argv_buf=0)
	<== EFAULT
<-- 21
`,
		},
		{
			name: "argvBuf exceeds the maximum valid address by 1",
			argv: validAddress,
			// "a", "bc" size = size of "a0bc0" = 5
			argvBuf: memorySize - 5 + 1,
			expectedLog: `
--> proxy.args_get(argv=0,argv_buf=65532)
	==> wasi_snapshot_preview1.args_get(argv=0,argv_buf=65532)
	<== EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, argsGetName, uint64(tc.argv), uint64(tc.argvBuf))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_argsSizesGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithArgs("a", "bc"))
	defer r.Close(testCtx)

	resultArgc := uint32(16)    // arbitrary offset
	resultArgvLen := uint32(21) // arbitrary offset
	expectedMemory := []byte{
		'?',                // resultArgc is after this
		0x2, 0x0, 0x0, 0x0, // little endian-encoded arg count
		'?',                // resultArgvLen is after this
		0x5, 0x0, 0x0, 0x0, // little endian-encoded size of null terminated strings
		'?', // stopped after encoding
	}

	maskMemory(t, testCtx, mod, int(resultArgc)+len(expectedMemory))

	// Invoke argsSizesGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, argsSizesGetName, uint64(resultArgc), uint64(resultArgvLen))
	require.Equal(t, `
--> proxy.args_sizes_get(result.argc=16,result.argv_len=21)
	--> wasi_snapshot_preview1.args_sizes_get(result.argc=16,result.argv_len=21)
		==> wasi_snapshot_preview1.argsSizesGet()
		<== (argc=2,argv_len=5,ESUCCESS)
	<-- ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, resultArgc-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_argsSizesGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithArgs("a", "bc"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
	validAddress := uint32(0) // arbitrary valid address as arguments to args_sizes_get. We chose 0 here.

	tests := []struct {
		name          string
		argc, argvLen uint32
		expectedLog   string
	}{
		{
			name:    "out-of-memory argc",
			argc:    memorySize,
			argvLen: validAddress,
			expectedLog: `
--> proxy.args_sizes_get(result.argc=65536,result.argv_len=0)
	--> wasi_snapshot_preview1.args_sizes_get(result.argc=65536,result.argv_len=0)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:    "out-of-memory argvLen",
			argc:    validAddress,
			argvLen: memorySize,
			expectedLog: `
--> proxy.args_sizes_get(result.argc=0,result.argv_len=65536)
	--> wasi_snapshot_preview1.args_sizes_get(result.argc=0,result.argv_len=65536)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:    "argc exceeds the maximum valid address by 1",
			argc:    memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			argvLen: validAddress,
			expectedLog: `
--> proxy.args_sizes_get(result.argc=65533,result.argv_len=0)
	--> wasi_snapshot_preview1.args_sizes_get(result.argc=65533,result.argv_len=0)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:    "argvLen exceeds the maximum valid size by 1",
			argc:    validAddress,
			argvLen: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
			expectedLog: `
--> proxy.args_sizes_get(result.argc=0,result.argv_len=65533)
	--> wasi_snapshot_preview1.args_sizes_get(result.argc=0,result.argv_len=65533)
	<-- EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, argsSizesGetName, uint64(tc.argc), uint64(tc.argvLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
