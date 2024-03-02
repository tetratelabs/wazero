package wasi_snapshot_preview1_test

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
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
			clockID:        wasip1.ClockIDRealtime,
			expectedMemory: expectedMemoryMicro,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=realtime)
<== (resolution=1000,errno=ESUCCESS)
`,
		},
		{
			name:           "Monotonic",
			clockID:        wasip1.ClockIDMonotonic,
			expectedMemory: expectedMemoryNano,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=monotonic)
<== (resolution=1,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultResolution := 16 // arbitrary offset
			maskMemory(t, mod, resultResolution+len(tc.expectedMemory))

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.ClockResGetName, uint64(tc.clockID), uint64(resultResolution))
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
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "process cputime",
			clockID:       2,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=2)
<== (resolution=,errno=EINVAL)
`,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=3)
<== (resolution=,errno=EINVAL)
`,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_res_get(id=100)
<== (resolution=,errno=EINVAL)
`,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultResolution := 16 // arbitrary offset
			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.ClockResGetName, uint64(tc.clockID), uint64(resultResolution))
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
			clockID: wasip1.ClockIDRealtime,
			expectedMemory: []byte{
				'?',                                          // resultTimestamp is after this
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // little endian-encoded epochNanos
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=realtime,precision=0)
<== (timestamp=1640995200000000000,errno=ESUCCESS)
`,
		},
		{
			name:    "Monotonic",
			clockID: wasip1.ClockIDMonotonic,
			expectedMemory: []byte{
				'?',                                    // resultTimestamp is after this
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // fake nanotime starts at zero
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=monotonic,precision=0)
<== (timestamp=0,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultTimestamp := 16 // arbitrary offset
			maskMemory(t, mod, resultTimestamp+len(tc.expectedMemory))

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.ClockTimeGetName, uint64(tc.clockID), 0 /* TODO: precision */, uint64(resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(uint32(resultTimestamp-1), uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Similar to https://github.com/WebAssembly/wasi-testsuite/blob/dc7f8d27be1030cd4788ebdf07d9b57e5d23441e/tests/c/testsuite/clock_gettime-monotonic.c
func Test_clockTimeGet_monotonic(t *testing.T) {
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().
		// Important not to use fake time!
		WithSysNanotime())
	defer r.Close(testCtx)

	getMonotonicTime := func() uint64 {
		const offset uint32 = 0
		requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.ClockTimeGetName, uint64(wasip1.ClockIDMonotonic),
			0 /* TODO: precision */, uint64(offset))
		timestamp, ok := mod.Memory().ReadUint64Le(offset)
		require.True(t, ok)
		return timestamp
	}

	t1 := getMonotonicTime()
	t2 := getMonotonicTime()
	t3 := getMonotonicTime()
	t4 := getMonotonicTime()

	require.True(t, t1 < t2)
	require.True(t, t2 < t3)
	require.True(t, t3 < t4)
}

func Test_clockTimeGet_Unsupported(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		clockID       uint32
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "process cputime",
			clockID:       2,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=2,precision=0)
<== (timestamp=,errno=EINVAL)
`,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=3,precision=0)
<== (timestamp=,errno=EINVAL)
`,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=100,precision=0)
<== (timestamp=,errno=EINVAL)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			resultTimestamp := 16 // arbitrary offset
			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.ClockTimeGetName, uint64(tc.clockID), uint64(0) /* TODO: precision */, uint64(resultTimestamp))
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
==> wasi_snapshot_preview1.clock_time_get(id=realtime,precision=0)
<== (timestamp=,errno=EFAULT)
`,
		},
		{
			name:            "resultTimestamp exceeds the maximum valid address by 1",
			resultTimestamp: memorySize - 4 + 1, // 4 is the size of uint32, the type of the count of args
			expectedLog: `
==> wasi_snapshot_preview1.clock_time_get(id=realtime,precision=0)
<== (timestamp=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, wasip1.ErrnoFault, mod, wasip1.ClockTimeGetName, uint64(0) /* TODO: id */, uint64(0) /* TODO: precision */, uint64(tc.resultTimestamp))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}
