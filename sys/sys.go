// Package sys includes constants and types used by both public and internal APIs.
package sys

import (
	"fmt"
)

// ExitError is returned to a caller of wasm.Function still running when wasm.Module CloseWithExitCode was invoked.
// ExitCode zero value means success, while any other value is an error.
//
// Here's an example of how to get the exit code:
//	main := module.ExportedFunction("main")
//	if err := main(nil); err != nil {
//		if exitErr, ok := err.(*wazero.ExitError); ok {
//			// If your main function expects to exit, this could be ok if Code == 0
//		}
//	--snip--
//
// Note: While possible the reason of this was "proc_exit" from wasi.ModuleSnapshotPreview1, it could be from other host
// functions, for example an AssemblyScript's abort handler, or any arbitrary caller of CloseWithExitCode.
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
// See https://www.assemblyscript.org/concepts.html#special-imports
type ExitError struct {
	exitCode uint32
}

func NewExitError(exitCode uint32) *ExitError {
	return &ExitError{exitCode: exitCode}
}

// ExitCode returns zero on success, and an arbitrary value otherwise.
func (e *ExitError) ExitCode() uint32 {
	return e.exitCode
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit_code(%d)", e.exitCode)
}
