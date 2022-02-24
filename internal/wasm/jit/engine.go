package jit

import (
	"fmt"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"unsafe"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

type (
	// engine is an JIT implementation of internalwasm.Engine
	engine struct {
		// compiledFunctions are the currently compiled functions.
		// The index means wasm.FunctionAddress, but we intentionally avoid using map
		// as the underlying memory region is accessed by assembly directly by
		// using compiledFunctionsElement0Address.
		compiledFunctions []*compiledFunction

		// mux is used for read/write access to compiledFunctions slice.
		// This is necessary as each compiled function will access the slice from the native code
		// when they make function calls while engine might be modifying the underlying slice when
		// adding a new compiled function. We take read lock when creating new virtualMachine
		// for each function invocation while take write lock in engine.addCompiledFunction.
		mux sync.RWMutex
	}

	// virtualMachine holds context per engine.Call, and shared across all the
	// function calls originating from the same engine.Call execution.
	virtualMachine struct {
		// These contexts are read and written by JITed code.
		// Note: we embed these structs so we can reduce the costs to access fields inside of them.
		// Also, that eases the calculation of offsets to each field.
		globalContext
		moduleContext
		valueStackContext
		exitContext
		archContext

		// The following fields are not accessed by JITed code directly.

		// valueStack is the go-allocated stack for holding Wasm values.
		// Note: We NEVER edit len or cap in JITed code so we won't get screwed when GC comes in.
		valueStack []uint64

		// callFrameStack is initially callFrameStack[callFrameStackPointer].
		// The currently executed function call frame lives at callFrameStack[callFrameStackPointer-1]
		// and that is equivalent to  engine.callFrameTop().
		callFrameStack []callFrame

		// TODO: comment
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

		// compiledFunctionsElement0Address is &engine.compiledFunctions[0] as uintptr.
		compiledFunctionsElement0Address uintptr
	}

	// moduleContext holds the per-function call specific module information.
	// This is subject to be manipulated from JITed native code whenever we make function calls.
	moduleContext struct {
		// moduleInstanceAddress is the address of module instance from which we initialize
		// the following fields. This is set whenever we enter a function or return from function calls.
		// This is only used by JIT code so mark this as nolint.
		moduleInstanceAddress uintptr //nolint

		// globalElement0Address is the address of the first element in the global slice,
		// i.e. &ModuleInstance.Globals[0] as uintptr.
		globalElement0Address uintptr
		// memoryElement0Address is the address of the first element in the global slice,
		// i.e. &ModuleInstance.MemoryInstance.Buffer[0] as uintptr.
		memoryElement0Address uintptr
		// memorySliceLen is the length of the memory buffer, i.e. len(ModuleInstance.MemoryInstance.Buffer).
		memorySliceLen uint64
		// tableElement0Address is the address of the first item in the global slice,
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
	// Offsets for virtualMachine.globalContext.
	virtualMachineGlobalContextValueStackElement0AddressOffset        = 0
	virtualMachineGlobalContextValueStackLenOffset                    = 8
	virtualMachineGlobalContextCallFrameStackElement0AddressOffset    = 16
	virtualMachineGlobalContextCallFrameStackLenOffset                = 24
	virtualMachineGlobalContextCallFrameStackPointerOffset            = 32
	virtualMachineGlobalContextCompiledFunctionsElement0AddressOffset = 40

	// Offsets for virtualMachine.moduleContext.
	virtualMachineModuleContextModuleInstanceAddressOffset = 48
	virtualMachineModuleContextGlobalElement0AddressOffset = 56
	virtualMachineModuleContextMemoryElement0AddressOffset = 64
	virtualMachineModuleContextMemorySliceLenOffset        = 72
	virtualMachineModuleContextTableElement0AddressOffset  = 80
	virtualMachineModuleContextTableSliceLenOffset         = 88

	// Offsets for virtualMachine.valueStackContext.
	virtualMachineValueStackContextStackPointerOffset     = 96
	virtualMachineValueStackContextStackBasePointerOffset = 104

	// Offsets for virtualMachine.exitContext.
	virtualMachineExitContextJITCallStatusCodeOffset   = 112
	virtualMachineExitContextFunctionCallAddressOffset = 120

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

func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[%s: return address=0x%x, return stack base pointer=%d]",
		c.compiledFunction.source.Name, c.returnAddress, c.returnStackBasePointer,
	)
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
		e.mux.Lock() // Write lock.
		defer e.mux.Unlock()
		// Double the size of compiled functions.
		e.compiledFunctions = append(e.compiledFunctions, make([]*compiledFunction, len(e.compiledFunctions))...)
	}
	e.compiledFunctions[addr] = compiled
}

func (e *engine) Call(ctx *wasm.ModuleContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	paramSignature := f.FunctionType.Type.Params
	paramCount := len(params)
	if len(paramSignature) != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", len(paramSignature), paramCount)
	}

	vm := e.newVirtualMachine()

	// We ensure that this Call method never panics as
	// this Call method is indirectly invoked by embedders via store.CallFunction,
	// and we have to make sure that all the runtime errors, including the one happening inside
	// host functions, will be captured as errors, not panics.
	defer func() {
		if v := recover(); v != nil {
			if buildoptions.IsDebugMode {
				debug.PrintStack()
			}

			var frames []string
			for i := uint64(0); i < vm.globalContext.callFrameStackPointer; i++ {
				f := vm.callFrameStack[vm.globalContext.callFrameStackPointer-1-i].compiledFunction
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
		}
	}()

	for _, param := range params {
		vm.pushValue(param)
	}

	compiled := vm.compiledFunctions[f.Address]
	if compiled == nil {
		err = fmt.Errorf("function not compiled")
		return
	}

	if f.FunctionKind == wasm.FunctionKindWasm {
		vm.execWasmFunction(ctx, compiled)
	} else {
		vm.execHostFunction(f.FunctionKind, compiled.source.HostFunction, ctx)
	}

	// Note the top value is the tail of the results,
	// so we assign them in reverse order.
	results = make([]uint64, len(f.FunctionType.Type.Results))
	for i := range results {
		results[len(results)-1-i] = vm.popValue()
	}
	return
}

func NewEngine() wasm.Engine {
	return newEngine()
}

func newEngine() *engine {
	return &engine{compiledFunctions: make([]*compiledFunction, initialCompiledFunctionsSliceSize)}
}

// TODO: better make them configurable?
const (
	initialValueStackSize             = 1024
	initialCallFrameStackSize         = 256
	initialCompiledFunctionsSliceSize = 128
)

func (e *engine) newVirtualMachine() *virtualMachine {
	e.mux.RLock()
	defer e.mux.RUnlock()

	vm := &virtualMachine{
		valueStack:        make([]uint64, initialValueStackSize),
		callFrameStack:    make([]callFrame, initialCallFrameStackSize),
		archContext:       newArchContext(),
		compiledFunctions: e.compiledFunctions,
	}

	valueStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&vm.valueStack))
	callFrameStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&vm.callFrameStack))
	compiledFunctionsHeader := (*reflect.SliceHeader)(unsafe.Pointer(&vm.compiledFunctions))
	vm.globalContext = globalContext{
		valueStackElement0Address:        valueStackHeader.Data,
		valueStackLen:                    uint64(valueStackHeader.Len),
		callFrameStackElementZeroAddress: callFrameStackHeader.Data,
		callFrameStackLen:                uint64(callFrameStackHeader.Len),
		callFrameStackPointer:            0,
		compiledFunctionsElement0Address: compiledFunctionsHeader.Data,
	}
	return vm
}

func (vm *virtualMachine) popValue() (ret uint64) {
	vm.valueStackContext.stackPointer--
	ret = vm.valueStack[vm.valueStackTopIndex()]
	return
}

func (vm *virtualMachine) pushValue(v uint64) {
	vm.valueStack[vm.valueStackTopIndex()] = v
	vm.valueStackContext.stackPointer++
}

func (vm *virtualMachine) callFrameTop() *callFrame {
	return &vm.callFrameStack[vm.globalContext.callFrameStackPointer-1]
}

func (vm *virtualMachine) callFrameAt(depth uint64) *callFrame {
	return &vm.callFrameStack[vm.globalContext.callFrameStackPointer-1-depth]
}

func (vm *virtualMachine) valueStackTopIndex() uint64 {
	return vm.valueStackContext.stackBasePointer + vm.valueStackContext.stackPointer
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
func (vm *virtualMachine) execHostFunction(fk wasm.FunctionKind, f *reflect.Value, ctx *wasm.ModuleContext) {
	// TODO: the signature won't ever change for a host function once instantiated. For this reason, we should be able
	// to optimize below based on known possible outcomes. This includes knowledge about if it has a context param[0]
	// and which type (if any) it returns.
	tp := f.Type()
	in := make([]reflect.Value, tp.NumIn())

	// We pop the value and pass them as arguments in a reverse order according to the
	// stack machine convention.
	wasmParamOffset := 0
	if fk != wasm.FunctionKindGoNoContext {
		wasmParamOffset = 1
	}
	for i := len(in) - 1; i >= wasmParamOffset; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := vm.popValue()
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
		case reflect.Float32:
			vm.pushValue(uint64(math.Float32bits(float32(ret.Float()))))
		case reflect.Float64:
			vm.pushValue(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			vm.pushValue(ret.Uint())
		case reflect.Int32, reflect.Int64:
			vm.pushValue(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
}

func (vm *virtualMachine) execWasmFunction(ctx *wasm.ModuleContext, f *compiledFunction) {
	vm.pushCallFrame(f)

jitentry:
	{
		frame := vm.callFrameTop()
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s, stackBasePointer: %d, stackPointer: %d\n",
				frame.String(), vm.valueStackContext.stackBasePointer, vm.valueStackContext.stackPointer)
		}

		// Call into the JIT code.
		jitcall(frame.returnAddress, uintptr(unsafe.Pointer(vm)))

		// Check the status code from JIT code.
		switch status := vm.exitContext.statusCode; status {
		case jitCallStatusCodeReturned:
			// Meaning that all the function frames above the previous call frame stack pointer are executed.
		case jitCallStatusCodeCallHostFunction:
			// Not "callFrameTop" but take the below of peek with "callFrameAt(1)" as the top frame is for host function,
			// but when making host function calls, we need to pass the memory instance of host function caller.
			fn := vm.compiledFunctions[vm.exitContext.functionCallAddress]
			callerCompiledFunction := vm.callFrameAt(1).compiledFunction
			// A host function is invoked with the calling frame's memory, which may be different if in another module.
			vm.execHostFunction(fn.source.FunctionKind, fn.source.HostFunction,
				ctx.WithMemory(callerCompiledFunction.source.ModuleInstance.MemoryInstance),
			)
			goto jitentry
		case jitCallStatusCodeCallBuiltInFunction:
			switch vm.exitContext.functionCallAddress {
			case builtinFunctionAddressMemoryGrow:
				callerCompiledFunction := vm.callFrameTop().compiledFunction
				vm.builtinFunctionMemoryGrow(callerCompiledFunction.source.ModuleInstance.MemoryInstance)
			case builtinFunctionAddressGrowValueStack:
				callerCompiledFunction := vm.callFrameTop().compiledFunction
				vm.builtinFunctionGrowValueStack(callerCompiledFunction.stackPointerCeil)
			case builtinFunctionAddressGrowCallFrameStack:
				vm.builtinFunctionGrowCallFrameStack()
			}
			if buildoptions.IsDebugMode {
				if vm.exitContext.functionCallAddress == builtinFunctionAddressBreakPoint {
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
func (vm *virtualMachine) pushCallFrame(f *compiledFunction) {
	// Push the new frame to the top of stack.
	vm.callFrameStack[vm.globalContext.callFrameStackPointer] = callFrame{returnAddress: f.codeInitialAddress, compiledFunction: f}
	vm.globalContext.callFrameStackPointer++

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
	vm.valueStackContext.stackBasePointer =
		vm.valueStackContext.stackBasePointer + vm.valueStackContext.stackPointer - uint64(len(f.source.FunctionType.Type.Params))
}

func (vm *virtualMachine) builtinFunctionGrowValueStack(stackPointerCeil uint64) {
	// Extends the valueStack's length to currentLen*2+stackPointerCeil.
	newLen := vm.globalContext.valueStackLen*2 + (stackPointerCeil)
	newStack := make([]uint64, newLen)
	top := vm.valueStackContext.stackBasePointer + vm.valueStackContext.stackPointer
	copy(newStack[:top], vm.valueStack[:top])
	vm.valueStack = newStack

	// Update the globalContext's fields as they become stale after the update ^^.
	stackSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&newStack))
	vm.globalContext.valueStackElement0Address = stackSliceHeader.Data
	vm.globalContext.valueStackLen = uint64(stackSliceHeader.Len)
}

var callStackCeiling = uint64(buildoptions.CallStackCeiling)

func (vm *virtualMachine) builtinFunctionGrowCallFrameStack() {
	if callStackCeiling < uint64(len(vm.callFrameStack)+1) {
		panic(wasm.ErrRuntimeCallStackOverflow)
	}

	// Double the callstack slice length.
	newLen := uint64(vm.globalContext.callFrameStackLen) * 2
	newStack := make([]callFrame, newLen)
	copy(newStack, vm.callFrameStack)
	vm.callFrameStack = newStack

	// Update the globalContext's fields as they become stale after the update ^^.
	stackSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&newStack))
	vm.globalContext.callFrameStackLen = uint64(stackSliceHeader.Len)
	vm.globalContext.callFrameStackElementZeroAddress = stackSliceHeader.Data
}

func (e *virtualMachine) builtinFunctionMemoryGrow(mem *wasm.MemoryInstance) {
	newPages := e.popValue()

	res := mem.Grow(uint32(newPages))
	e.pushValue(uint64(res))

	// Update the moduleContext's fields as they become stale after the update ^^.
	bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mem.Buffer))
	e.moduleContext.memorySliceLen = uint64(bufSliceHeader.Len)
	e.moduleContext.memoryElement0Address = bufSliceHeader.Data
}

func compileHostFunction(f *wasm.FunctionInstance) (*compiledFunction, error) {
	compiler, done, err := newCompiler(f, nil)
	defer done()

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

	compiler, done, err := newCompiler(f, ir)
	defer done()
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

	return &compiledFunction{
		source:             f,
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		stackPointerCeil:   stackPointerCeil,
		staticData:         staticData,
	}, nil
}
