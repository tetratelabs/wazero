package jit

import (
	"fmt"
	"math"
	"reflect"
	"unsafe"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

type engine struct {
	// The actual Go-allocated stack.
	// Note that we NEVER edit len or cap in JITed code so we won't get screwed when GC comes in.
	stack []uint64
	// Wasm stack pointer on .stack field which is accessed by currentBaseStackPointer+currentStackPointer
	currentStackPointer     uint64
	currentBaseStackPointer uint64
	// Where we store the status code of JIT execution.
	jitCallStatusCode jitCallStatusCode
	// Set when statusCode == jitStatusCall{Function,BuiltInFunction,HostFunction}
	// Indicating the function call index.
	functionCallIndex int64
	// Set when statusCode == jitStatusCall{Function,BuiltInFunction,HostFunction}
	// We use this value to continue the current function
	// after calling the target function exits.
	// Instructions after [base+continuationAddressOffset] must start with
	// restoring reserved registeres.
	continuationAddressOffset uintptr
	// Function call frames in linked list
	callFrameStack *callFrame
	// Store the compiled functions and indexes.
	compiledWasmFunctions     []*compiledWasmFunction
	compiledWasmFunctionIndex map[*wasm.FunctionInstance]int64
	// Store the host functions and indexes.
	hostFunctions     []hostFunction
	hostFunctionIndex map[*wasm.FunctionInstance]int64
}

type hostFunction = func(ctx *wasm.HostFunctionCallContext)

func (e *engine) Call(f *wasm.FunctionInstance, args ...uint64) (returns []uint64, err error) {
	for _, arg := range args {
		e.push(arg)
	}
	// Note that there's no conflict between e.hostFunctionIndex and e.compiledWasmFunctionIndex,
	// meaning that each *wasm.FunctionInstance is assigned to either host function index or wasm function one.
	if index, ok := e.hostFunctionIndex[f]; ok {
		e.hostFunctions[index](&wasm.HostFunctionCallContext{Memory: f.ModuleInstance.Memory})
	} else if index, ok := e.compiledWasmFunctionIndex[f]; ok {
		f := e.compiledWasmFunctions[index]
		e.exec(f)
	} else {
		err = fmt.Errorf("function not compiled")
		return
	}
	// Note the top value is the tail of the returns,
	// so we assign the returns in reverse order.
	returns = make([]uint64, len(f.Signature.ReturnTypes))
	for i := range returns {
		returns[len(returns)-1-i] = e.pop()
	}
	return
}

// Here we assign unique ids to all the function instances,
// so we can reference it when we compile each function instance.
func (e *engine) PreCompile(fs []*wasm.FunctionInstance) error {
	var newUniqueHostFunctions, newUniqueWasmFunctions int
	for _, f := range fs {
		if f.HostFunction != nil {
			if _, ok := e.hostFunctionIndex[f]; ok {
				continue
			}
			id := getNewID(e.hostFunctionIndex)
			e.hostFunctionIndex[f] = id
			newUniqueHostFunctions++
		} else {
			if _, ok := e.compiledWasmFunctionIndex[f]; ok {
				continue
			}
			id := getNewID(e.compiledWasmFunctionIndex)
			e.compiledWasmFunctionIndex[f] = id
			newUniqueWasmFunctions++
		}
	}
	e.hostFunctions = append(
		e.hostFunctions,
		make([]hostFunction, newUniqueHostFunctions)...,
	)
	e.compiledWasmFunctions = append(
		e.compiledWasmFunctions,
		make([]*compiledWasmFunction, newUniqueWasmFunctions)...,
	)
	return nil
}

func getNewID(idMap map[*wasm.FunctionInstance]int64) int64 {
	return int64(len(idMap))
}

func (e *engine) Compile(f *wasm.FunctionInstance) error {
	if f.HostFunction != nil {
		id := e.hostFunctionIndex[f]
		if e.hostFunctions[id] != nil {
			// Already compiled.
			return nil
		}
		hf := func(ctx *wasm.HostFunctionCallContext) {
			tp := f.HostFunction.Type()
			in := make([]reflect.Value, tp.NumIn())
			for i := len(in) - 1; i >= 1; i-- {
				val := reflect.New(tp.In(i)).Elem()
				raw := e.pop()
				kind := tp.In(i).Kind()
				switch kind {
				case reflect.Float64, reflect.Float32:
					val.SetFloat(math.Float64frombits(raw))
				case reflect.Uint32, reflect.Uint64:
					val.SetUint(raw)
				case reflect.Int32, reflect.Int64:
					val.SetInt(int64(raw))
				}
				in[i] = val
			}
			val := reflect.New(tp.In(0)).Elem()
			val.Set(reflect.ValueOf(ctx))
			in[0] = val
			for _, ret := range f.HostFunction.Call(in) {
				switch ret.Kind() {
				case reflect.Float64, reflect.Float32:
					e.push(math.Float64bits(ret.Float()))
				case reflect.Uint32, reflect.Uint64:
					e.push(ret.Uint())
				case reflect.Int32, reflect.Int64:
					e.push(uint64(ret.Int()))
				default:
					panic("invalid return type")
				}
			}
		}
		e.hostFunctions[id] = hf
	} else {
		id := e.compiledWasmFunctionIndex[f]
		if e.compiledWasmFunctions[id] != nil {
			// Already compiled.
			return nil
		}
		cf, err := e.compileWasmFunction(f)
		if err != nil {
			return fmt.Errorf("failed to compile Wasm function: %w", err)
		}
		e.compiledWasmFunctions[id] = cf
	}
	return nil
}

func NewEngine() wasm.Engine {
	return newEngine()
}

const initialStackSize = 1024

func newEngine() *engine {
	e := &engine{
		stack:                     make([]uint64, initialStackSize),
		compiledWasmFunctionIndex: make(map[*wasm.FunctionInstance]int64),
		hostFunctionIndex:         make(map[*wasm.FunctionInstance]int64),
	}
	return e
}

func (e *engine) pop() (ret uint64) {
	ret = e.stack[e.currentBaseStackPointer+e.currentStackPointer-1]
	e.currentStackPointer--
	return
}

func (e *engine) push(v uint64) {
	e.stack[e.currentBaseStackPointer+e.currentStackPointer] = v
	e.currentStackPointer++
}

// jitCallStatusCode represents the result of `jitcall`.
// This is set by the jitted native code.
type jitCallStatusCode uint32

const (
	// jitStatusReturned means the jitcall reaches the end of function, and returns successfully.
	jitCallStatusCodeReturned jitCallStatusCode = iota
	// jitCallStatusCodeCallWasmFunction means the jitcall returns to make a regular Wasm function call.
	jitCallStatusCodeCallWasmFunction
	// jitCallStatusCodeCallWasmFunction means the jitcall returns to make a builtin function call.
	jitCallStatusCodeCallBuiltInFunction
	// jitCallStatusCodeCallWasmFunction means the jitcall returns to make a host function call.
	jitCallStatusCodeCallHostFunction
	// TODO: trap, etc?
)

func (s jitCallStatusCode) String() (ret string) {
	switch s {
	case jitCallStatusCodeReturned:
		ret = "returned"
	case jitCallStatusCodeCallWasmFunction:
		ret = "call_wasm_function"
	case jitCallStatusCodeCallBuiltInFunction:
		ret = "call_builtin_function"
	case jitCallStatusCodeCallHostFunction:
		ret = "call_host_function"
	}
	return
}

// These consts are used in native codes to manipulate the engine's fields.
const (
	engineStackSliceOffset              = 0
	engineCurrentStackPointerOffset     = 24
	engineCurrentBaseStackPointerOffset = 32
	engineJITCallStatusCodeOffset       = 40
	engineFunctionCallIndexOffset       = 48
	engineContinuationAddressOffset     = 56
)

type callFrame struct {
	continuationAddress      uintptr
	continuationStackPointer uint64
	baseStackPointer         uint64
	f                        *compiledWasmFunction
	caller                   *callFrame
}

func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[continuation address=%d, continuation stack pointer=%d, base stack pointer=%d]",
		c.continuationAddress, c.continuationStackPointer, c.baseStackPointer,
	)
}

type compiledWasmFunction struct {
	// inputs,returns represents the number of input/returns of function.
	inputs, returns uint64
	// codeSegment is holding the compiled native code as a byte slice.
	codeSegment []byte
	// memory is the pointer to a memory instance which the original function instance refers to.
	memory *wasm.MemoryInstance
	// Pre-calculated pointer pointing to the initial byte of .codeSegment slice.
	// That mean codeInitialAddress always equals uintptr(unsafe.Pointer(&.codeSegment[0]))
	// and we cache the value (uintptr(unsafe.Pointer(&.codeSegment[0]))) to this field
	// so we don't need to repeat the calculation on each function call.
	codeInitialAddress uintptr
	// The same purpose as codeInitialAddress, but for memory.Buffer.
	memoryAddress uintptr
	// The max of the stack pointer this function can reach. Lazily applied via maybeGrowStack.
	maxStackPointer uint64
}

const (
	builtinFunctionIndexGrowMemory = iota
)

// Grow the stack size according to maxStackPointer argument
// which is the max stack pointer from the base pointer
// for the next function frame execution.
func (e *engine) maybeGrowStack(maxStackPointer uint64) {
	currentLen := uint64(len(e.stack))
	remained := currentLen - e.currentBaseStackPointer
	if maxStackPointer > remained {
		// This case we need to grow the stack as the empty slots
		// are not able to store all the stack items.
		// So we grow the stack with the new len = currentLen*2+required.
		newStack := make([]uint64, currentLen*2+(maxStackPointer))
		top := e.currentBaseStackPointer + e.currentStackPointer
		copy(newStack[:top], e.stack[:top])
		e.stack = newStack
	}
	// TODO: maybe better think about how to shrink the stack as well.
}

func (e *engine) exec(f *compiledWasmFunction) {
	e.callFrameStack = &callFrame{
		continuationAddress:      f.codeInitialAddress,
		f:                        f,
		caller:                   nil,
		continuationStackPointer: f.inputs,
	}
	// If the Go-allocated stack is running out, we grow it before calling into JITed code.
	e.maybeGrowStack(f.maxStackPointer)
	for e.callFrameStack != nil {
		currentFrame := e.callFrameStack
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s, currentBaseStackPointer: %d, currentStackPointer: %d, stack: %v\n",
				currentFrame.String(), e.currentBaseStackPointer, e.currentStackPointer,
				e.stack[:e.currentBaseStackPointer+e.currentStackPointer],
			)
		}

		// Call into the jitted code.
		jitcall(
			currentFrame.continuationAddress,
			uintptr(unsafe.Pointer(e)),
			currentFrame.f.memoryAddress,
		)

		// Check the status code from JIT code.
		switch e.jitCallStatusCode {
		case jitCallStatusCodeReturned:
			// Meaning that the current frame exits
			// so we just get back to the caller's frame.
			callerFrame := currentFrame.caller
			e.callFrameStack = callerFrame
			if callerFrame != nil {
				e.currentBaseStackPointer = callerFrame.baseStackPointer
				e.currentStackPointer = callerFrame.continuationStackPointer
			}
		case jitCallStatusCodeCallWasmFunction:
			// This never panics as we made sure that the index exists for all the referenced functions
			// in a module.
			nextFunc := e.compiledWasmFunctions[e.functionCallIndex]
			// Calculate the continuation address so
			// we can resume this caller function frame.
			currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
			currentFrame.continuationStackPointer = e.currentStackPointer + nextFunc.returns - nextFunc.inputs
			currentFrame.baseStackPointer = e.currentBaseStackPointer
			// Create the callee frame.
			frame := &callFrame{
				continuationAddress: nextFunc.codeInitialAddress,
				f:                   nextFunc,
				// Set the caller frame so we can return back to the current frame!
				caller: currentFrame,
				// Set the base pointer to the beginning of the function inputs
				baseStackPointer: e.currentBaseStackPointer + e.currentStackPointer - nextFunc.inputs,
			}
			// If the Go-allocated stack is running out, we grow it before calling into JITed code.
			e.maybeGrowStack(nextFunc.maxStackPointer)
			// Now move onto the callee function.
			e.callFrameStack = frame
			e.currentBaseStackPointer = frame.baseStackPointer
			// Set the stack pointer so that base+sp would point to the top of function inputs.
			e.currentStackPointer = nextFunc.inputs
		case jitCallStatusCodeCallBuiltInFunction:
			// TODO: check the signature and modify stack pointer.
			switch e.functionCallIndex {
			case builtinFunctionIndexGrowMemory:
				v := e.pop()
				e.memoryGrow(currentFrame.f.memory, v)
			}
			currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
		case jitCallStatusCodeCallHostFunction:
			e.hostFunctions[e.functionCallIndex](&wasm.HostFunctionCallContext{Memory: f.memory})
			// TODO: check the signature and modify stack pointer.
			currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
		}
	}
}

func (e *engine) memoryGrow(m *wasm.MemoryInstance, newPages uint64) {
	max := uint64(math.MaxUint32)
	if m.Max != nil {
		max = uint64(*m.Max) * wasm.PageSize
	}
	// If exceeds the max of memory size, we push -1 according to the spec
	if uint64(newPages*wasm.PageSize+uint64(len(m.Buffer))) > max {
		v := int32(-1)
		e.push(uint64(v))
	} else {
		e.push(uint64(uint64(len(m.Buffer)) / wasm.PageSize))
		m.Buffer = append(m.Buffer, make([]byte, newPages*wasm.PageSize)...)
	}
}
