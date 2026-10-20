//go:build arm64

package wazevo

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64"
)

func newMachine() backend.Machine {
	return arm64.NewBackend()
}

// unwindStack is a function to unwind the stack, and appends return addresses to `returnAddresses` slice.
// The implementation must be aligned with the ABI/Calling convention.
func unwindStack(sp, fp, top uintptr, returnAddresses []uintptr) []uintptr {
	return arm64.UnwindStack(sp, fp, top, returnAddresses)
}

// goCallStackView is a function to get a view of the stack before a Go call, which
// is the view of the stack allocated in CompileGoFunctionTrampoline.
func goCallStackView(stackPointerBeforeGoCall *uint64) []uint64 {
	return arm64.GoCallStackView(stackPointerBeforeGoCall)
}

// adjustClonedStack is a function to adjust the stack after it is grown.
// More precisely, absolute addresses (frame pointers) in the stack must be adjusted.
func adjustClonedStack(oldsp, oldTop, sp, fp, top uintptr) {
	// TODO: currently, the frame pointers are not used, and saved old sps are relative to the current stack pointer,
	//  so no need to adjustment on arm64. However, when we make it absolute, which in my opinion is better perf-wise
	//  at the expense of slightly costly stack growth, we need to adjust the pushed frame pointers.
}

// trampolineWindowBytes returns the size of the trampoline-owned stack
// region at the given Go-call stack pointer: 16 bytes of frame info
// (frame_size, sliceSize), the arg/ret area (frame_size, read from the
// first slot), and 16 bytes of return address + size_of_arg_ret. See the
// layout in backend/isa/arm64/abi_go_call.go.
func trampolineWindowBytes(sp *uint64, _ uintptr) uintptr {
	frameSize := *sp
	return uintptr(16 + frameSize + 16)
}

// rebaseTrampolineWindow is a no-op on arm64: the trampoline window contains
// no stack-absolute values (the saved return address is a code pointer and
// frame sizes are relative), so a stack-buffer move needs no fix-up.
func rebaseTrampolineWindow(*uint64, uintptr, uintptr) {}
