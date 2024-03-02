package wasi_snapshot_preview1_test

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
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

	maskMemory(t, mod, len(expectedMemory)+int(argvBuf))

	// Invoke argsGet and check the memory side effects.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.ArgsGetName, uint64(argv), uint64(argvBuf))
	require.Equal(t, `
==> wasi_snapshot_preview1.args_get(argv=22,argv_buf=16)
<== errno=ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(argvBuf-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_argsGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithArgs("a", "bc"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
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
==> wasi_snapshot_preview1.args_get(argv=65536,argv_buf=0)
<== errno=EFAULT
`,
		},
		{
			name:    "out-of-memory argvBuf",
			argv:    validAddress,
			argvBuf: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.args_get(argv=0,argv_buf=65536)
<== errno=EFAULT
`,
		},
		{
			name: "argv exceeds the maximum valid address by 1",
			// 4*argCount is the size of the result of the pointers to args, 4 is the size of uint32
			argv:    memorySize - 4*2 + 1,
			argvBuf: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.args_get(argv=65529,argv_buf=0)
<== errno=EFAULT
`,
		},
		{
			name: "argvBuf exceeds the maximum valid address by 1",
			argv: validAddress,
			// "a", "bc" size = size of "a0bc0" = 5
			argvBuf: memorySize - 5 + 1,
			expectedLog: `
==> wasi_snapshot_preview1.args_get(argv=0,argv_buf=65532)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, wasip1.ErrnoFault, mod, wasip1.ArgsGetName, uint64(tc.argv), uint64(tc.argvBuf))
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

	maskMemory(t, mod, int(resultArgc)+len(expectedMemory))

	// Invoke argsSizesGet and check the memory side effects.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.ArgsSizesGetName, uint64(resultArgc), uint64(resultArgvLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.args_sizes_get(result.argc=16,result.argv_len=21)
<== errno=ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(resultArgc-1, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_argsSizesGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithArgs("a", "bc"))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
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
==> wasi_snapshot_preview1.args_sizes_get(result.argc=65536,result.argv_len=0)
<== errno=EFAULT
`,
		},
		{
			name:    "out-of-memory argvLen",
			argc:    validAddress,
			argvLen: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.args_sizes_get(result.argc=0,result.argv_len=65536)
<== errno=EFAULT
`,
		},
		{
			name:    "argc exceeds the maximum valid address by 1",
			argc:    memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			argvLen: validAddress,
			expectedLog: `
==> wasi_snapshot_preview1.args_sizes_get(result.argc=65533,result.argv_len=0)
<== errno=EFAULT
`,
		},
		{
			name:    "argvLen exceeds the maximum valid size by 1",
			argc:    validAddress,
			argvLen: memorySize - 4 + 1, // 4 is count of bytes to encode uint32le
			expectedLog: `
==> wasi_snapshot_preview1.args_sizes_get(result.argc=0,result.argv_len=65533)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, wasip1.ErrnoFault, mod, wasip1.ArgsSizesGetName, uint64(tc.argc), uint64(tc.argvLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
