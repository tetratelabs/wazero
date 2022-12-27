package gojs

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/gojs/goarch"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Debug has unknown use, so stubbed.
//
// See https://github.com/golang/go/blob/go1.19/src/cmd/link/internal/wasm/asm.go#L133-L138
var Debug = goarch.StubFunction(custom.NameDebug)

// WasmExit implements runtime.wasmExit which supports runtime.exit.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.go#L28
var WasmExit = goarch.NewFunc(custom.NameRuntimeWasmExit, wasmExit)

func wasmExit(ctx context.Context, mod api.Module, stack goarch.Stack) {
	code := stack.ParamUint32(0)

	getState(ctx).clear()
	_ = mod.CloseWithExitCode(ctx, code) // TODO: should ours be signed bit (like -1 == 255)?
}

// WasmWrite implements runtime.wasmWrite which supports runtime.write and
// runtime.writeErr. This implements `println`.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/os_js.go#L29
var WasmWrite = goarch.NewFunc(custom.NameRuntimeWasmWrite, wasmWrite)

func wasmWrite(_ context.Context, mod api.Module, stack goarch.Stack) {
	fd := stack.ParamUint32(0)
	p := stack.ParamBytes(mod.Memory(), 1 /*, 2 */)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	writer := fsc.FdWriter(fd)
	if writer == nil {
		panic(fmt.Errorf("unexpected fd %d", fd))
	}

	if _, err := writer.Write(p); err != nil {
		panic(fmt.Errorf("error writing p: %w", err))
	}
}

// ResetMemoryDataView signals wasm.OpcodeMemoryGrow happened, indicating any
// cached view of memory should be reset.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/mem_js.go#L82
//
// TODO: Compiler-based memory.grow callbacks are ignored until we have a generic solution #601
var ResetMemoryDataView = goarch.NoopFunction(custom.NameRuntimeResetMemoryDataView)

// Nanotime1 implements runtime.nanotime which supports time.Since.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L184
var Nanotime1 = goarch.NewFunc(custom.NameRuntimeNanotime1, nanotime1)

func nanotime1(_ context.Context, mod api.Module, stack goarch.Stack) {
	nsec := mod.(*wasm.CallContext).Sys.Nanotime()

	stack.SetResultI64(0, nsec)
}

// Walltime implements runtime.walltime which supports time.Now.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L188
var Walltime = goarch.NewFunc(custom.NameRuntimeWalltime, walltime)

func walltime(_ context.Context, mod api.Module, stack goarch.Stack) {
	sec, nsec := mod.(*wasm.CallContext).Sys.Walltime()

	stack.SetResultI64(0, sec)
	stack.SetResultI32(1, nsec)
}

// ScheduleTimeoutEvent implements runtime.scheduleTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// Unlike other most functions prefixed by "runtime.", this both launches a
// goroutine and invokes code compiled into wasm "resume".
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L192
var ScheduleTimeoutEvent = goarch.StubFunction(custom.NameRuntimeScheduleTimeoutEvent)

// ^^ stubbed because signal handling is not implemented in GOOS=js

// ClearTimeoutEvent implements runtime.clearTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L196
var ClearTimeoutEvent = goarch.StubFunction(custom.NameRuntimeClearTimeoutEvent)

// ^^ stubbed because signal handling is not implemented in GOOS=js

// GetRandomData implements runtime.getRandomData, which initializes the seed
// for runtime.fastrand.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L200
var GetRandomData = goarch.NewFunc(custom.NameRuntimeGetRandomData, getRandomData)

func getRandomData(_ context.Context, mod api.Module, stack goarch.Stack) {
	r := stack.ParamBytes(mod.Memory(), 0 /*, 1 */)

	randSource := mod.(*wasm.CallContext).Sys.RandSource()

	bufLen := len(r)
	if n, err := randSource.Read(r); err != nil {
		panic(fmt.Errorf("RandSource.Read(r /* len=%d */) failed: %w", bufLen, err))
	} else if n != bufLen {
		panic(fmt.Errorf("RandSource.Read(r /* len=%d */) read %d bytes", bufLen, n))
	}
}
