package wasi_snapshot_preview1

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionClockResGet  = "clock_res_get"
	functionClockTimeGet = "clock_time_get"
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

// clockResGet is the WASI function named functionClockResGet that returns the
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
var clockResGet = &wasm.HostFunc{
	ExportNames: []string{functionClockResGet},
	Name:        functionClockResGet,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"id", "result.resolution"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(clockResGetFn),
	},
}

func clockResGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	id, resultResolution := uint32(params[0]), uint32(params[1])

	var resolution uint64 // ns
	switch id {
	case clockIDRealtime:
		resolution = uint64(sysCtx.WalltimeResolution())
	case clockIDMonotonic:
		resolution = uint64(sysCtx.NanotimeResolution())
	default:
		return ErrnoInval
	}

	if !mod.Memory().WriteUint64Le(ctx, resultResolution, resolution) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// clockTimeGet is the WASI function named functionClockTimeGet that returns
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
var clockTimeGet = &wasm.HostFunc{
	ExportNames: []string{functionClockTimeGet},
	Name:        functionClockTimeGet,
	ParamTypes:  []api.ValueType{i32, i64, i32},
	ParamNames:  []string{"id", "precision", "result.timestamp"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(clockTimeGetFn),
	},
}

func clockTimeGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	id := uint32(params[0])
	// TODO: precision is currently ignored.
	// precision = params[1]
	resultTimestamp := uint32(params[2])

	var val uint64
	switch id {
	case clockIDRealtime:
		sec, nsec := sysCtx.Walltime(ctx)
		val = (uint64(sec) * uint64(time.Second.Nanoseconds())) + uint64(nsec)
	case clockIDMonotonic:
		val = uint64(sysCtx.Nanotime(ctx))
	default:
		return ErrnoInval
	}

	if !mod.Memory().WriteUint64Le(ctx, resultTimestamp, val) {
		return ErrnoFault
	}
	return ErrnoSuccess
}
