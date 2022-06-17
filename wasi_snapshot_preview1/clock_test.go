package wasi_snapshot_preview1

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Test_ClockResGet only tests it is stubbed for GrainLang per #271
func Test_ClockResGet(t *testing.T) {
	mod, fn := instantiateModule(testCtx, t, functionClockResGet, importClockResGet, nil)
	defer mod.Close(testCtx)

	resultResolution := uint32(1) // arbitrary offset

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
		clockID        uint64
		expectedMemory []byte
		invocation     func(clockID uint64) Errno
	}{
		{
			name:           "wasi.ClockResGet",
			clockID:        0,
			expectedMemory: expectedMemoryMicro,
			invocation: func(clockID uint64) Errno {
				return a.ClockResGet(testCtx, mod, uint32(clockID), resultResolution)
			},
		},
		{
			name:           "wasi.ClockResGet",
			clockID:        1,
			expectedMemory: expectedMemoryNano,
			invocation: func(clockID uint64) Errno {
				return a.ClockResGet(testCtx, mod, uint32(clockID), resultResolution)
			},
		},
		{
			name:           functionClockResGet,
			clockID:        0,
			expectedMemory: expectedMemoryMicro,
			invocation: func(clockID uint64) Errno {
				results, err := fn.Call(testCtx, clockID, uint64(resultResolution))
				require.NoError(t, err)
				return Errno(results[0]) // results[0] is the errno
			},
		},
		{
			name:           functionClockResGet,
			clockID:        1,
			expectedMemory: expectedMemoryNano,
			invocation: func(clockID uint64) Errno {
				results, err := fn.Call(testCtx, clockID, uint64(resultResolution))
				require.NoError(t, err)
				return Errno(results[0]) // results[0] is the errno
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(fmt.Sprintf("%v/clockID=%v", tc.name, tc.clockID), func(t *testing.T) {
			maskMemory(t, testCtx, mod, len(tc.expectedMemory))

			errno := tc.invocation(tc.clockID)
			require.Equal(t, ErrnoSuccess, errno, ErrnoName(errno))

			actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_ClockResGet_Unsupported(t *testing.T) {
	resultResolution := uint32(1) // arbitrary offset
	mod, fn := instantiateModule(testCtx, t, functionClockResGet, importClockResGet, nil)
	defer mod.Close(testCtx)

	tests := []struct {
		name          string
		clockID       uint64
		expectedErrno Errno
	}{
		{
			name:          "process cputime",
			clockID:       2,
			expectedErrno: ErrnoNotsup,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: ErrnoNotsup,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: ErrnoInval,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(testCtx, tc.clockID, uint64(resultResolution))
			require.NoError(t, err)
			errno := Errno(results[0]) // results[0] is the errno
			require.Equal(t, tc.expectedErrno, errno, ErrnoName(errno))
		})
	}
}

func Test_ClockTimeGet(t *testing.T) {
	resultTimestamp := uint32(1) // arbitrary offset

	mod, fn := instantiateModule(testCtx, t, functionClockTimeGet, importClockTimeGet, nil)
	defer mod.Close(testCtx)

	clocks := []struct {
		clock          string
		id             uint32
		expectedMemory []byte
	}{
		{
			clock: "Realtime",
			id:    clockIDRealtime,
			expectedMemory: []byte{
				'?',                                          // resultTimestamp is after this
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // little endian-encoded epochNanos
				'?', // stopped after encoding
			},
		},
		{
			clock: "Monotonic",
			id:    clockIDMonotonic,
			expectedMemory: []byte{
				'?',                                    // resultTimestamp is after this
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // fake nanotime starts at zero
				'?', // stopped after encoding
			},
		},
	}

	for _, c := range clocks {
		cc := c
		t.Run(cc.clock, func(t *testing.T) {
			tests := []struct {
				name       string
				invocation func() Errno
			}{
				{
					name: "wasi.ClockTimeGet",
					invocation: func() Errno {
						return a.ClockTimeGet(testCtx, mod, cc.id, 0 /* TODO: precision */, resultTimestamp)
					},
				},
				{
					name: functionClockTimeGet,
					invocation: func() Errno {
						results, err := fn.Call(testCtx, uint64(cc.id), 0 /* TODO: precision */, uint64(resultTimestamp))
						require.NoError(t, err)
						errno := Errno(results[0]) // results[0] is the errno
						return errno
					},
				},
			}

			for _, tt := range tests {
				tc := tt
				t.Run(tc.name, func(t *testing.T) {
					// Reset the fake clock
					sysCtx, err := newSysContext(nil, nil, nil)
					require.NoError(t, err)
					mod.(*wasm.CallContext).Sys = sysCtx

					maskMemory(t, testCtx, mod, len(cc.expectedMemory))

					errno := tc.invocation()
					require.Zero(t, errno, ErrnoName(errno))

					actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(cc.expectedMemory)))
					require.True(t, ok)
					require.Equal(t, cc.expectedMemory, actual)
				})
			}
		})
	}
}

func Test_ClockTimeGet_Unsupported(t *testing.T) {
	resultTimestamp := uint32(1) // arbitrary offset
	mod, fn := instantiateModule(testCtx, t, functionClockTimeGet, importClockTimeGet, nil)
	defer mod.Close(testCtx)

	tests := []struct {
		name          string
		clockID       uint64
		expectedErrno Errno
	}{
		{
			name:          "process cputime",
			clockID:       2,
			expectedErrno: ErrnoNotsup,
		},
		{
			name:          "thread cputime",
			clockID:       3,
			expectedErrno: ErrnoNotsup,
		},
		{
			name:          "undefined",
			clockID:       100,
			expectedErrno: ErrnoInval,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			results, err := fn.Call(testCtx, tc.clockID, 0 /* TODO: precision */, uint64(resultTimestamp))
			require.NoError(t, err)
			errno := Errno(results[0]) // results[0] is the errno
			require.Equal(t, tc.expectedErrno, errno, ErrnoName(errno))
		})
	}
}

func Test_ClockTimeGet_Errors(t *testing.T) {
	mod, fn := instantiateModule(testCtx, t, functionClockTimeGet, importClockTimeGet, nil)
	defer mod.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)

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
			results, err := fn.Call(testCtx, 0 /* TODO: id */, 0 /* TODO: precision */, uint64(tc.resultTimestamp))
			require.NoError(t, err)
			errno := Errno(results[0]) // results[0] is the errno
			require.Equal(t, ErrnoFault, errno, ErrnoName(errno))
		})
	}
}
