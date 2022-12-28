package wasi_snapshot_preview1

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_clockResGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
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
==> wasi_snapshot_preview1.clock_res_get(id=0,result.resolution=16)
<== errno=ESUCCESS
`,
		},
		{
			name:           "Monotonic",
			clockID:        clockIDMonotonic,
			expectedMemory: expectedMemoryNano,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=1,result.resolution=16)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultResolution := 16 // arbitrary offset
			maskMemory(t, mod, resultResolution+len(tc.expectedMemory))

			requireErrno(t, ErrnoSuccess, mod, clockResGetName, uint64(tc.clockID), uint64(resultResolution))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(uint32(resultResolution-1), uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_clockResGet_Unsupported(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
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
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=2,result.resolution=16)
<== errno=EINVAL
`,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=3,result.resolution=16)
<== errno=EINVAL
`,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=100,result.resolution=16)
<== errno=EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultResolution := 16 // arbitrary offset
			requireErrno(t, tc.expectedErrno, mod, clockResGetName, uint64(tc.clockID), uint64(resultResolution))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_clockTimeGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
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
==> wasi_snapshot_preview1.clock_time_get(id=0,precision=0,result.timestamp=16)
<== errno=ESUCCESS
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
==> wasi_snapshot_preview1.clock_time_get(id=1,precision=0,result.timestamp=16)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultTimestamp := 16 // arbitrary offset
			maskMemory(t, mod, resultTimestamp+len(tc.expectedMemory))

			requireErrno(t, ErrnoSuccess, mod, clockTimeGetName, uint64(tc.clockID), 0 /* TODO: precision */, uint64(resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(uint32(resultTimestamp-1), uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_clockTimeGet_Unsupported(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
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
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=2,precision=0,result.timestamp=16)
<== errno=EINVAL
`,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=3,precision=0,result.timestamp=16)
<== errno=EINVAL
`,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=100,precision=0,result.timestamp=16)
<== errno=EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultTimestamp := 16 // arbitrary offset
			requireErrno(t, tc.expectedErrno, mod, clockTimeGetName, uint64(tc.clockID), uint64(0) /* TODO: precision */, uint64(resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_clockTimeGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()

	tests := []struct {
		name                     string
		resultTimestamp, argvLen uint32
		expectedLog              string
	}{
		{
			name:            "resultTimestamp OOM",
			resultTimestamp: memorySize,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=0,precision=0,result.timestamp=65536)
<== errno=EFAULT
`,
		},
		{
			name:            "resultTimestamp exceeds the maximum valid address by 1",
			resultTimestamp: memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=0,precision=0,result.timestamp=65533)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, ErrnoFault, mod, clockTimeGetName, uint64(0) /* TODO: id */, uint64(0) /* TODO: precision */, uint64(tc.resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
