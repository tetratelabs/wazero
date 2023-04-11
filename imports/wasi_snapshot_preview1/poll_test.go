package wasi_snapshot_preview1_test

import (
	"bufio"
	"io/fs"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
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
		'?', // stopped after encoding
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
	// subscription for a given timeout in ns
	clockNsSub := func(ns uint64) []byte {
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

	// subscription for an EventTypeFdRead on stdin
	fdReadSub := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
		wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		byte(sys.FdStdin), 0x0, 0x0, 0x0, // valid readable FD
		'?', // stopped after encoding
	}

	tests := []struct {
		name                                   string
		in, out, nsubscriptions, resultNevents uint32
		mem                                    []byte // at offset in
		stdioReader                            *sys.StdioFileReader
		expectedErrno                          wasip1.Errno
		expectedMem                            []byte // at offset out
		expectedLog                            string
	}{
		{
			name:           "Read without explicit timeout (no tty)",
			nsubscriptions: 1,
			stdioReader: sys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")),
				stdinFileInfo(0o640)), // isatty
			mem:           fdReadSub,
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=1)
<== (nevents=1,errno=ESUCCESS)
`,
		},
		{
			name:           "20ms timeout, fdread on tty (buffer ready): both events are written",
			nsubscriptions: 2,
			stdioReader: sys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")),
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640)), // isatty
			mem: append(
				clockNsSub(20*1000*1000),
				fdReadSub...,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', '?', '?', '?', '?', '?', '?', '?', // pad to 32
				'?', '?', '?', '?', '?', '?', '?', '?',
				'?', '?',

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:           "0ns timeout, fdread on tty (buffer ready): only clock is written",
			nsubscriptions: 2,
			stdioReader: sys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")),
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640)), // isatty
			mem: append(
				clockNsSub(0),
				fdReadSub...,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', '?', '?', '?', '?', '?', '?', '?', // pad to 32
				'?', '?', '?', '?', '?', '?', '?', '?',
				'?', '?',

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:           "0ns timeout, fdread on regular file: both events are written",
			nsubscriptions: 2,
			stdioReader: sys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")),
				stdinFileInfo(0o640)),
			mem: append(
				clockNsSub(0),
				fdReadSub...,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', '?', '?', '?', '?', '?', '?', '?', // pad to 32
				'?', '?', '?', '?', '?', '?', '?', '?',
				'?', '?',

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:           "1ns timeout, fdread on regular file: both events are written",
			nsubscriptions: 2,
			stdioReader: sys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")),
				stdinFileInfo(0o640)),
			mem: append(
				clockNsSub(1),
				fdReadSub...,
			),
			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', '?', '?', '?', '?', '?', '?', '?', // pad to 32
				'?', '?', '?', '?', '?', '?', '?', '?',
				'?', '?',

				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeFdRead, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
		{
			name:           "20ms timeout, fdread on blocked tty: only clock event is written",
			nsubscriptions: 2,
			stdioReader: sys.NewStdioFileReader(
				bufio.NewReader(newBlockingReader(t)),
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640)),
			mem: append(
				clockNsSub(20*1000*1000),
				fdReadSub...,
			),

			expectedErrno: wasip1.ErrnoSuccess,
			out:           128, // past in
			resultNevents: 512, // past out
			expectedMem: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // userdata
				byte(wasip1.ErrnoSuccess), 0x0, // errno is 16 bit
				wasip1.EventTypeClock, 0x0, 0x0, 0x0, // 4 bytes for type enum
				'?', '?', '?', '?', '?', '?', '?', '?', // pad to 32
				'?', '?', '?', '?', '?', '?', '?', '?',
				'?', '?',

				'?', // stopped after encoding
			},
			expectedLog: `
==> wasi_snapshot_preview1.poll_oneoff(in=0,out=128,nsubscriptions=2)
<== (nevents=2,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			tconfig := wazero.NewModuleConfig().WithStdin(tc.stdioReader)
			mod, r, log := requireProxyModule(t, tconfig)
			defer r.Close(testCtx)
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
				// require.Equal(t, uint32(1), nevents)
				_ = nevents
			}
		})
	}
}
