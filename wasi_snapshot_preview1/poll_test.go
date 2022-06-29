package wasi_snapshot_preview1

import (
	"testing"

	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_PollOneoff(t *testing.T) {
	mod, fn := instantiateModule(testCtx, t, functionPollOneoff, importPollOneoff, nil)
	defer mod.Close(testCtx)

	tests := []struct {
		name        string
		mem         []byte
		expectedMem []byte // at offset out
	}{
		{
			name: "monotonic relative",
			mem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				eventTypeClock, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // event type and padding
				clockIDMonotonic, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // clockID
				0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // timeout (ns)
				0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // precision (ns)
				0x00, 0x00, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // flags (relative)
				'?', // stopped after encoding
			},
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(ErrnoSuccess), 0x0, // errno is 16 bit
				eventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', // stopped after encoding
			},
		},
	}

	in := uint32(0)    // past in
	out := uint32(128) // past in
	nsubscriptions := uint32(1)
	resultNevents := uint32(512) // past out

	requireExpectedMem := func(expectedMem []byte) {
		outMem, ok := mod.Memory().Read(testCtx, out, uint32(len(expectedMem)))
		require.True(t, ok)
		require.Equal(t, expectedMem, outMem)

		nevents, ok := mod.Memory().ReadUint32Le(testCtx, resultNevents)
		require.True(t, ok)
		require.Equal(t, nsubscriptions, nevents)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Run("wasi.PollOneoff", func(t *testing.T) {
				maskMemory(t, testCtx, mod, 1024)
				mod.Memory().Write(testCtx, in, tc.mem)

				errno := a.PollOneoff(testCtx, mod, in, out, nsubscriptions, resultNevents)
				require.Equal(t, ErrnoSuccess, errno, ErrnoName(errno))
				requireExpectedMem(tc.expectedMem)
			})

			t.Run(functionPollOneoff, func(t *testing.T) {
				maskMemory(t, testCtx, mod, 1024)
				mod.Memory().Write(testCtx, in, tc.mem)

				results, err := fn.Call(testCtx, uint64(in), uint64(out), uint64(nsubscriptions), uint64(resultNevents))
				require.NoError(t, err)
				errno := Errno(results[0]) // results[0] is the errno
				require.Equal(t, ErrnoSuccess, errno, ErrnoName(errno))
				requireExpectedMem(tc.expectedMem)
			})
		})
	}
}

func Test_PollOneoff_Errors(t *testing.T) {
	mod, _ := instantiateModule(testCtx, t, functionPollOneoff, importPollOneoff, nil)
	defer mod.Close(testCtx)

	tests := []struct {
		name                                   string
		in, out, nsubscriptions, resultNevents uint32
		mem                                    []byte // at offset in
		expectedErrno                          Errno
		expectedMem                            []byte // at offset out
	}{
		{
			name:           "in out of range",
			in:             wasm.MemoryPageSize,
			nsubscriptions: 1,
			out:            128, // past in
			resultNevents:  512, //past out
			expectedErrno:  ErrnoFault,
		},
		{
			name:           "out out of range",
			out:            wasm.MemoryPageSize,
			resultNevents:  512, //past out
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
			out:           128, // past in
			resultNevents: 512, //past out
			expectedErrno: ErrnoInval,
		},
		{
			name:           "unsupported eventTypeFdRead",
			nsubscriptions: 1,
			mem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				eventTypeFdRead, 0x0, 0x0, 0x0,
				internalsys.FdStdin, 0x0, 0x0, 0x0, // valid readable FD
				'?', // stopped after encoding
			},
			expectedErrno: ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, //past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(ErrnoNotsup), 0x0, // errno is 16 bit
				eventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', // stopped after encoding
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			maskMemory(t, testCtx, mod, 1024)

			if tc.mem != nil {
				mod.Memory().Write(testCtx, tc.in, tc.mem)
			}

			errno := a.PollOneoff(testCtx, mod, tc.in, tc.out, tc.nsubscriptions, tc.resultNevents)
			require.Equal(t, tc.expectedErrno, errno, ErrnoName(errno))

			out, ok := mod.Memory().Read(testCtx, tc.out, uint32(len(tc.expectedMem)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMem, out)

			// Events should be written on success regardless of nested failure.
			if tc.expectedErrno == ErrnoSuccess {
				nevents, ok := mod.Memory().ReadUint32Le(testCtx, tc.resultNevents)
				require.True(t, ok)
				require.Equal(t, uint32(1), nevents)
			}
		})
	}
}
