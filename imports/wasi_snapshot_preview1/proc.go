package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

const (
	functionProcExit  = "proc_exit"
	functionProcRaise = "proc_raise"
)

// procExit is the WASI function named functionProcExit that terminates the
// execution of the module with an exit code. The only successful exit code is
// zero.
//
// # Parameters
//
//   - exitCode: exit code.
//
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
var procExit = &wasm.HostFunc{
	ExportNames: []string{functionProcExit},
	Name:        functionProcExit,
	ParamTypes:  []api.ValueType{i32},
	ParamNames:  []string{"rval"},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(procExitFn),
	},
}

func procExitFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	exitCode := uint32(params[0])

	// Ensure other callers see the exit code.
	_ = mod.CloseWithExitCode(ctx, exitCode)

	// Prevent any code from executing after this function. For example, LLVM
	// inserts unreachable instructions after calls to exit.
	// See: https://github.com/emscripten-core/emscripten/issues/12322
	panic(sys.NewExitError(mod.Name(), exitCode))
}

// procRaise is stubbed and will never be supported, as it was removed.
//
// See https://github.com/WebAssembly/WASI/pull/136
var procRaise = stubFunction(functionProcRaise, []wasm.ValueType{i32}, []string{"sig"})
