package gojs

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/spfunc"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	i32, i64 = api.ValueTypeI32, api.ValueTypeI64

	wasmExitName             = "runtime.wasmExit"
	wasmWriteName            = "runtime.wasmWrite"
	resetMemoryDataViewName  = "runtime.resetMemoryDataView"
	nanotime1Name            = "runtime.nanotime1"
	walltimeName             = "runtime.walltime"
	scheduleTimeoutEventName = "runtime.scheduleTimeoutEvent" // TODO: trigger usage
	clearTimeoutEventName    = "runtime.clearTimeoutEvent"    // TODO: trigger usage
	getRandomDataName        = "runtime.getRandomData"
)

// WasmExit implements runtime.wasmExit which supports runtime.exit.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.go#L28
var WasmExit = spfunc.MustCallFromSP(false, &wasm.HostFunc{
	ExportNames: []string{wasmExitName},
	Name:        wasmExitName,
	ParamTypes:  []api.ValueType{i32},
	ParamNames:  []string{"code"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(wasmExit),
	},
})

func wasmExit(ctx context.Context, mod api.Module, stack []uint64) {
	code := uint32(stack[0])

	getState(ctx).clear()
	_ = mod.CloseWithExitCode(ctx, code) // TODO: should ours be signed bit (like -1 == 255)?
}

// WasmWrite implements runtime.wasmWrite which supports runtime.write and
// runtime.writeErr. This implements `println`.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/os_js.go#L29
var WasmWrite = spfunc.MustCallFromSP(false, &wasm.HostFunc{
	ExportNames: []string{wasmWriteName},
	Name:        wasmWriteName,
	ParamTypes:  []api.ValueType{i32, i32, i32},
	ParamNames:  []string{"fd", "p", "n"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(wasmWrite),
	},
})

func wasmWrite(_ context.Context, mod api.Module, stack []uint64) {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, p, n := uint32(stack[0]), uint32(stack[1]), uint32(stack[2])

	writer := fsc.FdWriter(fd)
	if writer == nil {
		panic(fmt.Errorf("unexpected fd %d", fd))
	}

	if _, err := writer.Write(mustRead(mod.Memory(), "p", p, n)); err != nil {
		panic(fmt.Errorf("error writing p: %w", err))
	}
}

// ResetMemoryDataView signals wasm.OpcodeMemoryGrow happened, indicating any
// cached view of memory should be reset.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/mem_js.go#L82
var ResetMemoryDataView = &wasm.HostFunc{
	ExportNames: []string{resetMemoryDataViewName},
	Name:        resetMemoryDataViewName,
	ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
	ParamNames:  []string{parameterSp},
	// TODO: Compiler-based memory.grow callbacks are ignored until we have a generic solution #601
	Code: &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeEnd}},
}

// Nanotime1 implements runtime.nanotime which supports time.Since.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L184
var Nanotime1 = spfunc.MustCallFromSP(false, &wasm.HostFunc{
	ExportNames: []string{nanotime1Name},
	Name:        nanotime1Name,
	ResultTypes: []api.ValueType{i64},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(nanotime1),
	},
})

func nanotime1(_ context.Context, mod api.Module, stack []uint64) {
	time := mod.(*wasm.CallContext).Sys.Nanotime()
	stack[0] = api.EncodeI64(time)
}

// Walltime implements runtime.walltime which supports time.Now.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L188
var Walltime = spfunc.MustCallFromSP(false, &wasm.HostFunc{
	ExportNames: []string{walltimeName},
	Name:        walltimeName,
	ResultTypes: []api.ValueType{i64, i32},
	ResultNames: []string{"sec", "nsec"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(walltime),
	},
})

func walltime(_ context.Context, mod api.Module, stack []uint64) {
	sec, nsec := mod.(*wasm.CallContext).Sys.Walltime()
	stack[0] = api.EncodeI64(sec)
	stack[1] = api.EncodeI32(nsec)
}

// ScheduleTimeoutEvent implements runtime.scheduleTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// Unlike other most functions prefixed by "runtime.", this both launches a
// goroutine and invokes code compiled into wasm "resume".
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L192
var ScheduleTimeoutEvent = stubFunction(scheduleTimeoutEventName)

// ^^ stubbed because signal handling is not implemented in GOOS=js

// ClearTimeoutEvent implements runtime.clearTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L196
var ClearTimeoutEvent = stubFunction(clearTimeoutEventName)

// ^^ stubbed because signal handling is not implemented in GOOS=js

// GetRandomData implements runtime.getRandomData, which initializes the seed
// for runtime.fastrand.
//
// See https://github.com/golang/go/blob/go1.19/src/runtime/sys_wasm.s#L200
var GetRandomData = spfunc.MustCallFromSP(false, &wasm.HostFunc{
	ExportNames: []string{getRandomDataName},
	Name:        getRandomDataName,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"buf", "bufLen"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(getRandomData),
	},
})

func getRandomData(_ context.Context, mod api.Module, stack []uint64) {
	randSource := mod.(*wasm.CallContext).Sys.RandSource()
	buf, bufLen := uint32(stack[0]), uint32(stack[1])

	r := mustRead(mod.Memory(), "r", buf, bufLen)

	if n, err := randSource.Read(r); err != nil {
		panic(fmt.Errorf("RandSource.Read(r /* len=%d */) failed: %w", bufLen, err))
	} else if uint32(n) != bufLen {
		panic(fmt.Errorf("RandSource.Read(r /* len=%d */) read %d bytes", bufLen, n))
	}
}
