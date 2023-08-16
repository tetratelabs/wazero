package wazevo

import (
	"context"
	"encoding/binary"
	"reflect"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/internalapi"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

type (
	// callEngine implements api.Function.
	callEngine struct {
		internalapi.WazeroOnly
		stack []byte
		// stackTop is the pointer to the *aligned* top of the stack. This must be updated
		// whenever the stack is changed. This is passed to the assembly function
		// at the very beginning of api.Function Call/CallWithStack.
		stackTop uintptr
		// executable is the pointer to the executable code for this function.
		executable *byte
		// parent is the *moduleEngine from which this callEngine is created.
		parent *moduleEngine
		// indexInModule is the index of the function in the module.
		indexInModule wasm.Index
		// sizeOfParamResultSlice is the size of the parameter/result slice.
		sizeOfParamResultSlice int
		// execCtx holds various information to be read/written by assembly functions.
		execCtx executionContext
		// execCtxPtr holds the pointer to the executionContext which doesn't change after callEngine is created.
		execCtxPtr      uintptr
		numberOfResults int
	}

	// executionContext is the struct to be read/written by assembly functions.
	executionContext struct {
		// exitCode holds the wazevoapi.ExitCode describing the state of the function execution.
		exitCode wazevoapi.ExitCode
		// callerModuleContextPtr holds the moduleContextOpaque for Go function calls.
		callerModuleContextPtr *byte
		// originalFramePointer holds the original frame pointer of the caller of the assembly function.
		originalFramePointer uintptr
		// originalStackPointer holds the original stack pointer of the caller of the assembly function.
		originalStackPointer uintptr
		// goReturnAddress holds the return address to go back to the caller of the assembly function.
		goReturnAddress uintptr
		// stackBottomPtr holds the pointer to the bottom of the stack.
		stackBottomPtr *byte
		// goCallReturnAddress holds the return address to go back to the caller of the Go function.
		goCallReturnAddress *byte
		// stackPointerBeforeGoCall holds the stack pointer before calling a Go function.
		stackPointerBeforeGoCall uintptr
		// stackGrowRequiredSize holds the required size of stack grow.
		stackGrowRequiredSize uintptr
		// memoryGrowTrampolineAddress holds the address of memory grow trampoline function.
		memoryGrowTrampolineAddress *byte
		// savedRegisters is the opaque spaces for save/restore registers.
		// We want to align 16 bytes for each register, so we use [64][2]uint64.
		savedRegisters [64][2]uint64
		// goFunctionCallCalleeModuleContextOpaque is the pointer to the target Go function's moduleContextOpaque.
		goFunctionCallCalleeModuleContextOpaque uintptr
		// goFunctionCallStack is used to pass/receive parameters/results for Go function calls.
		goFunctionCallStack [goFunctionCallStackSize]uint64
	}
)

const goFunctionCallStackSize = 128

var initialStackSize uint64 = 512

func (c *callEngine) init() {
	stackSize := initialStackSize
	if c.sizeOfParamResultSlice > int(stackSize) {
		stackSize = uint64(c.sizeOfParamResultSlice)
	}

	c.stack = make([]byte, stackSize)
	c.stackTop = alignedStackTop(c.stack)
	c.execCtx.stackBottomPtr = &c.stack[0]
	c.execCtxPtr = uintptr(unsafe.Pointer(&c.execCtx))
}

// alignedStackTop returns 16-bytes aligned stack top of given stack.
// 16 bytes should be good for all platform (arm64/amd64).
func alignedStackTop(s []byte) uintptr {
	stackAddr := uintptr(unsafe.Pointer(&s[len(s)-1]))
	return stackAddr - (stackAddr & (16 - 1))
}

// Definition implements api.Function.
func (c *callEngine) Definition() api.FunctionDefinition {
	return &c.parent.module.Source.FunctionDefinitionSection[c.indexInModule]
}

// Call implements api.Function.
func (c *callEngine) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	paramResultSlice := make([]uint64, c.sizeOfParamResultSlice)
	copy(paramResultSlice, params)
	if err := c.CallWithStack(ctx, paramResultSlice); err != nil {
		return nil, err
	}
	return paramResultSlice[:c.numberOfResults], nil
}

// CallWithStack implements api.Function.
func (c *callEngine) CallWithStack(ctx context.Context, paramResultStack []uint64) error {
	var paramResultPtr *uint64
	if len(paramResultStack) > 0 {
		paramResultPtr = &paramResultStack[0]
	}

	entrypoint(c.executable, c.execCtxPtr, c.parent.opaquePtr, paramResultPtr, c.stackTop)
	for {
		switch ec := c.execCtx.exitCode; ec & wazevoapi.ExitCodeMask {
		case wazevoapi.ExitCodeOK:
			return nil
		case wazevoapi.ExitCodeGrowStack:
			newsp, err := c.growStack()
			if err != nil {
				return err
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, newsp)
		case wazevoapi.ExitCodeUnreachable:
			return wasmruntime.ErrRuntimeUnreachable
		case wazevoapi.ExitCodeMemoryOutOfBounds:
			return wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess
		case wazevoapi.ExitCodeGrowMemory:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			argRes := &c.execCtx.goFunctionCallStack[0]
			if res, ok := mem.Grow(uint32(*argRes)); !ok {
				*argRes = uint64(0xffffffff) // = -1 in signed 32-bit integer.
			} else {
				*argRes = uint64(res)
				calleeOpaque := opaqueViewFromPtr(uintptr(unsafe.Pointer(c.execCtx.callerModuleContextPtr)))
				if mod.Source.MemorySection != nil { // Local memory.
					putLocalMemory(calleeOpaque, 8 /* local memory begins at 8 */, mem)
				} else {
					// Imported memory's owner at offset 16 of the callerModuleContextPtr.
					opaquePtr := uintptr(binary.LittleEndian.Uint64(calleeOpaque[16:]))
					importedMemOwner := opaqueViewFromPtr(opaquePtr)
					putLocalMemory(importedMemOwner, 8 /* local memory begins at 8 */, mem)
				}
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, c.execCtx.stackPointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoFunction:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			f.Call(ctx, c.execCtx.goFunctionCallStack[:])
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, c.execCtx.stackPointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoModuleFunction:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoModuleFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			mod := c.callerModuleInstance()
			f.Call(ctx, mod, c.execCtx.goFunctionCallStack[:])
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, c.execCtx.stackPointerBeforeGoCall)
		case wazevoapi.ExitCodeTableOutOfBounds:
			return wasmruntime.ErrRuntimeInvalidTableAccess
		case wazevoapi.ExitCodeIndirectCallNullPointer:
			return wasmruntime.ErrRuntimeInvalidTableAccess
		case wazevoapi.ExitCodeIndirectCallTypeMismatch:
			return wasmruntime.ErrRuntimeIndirectCallTypeMismatch
		default:
			panic("BUG")
		}
	}
}

func (c *callEngine) callerModuleInstance() *wasm.ModuleInstance {
	return *(**wasm.ModuleInstance)(unsafe.Pointer(c.execCtx.callerModuleContextPtr))
}

func opaqueViewFromPtr(ptr uintptr) []byte {
	var opaque []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&opaque))
	sh.Data = ptr
	sh.Len = 24
	sh.Cap = 24
	return opaque
}

const callStackCeiling = uintptr(5000000) // in uint64 (8 bytes) == 40000000 bytes in total == 40mb.

// growStack grows the stack, and returns the new stack pointer.
func (c *callEngine) growStack() (newSP uintptr, err error) {
	currentLen := uintptr(len(c.stack))
	if callStackCeiling < currentLen {
		err = wasmruntime.ErrRuntimeStackOverflow
		return
	}

	newLen := 2*currentLen + c.execCtx.stackGrowRequiredSize
	newStack := make([]byte, newLen)

	relSp := c.stackTop - c.execCtx.stackPointerBeforeGoCall

	// Copy the existing contents in the previous Go-allocated stack into the new one.
	var prevStackAligned, newStackAligned []byte
	{
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&prevStackAligned))
		sh.Data = c.stackTop - relSp
		sh.Len = int(relSp)
		sh.Cap = int(relSp)
	}
	newTop := alignedStackTop(newStack)
	{
		newSP = newTop - relSp
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&newStackAligned))
		sh.Data = newSP
		sh.Len = int(relSp)
		sh.Cap = int(relSp)
	}
	copy(newStackAligned, prevStackAligned)

	c.stack = newStack
	c.stackTop = newTop
	c.execCtx.stackBottomPtr = &newStack[0]
	return
}
