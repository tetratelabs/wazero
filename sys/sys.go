// Package sys includes constants and types used by both public and internal APIs.
package sys

import (
	"context"
	"fmt"
)

// WallClock returns a reading similar to time.Time.
type WallClock interface {
	Clock

	// WallTime returns the current time in epoch seconds with a nanosecond fraction.
	WallTime(context.Context) (sec int64, nsec int32)
}

// MonotonicClock returns a relative reading used to measuring elapsed time.
type MonotonicClock interface {
	Clock

	// NanoTime returns nanoseconds since an arbitrary start point.
	//
	// Note: There are no constraints on the value return except that it
	// increments. For example, -1 is a valid if the next value is >= 0.
	NanoTime(context.Context) int64
}

// Clock is similar to time.Time in Go, except it splits wall-clock and
// monotonic-clock readings into separate interfaces.
type Clock interface {
	// Resolution returns a positive granularity of clock precision in
	// nanoseconds. For example, if the resolution is 1us, this returns 1000.
	//
	// Note: Some implementations return arbitrary resolution because there's
	// no perfect alternative. For example, according to the source in time.go,
	// windows monotonic resolution can be 15ms. See /RATIONALE.md.
	Resolution(context.Context) uint64
}

// ExitError is returned to a caller of api.Function still running when api.Module CloseWithExitCode was invoked.
// ExitCode zero value means success, while any other value is an error.
//
// Here's an example of how to get the exit code:
//	main := module.ExportedFunction("main")
//	if err := main(nil); err != nil {
//		if exitErr, ok := err.(*sys.ExitError); ok {
//			// If your main function expects to exit, this could be ok if Code == 0
//		}
//	--snip--
//
// Note: While possible the reason of this was "proc_exit" from "wasi_snapshot_preview1", it could be from other host
// functions, for example an AssemblyScript's abort handler, or any arbitrary caller of CloseWithExitCode.
//
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit and
// https://www.assemblyscript.org/concepts.html#special-imports
type ExitError struct {
	moduleName string
	exitCode   uint32
}

func NewExitError(moduleName string, exitCode uint32) *ExitError {
	return &ExitError{moduleName: moduleName, exitCode: exitCode}
}

// ModuleName is the api.Module that was closed.
func (e *ExitError) ModuleName() string {
	return e.moduleName
}

// ExitCode returns zero on success, and an arbitrary value otherwise.
func (e *ExitError) ExitCode() uint32 {
	return e.exitCode
}

// Error implements the error interface.
func (e *ExitError) Error() string {
	return fmt.Sprintf("module %q closed with exit_code(%d)", e.moduleName, e.exitCode)
}

// Is allows use via errors.Is
func (e *ExitError) Is(err error) bool {
	if target, ok := err.(*ExitError); ok {
		return e.moduleName == target.moduleName && e.exitCode == target.exitCode
	}
	return false
}
