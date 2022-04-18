package jit

import (
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/buildoptions"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

type (
	// engine is an JIT implementation of wasm.Engine
	engine struct {
		enabledFeatures wasm.Features
		codes           map[*wasm.Module][]*code // guarded by mutex.
		mux             sync.RWMutex
		// setFinalizer defaults to runtime.SetFinalizer, but overridable for tests.
		setFinalizer func(obj interface{}, finalizer interface{})
	}

	// moduleEngine implements wasm.ModuleEngine
	moduleEngine struct {
		// name is the name the module was instantiated with used for error handling.
		name string

		// functions are the functions in a module instances.
		// The index is module instance-scoped. We intentionally avoid using map
		// as the underlying memory region is accessed by assembly directly by using
		// codesElement0Address.
		functions []*function

		importedFunctionCount uint32
	}

	// callEngine holds context per moduleEngine.Call, and shared across all the
	// function calls originating from the same moduleEngine.Call execution.
	callEngine struct {
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
		// Note: We never edit len or cap in JITed code so we won't get screwed when GC comes in.
		valueStack []uint64

		// callFrameStack is initially callFrameStack[callFrameStackPointer].
		// The currently executed function call frame lives at callFrameStack[callFrameStackPointer-1]
		// and that is equivalent to  engine.callFrameTop().
		callFrameStack []callFrame
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
		// i.e. &ModuleInstance.Memory.Buffer[0] as uintptr.
		memoryElement0Address uintptr
		// memorySliceLen is the length of the memory buffer, i.e. len(ModuleInstance.Memory.Buffer).
		memorySliceLen uint64
		// tableElement0Address is the address of the first item in the global slice,
		// i.e. &ModuleInstance.Tables[0].Table[0] as uintptr.
		tableElement0Address uintptr
		// tableSliceLen is the length of the memory buffer, i.e. len(ModuleInstance.Tables[0].Table).
		tableSliceLen uint64

		// codesElement0Address is &moduleContext.engine.codes[0] as uintptr.
		codesElement0Address uintptr

		// typeIDsElement0Address holds the &ModuleInstance.typeIDs[0] as uintptr.
		typeIDsElement0Address uintptr
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
		// However, in reality, they have to use it from the middle of the stack depending on
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

		// Set when statusCode == jitStatusCallBuiltInFunction}
		// Indicating the function call index.
		builtinFunctionCallIndex wasm.Index
	}

	// callFrame holds the information to which the caller function can return.
	// callFrame is created for currently executed function frame as well,
	// so some of the fields are not yet set when native code is currently executing it.
	// That is, callFrameTop().returnAddress or returnStackBasePointer are not set
	// until it makes a function call.
	callFrame struct {
		// Set when making function call from this function frame, or for the initial function frame to call from
		// callEngine.execWasmFunction.
		returnAddress uintptr
		// Set when making function call from this function frame.
		returnStackBasePointer uint64
		// Set when making function call to this function frame.
		function *function
		// _ is a necessary padding to make the size of callFrame struct a power of 2.
		_ [8]byte
	}

	// Function corresponds to function instance in Wasm, and is created from `code`.
	function struct {
		// codeInitialAddress is the pre-calculated pointer pointing to the initial byte of .codeSegment slice.
		// That mean codeInitialAddress always equals uintptr(unsafe.Pointer(&.codeSegment[0]))
		// and we cache the value (uintptr(unsafe.Pointer(&.codeSegment[0]))) to this field,
		// so we don't need to repeat the calculation on each function call.
		codeInitialAddress uintptr
		// stackPointerCeil is the max of the stack pointer this function can reach. Lazily applied via maybeGrowValueStack.
		stackPointerCeil uint64
		// source is the source function instance from which this is compiled.
		source *wasm.FunctionInstance
		// moduleInstanceAddress holds the address of source.ModuleInstance.
		moduleInstanceAddress uintptr
		// parent holds code from which this is crated.
		parent *code
	}

	// code corresponds to a function in a module (not insantaited one). This holds the machine code
	// compiled by Wazero's JIT compiler.
	code struct {
		// codeSegment is holding the compiled native code as a byte slice.
		codeSegment []byte
		// See the doc for codeStaticData type.
		staticData codeStaticData
		// stackPointerCeil is the max of the stack pointer this function can reach. Lazily applied via maybeGrowValueStack.
		stackPointerCeil uint64

		// indexInModule is the index of this function in the module. For logging purpose.
		indexInModule wasm.Index
		// sourceModule is the module from which this function is compiled. For logging purpose.
		sourceModule *wasm.Module
	}

	// staticData holds the read-only data (i.e. out side of codeSegment which is marked as executable) per function.
	// This is used to store jump tables for br_table instructions.
	// The primary index is the logical separation of multiple data, for example data[0] and data[1]
	// correspond to different jump tables for different br_table instructions.
	codeStaticData = [][]byte
)

// createFunction creates a new function which uses the native code compiled.
func (c *code) createFunction(f *wasm.FunctionInstance) *function {
	return &function{
		codeInitialAddress:    uintptr(unsafe.Pointer(&c.codeSegment[0])),
		stackPointerCeil:      c.stackPointerCeil,
		moduleInstanceAddress: uintptr(unsafe.Pointer(f.Module)),
		source:                f,
		parent:                c,
	}
}

// Native code reads/writes Go's structs with the following constants.
// See TestVerifyOffsetValue for how to derive these values.
const (
	// Offsets for moduleEngine.functions
	moduleEngineFunctionsOffset = 16

	// Offsets for callEngine globalContext.
	callEngineGlobalContextValueStackElement0AddressOffset     = 0
	callEngineGlobalContextValueStackLenOffset                 = 8
	callEngineGlobalContextCallFrameStackElement0AddressOffset = 16
	callEngineGlobalContextCallFrameStackLenOffset             = 24
	callEngineGlobalContextCallFrameStackPointerOffset         = 32

	// Offsets for callEngine moduleContext.
	callEngineModuleContextModuleInstanceAddressOffset  = 40
	callEngineModuleContextGlobalElement0AddressOffset  = 48
	callEngineModuleContextMemoryElement0AddressOffset  = 56
	callEngineModuleContextMemorySliceLenOffset         = 64
	callEngineModuleContextTableElement0AddressOffset   = 72
	callEngineModuleContextTableSliceLenOffset          = 80
	callEngineModuleContextcodesElement0AddressOffset   = 88
	callEngineModuleContextTypeIDsElement0AddressOffset = 96

	// Offsets for callEngine valueStackContext.
	callEngineValueStackContextStackPointerOffset     = 104
	callEngineValueStackContextStackBasePointerOffset = 112

	// Offsets for callEngine exitContext.
	callEngineExitContextJITCallStatusCodeOffset          = 120
	callEngineExitContextBuiltinFunctionCallAddressOffset = 124

	// Offsets for callFrame.
	callFrameDataSize                      = 32
	callFrameDataSizeMostSignificantSetBit = 5
	callFrameReturnAddressOffset           = 0
	callFrameReturnStackBasePointerOffset  = 8
	callFrameFunctionOffset                = 16

	// Offsets for function.
	functionCodeInitialAddressOffset    = 0
	functionStackPointerCeilOffset      = 8
	functionSourceOffset                = 16
	functionModuleInstanceAddressOffset = 24

	// Offsets for wasm.ModuleInstance.
	moduleInstanceGlobalsOffset = 48
	moduleInstanceMemoryOffset  = 72
	moduleInstanceTableOffset   = 80
	moduleInstanceEngineOffset  = 120
	moduleInstanceTypeIDsOffset = 136

	// Offsets for wasm.TableInstance.
	tableInstanceTableOffset    = 0
	tableInstanceTableLenOffset = 8

	// Offsets for wasm.FunctionInstance.
	functionInstanceTypeIDOffset = 96

	// Offsets for wasm.MemoryInstance.
	memoryInstanceBufferOffset    = 0
	memoryInstanceBufferLenOffset = 8

	// Offsets for wasm.GlobalInstance.
	globalInstanceValueOffset = 8

	// Offsets for Go's interface.
	// https://research.swtch.com/interfaces
	// https://github.com/golang/go/blob/release-branch.go1.17/src/runtime/runtime2.go#L207-L210
	interfaceDataOffset = 8
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
	// jitCallStatusCodeInvalidFloatToIntConversion means an invalid conversion of integer to floats happened.
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
		err = wasmruntime.ErrRuntimeIntegerOverflow
	case jitCallStatusIntegerDivisionByZero:
		err = wasmruntime.ErrRuntimeIntegerDivideByZero
	case jitCallStatusCodeInvalidFloatToIntConversion:
		err = wasmruntime.ErrRuntimeInvalidConversionToInteger
	case jitCallStatusCodeUnreachable:
		err = wasmruntime.ErrRuntimeUnreachable
	case jitCallStatusCodeMemoryOutOfBounds:
		err = wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess
	case jitCallStatusCodeInvalidTableAccess:
		err = wasmruntime.ErrRuntimeInvalidTableAccess
	case jitCallStatusCodeTypeMismatchOnIndirectCall:
		err = wasmruntime.ErrRuntimeIndirectCallTypeMismatch
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
	case jitCallStatusCodeInvalidFloatToIntConversion:
		ret = "invalid float to int conversion"
	case jitCallStatusCodeMemoryOutOfBounds:
		ret = "memory out of bounds"
	case jitCallStatusCodeInvalidTableAccess:
		ret = "invalid table access"
	case jitCallStatusCodeTypeMismatchOnIndirectCall:
		ret = "type mismatch on indirect call"
	case jitCallStatusIntegerOverflow:
		ret = "integer overflow"
	case jitCallStatusIntegerDivisionByZero:
		ret = "integer division by zero"
	default:
		panic("BUG")
	}
	return
}

// String implements fmt.Stringer
func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[%s: return address=0x%x, return stack base pointer=%d]",
		c.function.source.DebugName, c.returnAddress, c.returnStackBasePointer,
	)
}

// releaseCode is a runtime.SetFinalizer function that munmaps the code.codeSegment.
func releaseCode(compiledFn *code) {
	codeSegment := compiledFn.codeSegment
	if codeSegment == nil {
		return // already released
	}

	// Setting this to nil allows tests to know the correct finalizer function was called.
	compiledFn.codeSegment = nil
	if err := munmapCodeSegment(codeSegment); err != nil {
		// munmap failure cannot recover, and happen asynchronously on the finalizer thread. While finalizer
		// functions can return errors, they are ignored. To make these visible for troubleshooting, we panic
		// with additional context. module+funcidx should be enough, but if not, we can add more later.
		panic(fmt.Errorf("jit: failed to munmap code segment for %s.function[%d]: %w", compiledFn.sourceModule.NameSection.ModuleName,
			compiledFn.indexInModule, err))
	}
}

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *engine) DeleteCompiledModule(module *wasm.Module) {
	e.deleteCodes(module)
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *engine) CompileModule(module *wasm.Module) error {
	if _, ok := e.getCodes(module); ok { // cache hit!
		return nil
	}

	funcs := make([]*code, 0, len(module.FunctionSection))

	if module.IsHostModule() {
		for funcIndex := range module.HostFunctionSection {
			compiled, err := compileHostFunction(module.TypeSection[module.FunctionSection[funcIndex]])
			if err != nil {
				return fmt.Errorf("function[%d/%d] %w", funcIndex, len(module.FunctionSection)-1, err)
			}

			// As this uses mmap, we need a finalizer in case moduleEngine.Close was never called. Regardless, we need a
			// finalizer due to how moduleEngine.Close is implemented.
			e.setFinalizer(compiled, releaseCode)

			compiled.indexInModule = wasm.Index(funcIndex)
			compiled.sourceModule = module
			funcs = append(funcs, compiled)
		}
	} else {
		irs, err := wazeroir.CompileFunctions(e.enabledFeatures, module)
		if err != nil {
			return err
		}

		for funcIndex := range module.FunctionSection {
			compiled, err := compileWasmFunction(e.enabledFeatures, irs[funcIndex])
			if err != nil {
				return fmt.Errorf("function[%d/%d] %w", funcIndex, len(module.FunctionSection)-1, err)
			}

			// As this uses mmap, we need to munmap on the compiled machine code when it's GCed.
			e.setFinalizer(compiled, releaseCode)

			compiled.indexInModule = wasm.Index(funcIndex)
			compiled.sourceModule = module

			funcs = append(funcs, compiled)
		}
	}
	e.addCodes(module, funcs)
	return nil
}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *engine) NewModuleEngine(name string, module *wasm.Module, importedFunctions, moduleFunctions []*wasm.FunctionInstance, table *wasm.TableInstance, tableInit map[wasm.Index]wasm.Index) (wasm.ModuleEngine, error) {
	imported := uint32(len(importedFunctions))
	me := &moduleEngine{
		name:                  name,
		functions:             make([]*function, 0, imported+uint32(len(moduleFunctions))),
		importedFunctionCount: imported,
	}

	for _, f := range importedFunctions {
		cf := f.Module.Engine.(*moduleEngine).functions[f.Index]
		me.functions = append(me.functions, cf)
	}

	codes, ok := e.getCodes(module)
	if !ok {
		return nil, fmt.Errorf("source module for %s must be compiled before instantiation", name)
	}

	for i, c := range codes {
		f := moduleFunctions[i]
		function := c.createFunction(f)
		me.functions = append(me.functions, function)
	}

	for elemIdx, funcidx := range tableInit { // Initialize any elements with compiled functions
		table.Table[elemIdx] = me.functions[funcidx]
	}
	return me, nil
}

func (e *engine) deleteCodes(module *wasm.Module) {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.codes, module)
}

func (e *engine) addCodes(module *wasm.Module, fs []*code) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.codes[module] = fs
}

func (e *engine) getCodes(module *wasm.Module) (fs []*code, ok bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()
	fs, ok = e.codes[module]
	return
}

// Name implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Name() string {
	return me.name
}

// Call implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Call(m *wasm.ModuleContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	// Note: The input parameters are pre-validated, so a compiled function is only absent on close. Updates to
	// codes on close aren't locked, neither is this read.
	compiled := me.functions[f.Index]
	if compiled == nil { // Lazy check the cause as it could be because the module was already closed.
		if err = m.FailIfClosed(); err == nil {
			panic(fmt.Errorf("BUG: %s.func[%d] was nil before close", me.name, f.Index))
		}
		return
	}

	paramSignature := f.Type.Params
	paramCount := len(params)
	if len(paramSignature) != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", len(paramSignature), paramCount)
	}

	ce := me.newCallEngine()

	// We ensure that this Call method never panics as
	// this Call method is indirectly invoked by embedders via store.CallFunction,
	// and we have to make sure that all the runtime errors, including the one happening inside
	// host functions, will be captured as errors, not panics.
	defer func() {
		// If the module closed during the call, and the call didn't err for another reason, set an ExitError.
		if err == nil {
			err = m.FailIfClosed()
		}
		// TODO: ^^ Will not fail if the function was imported from a closed module.

		if v := recover(); v != nil {
			builder := wasmdebug.NewErrorBuilder()
			// Handle edge-case where the host function is called directly by Go.
			if ce.globalContext.callFrameStackPointer == 0 {
				fn := compiled.source
				builder.AddFrame(fn.DebugName, fn.ParamTypes(), fn.ResultTypes())
			}
			for i := uint64(0); i < ce.globalContext.callFrameStackPointer; i++ {
				fn := ce.callFrameStack[ce.globalContext.callFrameStackPointer-1-i].function.source
				builder.AddFrame(fn.DebugName, fn.ParamTypes(), fn.ResultTypes())
			}
			err = builder.FromRecovered(v)
		}
	}()

	for _, v := range params {
		ce.pushValue(v)
	}

	if f.Kind == wasm.FunctionKindWasm {
		ce.execWasmFunction(m, compiled)
	} else {
		ce.execHostFunction(f.Kind, compiled.source.GoFunc, m)
	}

	// Note the top value is the tail of the results,
	// so we assign them in reverse order.
	results = make([]uint64, len(f.Type.Results))
	for i := range results {
		results[len(results)-1-i] = ce.popValue()
	}
	return
}

func NewEngine(enabledFeatures wasm.Features) wasm.Engine {
	return newEngine(enabledFeatures)
}

func newEngine(enabledFeatures wasm.Features) *engine {
	return &engine{
		enabledFeatures: enabledFeatures,
		codes:           map[*wasm.Module][]*code{},
		setFinalizer:    runtime.SetFinalizer,
	}
}

// Do not make these variables as constants, otherwise there would be
// dangerous memory access from native code.
//
// Background: Go has a mechanism called "goroutine stack-shrink" where Go
// runtime shrinks Goroutine's stack when it is GCing. Shrinking means that
// all the contents on the goroutine stack will be relocated by runtime,
// Therefore, the memory address of these contents change undeterministically.
// Not only shrinks, but also Go runtime grows the goroutine stack at any point
// of function call entries, which also might end up relocating contents.
//
// On the other hand, we hold pointers to the data region of value stack and
// call-frame stack slices and use these raw pointers from native code.
// Therefore, it is dangerous if these two stacks are allocated on stack
// as these stack's address might be changed by Goruntime which we cannot
// detect.
//
// By declaring these values as `var`, slices created via `make([]..., var)`
// will never be allocated on stack [1]. This means accessing these slices via
// raw pointers is safe: As of version 1.18, Go's garbage collector never relocates
// heap-allocated objects (aka no compilation of memory [2]).
//
// On Go upgrades, re-validate heap-allocation via `go build -gcflags='-m' ./internal/wasm/jit/...`.
//
// [1] https://github.com/golang/go/blob/68ecdc2c70544c303aa923139a5f16caf107d955/src/cmd/compile/internal/escape/utils.go#L206-L208
// [2] https://github.com/golang/go/blob/68ecdc2c70544c303aa923139a5f16caf107d955/src/runtime/mgc.go#L9
// [3] https://mayurwadekar2.medium.com/escape-analysis-in-golang-ee40a1c064c1
// [4] https://medium.com/@yulang.chu/go-stack-or-heap-2-slices-which-keep-in-stack-have-limitation-of-size-b3f3adfd6190
var (
	initialValueStackSize     = 64
	initialCallFrameStackSize = 16
)

func (me *moduleEngine) newCallEngine() *callEngine {
	ce := &callEngine{
		valueStack:     make([]uint64, initialValueStackSize),
		callFrameStack: make([]callFrame, initialCallFrameStackSize),
		archContext:    newArchContext(),
	}

	valueStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&ce.valueStack))
	callFrameStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&ce.callFrameStack))
	ce.globalContext = globalContext{
		valueStackElement0Address:        valueStackHeader.Data,
		valueStackLen:                    uint64(valueStackHeader.Len),
		callFrameStackElementZeroAddress: callFrameStackHeader.Data,
		callFrameStackLen:                uint64(callFrameStackHeader.Len),
		callFrameStackPointer:            0,
	}
	return ce
}

func (ce *callEngine) popValue() (ret uint64) {
	ce.valueStackContext.stackPointer--
	ret = ce.valueStack[ce.valueStackTopIndex()]
	return
}

func (ce *callEngine) pushValue(v uint64) {
	ce.valueStack[ce.valueStackTopIndex()] = v
	ce.valueStackContext.stackPointer++
}

func (ce *callEngine) callFrameTop() *callFrame {
	return &ce.callFrameStack[ce.globalContext.callFrameStackPointer-1]
}

func (ce *callEngine) callFrameAt(depth uint64) *callFrame {
	idx := ce.globalContext.callFrameStackPointer - 1 - depth
	return &ce.callFrameStack[idx]
}

func (ce *callEngine) valueStackTopIndex() uint64 {
	return ce.valueStackContext.stackBasePointer + ce.valueStackContext.stackPointer
}

const (
	builtinFunctionIndexMemoryGrow wasm.Index = iota
	builtinFunctionIndexGrowValueStack
	builtinFunctionIndexGrowCallFrameStack
	// builtinFunctionIndexBreakPoint is internal (only for wazero developers). Disabled by default.
	builtinFunctionIndexBreakPoint
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
func (ce *callEngine) execHostFunction(fk wasm.FunctionKind, f *reflect.Value, ctx *wasm.ModuleContext) {
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
		raw := ce.popValue()
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
			ce.pushValue(uint64(math.Float32bits(float32(ret.Float()))))
		case reflect.Float64:
			ce.pushValue(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			ce.pushValue(ret.Uint())
		case reflect.Int32, reflect.Int64:
			ce.pushValue(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
}

func (ce *callEngine) execWasmFunction(ctx *wasm.ModuleContext, f *function) {
	// Push the initial callframe.
	ce.callFrameStack[0] = callFrame{returnAddress: f.codeInitialAddress, function: f}
	ce.globalContext.callFrameStackPointer++

jitentry:
	{
		frame := ce.callFrameTop()
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s, stackBasePointer: %d, stackPointer: %d\n",
				frame.String(), ce.valueStackContext.stackBasePointer, ce.valueStackContext.stackPointer)
		}

		// Call into the JIT code.
		jitcall(frame.returnAddress, uintptr(unsafe.Pointer(ce)), f.moduleInstanceAddress)

		// Check the status code from JIT code.
		switch status := ce.exitContext.statusCode; status {
		case jitCallStatusCodeReturned:
			// Meaning that all the function frames above the previous call frame stack pointer are executed.
		case jitCallStatusCodeCallHostFunction:
			calleeHostFunction := ce.callFrameTop().function.source
			// Not "callFrameTop" but take the below of peek with "callFrameAt(1)" as the top frame is for host function,
			// but when making host function calls, we need to pass the memory instance of host function caller.
			callercode := ce.callFrameAt(1).function
			// A host function is invoked with the calling frame's memory, which may be different if in another module.
			ce.execHostFunction(calleeHostFunction.Kind, calleeHostFunction.GoFunc,
				ctx.WithMemory(callercode.source.Module.Memory),
			)
			goto jitentry
		case jitCallStatusCodeCallBuiltInFunction:
			switch ce.exitContext.builtinFunctionCallIndex {
			case builtinFunctionIndexMemoryGrow:
				callercode := ce.callFrameTop().function
				ce.builtinFunctionMemoryGrow(callercode.source.Module.Memory)
			case builtinFunctionIndexGrowValueStack:
				callercode := ce.callFrameTop().function
				ce.builtinFunctionGrowValueStack(callercode.stackPointerCeil)
			case builtinFunctionIndexGrowCallFrameStack:
				ce.builtinFunctionGrowCallFrameStack()
			}
			if buildoptions.IsDebugMode {
				if ce.exitContext.builtinFunctionCallIndex == builtinFunctionIndexBreakPoint {
					runtime.Breakpoint()
				}
			}
			goto jitentry
		default:
			status.causePanic()
		}
	}
}

func (ce *callEngine) builtinFunctionGrowValueStack(stackPointerCeil uint64) {
	// Extends the valueStack's length to currentLen*2+stackPointerCeil.
	newLen := ce.globalContext.valueStackLen*2 + (stackPointerCeil)
	newStack := make([]uint64, newLen)
	top := ce.valueStackContext.stackBasePointer + ce.valueStackContext.stackPointer
	copy(newStack[:top], ce.valueStack[:top])
	ce.valueStack = newStack
	valueStackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&ce.valueStack))
	ce.globalContext.valueStackElement0Address = valueStackHeader.Data
	ce.globalContext.valueStackLen = uint64(valueStackHeader.Len)
}

var callStackCeiling = uint64(buildoptions.CallStackCeiling)

func (ce *callEngine) builtinFunctionGrowCallFrameStack() {
	if callStackCeiling < uint64(len(ce.callFrameStack)+1) {
		panic(wasmruntime.ErrRuntimeCallStackOverflow)
	}

	// Double the callstack slice length.
	newLen := uint64(ce.globalContext.callFrameStackLen) * 2
	newStack := make([]callFrame, newLen)
	copy(newStack, ce.callFrameStack)
	ce.callFrameStack = newStack

	// Update the globalContext's fields as they become stale after the update ^^.
	stackSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&newStack))
	ce.globalContext.callFrameStackLen = uint64(stackSliceHeader.Len)
	ce.globalContext.callFrameStackElementZeroAddress = stackSliceHeader.Data
}

func (ce *callEngine) builtinFunctionMemoryGrow(mem *wasm.MemoryInstance) {
	newPages := ce.popValue()

	res := mem.Grow(uint32(newPages))
	ce.pushValue(uint64(res))

	// Update the moduleContext fields as they become stale after the update ^^.
	bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mem.Buffer))
	ce.moduleContext.memorySliceLen = uint64(bufSliceHeader.Len)
	ce.moduleContext.memoryElement0Address = bufSliceHeader.Data
}

func compileHostFunction(sig *wasm.FunctionType) (*code, error) {
	compiler, err := newCompiler(&wazeroir.CompilationResult{Signature: sig})
	if err != nil {
		return nil, err
	}

	if err = compiler.compileHostFunction(); err != nil {
		return nil, err
	}

	c, _, _, err := compiler.compile()
	if err != nil {
		return nil, err
	}

	return &code{codeSegment: c}, nil
}

func compileWasmFunction(enabledFeatures wasm.Features, ir *wazeroir.CompilationResult) (*code, error) {
	compiler, err := newCompiler(ir)
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
		case *wazeroir.OperationSignExtend32From8:
			err = compiler.compileSignExtend32From8()
		case *wazeroir.OperationSignExtend32From16:
			err = compiler.compileSignExtend32From16()
		case *wazeroir.OperationSignExtend64From8:
			err = compiler.compileSignExtend64From8()
		case *wazeroir.OperationSignExtend64From16:
			err = compiler.compileSignExtend64From16()
		case *wazeroir.OperationSignExtend64From32:
			err = compiler.compileSignExtend64From32()
		}
		if err != nil {
			return nil, fmt.Errorf("operation %s: %w", op.Kind().String(), err)
		}
	}

	c, staticData, stackPointerCeil, err := compiler.compile()
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}

	return &code{codeSegment: c, stackPointerCeil: stackPointerCeil, staticData: staticData}, nil
}
