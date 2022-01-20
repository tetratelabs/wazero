package jit

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
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
	// Set when statusCode == jitStatusCall{Function,BuiltInFunction}
	// Indicating the function call index.
	functionCallAddress wasm.FunctionAddress
	// Set when statusCode == jitStatusCall{Function,BuiltInFunction}
	// We use this value to continue the current function
	// after calling the target function exits.
	// Instructions after [base+continuationAddressOffset] must start with
	// restoring reserved registeres.
	continuationAddressOffset uintptr
	// The current compiledFunction.globalSliceAddress
	globalSliceAddress uintptr
	// memorySliceLen stores the address of the first byte in the memory slice used by the currently executed function.
	memorySliceAddress uintptr
	// memorySliceLen stores the length of memory slice used by the currently executed function.
	memorySliceLen uint64
	// The current compiledFunction.tableSliceAddress
	tableSliceAddress uintptr
	// tableSliceLen stores the length of the unique table used by the currently executed function.
	tableSliceLen uint64
	// Function call frames in linked list
	callFrameStack *callFrame
	// callFrameNum tracks the current number of call frames.
	// Note: this is not len(callFrameStack) because the stack is implemented as a linked list.
	callFrameNum uint64

	// The following fields are only used during compilation.

	// Store the compiled functions.
	compiledFunctions map[wasm.FunctionAddress]*compiledFunction
}

// Native code manipulates the engine's fields with these constants.
const (
	engineStackSliceOffset          = 0
	enginestackPointerOffset        = 24
	enginestackBasePointerOffset    = 32
	engineJITCallStatusCodeOffset   = 40
	engineFunctionCallAddressOffset = 48
	engineContinuationAddressOffset = 56
	engineglobalSliceAddressOffset  = 64
	engineMemorySliceAddressOffset  = 72
	engineMemorySliceLenOffset      = 80
	engineTableSliceAddressOffset   = 88
	engineTableSliceLenOffset       = 96
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
				if buildoptions.IsDebugMode {
					debug.PrintStack()
				}
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
				e.callFrameNum = 0
			}
		}
	}()

	for _, param := range params {
		e.push(param)
	}

	compiled, ok := e.compiledFunctions[f.Address]
	if !ok {
		err = fmt.Errorf("function not compiled")
		return
	}

	if compiled.isHostFunction() {
		e.execHostFunction(compiled.source.HostFunction, &wasm.HostFunctionCallContext{Memory: f.ModuleInstance.Memory})
	} else {
		e.execFunction(compiled)
	}

	// Note the top value is the tail of the results,
	// so we assign them in reverse order.
	results = make([]uint64, len(f.FunctionType.Type.Results))
	for i := range results {
		results[len(results)-1-i] = e.pop()
	}
	return
}

func (e *engine) Compile(f *wasm.FunctionInstance) error {
	if f.IsHostFunction() {
		e.compiledFunctions[f.Address] = &compiledFunction{
			source:      f,
			paramCount:  uint64(len(f.FunctionType.Type.Params)),
			resultCount: uint64(len(f.FunctionType.Type.Results)),
		}
	} else {
		cf, err := e.compileWasmFunction(f)
		if err != nil {
			return fmt.Errorf("failed to compile Wasm function: %w", err)
		}
		e.compiledFunctions[f.Address] = cf
	}
	return nil
}

func NewEngine() wasm.Engine {
	return newEngine()
}

const initialStackSize = 1024

func newEngine() *engine {
	e := &engine{
		stack:             make([]uint64, initialStackSize),
		compiledFunctions: make(map[wasm.FunctionAddress]*compiledFunction),
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

func (e *engine) callFramePush(callee *callFrame) {
	e.callFrameNum++
	if callStackCeiling < e.callFrameNum {
		panic(wasm.ErrCallStackOverflow)
	}

	// Push the new frame to the top of stack.
	callee.caller = e.callFrameStack
	e.callFrameStack = callee

	e.callFrameStack.stackBasePointer = e.stackBasePointer + e.stackPointer - callee.compiledFunction.paramCount
	e.stackBasePointer = callee.stackBasePointer
	e.stackPointer = callee.compiledFunction.paramCount
	e.initModuleInstance(callee.compiledFunction.source.ModuleInstance)
}

func (e *engine) callFramePop() {
	// Pop the old callframe from the top of stack.
	e.callFrameNum--
	caller := e.callFrameStack.caller
	e.callFrameStack = caller

	// If the caller is not nil, we have to go back into the caller's frame.
	if caller != nil {
		e.stackBasePointer = caller.stackBasePointer
		e.stackPointer = caller.continuationStackPointer
		e.initModuleInstance(caller.compiledFunction.source.ModuleInstance)
	}
}

// initModuleInstance initializes the engine's state based on the given module instance.
func (e *engine) initModuleInstance(m *wasm.ModuleInstance) {
	if len(m.Globals) > 0 {
		e.globalSliceAddress = uintptr(unsafe.Pointer(&m.Globals[0]))
	}
	if tables := m.Tables; len(tables) > 0 {
		// WebAssembly 1.0 (MVP) has at most 1 table
		// See https://www.w3.org/TR/wasm-core-1/#tables%E2%91%A0
		table := tables[0]
		if len(table.Table) > 0 {
			e.tableSliceAddress = uintptr(unsafe.Pointer(&table.Table[0]))
		}
		e.tableSliceLen = uint64(len(table.Table))
	}
	if m.Memory != nil {
		e.memorySliceLen = uint64(len(m.Memory.Buffer))
		if len(m.Memory.Buffer) > 0 {
			e.memorySliceAddress = uintptr(unsafe.Pointer(&m.Memory.Buffer[0]))
		}
	}
}

// jitCallStatusCode represents the result of `jitcall`.
// This is set by the jitted native code.
type jitCallStatusCode uint32

const (
	// jitStatusReturned means the jitcall reaches the end of function, and returns successfully.
	jitCallStatusCodeReturned jitCallStatusCode = iota
	// jitCallStatusCodeCallFunction means the jitcall returns to make a regular Wasm function call.
	jitCallStatusCodeCallFunction
	// jitCallStatusCodeCallFunction means the jitcall returns to make a builtin function call.
	jitCallStatusCodeCallBuiltInFunction
	// jitCallStatusCodeUnreachable means the function invocation reaches "unreachable" instruction.
	jitCallStatusCodeUnreachable
	// jitCallStatusCodeInvalidFloatToIntConversion means a invalid conversion of integer to floats happened.
	jitCallStatusCodeInvalidFloatToIntConversion
	// jitCallStatusCodeMemoryOutOfBounds means a out of bounds memory access happened.
	jitCallStatusCodeMemoryOutOfBounds
	// jitCallStatusCodeTableOutOfBounds means the offset to table exceeds the length of table during call_indirect.
	jitCallStatusCodeTableOutOfBounds
	// jitCallStatusCodeTypeMismatchOnIndirectCall means the type check failed during call_indirect.
	jitCallStatusCodeTypeMismatchOnIndirectCall
)

func (s jitCallStatusCode) String() (ret string) {
	switch s {
	case jitCallStatusCodeReturned:
		ret = "returned"
	case jitCallStatusCodeCallFunction:
		ret = "call_function"
	case jitCallStatusCodeCallBuiltInFunction:
		ret = "call_builtin_function"
	case jitCallStatusCodeUnreachable:
		ret = "unreachable"
	}
	return
}

type callFrame struct {
	continuationAddress      uintptr
	continuationStackPointer uint64
	stackBasePointer         uint64
	compiledFunction         *compiledFunction
	caller                   *callFrame
}

func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[%s: continuation address=%d, continuation stack pointer=%d, stack base pointer=%d]",
		c.getFunctionName(), c.continuationAddress, c.continuationStackPointer, c.stackBasePointer,
	)
}

func (c *callFrame) getFunctionName() string {
	return c.compiledFunction.source.Name
}

type compiledFunction struct {
	// The source function instance from which this is compiled.
	source                  *wasm.FunctionInstance
	paramCount, resultCount uint64
	// codeSegment is holding the compiled native code as a byte slice.
	codeSegment []byte
	// Pre-calculated pointer pointing to the initial byte of .codeSegment slice.
	// That mean codeInitialAddress always equals uintptr(unsafe.Pointer(&.codeSegment[0]))
	// and we cache the value (uintptr(unsafe.Pointer(&.codeSegment[0]))) to this field
	// so we don't need to repeat the calculation on each function call.
	codeInitialAddress uintptr
	// The max of the stack pointer this function can reach. Lazily applied via maybeGrowStack.
	maxStackPointer uint64
	staticData      compiledFunctionStaticData
}

// staticData holds the read-only data (i.e. out side of codeSegment which is marked as executable) per function.
// This is used to store jump tables for br_table instructions.
// The primary index is the logical sepration of multiple data, for example data[0] and data[1]
// correspond to different jump tables for different br_table instructions.
type compiledFunctionStaticData = [][]byte

func (f *compiledFunction) isHostFunction() bool {
	return f.source.HostFunction != nil
}

const (
	builtinFunctionAddressMemoryGrow wasm.FunctionAddress = iota
	builtinFunctionAddressMemorySize
	// builtinFunctionAddressBreakPoint is internal (only for wazero developers). Disabled by default.
	builtinFunctionAddressBreakPoint
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

// execHostFunction executes the given host function represented as *reflect.Value.
//
// The arguments to the function are popped from the stack stack following the convension of
// Wasm stack machine.
// For example, if the host function F requires the (x1 uint32, x2 float32) parameters, and
// the stack is [..., A, B], then the function is called as F(A, B) where A and B are interpreted
// as uint32 and float32 respectively.
//
// After the execution, the result of host function is pushed onto the stack.
//
// ctx parameter is passed to the host function as a first argument.
func (e *engine) execHostFunction(f *reflect.Value, ctx *wasm.HostFunctionCallContext) {
	tp := f.Type()
	in := make([]reflect.Value, tp.NumIn())

	// We pop the value and pass them as arguments in a reverse order according to the
	// stack machine convension.
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

	// Host function must receive *wasm.HostFunctionCallContext as a first argument.
	val := reflect.New(tp.In(0)).Elem()
	val.Set(reflect.ValueOf(ctx))
	in[0] = val

	// Excute the host function and push back the call result onto the stack.
	for _, ret := range f.Call(in) {
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

func (e *engine) execFunction(f *compiledFunction) {
	previousTopFrame := e.callFrameStack

	// Push a new call frame for the target function.
	e.callFramePush(&callFrame{continuationAddress: f.codeInitialAddress, compiledFunction: f})

	// If the Go-allocated stack is running out, we grow it before calling into JITed code.
	e.maybeGrowStack(f.maxStackPointer)

	// We continuously execute functions until we reach the previous top frame which is either
	// nil if this is the initial call into Wasm, or the host function frame if this is the
	// recursive function call.
	for e.callFrameStack != previousTopFrame {
		currentFrame := e.callFrameStack
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s (at %d), stackBasePointer: %d, stackPointer: %d\n",
				currentFrame.String(), e.callFrameNum, e.stackBasePointer, e.stackPointer)
		}

		// Call into the jitted code.
		jitcall(
			currentFrame.continuationAddress,
			uintptr(unsafe.Pointer(e)),
			e.memorySliceAddress,
		)

		// Check the status code from JIT code.
		switch e.jitCallStatusCode {
		case jitCallStatusCodeReturned:
			// Meaning that the current frame exits
			// so restore the caller's frame.
			e.callFramePop()
		case jitCallStatusCodeCallFunction:
			// We consolidate host function calls with normal wasm function calls.
			// This reduced the cost of checking isHost in the assembly as well as
			// the cost of doing fully native function calls between wasm functions we will do later.
			nextFunc := e.compiledFunctions[e.functionCallAddress]
			// Calculate the continuation address so we can resume this caller function frame.
			currentFrame.continuationAddress = currentFrame.compiledFunction.codeInitialAddress + e.continuationAddressOffset
			currentFrame.continuationStackPointer = e.stackPointer + nextFunc.resultCount - nextFunc.paramCount

			callee := &callFrame{compiledFunction: nextFunc}
			if nextFunc.isHostFunction() {
				e.callFramePush(callee)
				e.execHostFunction(nextFunc.source.HostFunction, &wasm.HostFunctionCallContext{Memory: currentFrame.compiledFunction.source.ModuleInstance.Memory})
				e.callFramePop()
			} else {
				callee.continuationAddress = nextFunc.codeInitialAddress
				e.callFramePush(callee)
				// If the Go-allocated stack is running out, we grow it before calling into JITed code.
				e.maybeGrowStack(nextFunc.maxStackPointer)
			}
		case jitCallStatusCodeCallBuiltInFunction:
			switch e.functionCallAddress {
			case builtinFunctionAddressMemoryGrow:
				e.builtinFunctionMemoryGrow(currentFrame.compiledFunction.source.ModuleInstance.Memory)
			case builtinFunctionAddressMemorySize:
				e.builtinFunctionMemorySize(currentFrame.compiledFunction.source.ModuleInstance.Memory)
			}
			if buildoptions.IsDebugMode {
				if e.functionCallAddress == builtinFunctionAddressBreakPoint {
					runtime.Breakpoint()
				}
			}
			currentFrame.continuationAddress = currentFrame.compiledFunction.codeInitialAddress + e.continuationAddressOffset
		case jitCallStatusCodeInvalidFloatToIntConversion:
			// TODO: have wasm.ErrInvalidFloatToIntConversion and use it here.
			panic("invalid float to int conversion")
		case jitCallStatusCodeUnreachable:
			// TODO: have wasm.ErrUnreachable and use it here.
			panic("unreachable")
		case jitCallStatusCodeMemoryOutOfBounds:
			// TODO: have wasm.ErrMemoryOutOfBounds and use it here.
			panic("out of bounds memory access")
		case jitCallStatusCodeTableOutOfBounds:
			// TODO: have wasm.ErrTableOutOfBounds and use it here.
			panic("out of bounds table access")
		case jitCallStatusCodeTypeMismatchOnIndirectCall:
			// TODO: have wasm.ErrTypeMismatchOnIndirectCall and use it here.
			panic("type mismatch on indirect function call")
		}
	}
}

func (e *engine) builtinFunctionMemoryGrow(mem *wasm.MemoryInstance) {
	newPages := e.pop()
	max := uint64(math.MaxUint32)
	if mem.Max != nil {
		max = uint64(*mem.Max) * wasm.PageSize
	}
	// If exceeds the max of memory size, we push -1 according to the spec.
	if uint64(newPages*wasm.PageSize+uint64(len(mem.Buffer))) > max {
		v := int32(-1)
		e.push(uint64(v))
	} else {
		e.builtinFunctionMemorySize(mem) // Grow returns the prior memory size on change.
		mem.Buffer = append(mem.Buffer, make([]byte, newPages*wasm.PageSize)...)
		e.memorySliceLen = uint64(len(mem.Buffer))
	}
}

func (e *engine) builtinFunctionMemorySize(mem *wasm.MemoryInstance) {
	e.push(uint64(len(mem.Buffer)) / wasm.PageSize)
}

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledFunction, error) {
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

	var skip bool
	for _, op := range ir.Operations {
		// Compiler determines whether or not skip the entire label.
		// For example, if the label doesn't have any caller,
		// we don't need to generate native code at all as we never reach the region.
		if op.Kind() == wazeroir.OperationKindLabel {
			skip = compiler.compileLabel(op.(*wazeroir.OperationLabel))
		}
		if skip {
			continue
		}

		if buildoptions.IsDebugMode {
			fmt.Printf("compiling op=%s: %s\n", op.Kind(), compiler)
		}
		var err error
		switch o := op.(type) {
		case *wazeroir.OperationUnreachable:
			err = compiler.compileUnreachable()
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

	code, staticData, maxStackPointer, err := compiler.generate()
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}

	if buildoptions.IsDebugMode {
		fmt.Printf("compiled code in hex: %s\n", hex.EncodeToString(code))
	}

	cf := &compiledFunction{
		source:             f,
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		paramCount:         uint64(len(f.FunctionType.Type.Params)),
		resultCount:        uint64(len(f.FunctionType.Type.Results)),
		maxStackPointer:    maxStackPointer,
		staticData:         staticData,
	}
	return cf, nil
}
