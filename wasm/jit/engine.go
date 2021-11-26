package jit

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/tetratelabs/wazero/wasm"
)

type engine struct {
	// The actual Go-allocated stack.
	// Note that we NEVER edit len or cap in JITed code so we won't get screwed when GC comes in.
	stack []uint64
	// Wasm stack pointer on .stack field
	sp uint64
	// Where we store the status code of JIT execution.
	jitCallStatusCode jitStatusCodes
	// Set when statusCode == jitStatusCall{Function,BuiltInFunction,HostFunction}
	// Indicating the function call index.
	functionCallIndex uint32
	// Set when statusCode == jitStatusCall{Function,BuiltInFunction,HostFunction}
	// We use this value to continue the current function
	// after calling the target function exits.
	// Instructions after [base+continuationAddressOffset] must start with
	// restoring reserved registeres.
	continuationAddressOffset uintptr
	// Function call frames in linked list
	callFrameStack *callFrame
	// Store the compiled functions and indexes.
	compiledWasmFunctions     []*compiledFunction
	compiledWasmFunctionIndex map[*wasm.FunctionInstance]int
	// Store the host functions and indexes.
	hostFunctions     []func()
	hostFunctionIndex map[*wasm.FunctionInstance]int
}

var _ wasm.Engine = &engine{}

func (e *engine) Call(f *wasm.FunctionInstance, args ...uint64) (returns []uint64, err error) {
	for _, arg := range args {
		e.push(arg)
	}
	if index, ok := e.hostFunctionIndex[f]; ok {
		e.hostFunctions[index]()
	} else if index, ok := e.compiledWasmFunctionIndex[f]; ok {
		f := e.compiledWasmFunctions[index]
		e.exec(f)
	} else {
		return nil, fmt.Errorf("invalid function")
	}
	returns = make([]uint64, len(f.Signature.ReturnTypes))
	for i := range returns {
		returns[len(returns)-1-i] = e.pop()
	}
	return
}

func (e *engine) Compile(f *wasm.FunctionInstance) error {
	if f.HostFunction != nil {
		if _, ok := e.hostFunctionIndex[f]; ok {
			return nil
		}
		hf := func() {
			// TODO:
		}
		id := len(e.hostFunctions)
		e.hostFunctionIndex[f] = id
		e.hostFunctions = append(e.hostFunctions, hf)
	} else {
		if _, ok := e.compiledWasmFunctionIndex[f]; ok {
			return nil
		}
		cf, err := e.compileWasmFunction(f)
		if err != nil {
			return fmt.Errorf("failed to compile Wasm function: %w", err)
		}
		id := len(e.compiledWasmFunctions)
		e.compiledWasmFunctionIndex[f] = id
		e.compiledWasmFunctions = append(e.compiledWasmFunctions, cf)
	}
	return nil
}

func NewEngine() wasm.Engine {
	return newEngine()
}

func newEngine() *engine {
	const initialStackSize = 100
	e := &engine{
		stack:                     make([]uint64, initialStackSize),
		compiledWasmFunctionIndex: make(map[*wasm.FunctionInstance]int),
		hostFunctionIndex:         make(map[*wasm.FunctionInstance]int),
	}
	return e
}

func (e *engine) pop() (ret uint64) {
	ret = e.stack[e.sp-1]
	e.sp--
	return
}

func (e *engine) push(v uint64) {
	e.stack[e.sp] = v
	e.sp++
}

type jitStatusCodes uint32

const (
	jitStatusReturned jitStatusCodes = iota
	jitStatusCallFunction
	jitStatusCallBuiltInFunction
	jitStatusCallHostFunction
	// TODO: trap, etc?
)

var (
	engineStackOffset               = int64(unsafe.Offsetof((&engine{}).stack))
	engineSPOffset                  = int64(unsafe.Offsetof((&engine{}).sp))
	engineJITStatusOffset           = int64(unsafe.Offsetof((&engine{}).jitCallStatusCode))
	engineFunctionCallIndexOffset   = int64(unsafe.Offsetof((&engine{}).functionCallIndex))
	engineContinuationAddressOffset = int64(unsafe.Offsetof((&engine{}).continuationAddressOffset))
)

type callFrame struct {
	continuationAddress uintptr
	f                   *compiledFunction
	prev                *callFrame
}

type compiledFunction struct {
	codeSegment []byte
	memoryInst  *wasm.MemoryInstance
}

func (c *compiledFunction) initialAddress() uintptr {
	return uintptr(unsafe.Pointer(&c.codeSegment[0]))
}

const (
	builtinFunctionIndexGrowMemory = iota
)

func (e *engine) stackGrow() {
	newStack := make([]uint64, len(e.stack)*2)
	copy(newStack[:len(e.stack)], e.stack)
	e.stack = newStack
}

func (e *engine) exec(f *compiledFunction) {
	e.callFrameStack = &callFrame{
		continuationAddress: f.initialAddress(),
		f:                   f,
		prev:                nil,
	}
	for e.callFrameStack != nil {
		currentFrame := e.callFrameStack
		// TODO: We should check the size of the stack,
		// and if it's running out, grow it before calling into JITed code.
		// It should be possible to check the necessity by statically
		// analyzing the max height of the stack in the function.
		if false {
			e.stackGrow()
		}

		// Call into the jitted code.
		jitcall(
			currentFrame.continuationAddress,
			uintptr(unsafe.Pointer(e)),
			uintptr(unsafe.Pointer(&currentFrame.f.memoryInst.Buffer[0])),
		)
		// Check the status code from JIT code.
		switch e.jitCallStatusCode {
		case jitStatusReturned:
			// Meaning that the current frame exits
			// so we just get back to the caller's frame.
			e.callFrameStack = currentFrame.prev
		case jitStatusCallFunction:
			nextFunc := e.compiledWasmFunctions[e.functionCallIndex]
			// Calculate the continuation address so
			// we can resume this caller function frame.
			currentFrame.continuationAddress = currentFrame.f.initialAddress() + e.continuationAddressOffset
			// Create the callee frame.
			frame := &callFrame{
				continuationAddress: nextFunc.initialAddress(),
				f:                   nextFunc,
				// Set the caller frame as prev so we can return back to the current frame!
				prev: currentFrame,
			}
			// Now move onto the callee function.
			e.callFrameStack = frame
		case jitStatusCallBuiltInFunction:
			switch e.functionCallIndex {
			case builtinFunctionIndexGrowMemory:
				v := e.pop()
				e.memoryGrow(currentFrame.f.memoryInst, v)
			default:
				panic("invalid builtin function index")
			}
			currentFrame.continuationAddress = currentFrame.f.initialAddress() + e.continuationAddressOffset
		case jitStatusCallHostFunction:
			e.hostFunctions[e.functionCallIndex]()
			currentFrame.continuationAddress = currentFrame.f.initialAddress() + e.continuationAddressOffset
		default:
			panic("invalid status code!")
		}
	}
}

func (e *engine) memoryGrow(m *wasm.MemoryInstance, newPages uint64) {
	max := uint64(math.MaxUint32)
	if m.Max != nil {
		max = uint64(*m.Max) * wasm.PageSize
	}
	if uint64(newPages*wasm.PageSize+uint64(len(m.Buffer))) > max {
		v := int32(-1)
		e.push(uint64(v))
	} else {
		e.push(uint64(uint64(len(m.Buffer)) / wasm.PageSize))
		m.Buffer = append(m.Buffer, make([]byte, newPages*wasm.PageSize)...)
	}
}

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledFunction, error) {
	return nil, nil
}
