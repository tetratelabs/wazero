package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_PollOneoff(t *testing.T) {
	mod, fn := instantiateModule(testCtx, t, functionPollOneoff, importPollOneoff, nil)
	defer mod.Close(testCtx)

	t.Run("wasi.PollOneoff", func(t *testing.T) {
		errno := a.PollOneoff(testCtx, mod, 0, 0, 1, 0)
		require.Equal(t, ErrnoSuccess, errno, ErrnoName(errno))
	})

	t.Run(functionPollOneoff, func(t *testing.T) {
		results, err := fn.Call(testCtx, 0, 0, 1, 0)
		require.NoError(t, err)
		errno := Errno(results[0]) // results[0] is the errno
		require.Equal(t, ErrnoSuccess, errno, ErrnoName(errno))
	})
}

func Test_PollOneoff_Errors(t *testing.T) {
	mod, _ := instantiateModule(testCtx, t, functionPollOneoff, importPollOneoff, nil)
	defer mod.Close(testCtx)

	tests := []struct {
		name                                   string
		in, out, nsubscriptions, resultNevents uint32
		expectedErrno                          Errno
	}{
		{
			name:           "in out of range",
			in:             wasm.MemoryPageSize,
			nsubscriptions: 1,
			expectedErrno:  ErrnoFault,
		},
		{
			name:           "out out of range",
			out:            wasm.MemoryPageSize,
			nsubscriptions: 1,
			expectedErrno:  ErrnoFault,
		},
		{
			name:           "resultNevents out of range",
			resultNevents:  wasm.MemoryPageSize,
			nsubscriptions: 1,
			expectedErrno:  ErrnoFault,
		},
		{
			name:          "nsubscriptions zero",
			expectedErrno: ErrnoInval,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			errno := a.PollOneoff(testCtx, mod, tc.in, tc.out, tc.nsubscriptions, tc.resultNevents)
			require.Equal(t, tc.expectedErrno, errno, ErrnoName(errno))
		})
	}
}
