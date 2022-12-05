package wasi_snapshot_preview1

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	clockResGetName  = "clock_res_get"
	clockTimeGetName = "clock_time_get"
)

// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clockid-enumu32
const (
	// clockIDRealtime is the name ID named "realtime" like sys.Walltime
	clockIDRealtime = iota
	// clockIDMonotonic is the name ID named "monotonic" like sys.Nanotime
	clockIDMonotonic
	// Note: clockIDProcessCputime and clockIDThreadCputime were removed by
	// WASI maintainers: https://github.com/WebAssembly/wasi-libc/pull/294
)

// clockResGet is the WASI function named clockResGetName that returns the
// resolution of time values returned by clockTimeGet.
//
// # Parameters
//
//   - id: clock ID to use
//   - resultResolution: offset to write the resolution to api.Memory
//   - the resolution is an uint64 little-endian encoding
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoNotsup: the clock ID is not supported.
//   - ErrnoInval: the clock ID is invalid.
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if the resolution is 100ns, this function writes the below to
// api.Memory:
//
//	                                   uint64le
//	                   +-------------------------------------+
//	                   |                                     |
//	         []byte{?, 0x64, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ?}
//	resultResolution --^
//
// Note: This is similar to `clock_getres` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
// See https://linux.die.net/man/3/clock_getres
var clockResGet = proxyResultParams(&wasm.HostFunc{
	Name:        "clockResGet",
	ParamTypes:  []api.ValueType{i32},
	ParamNames:  []string{"id"},
	ResultTypes: []api.ValueType{i64, i32},
	ResultNames: []string{"resolution", "errno"},
	Code:        &wasm.Code{IsHostFunction: true, GoFunc: u64ResultParam(clockResGetFn)},
}, clockResGetName)

func clockResGetFn(_ context.Context, mod api.Module, stack []uint64) (resolution uint64, errno Errno) {
	sysCtx := mod.(*wasm.CallContext).Sys
	id := uint32(stack[0])

	errno = ErrnoSuccess
	switch id {
	case clockIDRealtime:
		resolution = uint64(sysCtx.WalltimeResolution())
	case clockIDMonotonic:
		resolution = uint64(sysCtx.NanotimeResolution())
	default:
		errno = ErrnoInval
	}
	return
}

// clockTimeGet is the WASI function named clockTimeGetName that returns
// the time value of a name (time.Now).
//
// # Parameters
//
//   - id: clock ID to use
//   - precision: maximum lag (exclusive) that the returned time value may have,
//     compared to its actual value
//   - resultTimestamp: offset to write the timestamp to api.Memory
//   - the timestamp is epoch nanos encoded as a little-endian uint64
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoNotsup: the clock ID is not supported.
//   - ErrnoInval: the clock ID is invalid.
//   - ErrnoFault: there is not enough memory to write results
//
// For example, if time.Now returned exactly midnight UTC 2022-01-01
// (1640995200000000000), and parameters resultTimestamp=1, this function
// writes the below to api.Memory:
//
//	                                    uint64le
//	                  +------------------------------------------+
//	                  |                                          |
//	        []byte{?, 0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, ?}
//	resultTimestamp --^
//
// Note: This is similar to `clock_gettime` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
// See https://linux.die.net/man/3/clock_gettime
var clockTimeGet = proxyResultParams(&wasm.HostFunc{
	Name:        "clockTimeGet",
	ParamTypes:  []api.ValueType{i32, i64},
	ParamNames:  []string{"id", "precision"},
	ResultTypes: []api.ValueType{i64, i32},
	ResultNames: []string{"timestamp", "errno"},
	Code:        &wasm.Code{IsHostFunction: true, GoFunc: i64ResultParam(clockTimeGetFn)},
}, clockTimeGetName)

func clockTimeGetFn(ctx context.Context, mod api.Module, stack []uint64) (timestamp int64, errno Errno) {
	sysCtx := mod.(*wasm.CallContext).Sys
	id := uint32(stack[0])
	// TODO: precision is currently ignored.
	// precision = params[1]

	switch id {
	case clockIDRealtime:
		sec, nsec := sysCtx.Walltime(ctx)
		timestamp = (sec * time.Second.Nanoseconds()) + int64(nsec)
	case clockIDMonotonic:
		timestamp = sysCtx.Nanotime(ctx)
	default:
		return 0, ErrnoInval
	}
	errno = ErrnoSuccess
	return
}
