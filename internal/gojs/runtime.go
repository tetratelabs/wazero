package gojs

import (
	"context"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/spfunc"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionWasmExit             = "runtime.wasmExit"
	functionWasmWrite            = "runtime.wasmWrite"
	functionResetMemoryDataView  = "runtime.resetMemoryDataView"
	functionNanotime1            = "runtime.nanotime1"
	functionWalltime             = "runtime.walltime"
	functionScheduleTimeoutEvent = "runtime.scheduleTimeoutEvent" // TODO: trigger usage
	functionClearTimeoutEvent    = "runtime.clearTimeoutEvent"    // TODO: trigger usage
	functionGetRandomData        = "runtime.getRandomData"
)

// WasmExit implements runtime.wasmExit which supports runtime.exit.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.go#L28
var WasmExit = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionWasmExit, functionWasmExit,
	[]string{"code"},
	func(ctx context.Context, mod api.Module, code int32) {
		getState(ctx).clear()
		_ = mod.CloseWithExitCode(ctx, uint32(code)) // TODO: should ours be signed bit (like -1 == 255)?
	},
))

// WasmWrite implements runtime.wasmWrite which supports runtime.write and
// runtime.writeErr. This implements `println`.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/os_js.go#L29
var WasmWrite = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionWasmWrite, functionWasmWrite,
	[]string{"code"},
	func(ctx context.Context, mod api.Module, fd, p, n uint32) {
		var writer io.Writer

		switch fd {
		case 1:
			writer = mod.(*wasm.CallContext).Sys.Stdout()
		case 2:
			writer = mod.(*wasm.CallContext).Sys.Stderr()
		default:
			// Keep things simple by expecting nothing past 2
			panic(fmt.Errorf("unexpected fd %d", fd))
		}

		if _, err := writer.Write(mustRead(ctx, mod.Memory(), "p", p, n)); err != nil {
			panic(fmt.Errorf("error writing p: %w", err))
		}
	},
))

// ResetMemoryDataView signals wasm.OpcodeMemoryGrow happened, indicating any
// cached view of memory should be reset.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/mem_js.go#L82
var ResetMemoryDataView = &wasm.HostFunc{
	ExportNames: []string{functionResetMemoryDataView},
	Name:        functionResetMemoryDataView,
	ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
	ParamNames:  []string{parameterSp},
	// TODO: Compiler-based memory.grow callbacks are ignored until we have a generic solution #601
	Code: &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeEnd}},
}

// Nanotime1 implements runtime.nanotime which supports time.Since.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L184
var Nanotime1 = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionNanotime1, functionNanotime1,
	[]string{},
	func(ctx context.Context, mod api.Module) int64 {
		return mod.(*wasm.CallContext).Sys.Nanotime(ctx)
	}))

// Walltime implements runtime.walltime which supports time.Now.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L188
var Walltime = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionWalltime, functionWalltime,
	[]string{},
	func(ctx context.Context, mod api.Module) (sec int64, nsec int32) {
		return mod.(*wasm.CallContext).Sys.Walltime(ctx)
	}))

// ScheduleTimeoutEvent implements runtime.scheduleTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// Unlike other most functions prefixed by "runtime.", this both launches a
// goroutine and invokes code compiled into wasm "resume".
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L192
var ScheduleTimeoutEvent = stubFunction(functionScheduleTimeoutEvent)

// ^^ stubbed because signal handling is not implemented in GOOS=js

// ClearTimeoutEvent implements runtime.clearTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L196
var ClearTimeoutEvent = stubFunction(functionClearTimeoutEvent)

// ^^ stubbed because signal handling is not implemented in GOOS=js

// GetRandomData implements runtime.getRandomData, which initializes the seed
// for runtime.fastrand.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L200
var GetRandomData = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionGetRandomData, functionGetRandomData,
	[]string{"buf", "bufLen"},
	func(ctx context.Context, mod api.Module, buf, bufLen uint32) {
		randSource := mod.(*wasm.CallContext).Sys.RandSource()

		r := mustRead(ctx, mod.Memory(), "r", buf, bufLen)

		if n, err := randSource.Read(r); err != nil {
			panic(fmt.Errorf("RandSource.Read(r /* len=%d */) failed: %w", bufLen, err))
		} else if uint32(n) != bufLen {
			panic(fmt.Errorf("RandSource.Read(r /* len=%d */) read %d bytes", bufLen, n))
		}
	},
))
