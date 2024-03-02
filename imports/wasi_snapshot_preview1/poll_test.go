package wasi_snapshot_preview1_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
	sysapi "github.com/tetratelabs/wazero/sys"
)

func Test_pollOneoff(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	mem := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		wasip1.EventTypeClock, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // event type and padding
		wasip1.ClockIDMonotonic, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // clockID
		0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // timeout (ns)
		0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // precision (ns)
		0x00, 0x00, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // flags (relative)
		'?', // stopped after encoding
	}

	expectedMem := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
		wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, '?', // stopped after encoding
	}

	in := uint32(0)    // past in
	out := uint32(128) // past in
	nsubscriptions := uint32(1)
	resultNevents := uint32(512) // past out

	maskMemory(t, mod, 1024)
	mod.Memory().Write(in, mem)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PollOneoffName, uint64(in), uint64(out), uint64(nsubscriptions),
		uint64(resultNevents))
	require.Equal(t, `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=1)
<== (nevents=1,errno=ESUCCESS)
`, "\n"+log.String())

	outMem, ok := mod.Memory().Read(out, uint32(len(expectedMem)))
	require.True(t, ok)
	require.Equal(t, expectedMem, outMem)

	nevents, ok := mod.Memory().ReadUint32Le(resultNevents)
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
		expectedErrno                          wasip1.Errno
		expectedMem                            []byte // at offset out
		expectedLog                            string
	}{
		{
			name:           "in out of range",
			in:             wasm.MemoryPageSize,
			nsubscriptions: 1,
			out:            128, // past in
			resultNevents:  512, // past out
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=65536,out=128,nsubscriptions=1)
<== (nevents=,errno=EFAULT)
`,
		},
		{
			name:           "out out of range",
			out:            wasm.MemoryPageSize,
			resultNevents:  512, // past out
			nsubscriptions: 1,
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=65536,nsubscriptions=1)
<== (nevents=,errno=EFAULT)
`,
		},
		{
			name:           "resultNevents out of range",
			resultNevents:  wasm.MemoryPageSize,
			nsubscriptions: 1,
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=0,nsubscriptions=1)
<== (nevents=,errno=EFAULT)
`,
		},
		{
			name:          "nsubscriptions zero",
			out:           128, // past in
			resultNevents: 512, // past out
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=0)
<== (nevents=,errno=EINVAL)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, 1024)

			if tc.mem != nil {
				mod.Memory().Write(tc.in, tc.mem)
			}

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PollOneoffName, uint64(tc.in), uint64(tc.out),
				uint64(tc.nsubscriptions), uint64(tc.resultNevents))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			out, ok := mod.Memory().Read(tc.out, uint32(len(tc.expectedMem)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMem, out)

			// Events should be written on success regardless of nested failure.
			if tc.expectedErrno == wasip1.ErrnoSuccess {
				nevents, ok := mod.Memory().ReadUint32Le(tc.resultNevents)
				require.True(t, ok)
				require.Equal(t, uint32(1), nevents)
				_ = nevents
			}
		})
	}
}

func Test_pollOneoff_Stdin(t *testing.T) {
	w, r, err := os.Pipe()
	require.NoError(t, err)
	defer w.Close()
	defer r.Close()
	_, _ = w.Write([]byte("wazero"))

	tests := []struct {
		name                                   string
		in, out, nsubscriptions, resultNevents uint32
		mem                                    []byte // at offset in
		stdin                                  fsapi.File
		expectedErrno                          wasip1.Errno
		expectedMem                            []byte // at offset out
		expectedLog                            string
		expectedNevents                        uint32
	}{
		{
			name:            "Read without explicit timeout (no tty)",
			nsubscriptions:  1,
			expectedNevents: 1,
			stdin:           &sys.StdinFile{Reader: strings.NewReader("test")},
			mem:             fdReadSub,
			expectedErrno:   wasip1.ErrnoSuccess,
			out:             128, // past in
			resultNevents:   512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=1)
<== (nevents=1,errno=ESUCCESS)
`,
		},
		{
			name:            "20ms timeout, fdread on tty (buffer ready): both events are written",
			nsubscriptions:  2,
			expectedNevents: 2,
			stdin:           &ttyStdinFile{StdinFile: sys.StdinFile{Reader: strings.NewReader("test")}},
			mem: concat(
				clockNsSub(20*1000*1000),
				fdReadSub,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:            "0ns timeout, fdread on tty (buffer ready): both are written",
			nsubscriptions:  2,
			expectedNevents: 2,
			stdin:           &ttyStdinFile{StdinFile: sys.StdinFile{Reader: strings.NewReader("test")}},
			mem: concat(
				clockNsSub(20*1000*1000),
				fdReadSub,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:            "0ns timeout, fdread on regular file: both events are written",
			nsubscriptions:  2,
			expectedNevents: 2,
			stdin:           &sys.StdinFile{Reader: strings.NewReader("test")},
			mem: concat(
				clockNsSub(20*1000*1000),
				fdReadSub,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:            "1ns timeout, fdread on regular file: both events are written",
			nsubscriptions:  2,
			expectedNevents: 2,
			stdin:           &sys.StdinFile{Reader: strings.NewReader("test")},
			mem: concat(
				clockNsSub(20*1000*1000),
				fdReadSub,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:            "20ms timeout, fdread on blocked tty: only clock event is written",
			nsubscriptions:  2,
			expectedNevents: 1,
			stdin:           &neverReadyTtyStdinFile{StdinFile: sys.StdinFile{Reader: newBlockingReader(t)}},
			mem: concat(
				clockNsSub(20*1000*1000),
				fdReadSub,
			),

			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				// 32 empty bytes
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=1,errno=ESUCCESS)
`,
		},
		{
			name:            "pollable pipe, multiple subs, events returned out of order",
			nsubscriptions:  3,
			expectedNevents: 3,
			mem: concat(
				fdReadSub,
				clockNsSub(20*1000*1000),
				// Illegal file fd with custom user data to recognize it in the event buffer.
				fdReadSubFdWithUserData(100, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77})),
			stdin:         &sys.StdinFile{Reader: w},
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				// Clock is acknowledged first.
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				// Then an illegal file with custom user data.
				0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, // userdata
				byte(wasip1.ErrnoBadf), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				// Stdin pipes are delayed to invoke sysfs.poll
				// thus, they are written back last.
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0,

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=3)
<== (nevents=3,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
			defer r.Close(testCtx)
			defer log.Reset()

			setStdin(t, mod, tc.stdin)

			maskMemory(t, mod, 1024)
			if tc.mem != nil {
				mod.Memory().Write(tc.in, tc.mem)
			}

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PollOneoffName, uint64(tc.in), uint64(tc.out),
				uint64(tc.nsubscriptions), uint64(tc.resultNevents))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			out, ok := mod.Memory().Read(tc.out, uint32(len(tc.expectedMem)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMem, out)

			// Events should be written on success regardless of nested failure.
			if tc.expectedErrno == wasip1.ErrnoSuccess {
				nevents, ok := mod.Memory().ReadUint32Le(tc.resultNevents)
				require.True(t, ok)
				require.Equal(t, tc.expectedNevents, nevents)
				_ = nevents
			}
		})
	}
}

func setStdin(t *testing.T, mod api.Module, stdin fsapi.File) {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	f, ok := fsc.LookupFile(sys.FdStdin)
	require.True(t, ok)
	f.File = stdin
}

func Test_pollOneoff_Zero(t *testing.T) {
	poller := &pollStdinFile{StdinFile: sys.StdinFile{Reader: strings.NewReader("test")}, ready: true}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)
	defer log.Reset()

	setStdin(t, mod, poller)

	maskMemory(t, mod, 1024)

	out := uint32(128)
	nsubscriptions := 2
	resultNevents := uint32(512)

	mod.Memory().Write(0,
		concat(
			clockNsSub(20*1000*1000),
			fdReadSub,
		),
	)

	expectedMem := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
		wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,

		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
		wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // pad to 32
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0,

		'?', // stopped after encoding
	}

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PollOneoffName, uint64(0), uint64(out),
		uint64(nsubscriptions), uint64(resultNevents))

	outMem, ok := mod.Memory().Read(out, uint32(len(expectedMem)))
	require.True(t, ok)
	require.Equal(t, expectedMem, outMem)

	// Events should be written on success regardless of nested failure.
	nevents, ok := mod.Memory().ReadUint32Le(resultNevents)
	require.True(t, ok)
	require.Equal(t, uint32(2), nevents)

	// second run: simulate no more data on the fd
	poller.ready = false

	mod.Memory().Write(0,
		concat(
			clockNsSub(20*1000*1000),
			fdReadSub,
		),
	)

	expectedMem = []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
		wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,

		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,

		'?', // stopped after encoding
	}

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PollOneoffName, uint64(0), uint64(out),
		uint64(nsubscriptions), uint64(resultNevents))

	outMem, ok = mod.Memory().Read(out, uint32(len(expectedMem)))
	require.True(t, ok)
	require.Equal(t, expectedMem, outMem)

	nevents, ok = mod.Memory().ReadUint32Le(resultNevents)
	require.True(t, ok)
	require.Equal(t, uint32(1), nevents)
}

func concat(bytes ...[]byte) []byte {
	var res []byte
	for i := range bytes {
		res = append(res, bytes[i]...)
	}
	return res
}

// subscription for a given timeout in ns
func clockNsSub(ns uint64) []byte {
	return []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		wasip1.EventTypeClock, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // event type and padding
		wasip1.ClockIDMonotonic, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		byte(ns), byte(ns >> 8), byte(ns >> 16), byte(ns >> 24),
		byte(ns >> 32), byte(ns >> 40), byte(ns >> 48), byte(ns >> 56), // timeout (ns)
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // precision (ns)
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, // flags
	}
}

// subscription for an EventTypeFdRead on a given fd
func fdReadSubFd(fd byte) []byte {
	return []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		fd, 0x0, 0x0, 0x0, // valid readable FD
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, // pad to 32 bytes
	}
}

func fdReadSubFdWithUserData(fd byte, userdata []byte) []byte {
	return concat(
		userdata,
		[]byte{
			wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			fd, 0x0, 0x0, 0x0, // valid readable FD
			0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			0x0, 0x0, 0x0, 0x0, // pad to 32 bytes
		})
}

// subscription for an EventTypeFdRead on stdin
var fdReadSub = fdReadSubFd(byte(sys.FdStdin))

// ttyStat returns fs.ModeCharDevice | fs.ModeCharDevice as an approximation
// for isatty.
//
// See go-isatty for a more specific approach:
// https://github.com/mattn/go-isatty/blob/v0.0.18/isatty_tcgets.go#LL11C1-L12C1
type ttyStat struct{}

// Stat implements the same method as documented on sys.File
func (ttyStat) Stat() (sysapi.Stat_t, experimentalsys.Errno) {
	return sysapi.Stat_t{
		Mode:  fs.ModeDevice | fs.ModeCharDevice,
		Nlink: 1,
	}, 0
}

type ttyStdinFile struct {
	sys.StdinFile
	ttyStat
}

type neverReadyTtyStdinFile struct {
	sys.StdinFile
	ttyStat
}

// Poll implements the same method as documented on sys.File
func (neverReadyTtyStdinFile) Poll(flag fsapi.Pflag, timeoutMillis int32) (ready bool, errno experimentalsys.Errno) {
	if flag != fsapi.POLLIN {
		return false, experimentalsys.ENOTSUP
	}
	switch {
	case timeoutMillis <= 0:
		return
	}
	time.Sleep(time.Duration(timeoutMillis) * time.Millisecond)
	return false, 0
}

type pollStdinFile struct {
	sys.StdinFile
	ttyStat
	ready bool
}

// Poll implements the same method as documented on sys.File
func (p *pollStdinFile) Poll(flag fsapi.Pflag, timeoutMillis int32) (ready bool, errno experimentalsys.Errno) {
	if flag != fsapi.POLLIN {
		return false, experimentalsys.ENOTSUP
	}
	return p.ready, 0
}
