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
	jitCallStatusCode jitStatusCodes
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
	hostFunctions     []func(ctx *wasm.HostFunctionCallContext)
	hostFunctionIndex map[*wasm.FunctionInstance]int64
}

var _ wasm.Engine = &engine{}

func (e *engine) Call(f *wasm.FunctionInstance, args ...uint64) (returns []uint64, err error) {
	for _, arg := range args {
		e.push(arg)
	}
	if index, ok := e.hostFunctionIndex[f]; ok {
		e.hostFunctions[index](&wasm.HostFunctionCallContext{Memory: f.ModuleInstance.Memory})
	} else if index, ok := e.compiledWasmFunctionIndex[f]; ok {
		f := e.compiledWasmFunctions[index]
		e.exec(f)
	} else {
		err = fmt.Errorf("invalid function")
		return
	}
	returns = make([]uint64, len(f.Signature.ReturnTypes))
	for i := range returns {
		returns[len(returns)-1-i] = e.pop()
	}
	return
}

// PreCompile implements wasm.Engine for engine.
// Here we assign unique ids to all the function instances,
// so we can reference it when we compile each function instance.
func (e *engine) PreCompile(fs []*wasm.FunctionInstance) error {
	var newUniqueHostFunctions, newUniqueWasmFunctions int
	for _, f := range fs {
		if f.HostFunction != nil {
			if _, ok := e.hostFunctionIndex[f]; ok {
				continue
			}
			id := int64(len(e.hostFunctionIndex))
			e.hostFunctionIndex[f] = id
			newUniqueHostFunctions++
		} else {
			if _, ok := e.compiledWasmFunctionIndex[f]; ok {
				continue
			}
			id := int64(len(e.compiledWasmFunctionIndex))
			e.compiledWasmFunctionIndex[f] = id
			newUniqueWasmFunctions++
		}
	}
	e.hostFunctions = append(
		e.hostFunctions,
		make([]func(ctx *wasm.HostFunctionCallContext), newUniqueHostFunctions)...,
	)
	e.compiledWasmFunctions = append(
		e.compiledWasmFunctions,
		make([]*compiledWasmFunction, newUniqueWasmFunctions)...,
	)
	return nil
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

const initialStackSize = 100

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

type jitStatusCodes uint32

const (
	jitStatusReturned jitStatusCodes = iota
	jitStatusCallWasmFunction
	jitStatusCallBuiltInFunction
	jitStatusCallHostFunction
	// TODO: trap, etc?
)

var (
	engineStackSliceOffset              = int64(unsafe.Offsetof((&engine{}).stack))
	engineCurrentStackPointerOffset     = int64(unsafe.Offsetof((&engine{}).currentStackPointer))
	engineCurrentBaseStackPointerOffset = int64(unsafe.Offsetof((&engine{}).currentBaseStackPointer))
	engineJITStatusOffset               = int64(unsafe.Offsetof((&engine{}).jitCallStatusCode))
	engineFunctionCallIndexOffset       = int64(unsafe.Offsetof((&engine{}).functionCallIndex))
	engineContinuationAddressOffset     = int64(unsafe.Offsetof((&engine{}).continuationAddressOffset))
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
		"[continuation address=%d, continuation stack poitner=%d, base stack pointer=%d]",
		c.continuationAddress, c.continuationStackPointer, c.baseStackPointer,
	)
}

type compiledWasmFunction struct {
	inputNum, outputNum uint64
	codeSegment         []byte
	memoryInst          *wasm.MemoryInstance
	codeInitialAddress  uintptr
	memoryAddress       uintptr
}

const (
	builtinFunctionIndexGrowMemory = iota
)

func (e *engine) stackGrow() {
	newStack := make([]uint64, len(e.stack)*2)
	copy(newStack[:len(e.stack)], e.stack)
	e.stack = newStack
}

func (e *engine) exec(f *compiledWasmFunction) {
	e.callFrameStack = &callFrame{
		continuationAddress:      f.codeInitialAddress,
		f:                        f,
		caller:                   nil,
		continuationStackPointer: f.inputNum,
	}
	// TODO: We should check the size of the stack,
	// and if it's running out, grow it before calling into JITed code.
	// It should be possible to check the necessity by statically
	// analyzing the max height of the stack in the function.
	if false {
		e.stackGrow()
	}
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
		case jitStatusReturned:
			// Meaning that the current frame exits
			// so we just get back to the caller's frame.
			callerFrame := currentFrame.caller
			e.callFrameStack = callerFrame
			if callerFrame != nil {
				e.currentBaseStackPointer = callerFrame.baseStackPointer
				e.currentStackPointer = callerFrame.continuationStackPointer
			}
		case jitStatusCallWasmFunction:
			nextFunc := e.compiledWasmFunctions[e.functionCallIndex]
			// Calculate the continuation address so
			// we can resume this caller function frame.
			currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
			currentFrame.continuationStackPointer = e.currentStackPointer + nextFunc.outputNum - nextFunc.inputNum
			currentFrame.baseStackPointer = e.currentBaseStackPointer
			// Create the callee frame.
			frame := &callFrame{
				continuationAddress: nextFunc.codeInitialAddress,
				f:                   nextFunc,
				// Set the caller frame so we can return back to the current frame!
				caller: currentFrame,
				// Set the base pointer to the beginning of the function inputs
				baseStackPointer: e.currentBaseStackPointer + e.currentStackPointer - nextFunc.inputNum,
			}
			// TODO: We should check the size of the stack,
			// and if it's running out, grow it before calling into JITed code.
			// It should be possible to check the necessity by statically
			// analyzing the max height of the stack in the function.
			if false {
				e.stackGrow()
			}
			// Now move onto the callee function.
			e.callFrameStack = frame
			e.currentBaseStackPointer = frame.baseStackPointer
			// Set the stack pointer so that base+sp would point to the top of function inputs.
			e.currentStackPointer = nextFunc.inputNum
		case jitStatusCallBuiltInFunction:
			// TODO: check the signature and modify stack pointer.
			switch e.functionCallIndex {
			case builtinFunctionIndexGrowMemory:
				v := e.pop()
				e.memoryGrow(currentFrame.f.memoryInst, v)
			default:
				panic("invalid builtin function index")
			}
			currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
		case jitStatusCallHostFunction:
			e.hostFunctions[e.functionCallIndex](&wasm.HostFunctionCallContext{Memory: f.memoryInst})
			// TODO: check the signature and modify stack pointer.
			currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
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
