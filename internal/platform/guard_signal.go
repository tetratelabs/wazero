package platform

// Guard-page-backed memories need a way to turn a hardware fault raised by
// wazevo-generated code into a regular wasm out-of-bounds trap. The Go
// runtime cannot recover faults at non-Go (JIT) program counters, so that
// translation is done by an optional, cgo-based signal handler that lives in
// the separate experimental/guardsig package. That package injects its
// registration functions here at init; when it is not linked in (or cgo is
// disabled), the hooks stay nil and the engine keeps generating explicit
// bounds checks even when guard-page memory is requested.
var guardFaultHooks struct {
	// installed reports whether the fault handler was successfully
	// installed before the Go runtime started.
	installed func() bool
	// regionAdd/regionDel mirror the Go-side guard-region registry into the
	// handler's async-signal-safe registry. regionAdd reports whether the
	// registration succeeded (the handler's table is fixed-size).
	regionAdd func(base, end uintptr) bool
	regionDel func(base uintptr)
	// stackAdd/stackDel register the [lo, hi) range of a call engine's
	// stack together with the execution context pointer and the address of
	// the compiled guard-fault exit sequence for that engine, so the
	// handler can redirect a faulting thread. stackAdd reports whether the
	// registration succeeded.
	stackAdd func(lo, hi, execCtx, exitSeq uintptr) bool
	stackDel func(lo uintptr)
}

// SetGuardFaultHandlerHooks is called (at most once, from an init function)
// by the experimental/guardsig package to make its signal handler available
// to the engine. It must not be called after any engine has been created.
func SetGuardFaultHandlerHooks(
	installed func() bool,
	regionAdd func(base, end uintptr) bool,
	regionDel func(base uintptr),
	stackAdd func(lo, hi, execCtx, exitSeq uintptr) bool,
	stackDel func(lo uintptr),
) {
	guardFaultHooks.installed = installed
	guardFaultHooks.regionAdd = regionAdd
	guardFaultHooks.regionDel = regionDel
	guardFaultHooks.stackAdd = stackAdd
	guardFaultHooks.stackDel = stackDel
}

// GuardFaultHandlerInstalled returns true when the guard-fault signal
// handler is linked in and active, which is a precondition for generating
// code without per-access memory bounds checks.
func GuardFaultHandlerInstalled() bool {
	return guardFaultHooks.installed != nil && guardFaultHooks.installed()
}

// GuardSigStackAdd registers a call engine stack with the fault handler.
// Returns true on success, or when no handler is present (nothing to do).
func GuardSigStackAdd(lo, hi, execCtx, exitSeq uintptr) bool {
	if guardFaultHooks.stackAdd == nil {
		return true
	}
	return guardFaultHooks.stackAdd(lo, hi, execCtx, exitSeq)
}

// GuardSigStackDel removes a previously registered call engine stack.
func GuardSigStackDel(lo uintptr) {
	if guardFaultHooks.stackDel != nil {
		guardFaultHooks.stackDel(lo)
	}
}

func guardSigRegionAdd(base, end uintptr) bool {
	if guardFaultHooks.regionAdd == nil {
		return true
	}
	return guardFaultHooks.regionAdd(base, end)
}

func guardSigRegionDel(base uintptr) {
	if guardFaultHooks.regionDel != nil {
		guardFaultHooks.regionDel(base)
	}
}
