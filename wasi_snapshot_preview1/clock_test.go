package wasi_snapshot_preview1

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_clockResGet(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	expectedMemoryMicro := []byte{
		'?',                                     // resultResolution is after this
		0xe8, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // little endian-encoded resolution (fixed to 1000).
		'?', // stopped after encoding
	}

	expectedMemoryNano := []byte{
		'?',                                    // resultResolution is after this
		0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // little endian-encoded resolution (fixed to 1000).
		'?', // stopped after encoding
	}

	tests := []struct {
		name           string
		clockID        uint32
		expectedMemory []byte
		expectedLog    string
	}{
		{
			name:           "Realtime",
			clockID:        clockIDRealtime,
			expectedMemory: expectedMemoryMicro,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=0,result.resolution=1)
<== ESUCCESS
`,
		},
		{
			name:           "Monotonic",
			clockID:        clockIDMonotonic,
			expectedMemory: expectedMemoryNano,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=1,result.resolution=1)
<== ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, testCtx, mod, len(tc.expectedMemory))

			resultResolution := uint32(1) // arbitrary offset
			requireErrno(t, ErrnoSuccess, mod, functionClockResGet, uint64(tc.clockID), uint64(resultResolution))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_clockResGet_Unsupported(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		clockID       uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "process cputime",
			clockID:       2,
			expectedErrno: ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=2,result.resolution=1)
<== ENOTSUP
`,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=3,result.resolution=1)
<== ENOTSUP
`,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=100,result.resolution=1)
<== EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultResolution := uint32(1) // arbitrary offset
			requireErrno(t, tc.expectedErrno, mod, functionClockResGet, uint64(tc.clockID), uint64(resultResolution))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_clockTimeGet(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	tests := []struct {
		name           string
		clockID        uint32
		expectedMemory []byte
		expectedLog    string
	}{
		{
			name:    "Realtime",
			clockID: clockIDRealtime,
			expectedMemory: []byte{
				'?',                                          // resultTimestamp is after this
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // little endian-encoded epochNanos
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=0,precision=0,result.timestamp=1)
<== ESUCCESS
`,
		},
		{
			name:    "Monotonic",
			clockID: clockIDMonotonic,
			expectedMemory: []byte{
				'?',                                    // resultTimestamp is after this
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // fake nanotime starts at zero
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=1,precision=0,result.timestamp=1)
<== ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, testCtx, mod, len(tc.expectedMemory))

			resultTimestamp := uint32(1) // arbitrary offset
			requireErrno(t, ErrnoSuccess, mod, functionClockTimeGet, uint64(tc.clockID), 0 /* TODO: precision */, uint64(resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_clockTimeGet_Unsupported(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		clockID       uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "process cputime",
			clockID:       2,
			expectedErrno: ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=2,precision=0,result.timestamp=1)
<== ENOTSUP
`,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=3,precision=0,result.timestamp=1)
<== ENOTSUP
`,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=100,precision=0,result.timestamp=1)
<== EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultTimestamp := uint32(1) // arbitrary offset

			requireErrno(t, tc.expectedErrno, mod, functionClockTimeGet, uint64(tc.clockID), uint64(0) /* TODO: precision */, uint64(resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_clockTimeGet_Errors(t *testing.T) {
	mod, r, log := requireModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)

	tests := []struct {
		name                         string
		resultTimestamp, argvBufSize uint32
		expectedLog                  string
	}{
		{
			name:            "resultTimestamp out-of-memory",
			resultTimestamp: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=0,precision=0,result.timestamp=65536)
<== EFAULT
`,
		},
		{
			name:            "resultTimestamp exceeds the maximum valid address by 1",
			resultTimestamp: memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=0,precision=0,result.timestamp=65533)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, functionClockTimeGet, uint64(0) /* TODO: id */, uint64(0) /* TODO: precision */, uint64(tc.resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
