package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
)

const (
	// functionProcExit terminates the execution of the module with an exit code.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
	functionProcExit = "proc_exit"

	// importProcExit is the WebAssembly 1.0 Text format import of functionProcExit.
	importProcExit = `(import "wasi_snapshot_preview1" "proc_exit"
    (func $wasi.proc_exit (param $rval i32)))`

	// functionProcRaise sends a signal to the process of the calling thread.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-proc_raisesig-signal---errno
	functionProcRaise = "proc_raise"

	// importProcRaise is the WebAssembly 1.0 Text format import of functionProcRaise.
	importProcRaise = `(import "wasi_snapshot_preview1" "proc_raise"
    (func $wasi.proc_raise (param $sig i32) (result (;errno;) i32)))`
)

// ProcExit is the WASI function that terminates the execution of the module with an exit code.
// An exit code of 0 indicates successful termination. The meanings of other values are not defined by WASI.
//
// * rval - The exit code.
//
// In wazero, this calls api.Module CloseWithExitCode.
//
// Note: importProcExit shows this signature in the WebAssembly 1.0 Text Format.
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
func (a *wasi) ProcExit(ctx context.Context, mod api.Module, exitCode uint32) {
	// Ensure other callers see the exit code.
	_ = mod.CloseWithExitCode(ctx, exitCode)

	// Prevent any code from executing after this function. For example, LLVM
	// inserts unreachable instructions after calls to exit.
	// See: https://github.com/emscripten-core/emscripten/issues/12322
	panic(sys.NewExitError(mod.Name(), exitCode))
}

// ProcRaise is the WASI function named functionProcRaise
func (a *wasi) ProcRaise(ctx context.Context, mod api.Module, sig uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}
