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

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

type (
	engine struct {
		// These contexts are read and written by JITed code.
		// Note that we embed these structs so we can reduce the costs to access fields inside of them.
		// Also, that eases the calculation of offsets to each field.

		globalContext
		moduleContext
		valueStackContext
		exitContext
		archContext

		// The following fields are NOT acceessed by JITed code directly.

		// valueStack is the go-allocated stack for holding Wasm values.
		// Note: We NEVER edit len or cap in JITed code so we won't get screwed when GC comes in.
		valueStack []uint64

		// callFrameStack is initially callFrameStack[callFrameStackPointer].
		// The currently executed function call frame lives at callFrameStack[callFrameStackPointer-1]
		// and that is equivalent to  engine.callFrameTop().
		callFrameStack []callFrame

		// compiledFunctions are the currently compiled functions.
		// The index means wasm.FunctionAddress, but we intentionally avoid using map
		// as the underlying memory region is accessed by assembly directly by
		// using compiledFunctionsElement0Address.
		compiledFunctions []*compiledFunction
	}

	// globalContext holds the data which is constant across multiple function calls.
	globalContext struct {
		// valueStackElement0Address is &engine.valueStack[0] as uintptr.
		// Note: this is updated when growing the stack in builtinFunctionGrowValueStack.
		valueStackElement0Address uintptr
		// valueStackLen is len(engine.valueStack[0]).
		// Note: this is updated when growing the stack in builtinFunctionGrowValueStack.
		valueStackLen uint64

		// callFrameStackElementZeroAddress is &engine.callFrameStack[0] as uintptr.
		// Note: this is updated when growing the stack in builtinFunctionGrowCallFrameStack.
		callFrameStackElementZeroAddress uintptr
		// callFrameStackLen is len(engine.callFrameStack).
		// Note: this is updated when growing the stack in builtinFunctionGrowCallFrameStack.
		callFrameStackLen uint64
		// callFrameStackPointer points at the next empty slot on the call frame stack.
		// For example, for the next function call, we push the new callFrame onto
		// callFrameStack[callFrameStackPointer]. This value is incremented/decremented in assembly
		// when making function calls or returning from them.
		callFrameStackPointer uint64
		// previousCallFrameStackPointer is to support re-entrant execution.
		// This is updated whenever exntering engine.execWasmFunction.
		// If this is the initial call into Wasm, the value equals zero,
		// but if this is the recursive function call from the host function, the value becomes non-zero.
		previousCallFrameStackPointer uint64

		// compiledFunctionsElement0Address is &engine.compiledFunctions[0] as uintptr.
		// Note: this is updated when growing the slice in addCompileFunction.
		compiledFunctionsElement0Address uintptr
	}

	// moduleContext holds the per-function call specific module information.
	// This is subject to be manipulated from JITed native code whenever we make function calls.
	moduleContext struct {
		// moduleInstanceAddress is the address of module instance from which we initialize
		// the following fields. This is set whenever we enter a function or return from function calls.
		// This is only used by JIT code so mark this as nolint.
		moduleInstanceAddress uintptr //nolint

		// globalElement0Address is the address of the first element in the globa slice,
		// i.e. &ModuleInstance.Globals[0] as uintptr.
		globalElement0Address uintptr
		// memoryElement0Address is the address of the first element in the globa slice,
		// i.e. &ModuleInstance.Memory.Buffer[0] as uintptr.
		memoryElement0Address uintptr
		// memorySliceLen is the length of the memory buffer, i.e. len(ModuleInstance.Memory.Buffer).
		memorySliceLen uint64
		// tableElement0Address is the address of the first item in the globa slice,
		// i.e. &ModuleInstance.Tables[0].Table[0] as uintptr.
		tableElement0Address uintptr
		// tableSliceLen is the length of the memory buffer, i.e. len(ModuleInstance.Tables[0].Table).
		tableSliceLen uint64
	}

	// valueStackContext stores the data to access engine.valueStack.
	valueStackContext struct {
		// stackPointer on .valueStack field which is accessed by [stackBasePointer] + [stackPointer].
		//
		// Note: stackPointer is not used in assembly since the native code knows exact position of
		// each variable in the value stack from the info from compilation.
		// Therefore, only updated when native code exit from the JIT world and go back to the Go function.
		stackPointer uint64

		// stackBasePointer is updated whenever we make function calls.
		// Background: Functions might be compiled as if they use the stack from the bottom.
		// However in reality, they have to use it from the middle of the stack depending on
		// when these function calls are made. So instead of accessing stack via stackPointer alone,
		// functions are compiled so they access the stack via [stackBasePointer](fixed for entire function) + [stackPointer].
		// More precisely, stackBasePointer is set to [callee's stack pointer] + [callee's stack base pointer] - [caller's params].
		// This way, compiled functions can be independent of the timing of functions calls made against them.
		//
		// Note: This is saved on callFrameTop().returnStackBasePointer whenever making function call.
		// Also, this is changed whenever we make function call or return from functions where we execute jump instruction.
		// In either case, the caller of "jmp" instruction must set this field properly.
		stackBasePointer uint64
	}

	// exitContext will be manipulated whenever JITed native code returns into the Go function.
	exitContext struct {
		// Where we store the status code of JIT execution.
		statusCode jitCallStatusCode

		// Set when statusCode == jitStatusCall{HostFunction,BuiltInFunction}
		// Indicating the function call index.
		functionCallAddress wasm.FunctionAddress
	}

	// callFrame holds the information to which the caller function can return.
	// callFrame is created for currently executed function frame as well,
	// so some of the fields are not yet set when native code is currently executing it.
	// That is, callFrameTop().returnAddress or returnStackBasePointer are not set
	// until it makes a function call.
	callFrame struct {
		// Set when making function call from this function frame, or for the initial function frame to call from engine.execWasmFunction.
		returnAddress uintptr
		// Set when making function call from this function frame.
		returnStackBasePointer uint64
		// Set when making function call to this function frame.
		compiledFunction *compiledFunction
		// _ is a necessary padding to make the size of callFrame struct a power of 2.
		_ [8]byte
	}

	compiledFunction struct {
		// The following fields are accessed by JITed code.

		// Pre-calculated pointer pointing to the initial byte of .codeSegment slice.
		// That mean codeInitialAddress always equals uintptr(unsafe.Pointer(&.codeSegment[0]))
		// and we cache the value (uintptr(unsafe.Pointer(&.codeSegment[0]))) to this field
		// so we don't need to repeat the calculation on each function call.
		codeInitialAddress uintptr
		// The max of the stack pointer this function can reach. Lazily applied via maybeGrowValueStack.
		stackPointerCeil uint64

		// Followings are not accessed by JITed code.

		// The source function instance from which this is compiled.
		source *wasm.FunctionInstance
		// codeSegment is holding the compiled native code as a byte slice.
		codeSegment []byte
		// See the doc for compiledFunctionStaticData type.
		staticData compiledFunctionStaticData
	}

	// staticData holds the read-only data (i.e. out side of codeSegment which is marked as executable) per function.
	// This is used to store jump tables for br_table instructions.
	// The primary index is the logical sepration of multiple data, for example data[0] and data[1]
	// correspond to different jump tables for different br_table instructions.
	compiledFunctionStaticData = [][]byte
)

// Native code reads/writes Go's structs with the following constants.
// See TestVerifyOffsetValue for how to derive these values.
const (
	// Offsets for engine.globalContext.
	engineGlobalContextValueStackElement0AddressOffset        = 0
	engineGlobalContextValueStackLenOffset                    = 8
	engineGlobalContextCallFrameStackElement0AddressOffset    = 16
	engineGlobalContextCallFrameStackLenOffset                = 24
	engineGlobalContextCallFrameStackPointerOffset            = 32
	engineGlobalContextPreviouscallFrameStackPointer          = 40
	engineGlobalContextCompiledFunctionsElement0AddressOffset = 48

	// Offsets for engine.moduleContext.
	engineModuleContextModuleInstanceAddressOffset = 56
	engineModuleContextGlobalElement0AddressOffset = 64
	engineModuleContextMemoryElement0AddressOffset = 72
	engineModuleContextMemorySliceLenOffset        = 80
	engineModuleContextTableElement0AddressOffset  = 88
	engineModuleContextTableSliceLenOffset         = 96

	// Offsets for engine.valueStackContext.
	engineValueStackContextStackPointerOffset     = 104
	engineValueStackContextStackBasePointerOffset = 112

	// Offsets for engine.exitContext.
	engineExitContextJITCallStatusCodeOffset   = 120
	engineExitContextFunctionCallAddressOffset = 128

	// Offsets for callFrame.
	callFrameDataSize                      = 32
	callFrameDataSizeMostSignificantSetBit = 5
	callFrameReturnAddressOffset           = 0
	callFrameReturnStackBasePointerOffset  = 8
	callFrameCompiledFunctionOffset        = 16

	// Offsets for compiledFunction.
	compiledFunctionCodeInitialAddressOffset = 0
	compiledFunctionStackPointerCeilOffset   = 8

	// Offsets for wasm.TableElement
	tableElementFunctionAddressOffset = 0
	tableElementFunctionTypeIDOffset  = 8

	// Offsets for wasm.ModuleInstance
	moduleInstanceGlobalsOffset = 48
	moduleInstanceMemoryOffset  = 72
	moduleInstanceTablesOffset  = 80

	// Offsets for wasm.TableInstance.
	tableInstanceTableOffset    = 0
	tableInstanceTableLenOffset = 8

	// Offsets for wasm.MemoryInstance.
	memoryInstanceBufferOffset    = 0
	memoryInstanceBufferLenOffset = 8

	// Offsets for wasm.GlobalInstance.
	globalInstanceValueOffset = 8
)

// jitCallStatusCode represents the result of `jitcall`.
// This is set by the jitted native code.
type jitCallStatusCode byte

const (
	// jitStatusReturned means the jitcall reaches the end of function, and returns successfully.
	jitCallStatusCodeReturned jitCallStatusCode = iota
	// jitCallStatusCodeCallFunction means the jitcall returns to make a host function call.
	jitCallStatusCodeCallHostFunction
	// jitCallStatusCodeCallFunction means the jitcall returns to make a builtin function call.
	jitCallStatusCodeCallBuiltInFunction
	// jitCallStatusCodeUnreachable means the function invocation reaches "unreachable" instruction.
	jitCallStatusCodeUnreachable
	// jitCallStatusCodeInvalidFloatToIntConversion means a invalid conversion of integer to floats happened.
	jitCallStatusCodeInvalidFloatToIntConversion
	// jitCallStatusCodeMemoryOutOfBounds means an out of bounds memory access happened.
	jitCallStatusCodeMemoryOutOfBounds
	// jitCallStatusCodeInvalidTableAccess means either offset to the table was out of bounds of table, or
	// the target element in the table was uninitialized during call_indirect instruction.
	jitCallStatusCodeInvalidTableAccess
	// jitCallStatusCodeTypeMismatchOnIndirectCall means the type check failed during call_indirect.
	jitCallStatusCodeTypeMismatchOnIndirectCall
	jitCallStatusIntegerOverflow
	jitCallStatusIntegerDivisionByZero
)

// causePanic causes a panic with the corresponding error to the status code.
func (s jitCallStatusCode) causePanic() {
	var err error
	switch s {
	case jitCallStatusIntegerOverflow:
		err = wasm.ErrRuntimeIntegerOverflow
	case jitCallStatusIntegerDivisionByZero:
		err = wasm.ErrRuntimeIntegerDivideByZero
	case jitCallStatusCodeInvalidFloatToIntConversion:
		err = wasm.ErrRuntimeInvalidConversionToInteger
	case jitCallStatusCodeUnreachable:
		err = wasm.ErrRuntimeUnreachable
	case jitCallStatusCodeMemoryOutOfBounds:
		err = wasm.ErrRuntimeOutOfBoundsMemoryAccess
	case jitCallStatusCodeInvalidTableAccess:
		err = wasm.ErrRuntimeInvalidTableAccess
	case jitCallStatusCodeTypeMismatchOnIndirectCall:
		err = wasm.ErrRuntimeIndirectCallTypeMismatch
	}
	panic(err)
}

func (s jitCallStatusCode) String() (ret string) {
	switch s {
	case jitCallStatusCodeReturned:
		ret = "returned"
	case jitCallStatusCodeCallHostFunction:
		ret = "call_host_function"
	case jitCallStatusCodeCallBuiltInFunction:
		ret = "call_builtin_function"
	case jitCallStatusCodeUnreachable:
		ret = "unreachable"
	}
	return
}

func (e *engine) callFrameTop() *callFrame {
	return &e.callFrameStack[e.globalContext.callFrameStackPointer-1]
}

func (e *engine) callFrameAt(depth uint64) *callFrame {
	return &e.callFrameStack[e.globalContext.callFrameStackPointer-1-depth]
}

// resetState resets the engine state so this engine can be reused.
func (e *engine) resetState() {
	e.initializeGlobalContext()
	e.moduleContext = moduleContext{}
	e.valueStackContext = valueStackContext{}
}

func (e *engine) initializeGlobalContext() {
	valueStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&e.valueStack))
	callFrameStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&e.callFrameStack))
	compiledFunctionsHeader := (*reflect.SliceHeader)(unsafe.Pointer(&e.compiledFunctions))
	e.globalContext = globalContext{
		valueStackElement0Address:        valueStackHeader.Data,
		valueStackLen:                    uint64(valueStackHeader.Len),
		callFrameStackElementZeroAddress: callFrameStackHeader.Data,
		callFrameStackLen:                uint64(callFrameStackHeader.Len),
		callFrameStackPointer:            0,
		compiledFunctionsElement0Address: compiledFunctionsHeader.Data,
	}
}

func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[%s: return address=0x%x, return stack base pointer=%d]",
		c.compiledFunction.source.Name, c.returnAddress, c.returnStackBasePointer,
	)
}

func (e *engine) Call(ctx *wasm.HostFunctionCallContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	// We ensure that this Call method never panics as
	// this Call method is indirectly invoked by embedders via store.CallFunction,
	// and we have to make sure that all the runtime errors, including the one happening inside
	// host functions, will be captured as errors, not panics.

	// shouldRecover is true when a panic at the origin of callstack should be recovered
	//
	// If this is the recursive call into Wasm (callFrameStackPointer != 0), we do not recover, and delegate the
	// recovery to the first engine.Call.
	//
	// For example, given the call stack:
	//	 "original host function" --(engine.Call)--> Wasm func A --> Host func --(engine.Call)--> Wasm function B,
	// if the top Wasm function panics, we go back to the "original host function".
	shouldRecover := e.globalContext.callFrameStackPointer == 0
	defer func() {
		if shouldRecover {
			if v := recover(); v != nil {
				if buildoptions.IsDebugMode {
					debug.PrintStack()
				}

				var frames []string
				for i := uint64(0); i < e.globalContext.callFrameStackPointer; i++ {
					f := e.callFrameStack[e.globalContext.callFrameStackPointer-1-i].compiledFunction
					frames = append(frames, fmt.Sprintf("\t%d: %s", i, f.source.Name))
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

				e.resetState()
			}
		}
	}()

	for _, param := range params {
		e.pushValue(param)
	}

	compiled := e.compiledFunctions[f.Address]
	if compiled == nil {
		err = fmt.Errorf("function not compiled")
		return
	}

	if f.FunctionKind == wasm.FunctionKindWasm {
		e.execWasmFunction(ctx, compiled)
	} else {
		e.execHostFunction(f.FunctionKind, compiled.source.HostFunction, ctx)
	}

	// Note the top value is the tail of the results,
	// so we assign them in reverse order.
	results = make([]uint64, len(f.FunctionType.Type.Results))
	for i := range results {
		results[len(results)-1-i] = e.popValue()
	}
	return
}

func NewEngine() wasm.Engine {
	return newEngine()
}

// TODO: better make them configurable?
const (
	initialValueStackSize             = 1024
	initialCallStackSize              = 256
	initialCompiledFunctionsSliceSize = 128
)

func newEngine() *engine {
	e := &engine{
		valueStack:        make([]uint64, initialValueStackSize),
		callFrameStack:    make([]callFrame, initialCallStackSize),
		compiledFunctions: make([]*compiledFunction, initialCompiledFunctionsSliceSize),
		archContext:       newArchContext(),
	}
	e.initializeGlobalContext()
	return e
}

func (e *engine) popValue() (ret uint64) {
	e.valueStackContext.stackPointer--
	ret = e.valueStack[e.valueStackTopIndex()]
	return
}

func (e *engine) pushValue(v uint64) {
	e.valueStack[e.valueStackTopIndex()] = v
	e.valueStackContext.stackPointer++
}

func (e *engine) valueStackTopIndex() uint64 {
	return e.valueStackContext.stackBasePointer + e.valueStackContext.stackPointer
}

const (
	builtinFunctionAddressMemoryGrow wasm.FunctionAddress = iota
	builtinFunctionAddressGrowValueStack
	builtinFunctionAddressGrowCallFrameStack
	// builtinFunctionAddressBreakPoint is internal (only for wazero developers). Disabled by default.
	builtinFunctionAddressBreakPoint
)

// execHostFunction executes the given host function represented as *reflect.Value.
//
// The arguments to the function are popped from the stack following the convention of the Wasm stack machine.
// For example, if the host function F requires the (x1 uint32, x2 float32) parameters, and
// the stack is [..., A, B], then the function is called as F(A, B) where A and B are interpreted
// as uint32 and float32 respectively.
//
// After the execution, the result of host function is pushed onto the stack.
//
// ctx parameter is passed to the host function as a first argument.
func (e *engine) execHostFunction(fk wasm.FunctionKind, f *reflect.Value, ctx *wasm.HostFunctionCallContext) {
	// TODO: the signature won't ever change for a host function once instantiated. For this reason, we should be able
	// to optimize below based on known possible outcomes. This includes knowledge about if it has a context param[0]
	// and which type (if any) it returns.
	tp := f.Type()
	in := make([]reflect.Value, tp.NumIn())

	// We pop the value and pass them as arguments in a reverse order according to the
	// stack machine convention.
	wasmParamOffset := 0
	if fk != wasm.FunctionKindHostNoContext {
		wasmParamOffset = 1
	}
	for i := len(in) - 1; i >= wasmParamOffset; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := e.popValue()
		kind := tp.In(i).Kind()
		switch kind {
		case reflect.Float32:
			val.SetFloat(float64(math.Float32frombits(uint32(raw))))
		case reflect.Float64:
			val.SetFloat(math.Float64frombits(raw))
		case reflect.Uint32, reflect.Uint64:
			val.SetUint(raw)
		case reflect.Int32, reflect.Int64:
			val.SetInt(int64(raw))
		}
		in[i] = val
	}

	// Handle any special parameter zero
	if val := wasm.GetHostFunctionCallContextValue(fk, ctx); val != nil {
		in[0] = *val
	}

	// Execute the host function and push back the call result onto the stack.
	for _, ret := range f.Call(in) {
		switch ret.Kind() {
		case reflect.Float64, reflect.Float32:
			e.pushValue(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			e.pushValue(ret.Uint())
		case reflect.Int32, reflect.Int64:
			e.pushValue(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
}

func (e *engine) execWasmFunction(ctx *wasm.HostFunctionCallContext, f *compiledFunction) {
	// We continuously execute functions until we reach the previous top frame
	// to support recursive Wasm function executions.
	e.globalContext.previousCallFrameStackPointer = e.globalContext.callFrameStackPointer

	e.pushCallFrame(f)

jitentry:
	{
		frame := e.callFrameTop()
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s, stackBasePointer: %d, stackPointer: %d\n",
				frame.String(), e.valueStackContext.stackBasePointer, e.valueStackContext.stackPointer)
		}

		// Call into the JIT code.
		jitcall(frame.returnAddress, uintptr(unsafe.Pointer(e)))

		// Check the status code from JIT code.
		switch status := e.exitContext.statusCode; status {
		case jitCallStatusCodeReturned:
			// Meaning that all the function frames above the previous call frame stack pointer are executed.
			if e.globalContext.previousCallFrameStackPointer != e.globalContext.callFrameStackPointer {
				panic("bug in JIT compiler")
			}
		case jitCallStatusCodeCallHostFunction:
			// Not "callFrameTop" but take the below of peek with "callFrameAt(1)" as the top frame is for host function,
			// but when making host function calls, we need to pass the memory instance of host function caller.
			fn := e.compiledFunctions[e.exitContext.functionCallAddress]
			callerCompiledFunction := e.callFrameAt(1).compiledFunction
			if buildoptions.IsDebugMode {
				if fn.source.FunctionKind == wasm.FunctionKindWasm {
					panic("jitCallStatusCodeCallHostFunction is only for host functions")
				}
			}
			saved := e.globalContext.previousCallFrameStackPointer
			e.execHostFunction(fn.source.FunctionKind, fn.source.HostFunction,
				ctx.WithMemory(callerCompiledFunction.source.ModuleInstance.Memory),
			)
			e.globalContext.previousCallFrameStackPointer = saved
			goto jitentry
		case jitCallStatusCodeCallBuiltInFunction:
			switch e.exitContext.functionCallAddress {
			case builtinFunctionAddressMemoryGrow:
				callerCompiledFunction := e.callFrameTop().compiledFunction
				e.builtinFunctionMemoryGrow(callerCompiledFunction.source.ModuleInstance.Memory)
			case builtinFunctionAddressGrowValueStack:
				callerCompiledFunction := e.callFrameTop().compiledFunction
				e.builtinFunctionGrowValueStack(callerCompiledFunction.stackPointerCeil)
			case builtinFunctionAddressGrowCallFrameStack:
				e.builtinFunctionGrowCallFrameStack()
			}
			if buildoptions.IsDebugMode {
				if e.exitContext.functionCallAddress == builtinFunctionAddressBreakPoint {
					runtime.Breakpoint()
				}
			}
			goto jitentry
		default:
			status.causePanic()
		}
	}
}

// pushInitialFrame is implemented in assembly as well, but this Go version is used BEFORE jit entry.
func (e *engine) pushCallFrame(f *compiledFunction) {
	// Push the new frame to the top of stack.
	e.callFrameStack[e.globalContext.callFrameStackPointer] = callFrame{returnAddress: f.codeInitialAddress, compiledFunction: f}
	e.globalContext.callFrameStackPointer++

	// For example, if we have the following state (where "_" means no value pushed),
	//       base            sp
	//        |              |
	// [...., A, B, C, D, E, _, _ ]
	//
	// and the target function requires 2 params, we need to pass D and E as arguments.
	//
	// Therefore, the target function start executing under the following state:
	//                base   sp
	//                 |     |
	// [...., A, B, C, D, E, _, _ ]
	//
	// That maens the next stack base poitner is calculated as follows (note stack pointer is relative to base):
	e.valueStackContext.stackBasePointer =
		e.valueStackContext.stackBasePointer + e.valueStackContext.stackPointer - uint64(len(f.source.FunctionType.Type.Params))
}

func (e *engine) builtinFunctionGrowValueStack(stackPointerCeil uint64) {
	// Extends the valueStack's length to currentLen*2+stackPointerCeil.
	newLen := e.globalContext.valueStackLen*2 + (stackPointerCeil)
	newStack := make([]uint64, newLen)
	top := e.valueStackContext.stackBasePointer + e.valueStackContext.stackPointer
	copy(newStack[:top], e.valueStack[:top])
	e.valueStack = newStack

	// Update the globalContext's fields as they become stale after the update ^^.
	stackSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&newStack))
	e.globalContext.valueStackElement0Address = stackSliceHeader.Data
	e.globalContext.valueStackLen = uint64(stackSliceHeader.Len)
}

var callStackCeiling = uint64(buildoptions.CallStackCeiling)

func (e *engine) builtinFunctionGrowCallFrameStack() {
	if callStackCeiling < uint64(len(e.callFrameStack)+1) {
		panic(wasm.ErrRuntimeCallStackOverflow)
	}

	// Double the callstack slice length.
	newLen := uint64(e.globalContext.callFrameStackLen) * 2
	newStack := make([]callFrame, newLen)
	copy(newStack, e.callFrameStack)
	e.callFrameStack = newStack

	// Update the globalContext's fields as they become stale after the update ^^.
	stackSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&newStack))
	e.globalContext.callFrameStackLen = uint64(stackSliceHeader.Len)
	e.globalContext.callFrameStackElementZeroAddress = stackSliceHeader.Data
}

func (e *engine) builtinFunctionMemoryGrow(mem *wasm.MemoryInstance) {
	newPages := e.popValue()

	res := mem.Grow(uint32(newPages))
	e.pushValue(uint64(res))

	// Update the moduleContext's fields as they become stale after the update ^^.
	bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mem.Buffer))
	e.moduleContext.memorySliceLen = uint64(bufSliceHeader.Len)
	e.moduleContext.memoryElement0Address = bufSliceHeader.Data
}

func (e *engine) Compile(f *wasm.FunctionInstance) (err error) {
	var compiled *compiledFunction
	if f.FunctionKind == wasm.FunctionKindWasm {
		compiled, err = compileWasmFunction(f)
	} else {
		compiled, err = compileHostFunction(f)
	}
	if err != nil {
		return fmt.Errorf("failed to compile function: %w", err)
	}

	e.addCompiledFunction(f.Address, compiled)
	return
}

func (e *engine) addCompiledFunction(addr wasm.FunctionAddress, compiled *compiledFunction) {
	if len(e.compiledFunctions) <= int(addr) {
		// Double the size of compiled functions.
		e.compiledFunctions = append(e.compiledFunctions, make([]*compiledFunction, len(e.compiledFunctions))...)
		compiledFunctionsHeader := (*reflect.SliceHeader)(unsafe.Pointer(&e.compiledFunctions))
		e.globalContext.compiledFunctionsElement0Address = compiledFunctionsHeader.Data
	}
	e.compiledFunctions[addr] = compiled
}

func compileHostFunction(f *wasm.FunctionInstance) (*compiledFunction, error) {
	compiler, err := newCompiler(f, nil)
	if err != nil {
		return nil, err
	}

	if err = compiler.compileHostFunction(f.Address); err != nil {
		return nil, err
	}

	code, _, _, err := compiler.compile()
	if err != nil {
		return nil, err
	}

	stackPointerCeil := uint64(len(f.FunctionType.Type.Params))
	if res := uint64(len(f.FunctionType.Type.Results)); stackPointerCeil < res {
		stackPointerCeil = res
	}

	return &compiledFunction{
		source:             f,
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		stackPointerCeil:   stackPointerCeil,
	}, nil
}

func compileWasmFunction(f *wasm.FunctionInstance) (*compiledFunction, error) {
	ir, err := wazeroir.Compile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to lower to wazeroir: %w", err)
	}

	if buildoptions.IsDebugMode {
		fmt.Printf("compilation target wazeroir:\n%s\n", wazeroir.Format(ir.Operations))
	}

	compiler, err := newCompiler(f, ir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize assembly builder: %w", err)
	}

	if err := compiler.compilePreamble(); err != nil {
		return nil, fmt.Errorf("failed to emit preamble: %w", err)
	}

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

	code, staticData, stackPointerCeil, err := compiler.compile()
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}

	if buildoptions.IsDebugMode {
		fmt.Printf("compiled code in hex: %s\n", hex.EncodeToString(code))
	}

	return &compiledFunction{
		source:             f,
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		stackPointerCeil:   stackPointerCeil,
		staticData:         staticData,
	}, nil
}
