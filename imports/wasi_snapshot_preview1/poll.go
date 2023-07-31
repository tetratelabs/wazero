package wasi_snapshot_preview1

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental/sys"
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
//   - nsubscriptions: count of subscriptions, zero returns sys.EINVAL.
//   - resultNevents: count of events.
//
// Result (Errno)
//
// The return value is 0 except the following error conditions:
//   - sys.EINVAL: the parameters are invalid
//   - sys.ENOTSUP: a parameters is valid, but not yet supported.
//   - sys.EFAULT: there is not enough memory to read the subscriptions or
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

type pollEvent struct {
	eventType byte
	userData  []byte
	errno     wasip1.Errno
	outOffset uint32
}

type filePollEvent struct {
	f *internalsys.FileEntry
	e *pollEvent
}

func pollOneoffFn(_ context.Context, mod api.Module, params []uint64) sys.Errno {
	in := uint32(params[0])
	out := uint32(params[1])
	nsubscriptions := uint32(params[2])
	resultNevents := uint32(params[3])

	if nsubscriptions == 0 {
		return sys.EINVAL
	}

	mem := mod.Memory()

	// Ensure capacity prior to the read loop to reduce error handling.
	inBuf, ok := mem.Read(in, nsubscriptions*48)
	if !ok {
		return sys.EFAULT
	}
	outBuf, ok := mem.Read(out, nsubscriptions*32)
	// zero-out all buffer before writing
	for i := range outBuf {
		outBuf[i] = 0
	}

	if !ok {
		return sys.EFAULT
	}

	// Eagerly write the number of events which will equal subscriptions unless
	// there's a fault in parsing (not processing).
	if !mod.Memory().WriteUint32Le(resultNevents, nsubscriptions) {
		return sys.EFAULT
	}

	// Loop through all subscriptions and write their output.

	// Extract FS context, used in the body of the for loop for FS access.
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	// Slice of events that are processed out of the loop (blocking subscribers).
	var blockingSubs []*filePollEvent
	// The timeout is initialized at max Duration, the loop will find the minimum.
	var timeout time.Duration = 1<<63 - 1
	// Count of all the clock subscribers that have been already written back to outBuf.
	clockEvents := uint32(0)
	// Count of all the non-clock subscribers that have been already written back to outBuf.
	readySubs := uint32(0)

	// Layout is subscription_u: Union
	// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#subscription_u
	for i := uint32(0); i < nsubscriptions; i++ {
		inOffset := i * 48
		outOffset := i * 32

		eventType := inBuf[inOffset+8] // +8 past userdata
		// +8 past userdata +8 contents_offset
		argBuf := inBuf[inOffset+8+8:]
		userData := inBuf[inOffset : inOffset+8]

		evt := &pollEvent{
			eventType: eventType,
			userData:  userData,
			errno:     wasip1.ErrnoSuccess,
			outOffset: outOffset,
		}

		switch eventType {
		case wasip1.EventTypeClock: // handle later
			clockEvents++
			newTimeout, err := processClockEvent(argBuf)
			if err != 0 {
				return err
			}
			// Min timeout.
			if newTimeout < timeout {
				timeout = newTimeout
			}
			// Ack the clock event to the outBuf.
			writeEvent(outBuf, evt)
		case wasip1.EventTypeFdRead:
			fd := int32(le.Uint32(argBuf))
			if fd < 0 {
				return sys.EBADF
			}
			if file, ok := fsc.LookupFile(fd); !ok {
				evt.errno = wasip1.ErrnoBadf
				writeEvent(outBuf, evt)
				readySubs++
			} else if !file.File.IsNonblock() {
				// If the fd is blocking, do not ack yet,
				// append to a slice for delayed evaluation.
				fe := &filePollEvent{f: file, e: evt}
				blockingSubs = append(blockingSubs, fe)
			} else {
				writeEvent(outBuf, evt)
				readySubs++
			}
		case wasip1.EventTypeFdWrite:
			fd := int32(le.Uint32(argBuf))
			if fd < 0 {
				return sys.EBADF
			}
			if _, ok := fsc.LookupFile(fd); ok {
				evt.errno = wasip1.ErrnoNotsup
			} else {
				evt.errno = wasip1.ErrnoBadf
			}
			readySubs++
			writeEvent(outBuf, evt)
		default:
			return sys.EINVAL
		}
	}

	sysCtx := mod.(*wasm.ModuleInstance).Sys

	// There are no blocking subscribers, we just wait for the given timeout.
	if len(blockingSubs) == 0 {
		sysCtx.Nanosleep(int64(timeout))
	} else {
		// If there are blocking subscribers, check the fds using poll.
		n, errno := pollFileEventsOnce(blockingSubs, outBuf)
		if errno != 0 {
			return errno
		}
		readySubs += n

		// If there are any subscribers ready, including those we checked earlier,
		// we don't need to poll further; otherwise, poll until the given timeout.
		if readySubs == 0 {
			readySubs, errno = pollFileEventsUntil(sysCtx, timeout, blockingSubs, outBuf)
			if errno != 0 {
				return errno
			}
		}
	}

	if readySubs != nsubscriptions {
		if !mod.Memory().WriteUint32Le(resultNevents, readySubs+clockEvents) {
			return sys.EFAULT
		}
	}
	return 0
}

// processClockEvent supports only relative name events, as that's what's used
// to implement sleep in various compilers including Rust, Zig and TinyGo.
func processClockEvent(inBuf []byte) (time.Duration, sys.Errno) {
	_ /* ID */ = le.Uint32(inBuf[0:8])          // See below
	timeout := le.Uint64(inBuf[8:16])           // nanos if relative
	_ /* precision */ = le.Uint64(inBuf[16:24]) // Unused
	flags := le.Uint16(inBuf[24:32])

	var err sys.Errno
	// subclockflags has only one flag defined:  subscription_clock_abstime
	switch flags {
	case 0: // relative time
	case 1: // subscription_clock_abstime
		err = sys.ENOTSUP
	default: // subclockflags has only one flag defined.
		err = sys.EINVAL
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

// writeEvent writes the event corresponding to the processed subscription.
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-event-struct
func writeEvent(outBuf []byte, evt *pollEvent) {
	copy(outBuf[evt.outOffset:], evt.userData) // userdata
	outBuf[evt.outOffset+8] = byte(evt.errno)  // uint16, but safe as < 255
	outBuf[evt.outOffset+9] = 0
	le.PutUint32(outBuf[evt.outOffset+10:], uint32(evt.eventType))
	// TODO: When FD events are supported, write outOffset+16
}

// closeChAfter closes a channel after the given timeout.
// It is similar to time.After but it uses sysCtx.Nanosleep.
func closeChAfter(sysCtx *internalsys.Context, timeout time.Duration, timeoutCh chan struct{}) {
	sysCtx.Nanosleep(int64(timeout))
	close(timeoutCh)
}

// pollFileEventsOnce invokes Poll on each sys.FileEntry in the given slice
// and writes back the result to outBuf for each file reported "ready";
// i.e., when Poll() returns true, and no error.
func pollFileEventsOnce(evts []*filePollEvent, outBuf []byte) (n uint32, errno sys.Errno) {
	// For simplicity, we assume that there are no multiple subscriptions for the same file.
	for _, e := range evts {
		isReady, errno := e.f.File.Poll(sys.POLLIN, 0)
		if errno != 0 {
			return 0, errno
		}
		if isReady {
			e.e.errno = 0
			writeEvent(outBuf, e.e)
			n++
		}
	}
	return
}

// pollFileEventsUntil repeatedly invokes pollFileEventsOnce until the given timeout is reached.
// The poll interval is currently fixed at 100 millis.
func pollFileEventsUntil(sysCtx *internalsys.Context, timeout time.Duration, blockingSubs []*filePollEvent, outBuf []byte) (n uint32, errno sys.Errno) {
	timeoutCh := make(chan struct{}, 1)
	go closeChAfter(sysCtx, timeout, timeoutCh)

	pollInterval := 100 * time.Millisecond
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			// Give one last chance before returning.
			return pollFileEventsOnce(blockingSubs, outBuf)
		case <-ticker.C:
			n, errno = pollFileEventsOnce(blockingSubs, outBuf)
			if errno != 0 || n > 0 {
				return
			}
		}
	}
}
