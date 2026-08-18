//go:build amd64

package wazevo

import (
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/amd64"
)

func newMachine() backend.Machine {
	return amd64.NewBackend()
}

// unwindStack is a function to unwind the stack, and appends return addresses to `returnAddresses` slice.
// The implementation must be aligned with the ABI/Calling convention.
func unwindStack(sp, fp, top uintptr, returnAddresses []uintptr) []uintptr {
	return amd64.UnwindStack(sp, fp, top, returnAddresses)
}

// goCallStackView is a function to get a view of the stack before a Go call, which
// is the view of the stack allocated in CompileGoFunctionTrampoline.
func goCallStackView(stackPointerBeforeGoCall *uint64) []uint64 {
	return amd64.GoCallStackView(stackPointerBeforeGoCall)
}

// adjustClonedStack is a function to adjust the stack after it is grown.
// More precisely, absolute addresses (frame pointers) in the stack must be adjusted.
func adjustClonedStack(oldsp, oldTop, sp, fp, top uintptr) {
	amd64.AdjustClonedStack(oldsp, oldTop, sp, fp, top)
}

// trampolineWindowBytes returns the size of the trampoline-owned stack
// region at a Go-call exit: everything between RSP (stackPointerBeforeGoCall)
// and the caller's stack pointer, i.e. the slice-size slot, the (16-byte
// aligned) arg/ret area, and the Caller_RBP + Return Addr slots at [fp] and
// [fp+8] that the trampoline's `mov rsp, rbp; pop rbp; ret` epilogue reads on
// resume. See the layout in backend/isa/amd64/abi_go_call.go
// CompileGoFunctionTrampoline.
func trampolineWindowBytes(sp *uint64, fp uintptr) uintptr {
	return fp + 16 - uintptr(unsafe.Pointer(sp))
}

// rebaseTrampolineWindow fixes stack-absolute values inside a restored
// trampoline window after the stack buffer moved by topDelta (growStack
// reallocation between the try_table entry and the throw). On amd64 the
// window's Caller_RBP slot at [fp] holds an absolute pointer into the stack.
func rebaseTrampolineWindow(sp *uint64, windowBytes, topDelta uintptr) {
	if topDelta == 0 {
		return
	}
	callerRBP := (*uint64)(unsafe.Add(unsafe.Pointer(sp), windowBytes-16))
	*callerRBP += uint64(topDelta)
}
