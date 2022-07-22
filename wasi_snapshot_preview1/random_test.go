package wasi_snapshot_preview1

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_randomGet(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig().
		WithRandSource(deterministicRandomSource()))
	defer r.Close(testCtx)

	expectedMemory := []byte{
		'?',                          // `offset` is after this
		0x53, 0x8c, 0x7f, 0x96, 0xb1, // random data from seed value of 42
		'?', // stopped after encoding
	}

	length := uint32(5) // arbitrary length,
	offset := uint32(1) // offset,

	maskMemory(t, testCtx, mod, len(expectedMemory))

	// Invoke randomGet and check the memory side effects!
	requireErrno(t, ErrnoSuccess, mod, functionRandomGet, uint64(offset), uint64(length))
	require.Equal(t, `
==> wasi_snapshot_preview1.random_get(buf=1,buf_len=5)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, offset+length+1)
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_randomGet_Errors(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig().
		WithRandSource(deterministicRandomSource()))
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)

	tests := []struct {
		name           string
		offset, length uint32
		expectedLog    string
	}{
		{
			name:   "out-of-memory",
			offset: memorySize,
			length: 1,
			expectedLog: `
==> wasi_snapshot_preview1.random_get(buf=65536,buf_len=1)
<== EFAULT
`,
		},
		{
			name:   "random length exceeds maximum valid address by 1",
			offset: 0, // arbitrary valid offset
			length: memorySize + 1,
			expectedLog: `
==> wasi_snapshot_preview1.random_get(buf=0,buf_len=65537)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, functionRandomGet, uint64(tc.offset), uint64(tc.length))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_randomGet_SourceError(t *testing.T) {
	tests := []struct {
		name        string
		randSource  io.Reader
		expectedLog string
	}{
		{
			name:       "error",
			randSource: iotest.ErrReader(errors.New("RandSource error")),
			expectedLog: `
==> wasi_snapshot_preview1.random_get(buf=1,buf_len=5)
<== EIO
`,
		},
		{
			name:       "incomplete",
			randSource: bytes.NewReader([]byte{1, 2}),
			expectedLog: `
==> wasi_snapshot_preview1.random_get(buf=1,buf_len=5)
<== EIO
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireModule(t, wazero.NewModuleConfig().
				WithRandSource(tc.randSource))
			defer r.Close(testCtx)

			requireErrno(t, ErrnoIo, mod, functionRandomGet, uint64(1), uint64(5)) // arbitrary offset and length
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
