package jit

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unsafe"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

type engine struct {
	// These fields are used and manipulated by JITed code.

	// The actual Go-allocated stack.
	// Note that we NEVER edit len or cap in JITed code so we won't get screwed when GC comes in.
	stack []uint64
	// Stack pointer on .stack field which is accessed by [stackBasePointer] + [stackPointer].
	stackPointer uint64
	// stackBasePointer is set whenever we make function calls.
	// Background: Functions might be compiled as if they use the stack from the bottom,
	// however in reality, they have to use it from the middle of the stack depending on
	// when these function calls are made. So instead of accessing stack via stackPointer alone,
	// functions are compiled so they access the stack via [stackBasePointer](fixed for entire function) + [stackPointer].
	// More precisely, stackBasePointer is set to [callee's stack pointer] + [callee's stack base pointer] - [caller's params].
	// This way, compiled functions can be independent of the timing of functions calls made against them.
	stackBasePointer uint64
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
	// The current compiledWasmFunction.globalSliceAddress
	globalSliceAddress uintptr
	// memorySliceLen stores the length of memory slice used by the currently executed function.
	memorySliceLen int
	// Function call frames in linked list
	callFrameStack *callFrame
	// callFrameNum tracks the current number of call frames.
	// Note: this is not len(callFrameStack) because the stack is implemented as a linked list.
	callFrameNum uint64

	// The following fields are only used during compilation.

	// Store the compiled functions and indexes.
	compiledWasmFunctions     []*compiledWasmFunction
	compiledWasmFunctionIndex map[*wasm.FunctionInstance]int64
	// Store the host functions and indexes.
	compiledHostFunctions     []*compiledHostFunction
	compiledHostFunctionIndex map[*wasm.FunctionInstance]int64
}

// Native code manipulates the engine's fields with these constants.
const (
	engineStackSliceOffset          = 0
	enginestackPointerOffset        = 24
	enginestackBasePointerOffset    = 32
	engineJITCallStatusCodeOffset   = 40
	engineFunctionCallIndexOffset   = 48
	engineContinuationAddressOffset = 56
	engineglobalSliceAddressOffset  = 64
	engineMemorySliceLenOffset      = 72
)

func (e *engine) Call(f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	// We ensure that this Call method never panics as
	// this Call method is indirectly invoked by embedders via store.CallFunction,
	// and we have to make sure that all the runtime errors, including the one happening inside
	// host functions, will be captured as errors, not panics.

	// shouldRecover is true when a panic at the origin of callstack should be recovered
	//
	// If this is the recursive call into Wasm (e.callFrameStack != nil), we do not recover, and delegate the
	// recovery to the first engine.Call.
	//
	// For example, given the call stack:
	//	 "original host function" --(engine.Call)--> Wasm func A --> Host func --(engine.Call)--> Wasm function B,
	// if the top Wasm function panics, we go back to the "original host function".
	shouldRecover := e.callFrameStack == nil
	defer func() {
		if shouldRecover {
			if v := recover(); v != nil {
				top := e.callFrameStack
				var frames []string
				var counter int
				for top != nil {
					frames = append(frames, fmt.Sprintf("\t%d: %s", counter, top.getFunctionName()))
					top = top.caller
					counter++
					// TODO: include DWARF symbols. See #58
				}
				runtimeErr, ok := v.(error)
				if ok {
					err = fmt.Errorf("wasm runtime error: %w", runtimeErr)
				} else {
					err = fmt.Errorf("wasm runtime error: %v", v)
				}

				if len(frames) > 0 {
					err = fmt.Errorf("%w\nwasm backtrace:\n%s", err, strings.Join(frames, "\n"))
				}
				// Reset the state so this engine can be reused
				e.callFrameStack = nil
				e.stackBasePointer = 0
				e.stackPointer = 0
			}
		}
	}()

	for _, param := range params {
		e.push(param)
	}
	// Note that there's no conflict between e.hostFunctionIndex and e.compiledWasmFunctionIndex,
	// meaning that each *wasm.FunctionInstance is assigned to either host function index or wasm function one.
	if index, ok := e.compiledHostFunctionIndex[f]; ok {
		e.compiledHostFunctions[index].f(&wasm.HostFunctionCallContext{Memory: f.ModuleInstance.Memory})
	} else if index, ok := e.compiledWasmFunctionIndex[f]; ok {
		f := e.compiledWasmFunctions[index]
		e.exec(f)
	} else {
		err = fmt.Errorf("function not compiled")
		return
	}
	// Note the top value is the tail of the results,
	// so we assign them in reverse order.
	results = make([]uint64, len(f.Signature.Results))
	for i := range results {
		results[len(results)-1-i] = e.pop()
	}
	return
}

// Here we assign unique ids to all the function instances,
// so we can reference it when we compile each function instance.
func (e *engine) PreCompile(fs []*wasm.FunctionInstance) error {
	var newUniqueHostFunctions, newUniqueWasmFunctions int
	for _, f := range fs {
		if f.IsHostFunction() {
			if _, ok := e.compiledHostFunctionIndex[f]; ok {
				continue
			}
			id := getNewID(e.compiledHostFunctionIndex)
			e.compiledHostFunctionIndex[f] = id
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
	e.compiledHostFunctions = append(
		e.compiledHostFunctions,
		make([]*compiledHostFunction, newUniqueHostFunctions)...,
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
	if f.IsHostFunction() {
		id := e.compiledHostFunctionIndex[f]
		if e.compiledHostFunctions[id] != nil {
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
		e.compiledHostFunctions[id] = &compiledHostFunction{f: hf, name: f.Name}
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
		compiledHostFunctionIndex: make(map[*wasm.FunctionInstance]int64),
	}
	return e
}

func (e *engine) pop() (ret uint64) {
	ret = e.stack[e.stackBasePointer+e.stackPointer-1]
	e.stackPointer--
	return
}

func (e *engine) push(v uint64) {
	e.stack[e.stackBasePointer+e.stackPointer] = v
	e.stackPointer++
}

var callStackCeiling = uint64(buildoptions.CallStackCeiling)

func (e *engine) callFramePush(next *callFrame) {
	e.callFrameNum++
	if callStackCeiling < e.callFrameNum {
		panic(wasm.ErrCallStackOverflow)
	}
	// Push the new frame to the top of stack.
	next.caller = e.callFrameStack
	e.callFrameStack = next

	// Initialize the callframe-specific fields (stackBasePointer, stackPointer, globalSliceAddress, memorySliceLen).
	e.stackBasePointer = next.stackBasePointer
	// Set the stack pointer so that base+sp would point to the top of function params.
	e.stackPointer = next.wasmFunction.paramCount
	e.globalSliceAddress = next.wasmFunction.globalSliceAddress
	if next.wasmFunction.memory != nil {
		e.memorySliceLen = len(next.wasmFunction.memory.Buffer)
	}
}

func (e *engine) callFramePop() {
	// Pop the old callframe from the top of stack.
	e.callFrameNum--
	caller := e.callFrameStack.caller
	e.callFrameStack = caller

	// If the caller is not nil, we have to go back into the caller's frame.
	if caller != nil {
		// Revert the callframe-specific fields (stackBasePointer, stackPointer, globalSliceAddress, memorySliceLen).
		// so we can go back to the exact state when the caller made the function call.
		e.stackBasePointer = caller.stackBasePointer
		e.stackPointer = caller.continuationStackPointer
		e.globalSliceAddress = caller.wasmFunction.globalSliceAddress
		if caller.wasmFunction.memory != nil {
			e.memorySliceLen = len(caller.wasmFunction.memory.Buffer)
		}
	}
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
	// jitCallStatusCodeUnreachable means the function invocation reaches "unreachable" instruction.
	jitCallStatusCodeUnreachable
	// jitCallStatusCodeInvalidFloatToIntConversion means a invalid conversion of integer to floats happened.
	jitCallStatusCodeInvalidFloatToIntConversion
	// jitCallStatusCodeMemoryOutOfBounds means a out of bounds memory access happened.
	jitCallStatusCodeMemoryOutOfBounds
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
	case jitCallStatusCodeUnreachable:
		ret = "unreachable"
	}
	return
}

type callFrame struct {
	continuationAddress      uintptr
	continuationStackPointer uint64
	stackBasePointer         uint64
	wasmFunction             *compiledWasmFunction
	hostFunction             *compiledHostFunction
	caller                   *callFrame
}

func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[%s: continuation address=%d, continuation stack pointer=%d, stack base pointer=%d]",
		c.getFunctionName(), c.continuationAddress, c.continuationStackPointer, c.stackBasePointer,
	)
}

func (c *callFrame) getFunctionName() string {
	if c.wasmFunction != nil {
		return c.wasmFunction.source.Name
	} else {
		return c.hostFunction.name
	}
}

type compiledHostFunction = struct {
	f    func(ctx *wasm.HostFunctionCallContext)
	name string
}

type compiledWasmFunction struct {
	// The source function instance from which this is compiled.
	source                  *wasm.FunctionInstance
	paramCount, resultCount uint64
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
	// globalSliceAddress is like codeInitialAddress, but for .globals.
	globalSliceAddress uintptr
	// The max of the stack pointer this function can reach. Lazily applied via maybeGrowStack.
	maxStackPointer uint64
}

const (
	builtinFunctionIndexMemoryGrow = iota
	builtinFunctionIndexMemorySize
)

// Grow the stack size according to maxStackPointer argument
// which is the max stack pointer from the base pointer
// for the next function frame execution.
func (e *engine) maybeGrowStack(maxStackPointer uint64) {
	currentLen := uint64(len(e.stack))
	remained := currentLen - e.stackBasePointer
	if maxStackPointer > remained {
		// This case we need to grow the stack as the empty slots
		// are not able to store all the stack items.
		// So we grow the stack with the new len = currentLen*2+maxStackPointer.
		newStack := make([]uint64, currentLen*2+(maxStackPointer))
		top := e.stackBasePointer + e.stackPointer
		copy(newStack[:top], e.stack[:top])
		e.stack = newStack
	}
	// TODO: maybe better think about how to shrink the stack as well.
}

func (e *engine) exec(f *compiledWasmFunction) {
	// Push a new call frame for the target function.
	e.callFramePush(&callFrame{continuationAddress: f.codeInitialAddress, wasmFunction: f})

	// If the Go-allocated stack is running out, we grow it before calling into JITed code.
	e.maybeGrowStack(f.maxStackPointer)
	for e.callFrameStack != nil {
		currentFrame := e.callFrameStack
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s (at %d), stackBasePointer: %d, stackPointer: %d\n",
				currentFrame.String(), e.callFrameNum, e.stackBasePointer, e.stackPointer)
		}

		// Call into the jitted code.
		jitcall(
			currentFrame.continuationAddress,
			uintptr(unsafe.Pointer(e)),
			currentFrame.wasmFunction.memoryAddress,
		)

		// Check the status code from JIT code.
		switch e.jitCallStatusCode {
		case jitCallStatusCodeReturned:
			// Meaning that the current frame exits
			// so restore the caller's frame.
			e.callFramePop()
		case jitCallStatusCodeCallWasmFunction:
			// This never panics as we made sure that the index exists for all the referenced functions
			// in a module.
			nextFunc := e.compiledWasmFunctions[e.functionCallIndex]
			// Calculate the continuation address so we can resume this caller function frame.
			currentFrame.continuationAddress = currentFrame.wasmFunction.codeInitialAddress + e.continuationAddressOffset
			currentFrame.continuationStackPointer = e.stackPointer + nextFunc.resultCount - nextFunc.paramCount
			// Create the callee frame.
			frame := &callFrame{
				continuationAddress: nextFunc.codeInitialAddress,
				wasmFunction:        nextFunc,
				// Set the base pointer to the beginning of the function params
				stackBasePointer: e.stackBasePointer + e.stackPointer - nextFunc.paramCount,
			}
			e.callFramePush(frame)
			// If the Go-allocated stack is running out, we grow it before calling into JITed code.
			e.maybeGrowStack(nextFunc.maxStackPointer)
		case jitCallStatusCodeCallBuiltInFunction:
			switch e.functionCallIndex {
			case builtinFunctionIndexMemoryGrow:
				e.builtinFunctionMemoryGrow(currentFrame.wasmFunction)
			case builtinFunctionIndexMemorySize:
				e.builtinFunctionMemorySize(currentFrame.wasmFunction)
			}
			currentFrame.continuationAddress = currentFrame.wasmFunction.codeInitialAddress + e.continuationAddressOffset
		case jitCallStatusCodeCallHostFunction:
			targetHostFunction := e.compiledHostFunctions[e.functionCallIndex]
			currentFrame.continuationAddress = currentFrame.wasmFunction.codeInitialAddress + e.continuationAddressOffset
			// Push the call frame for this host function.
			e.callFrameStack = &callFrame{hostFunction: targetHostFunction, caller: currentFrame}
			// Call into the host function.
			targetHostFunction.f(&wasm.HostFunctionCallContext{Memory: currentFrame.wasmFunction.memory})
			// Pop the call frame.
			e.callFrameStack = currentFrame
		case jitCallStatusCodeInvalidFloatToIntConversion:
			// TODO: have wasm.ErrInvalidFloatToIntConversion and use it here.
			panic("invalid float to int conversion")
		case jitCallStatusCodeUnreachable:
			// TODO: have wasm.ErrUnreachable and use it here.
			panic("unreachable")
		case jitCallStatusCodeMemoryOutOfBounds:
			// TODO: have wasm.ErrMemoryOutOfBounds and use it here.
			panic("out of bounds memory access")
		}
	}
}

func (e *engine) builtinFunctionMemoryGrow(f *compiledWasmFunction) {
	newPages := e.pop()
	max := uint64(math.MaxUint32)
	if f.memory.Max != nil {
		max = uint64(*f.memory.Max) * wasm.PageSize
	}
	// If exceeds the max of memory size, we push -1 according to the spec.
	if uint64(newPages*wasm.PageSize+uint64(len(f.memory.Buffer))) > max {
		v := int32(-1)
		e.push(uint64(v))
	} else {
		e.builtinFunctionMemorySize(f) // Grow returns the prior memory size on change.
		f.memory.Buffer = append(f.memory.Buffer, make([]byte, newPages*wasm.PageSize)...)
		f.memoryAddress = uintptr(unsafe.Pointer(&f.memory.Buffer[0]))
	}
}

func (e *engine) builtinFunctionMemorySize(f *compiledWasmFunction) {
	e.push(uint64(len(f.memory.Buffer)) / wasm.PageSize)
}

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledWasmFunction, error) {
	ir, err := wazeroir.Compile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to lower to wazeroir: %w", err)
	}

	if buildoptions.IsDebugMode {
		fmt.Printf("compilation target wazeroir:\n%s\n", wazeroir.Format(ir.Operations))
	}

	compiler, err := newCompiler(e, f, ir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize assembly builder: %w", err)
	}

	compiler.emitPreamble()

	for _, op := range ir.Operations {
		if buildoptions.IsDebugMode {
			fmt.Printf("compiling op=%s: %s\n", op.Kind(), compiler)
		}
		var err error
		switch o := op.(type) {
		case *wazeroir.OperationUnreachable:
			err = compiler.compileUnreachable()
		case *wazeroir.OperationLabel:
			err = compiler.compileLabel(o)
		case *wazeroir.OperationBr:
			err = compiler.compileBr(o)
		case *wazeroir.OperationBrIf:
			err = compiler.compileBrIf(o)
		case *wazeroir.OperationBrTable:
			err = compiler.compileBrTable(o)
		case *wazeroir.OperationCall:
			err = compiler.compileCall(o)
		case *wazeroir.OperationCallIndirect:
			err = compiler.compileCallIndirect(o)
		case *wazeroir.OperationDrop:
			err = compiler.compileDrop(o)
		case *wazeroir.OperationSelect:
			err = compiler.compileSelect()
		case *wazeroir.OperationPick:
			err = compiler.compilePick(o)
		case *wazeroir.OperationSwap:
			err = compiler.compileSwap(o)
		case *wazeroir.OperationGlobalGet:
			err = compiler.compileGlobalGet(o)
		case *wazeroir.OperationGlobalSet:
			err = compiler.compileGlobalSet(o)
		case *wazeroir.OperationLoad:
			err = compiler.compileLoad(o)
		case *wazeroir.OperationLoad8:
			err = compiler.compileLoad8(o)
		case *wazeroir.OperationLoad16:
			err = compiler.compileLoad16(o)
		case *wazeroir.OperationLoad32:
			err = compiler.compileLoad32(o)
		case *wazeroir.OperationStore:
			err = compiler.compileStore(o)
		case *wazeroir.OperationStore8:
			err = compiler.compileStore8(o)
		case *wazeroir.OperationStore16:
			err = compiler.compileStore16(o)
		case *wazeroir.OperationStore32:
			err = compiler.compileStore32(o)
		case *wazeroir.OperationMemorySize:
			err = compiler.compileMemorySize()
		case *wazeroir.OperationMemoryGrow:
			err = compiler.compileMemoryGrow()
		case *wazeroir.OperationConstI32:
			err = compiler.compileConstI32(o)
		case *wazeroir.OperationConstI64:
			err = compiler.compileConstI64(o)
		case *wazeroir.OperationConstF32:
			err = compiler.compileConstF32(o)
		case *wazeroir.OperationConstF64:
			err = compiler.compileConstF64(o)
		case *wazeroir.OperationEq:
			err = compiler.compileEq(o)
		case *wazeroir.OperationNe:
			err = compiler.compileNe(o)
		case *wazeroir.OperationEqz:
			err = compiler.compileEqz(o)
		case *wazeroir.OperationLt:
			err = compiler.compileLt(o)
		case *wazeroir.OperationGt:
			err = compiler.compileGt(o)
		case *wazeroir.OperationLe:
			err = compiler.compileLe(o)
		case *wazeroir.OperationGe:
			err = compiler.compileGe(o)
		case *wazeroir.OperationAdd:
			err = compiler.compileAdd(o)
		case *wazeroir.OperationSub:
			err = compiler.compileSub(o)
		case *wazeroir.OperationMul:
			err = compiler.compileMul(o)
		case *wazeroir.OperationClz:
			err = compiler.compileClz(o)
		case *wazeroir.OperationCtz:
			err = compiler.compileCtz(o)
		case *wazeroir.OperationPopcnt:
			err = compiler.compilePopcnt(o)
		case *wazeroir.OperationDiv:
			err = compiler.compileDiv(o)
		case *wazeroir.OperationRem:
			err = compiler.compileRem(o)
		case *wazeroir.OperationAnd:
			err = compiler.compileAnd(o)
		case *wazeroir.OperationOr:
			err = compiler.compileOr(o)
		case *wazeroir.OperationXor:
			err = compiler.compileXor(o)
		case *wazeroir.OperationShl:
			err = compiler.compileShl(o)
		case *wazeroir.OperationShr:
			err = compiler.compileShr(o)
		case *wazeroir.OperationRotl:
			err = compiler.compileRotl(o)
		case *wazeroir.OperationRotr:
			err = compiler.compileRotr(o)
		case *wazeroir.OperationAbs:
			err = compiler.compileAbs(o)
		case *wazeroir.OperationNeg:
			err = compiler.compileNeg(o)
		case *wazeroir.OperationCeil:
			err = compiler.compileCeil(o)
		case *wazeroir.OperationFloor:
			err = compiler.compileFloor(o)
		case *wazeroir.OperationTrunc:
			err = compiler.compileTrunc(o)
		case *wazeroir.OperationNearest:
			err = compiler.compileNearest(o)
		case *wazeroir.OperationSqrt:
			err = compiler.compileSqrt(o)
		case *wazeroir.OperationMin:
			err = compiler.compileMin(o)
		case *wazeroir.OperationMax:
			err = compiler.compileMax(o)
		case *wazeroir.OperationCopysign:
			err = compiler.compileCopysign(o)
		case *wazeroir.OperationI32WrapFromI64:
			err = compiler.compileI32WrapFromI64()
		case *wazeroir.OperationITruncFromF:
			err = compiler.compileITruncFromF(o)
		case *wazeroir.OperationFConvertFromI:
			err = compiler.compileFConvertFromI(o)
		case *wazeroir.OperationF32DemoteFromF64:
			err = compiler.compileF32DemoteFromF64()
		case *wazeroir.OperationF64PromoteFromF32:
			err = compiler.compileF64PromoteFromF32()
		case *wazeroir.OperationI32ReinterpretFromF32:
			err = compiler.compileI32ReinterpretFromF32()
		case *wazeroir.OperationI64ReinterpretFromF64:
			err = compiler.compileI64ReinterpretFromF64()
		case *wazeroir.OperationF32ReinterpretFromI32:
			err = compiler.compileF32ReinterpretFromI32()
		case *wazeroir.OperationF64ReinterpretFromI64:
			err = compiler.compileF64ReinterpretFromI64()
		case *wazeroir.OperationExtend:
			err = compiler.compileExtend(o)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to compile operation %s: %w", op.Kind().String(), err)
		}
	}

	code, maxStackPointer, err := compiler.generate()
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}

	if buildoptions.IsDebugMode {
		fmt.Printf("compiled code in hex: %s\n", hex.EncodeToString(code))
	}

	cf := &compiledWasmFunction{
		source:          f,
		codeSegment:     code,
		paramCount:      uint64(len(f.Signature.Params)),
		resultCount:     uint64(len(f.Signature.Results)),
		memory:          f.ModuleInstance.Memory,
		maxStackPointer: maxStackPointer,
	}
	if cf.memory != nil && len(cf.memory.Buffer) > 0 {
		cf.memoryAddress = uintptr(unsafe.Pointer(&cf.memory.Buffer[0]))
	}
	if len(f.ModuleInstance.Globals) > 0 {
		cf.globalSliceAddress = uintptr(unsafe.Pointer(&f.ModuleInstance.Globals[0]))
	}
	cf.codeInitialAddress = uintptr(unsafe.Pointer(&cf.codeSegment[0]))
	return cf, nil
}
