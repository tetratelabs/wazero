package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_pollOneoff(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	mem := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		eventTypeClock, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // event type and padding
		clockIDMonotonic, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // clockID
		0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // timeout (ns)
		0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // precision (ns)
		0x00, 0x00, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // flags (relative)
		'?', // stopped after encoding
	}

	expectedMem := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		byte(ErrnoSuccess), 0x0, // errno is 16 bit
		eventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
		'?', // stopped after encoding
	}

	in := uint32(0)    // past in
	out := uint32(128) // past in
	nsubscriptions := uint32(1)
	resultNevents := uint32(512) // past out

	maskMemory(t, testCtx, mod, 1024)
	mod.Memory().Write(testCtx, in, mem)

	requireErrno(t, ErrnoSuccess, mod, functionPollOneoff, uint64(in), uint64(out), uint64(nsubscriptions),
		uint64(resultNevents))
	require.Equal(t, `
--> proxy.poll_oneoff(in=0,out=128,nsubscriptions=1,result.nevents=512)
	==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=1,result.nevents=512)
	<== ESUCCESS
<-- (0)
`, "\n"+log.String())

	outMem, ok := mod.Memory().Read(testCtx, out, uint32(len(expectedMem)))
	require.True(t, ok)
	require.Equal(t, expectedMem, outMem)

	nevents, ok := mod.Memory().ReadUint32Le(testCtx, resultNevents)
	require.True(t, ok)
	require.Equal(t, nsubscriptions, nevents)
}

func Test_pollOneoff_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	tests := []struct {
		name                                   string
		in, out, nsubscriptions, resultNevents uint32
		mem                                    []byte // at offset in
		expectedErrno                          Errno
		expectedMem                            []byte // at offset out
		expectedLog                            string
	}{
		{
			name:           "in out of range",
			in:             wasm.MemoryPageSize,
			nsubscriptions: 1,
			out:            128, // past in
			resultNevents:  512, //past out
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.poll_oneoff(in=65536,out=128,nsubscriptions=1,result.nevents=512)
	==> wasi_snapshot_preview1.poll_oneoff(in=65536,out=128,nsubscriptions=1,result.nevents=512)
	<== EFAULT
<-- (21)
`,
		},
		{
			name:           "out out of range",
			out:            wasm.MemoryPageSize,
			resultNevents:  512, //past out
			nsubscriptions: 1,
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.poll_oneoff(in=0,out=65536,nsubscriptions=1,result.nevents=512)
	==> wasi_snapshot_preview1.poll_oneoff(in=0,out=65536,nsubscriptions=1,result.nevents=512)
	<== EFAULT
<-- (21)
`,
		},
		{
			name:           "resultNevents out of range",
			resultNevents:  wasm.MemoryPageSize,
			nsubscriptions: 1,
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.poll_oneoff(in=0,out=0,nsubscriptions=1,result.nevents=65536)
	==> wasi_snapshot_preview1.poll_oneoff(in=0,out=0,nsubscriptions=1,result.nevents=65536)
	<== EFAULT
<-- (21)
`,
		},
		{
			name:          "nsubscriptions zero",
			out:           128, // past in
			resultNevents: 512, //past out
			expectedErrno: ErrnoInval,
			expectedLog: `
--> proxy.poll_oneoff(in=0,out=128,nsubscriptions=0,result.nevents=512)
	==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=0,result.nevents=512)
	<== EINVAL
<-- (28)
`,
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
			expectedLog: `
--> proxy.poll_oneoff(in=0,out=128,nsubscriptions=1,result.nevents=512)
	==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=1,result.nevents=512)
	<== ESUCCESS
<-- (0)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, testCtx, mod, 1024)

			if tc.mem != nil {
				mod.Memory().Write(testCtx, tc.in, tc.mem)
			}

			requireErrno(t, tc.expectedErrno, mod, functionPollOneoff, uint64(tc.in), uint64(tc.out),
				uint64(tc.nsubscriptions), uint64(tc.resultNevents))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

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
