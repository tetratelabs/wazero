package amd64

func UnwindStack(sp, top uintptr, returnAddresses []uintptr) []uintptr {
	panic("implement me")
}

// GoCallStackView is a function to get a view of the stack before a Go call, which
// is the view of the stack allocated in CompileGoFunctionTrampoline.
func GoCallStackView(stackPointerBeforeGoCall *uint64) []uint64 {
	panic("implement me")
}
