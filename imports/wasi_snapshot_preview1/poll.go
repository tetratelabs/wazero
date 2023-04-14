package wasi_snapshot_preview1

import (
	"context"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// pollOneoff is the WASI function named PollOneoffName that concurrently
// polls for the occurrence of a set of events.
//
// # Parameters
//
//   - in: pointer to the subscriptions (48 bytes each)
//   - out: pointer to the resulting events (32 bytes each)
//   - nsubscriptions: count of subscriptions, zero returns syscall.EINVAL.
//   - resultNevents: count of events.
//
// Result (Errno)
//
// The return value is 0 except the following error conditions:
//   - syscall.EINVAL: the parameters are invalid
//   - syscall.ENOTSUP: a parameters is valid, but not yet supported.
//   - syscall.EFAULT: there is not enough memory to read the subscriptions or
//     write results.
//
// # Notes
//
//   - Since the `out` pointer nests Errno, the result is always 0.
//   - This is similar to `poll` in POSIX.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#poll_oneoff
// See https://linux.die.net/man/3/poll
var pollOneoff = newHostFunc(
	wasip1.PollOneoffName, pollOneoffFn,
	[]api.ValueType{i32, i32, i32, i32},
	"in", "out", "nsubscriptions", "result.nevents",
)

type event struct {
	eventType byte
	userData  []byte
	errno     wasip1.Errno
	outOffset uint32
}

func pollOneoffFn(ctx context.Context, mod api.Module, params []uint64) syscall.Errno {
	in := uint32(params[0])
	out := uint32(params[1])
	nsubscriptions := uint32(params[2])
	resultNevents := uint32(params[3])

	if nsubscriptions == 0 {
		return syscall.EINVAL
	}

	mem := mod.Memory()

	// Ensure capacity prior to the read loop to reduce error handling.
	inBuf, ok := mem.Read(in, nsubscriptions*48)
	if !ok {
		return syscall.EFAULT
	}
	outBuf, ok := mem.Read(out, nsubscriptions*32)
	if !ok {
		return syscall.EFAULT
	}

	// Eagerly write the number of events which will equal subscriptions unless
	// there's a fault in parsing (not processing).
	if !mod.Memory().WriteUint32Le(resultNevents, nsubscriptions) {
		return syscall.EFAULT
	}

	// Loop through all subscriptions and write their output.

	var stdinSubs []*event
	var timeout time.Duration = 1<<63 - 1 // max timeout
	readySubs := 0
	var reader *internalsys.StdioFileReader

	// Layout is subscription_u: Union
	// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#subscription_u
	for i := uint32(0); i < nsubscriptions; i++ {
		inOffset := i * 48
		outOffset := i * 32

		eventType := inBuf[inOffset+8] // +8 past userdata
		// +8 past userdata +8 contents_offset
		argBuf := inBuf[inOffset+8+8:]
		userData := inBuf[inOffset : inOffset+8]

		evt := &event{
			eventType: eventType,
			userData:  userData,
			errno:     wasip1.ErrnoSuccess,
			outOffset: outOffset,
		}

		switch eventType {
		case wasip1.EventTypeClock: // handle later
			newTimeout, err := processClockEvent(argBuf)
			if err != 0 {
				return err
			}
			if newTimeout < timeout {
				timeout = newTimeout
			}
			writeEvent(outBuf, evt)
		case wasip1.EventTypeFdRead, wasip1.EventTypeFdWrite:
			fsc := mod.(*wasm.ModuleInstance).Sys.FS()
			fd := le.Uint32(argBuf)

			reader = processFDEvent(fsc, fd, evt)

			if reader != nil {
				// delay processing with the timeout
				stdinSubs = append(stdinSubs, evt)
			} else {
				// if stdinReader is not an interactive session, then write back immediately
				readySubs++
				writeEvent(outBuf, evt)

			}
		default:
			return syscall.EINVAL
		}
	}

	// process timeout and interactive inputs (if any)
	if readySubs != 0 {
		return 0
	}

	if len(stdinSubs) > 0 {
		stdinReady, err := reader.Poll(timeout)
		if err != nil {
			if err, ok := err.(syscall.Errno); ok {
				return err
			} else {
				return syscall.EBADF
			}
		}
		if stdinReady {
			for i := range stdinSubs {
				evt := stdinSubs[i]
				evt.errno = 0
				writeEvent(outBuf, evt)
			}
		}
	} else {
		err := platform.SelectTimeout(timeout)
		if err != nil {
			if err, ok := err.(syscall.Errno); ok {
				return err
			} else {
				return syscall.EBADF
			}
		}

	}

	return 0
}

// processClockEvent supports only relative name events, as that's what's used
// to implement sleep in various compilers including Rust, Zig and TinyGo.
func processClockEvent(inBuf []byte) (time.Duration, syscall.Errno) {
	_ /* ID */ = le.Uint32(inBuf[0:8])          // See below
	timeout := le.Uint64(inBuf[8:16])           // nanos if relative
	_ /* precision */ = le.Uint64(inBuf[16:24]) // Unused
	flags := le.Uint16(inBuf[24:32])

	var err syscall.Errno
	// subclockflags has only one flag defined:  subscription_clock_abstime
	switch flags {
	case 0: // relative time
	case 1: // subscription_clock_abstime
		err = syscall.ENOTSUP
	default: // subclockflags has only one flag defined.
		err = syscall.EINVAL
	}

	if err != 0 {
		return 0, err
	} else {
		// https://linux.die.net/man/3/clock_settime says relative timers are
		// unaffected. Since this function only supports relative timeout, we can
		// skip name ID validation and use a single sleep function.

		return time.Duration(timeout), 0
	}
}

func processFDEvent(fsc *internalsys.FSContext, fd uint32, e *event) *internalsys.StdioFileReader {
	if e.eventType == wasip1.EventTypeFdRead {
		if f, ok := fsc.LookupFile(fd); ok {
			if reader, ok := f.File.(*internalsys.StdioFileReader); ok {
				return reader
			}
			e.errno = wasip1.ErrnoSuccess
		} else {
			e.errno = wasip1.ErrnoBadf
		}
	} else if e.eventType == wasip1.EventTypeFdWrite && internalsys.WriterForFile(fsc, fd) == nil {
		e.errno = wasip1.ErrnoBadf
	}
	return nil
}

// writeEvent writes the event corresponding to the processed subscription.
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-event-struct
func writeEvent(outBuf []byte, evt *event) {
	copy(outBuf[evt.outOffset:], evt.userData) // userdata
	outBuf[evt.outOffset+8] = byte(evt.errno)  // uint16, but safe as < 255
	outBuf[evt.outOffset+9] = 0
	le.PutUint32(outBuf[evt.outOffset+10:], uint32(evt.eventType))
	// TODO: When FD events are supported, write outOffset+16
}
