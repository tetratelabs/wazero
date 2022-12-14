package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const pollOneoffName = "poll_oneoff"

// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-eventtype-enumu8
const (
	// eventTypeClock is the timeout event named "name".
	eventTypeClock = iota
	// eventTypeFdRead is the data available event named "fd_read".
	eventTypeFdRead
	// eventTypeFdWrite is the capacity available event named "fd_write".
	eventTypeFdWrite
)

// pollOneoff is the WASI function named pollOneoffName that concurrently
// polls for the occurrence of a set of events.
//
// # Parameters
//
//   - in: pointer to the subscriptions (48 bytes each)
//   - out: pointer to the resulting events (32 bytes each)
//   - nsubscriptions: count of subscriptions, zero returns ErrnoInval.
//   - resultNevents: count of events.
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoInval: the parameters are invalid
//   - ErrnoNotsup: a parameters is valid, but not yet supported.
//   - ErrnoFault: there is not enough memory to read the subscriptions or
//     write results.
//
// # Notes
//
//   - Since the `out` pointer nests Errno, the result is always ErrnoSuccess.
//   - importPollOneoff shows this signature in the WebAssembly 1.0 Text Format.
//   - This is similar to `poll` in POSIX.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#poll_oneoff
// See https://linux.die.net/man/3/poll
var pollOneoff = proxyResultParams(&wasm.HostFunc{
	Name:        "pollOneoff",
	ParamTypes:  []api.ValueType{i32, i32, i32},
	ParamNames:  []string{"in", "out", "nsubscriptions"},
	ResultTypes: []api.ValueType{i32, i32},
	ResultNames: []string{"nevents", "errno"},
	Code:        &wasm.Code{IsHostFunction: true, GoFunc: u32ResultParam(pollOneoffFn)},
}, pollOneoffName)

func pollOneoffFn(ctx context.Context, mod api.Module, params []uint64) (nevents uint32, errno Errno) {
	in := uint32(params[0])
	out := uint32(params[1])
	nsubscriptions := uint32(params[2])

	if nsubscriptions == 0 {
		return 0, ErrnoInval
	}

	mem := mod.Memory()

	// Ensure capacity prior to the read loop to reduce error handling.
	inBuf, ok := mem.Read(ctx, in, nsubscriptions*48)
	if !ok {
		return 0, ErrnoFault
	}
	outBuf, ok := mem.Read(ctx, out, nsubscriptions*32)
	if !ok {
		return 0, ErrnoFault
	}

	// Loop through all subscriptions and write their output.
	for ; nevents < nsubscriptions; nevents++ {
		inOffset := nevents * 48
		outOffset := nevents * 32

		eventType := inBuf[inOffset+8] // +8 past userdata
		var errno Errno                // errno for this specific event
		switch eventType {
		case eventTypeClock: // handle later
			// +8 past userdata +8 name alignment
			errno = processClockEvent(ctx, mod, inBuf[inOffset+8+8:])
		case eventTypeFdRead, eventTypeFdWrite:
			// +8 past userdata +4 FD alignment
			errno = processFDEvent(ctx, mod, eventType, inBuf[inOffset+8+4:])
		default:
			return 0, ErrnoInval
		}

		// Write the event corresponding to the processed subscription.
		// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-event-struct
		copy(outBuf, inBuf[inOffset:inOffset+8]) // userdata
		outBuf[outOffset+8] = byte(errno)        // uint16, but safe as < 255
		outBuf[outOffset+9] = 0
		le.PutUint32(outBuf[outOffset+10:], uint32(eventType))
		// TODO: When FD events are supported, write outOffset+16
	}
	return nevents, ErrnoSuccess
}

// processClockEvent supports only relative name events, as that's what's used
// to implement sleep in various compilers including Rust, Zig and TinyGo.
func processClockEvent(ctx context.Context, mod api.Module, inBuf []byte) Errno {
	_ /* ID */ = le.Uint32(inBuf[0:8])          // See below
	timeout := le.Uint64(inBuf[8:16])           // nanos if relative
	_ /* precision */ = le.Uint64(inBuf[16:24]) // Unused
	flags := le.Uint16(inBuf[24:32])

	// subclockflags has only one flag defined:  subscription_clock_abstime
	switch flags {
	case 0: // relative time
	case 1: // subscription_clock_abstime
		return ErrnoNotsup
	default: // subclockflags has only one flag defined.
		return ErrnoInval
	}

	// https://linux.die.net/man/3/clock_settime says relative timers are
	// unaffected. Since this function only supports relative timeout, we can
	// skip name ID validation and use a single sleep function.

	sysCtx := mod.(*wasm.CallContext).Sys
	sysCtx.Nanosleep(ctx, int64(timeout))
	return ErrnoSuccess
}

// processFDEvent returns a validation error or ErrnoNotsup as file or socket
// subscriptions are not yet supported.
func processFDEvent(_ context.Context, mod api.Module, eventType byte, inBuf []byte) Errno {
	fd := le.Uint32(inBuf)
	fsc := mod.(*wasm.CallContext).Sys.FS()

	// Choose the best error, which falls back to unsupported, until we support
	// files.
	errno := ErrnoNotsup
	if eventType == eventTypeFdRead && fsc.FdReader(fd) == nil {
		errno = ErrnoBadf
	} else if eventType == eventTypeFdWrite && fsc.FdWriter(fd) == nil {
		errno = ErrnoBadf
	}

	return errno
}
