package wasi_snapshot_preview1

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	// functionClockResGet returns the resolution of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	functionClockResGet = "clock_res_get"

	// importClockResGet is the WebAssembly 1.0 Text format import of functionClockResGet.
	importClockResGet = `(import "wasi_snapshot_preview1" "clock_res_get"
    (func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))`

	// functionClockTimeGet returns the time value of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	functionClockTimeGet = "clock_time_get"

	// importClockTimeGet is the WebAssembly 1.0 Text format import of functionClockTimeGet.
	importClockTimeGet = `(import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))`
)

// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clockid-enumu32
const (
	// clockIDRealtime is the clock ID named "realtime" associated with sys.Walltime
	clockIDRealtime = iota
	// clockIDMonotonic is the clock ID named "monotonic" with sys.Nanotime
	clockIDMonotonic
	// clockIDProcessCputime is the unsupported clock ID named "process_cputime_id"
	clockIDProcessCputime
	// clockIDThreadCputime is the unsupported clock ID named "thread_cputime_id"
	clockIDThreadCputime
)

// ClockResGet is the WASI function named functionClockResGet that returns the resolution of time values returned by ClockTimeGet.
//
// * id - The clock id for which to return the time.
// * resultResolution - the offset to write the resolution to mod.Memory
//   * the resolution is an uint64 little-endian encoding.
//
// For example, if the resolution is 100ns, this function writes the below to `mod.Memory`:
//
//                                      uint64le
//                    +-------------------------------------+
//                    |                                     |
//          []byte{?, 0x64, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ?}
//  resultResolution --^
//
// Note: importClockResGet shows this signature in the WebAssembly 1.0 Text Format.
// Note: This is similar to `clock_getres` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
// See https://linux.die.net/man/3/clock_getres
func (a *wasi) ClockResGet(ctx context.Context, mod api.Module, id uint32, resultResolution uint32) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys

	var resolution uint64 // ns
	switch id {
	case clockIDRealtime:
		resolution = uint64(sysCtx.WalltimeResolution())
	case clockIDMonotonic:
		resolution = uint64(sysCtx.NanotimeResolution())
	case clockIDProcessCputime, clockIDThreadCputime:
		// Similar to many other runtimes, we only support realtime and monotonic clocks. Other types
		// are slated to be removed from the next version of WASI.
		return ErrnoNotsup
	default:
		return ErrnoInval
	}
	if !mod.Memory().WriteUint64Le(ctx, resultResolution, resolution) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// ClockTimeGet is the WASI function named functionClockTimeGet that returns the time value of a clock (time.Now).
//
// * id - The clock id for which to return the time.
// * precision - The maximum lag (exclusive) that the returned time value may have, compared to its actual value.
// * resultTimestamp - the offset to write the timestamp to mod.Memory
//   * the timestamp is epoch nanoseconds encoded as a uint64 little-endian encoding.
//
// For example, if time.Now returned exactly midnight UTC 2022-01-01 (1640995200000000000), and
//   parameters resultTimestamp=1, this function writes the below to `mod.Memory`:
//
//                                      uint64le
//                    +------------------------------------------+
//                    |                                          |
//          []byte{?, 0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, ?}
//  resultTimestamp --^
//
// Note: importClockTimeGet shows this signature in the WebAssembly 1.0 Text Format.
// Note: This is similar to `clock_gettime` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
// See https://linux.die.net/man/3/clock_gettime
func (a *wasi) ClockTimeGet(ctx context.Context, mod api.Module, id uint32, precision uint64, resultTimestamp uint32) Errno {
	// TODO: precision is currently ignored.
	sysCtx := mod.(*wasm.CallContext).Sys

	var val uint64
	switch id {
	case clockIDRealtime:
		sec, nsec := sysCtx.Walltime(ctx)
		val = (uint64(sec) * uint64(time.Second.Nanoseconds())) + uint64(nsec)
	case clockIDMonotonic:
		val = uint64(sysCtx.Nanotime(ctx))
	case clockIDProcessCputime, clockIDThreadCputime:
		// Similar to many other runtimes, we only support realtime and monotonic clocks. Other types
		// are slated to be removed from the next version of WASI.
		return ErrnoNotsup
	default:
		return ErrnoInval
	}

	if !mod.Memory().WriteUint64Le(ctx, resultTimestamp, val) {
		return ErrnoFault
	}
	return ErrnoSuccess
}
