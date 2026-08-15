package wazevo

import (
	_ "unsafe" // for go:linkname
)

// runtime_entersyscall and runtime_exitsyscall mark the goroutine as being
// in a syscall, which detaches its P. This lets the Go runtime's sysmon
// retake the P after ~10ms and run other goroutines on it, even if the
// wasm goroutine is in a long-running native loop.
//
// Same trick used by gvisor (see runtime/proc.go: comment
// on entersyscallblock — the runtime team has committed to not changing
// the type signatures of these entry points).

//go:linkname runtime_entersyscall runtime.entersyscall
func runtime_entersyscall()

//go:linkname runtime_exitsyscall runtime.exitsyscall
func runtime_exitsyscall()

type (
	// entrypointFn enters compiled code at a function's entry preamble.
	entrypointFn func(preambleExecutable, functionExecutable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultStackPtr *uint64, goAllocatedStackSlicePtr uintptr)
	// afterGoFunctionCallEntrypointFn re-enters compiled code after a Go-side dispatcher
	// iteration (host call, FailIfClosed, stack grow, etc.).
	afterGoFunctionCallEntrypointFn func(executable *byte, executionContextPtr uintptr, stackPointer, framePointer uintptr)
)

// entrypoints returns the pair of functions a callEngine of a module compiled with the given
// ensureTermination enters compiled code through.
//
// Only ensureTermination needs the syscall bracketing: its loops check ModuleInstance.Closed
// themselves instead of calling back into Go, so this goroutine has to have released its P for
// the watchdog goroutine to ever run and set that flag. Without it nothing has to run while
// wasm does, and the bracketing would just be overhead on every call and every host call.
func entrypoints(ensureTermination bool) (entrypointFn, afterGoFunctionCallEntrypointFn) {
	if ensureTermination {
		return entrypointInSyscall, afterGoFunctionCallEntrypointInSyscall
	}
	return entrypointAsm, afterGoFunctionCallEntrypointAsm
}

// entrypointInSyscall is entrypointAsm bracketed with entersyscall / exitsyscall so the Go
// runtime can retake this goroutine's P while wasm is running. Marked nosplit because
// entersyscall sets throwsplit and we must not grow the stack between entersyscall and
// exitsyscall.
//
//go:nosplit
func entrypointInSyscall(preambleExecutable, functionExecutable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultStackPtr *uint64, goAllocatedStackSlicePtr uintptr) {
	runtime_entersyscall()
	entrypointAsm(preambleExecutable, functionExecutable, executionContextPtr, moduleContextPtr, paramResultStackPtr, goAllocatedStackSlicePtr)
	runtime_exitsyscall()
}

// afterGoFunctionCallEntrypointInSyscall is afterGoFunctionCallEntrypointAsm with the same
// syscall bracketing as entrypointInSyscall, and the same nosplit constraint.
//
//go:nosplit
func afterGoFunctionCallEntrypointInSyscall(executable *byte, executionContextPtr uintptr, stackPointer, framePointer uintptr) {
	runtime_entersyscall()
	afterGoFunctionCallEntrypointAsm(executable, executionContextPtr, stackPointer, framePointer)
	runtime_exitsyscall()
}
