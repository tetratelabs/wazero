package compiler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/version"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// NOTE: The offset of many of the struct fields defined here are referenced from
// assembly using the constants below such as moduleEngineFunctionsOffset.
// If changing a struct, update the constant and associated tests as needed.
type (
	// engine is a Compiler implementation of wasm.Engine
	engine struct {
		enabledFeatures api.CoreFeatures
		codes           map[wasm.ModuleID][]*code // guarded by mutex.
		fileCache       filecache.Cache
		mux             sync.RWMutex
		// setFinalizer defaults to runtime.SetFinalizer, but overridable for tests.
		setFinalizer  func(obj interface{}, finalizer interface{})
		wazeroVersion string
	}

	// moduleEngine implements wasm.ModuleEngine
	moduleEngine struct {
		// See note at top of file before modifying this struct.

		// functions are the functions in a module instances.
		// The index is module instance-scoped. We intentionally avoid using map
		// as the underlying memory region is accessed by assembly directly by using
		// codesElement0Address.
		functions []function
	}

	// callEngine holds context per moduleEngine.Call, and shared across all the
	// function calls originating from the same moduleEngine.Call execution.
	//
	// This implements api.Function.
	callEngine struct {
		// See note at top of file before modifying this struct.

		// These contexts are read and written by compiled code.
		// Note: structs are embedded to reduce the costs to access fields inside them. Also, this eases field offset
		// calculation.
		moduleContext
		stackContext
		exitContext
		archContext

		// The following fields are not accessed by compiled code directly.

		// stack is the go-allocated stack for holding values and call frames.
		// Note: We never edit len or cap in compiled code, so we won't get screwed when GC comes in.
		//
		// At any point of execution, say currently executing function F2 which was called by F1, then
		// the stack should look like like:
		//
		// 	[..., arg0, arg1, ..., argN, _, _, _, v1, v2, v3, ....
		//	      ^                     {       }
		//	      |                F1's callFrame
		//	      |
		//  stackBasePointer
		//
		// where
		//  - callFrame is the F1's callFrame which called F2. It contains F1's return address, F1's base pointer, and F1's *function.
		//  - stackBasePointer is the stack base pointer stored at (callEngine stackContext.stackBasePointerInBytes)
		//  - arg0, ..., argN are the function parameters, and v1, v2, v3,... are the local variables
		//    including the non-function param locals as well as the temporary variable produced by instructions (e.g i32.const).
		//
		// If the F2 makes a function call to F3 which takes two arguments, then the stack will become:
		//
		// 	[..., arg0, arg1, ..., argN, _, _, _, v1, v2, v3, _, _, _
		//	                            {       }     ^      {       }
		//	                       F1's callFrame     | F2's callFrame
		//	                                          |
		//                                     stackBasePointer
		// where
		// 	- F2's callFrame is pushed above the v2 and v3 (arguments for F3).
		//  - The previous stackBasePointer (pointed at arg0) was saved inside the F2's callFrame.
		//
		// Then, if F3 returns one result, say w1, then the result will look like:
		//
		// 	[..., arg0, arg1, ..., argN, _, _, _, v1, w1, ...
		//	      ^                     {       }
		//	      |                F1's callFrame
		//	      |
		//  stackBasePointer
		//
		// where
		// 	- stackBasePointer was reverted to the position at arg0
		//  - The result from F3 was pushed above v1
		//
		// If the number of parameters is smaller than that of return values, then the empty slots are reserved
		// below the callFrame to store the results on teh return.
		// For example, if F3 takes no parameter but returns N(>0) results, then the stack
		// after making a call against F3 will look like:
		//
		// 	[..., arg0, arg1, ..., argN, _, _, _, v1, v2, v3, res_1, _, res_N, _, _, _
		//	                            {       }            ^                {       }
		//	                       F1's callFrame            |           F2's callFrame
		//	                                                 |
		//                                            stackBasePointer
		// where res_1, ..., res_N are the reserved slots below the call frame. In general,
		// the number of reserved slots equals max(0, len(results)-len(params).
		//
		// This reserved slots are necessary to save the result values onto the stack while not destroying
		// the callFrame value on function returns.
		stack []uint64

		// initialFn is the initial function for this call engine.
		initialFn *function

		// ctx is the context.Context passed to all the host function calls.
		// This is modified when there's a function listener call, otherwise it's always the context.Context
		// passed to the Call API.
		ctx context.Context
		// contextStack is a stack of contexts which is pushed and popped by function listeners.
		// This is used and modified when there are function listeners.
		contextStack *contextStack

		// stackIterator provides a way to iterate over the stack for Listeners.
		// It is setup and valid only during a call to a Listener hook.
		stackIterator stackIterator
	}

	// contextStack is a stack of context.Context.
	contextStack struct {
		// See note at top of file before modifying this struct.

		self context.Context
		prev *contextStack
	}

	// moduleContext holds the per-function call specific module information.
	// This is subject to be manipulated from compiled native code whenever we make function calls.
	moduleContext struct {
		// See note at top of file before modifying this struct.

		// fn holds the currently executed *function.
		fn *function

		// moduleInstance is the address of module instance from which we initialize
		// the following fields. This is set whenever we enter a function or return from function calls.
		//
		// On the entry to the native code, this must be initialized to zero to let native code preamble know
		// that this is the initial function call (which leads to moduleContext initialization pass).
		moduleInstance *wasm.ModuleInstance //lint:ignore U1000 This is only used by Compiler code.

		// globalElement0Address is the address of the first element in the global slice,
		// i.e. &ModuleInstance.Globals[0] as uintptr.
		globalElement0Address uintptr
		// memoryElement0Address is the address of the first element in the global slice,
		// i.e. &ModuleInstance.Memory.Buffer[0] as uintptr.
		memoryElement0Address uintptr
		// memorySliceLen is the length of the memory buffer, i.e. len(ModuleInstance.Memory.Buffer).
		memorySliceLen uint64
		// memoryInstance holds the memory instance for this module instance.
		memoryInstance *wasm.MemoryInstance
		// tableElement0Address is the address of the first item in the tables slice,
		// i.e. &ModuleInstance.Tables[0] as uintptr.
		tablesElement0Address uintptr

		// functionsElement0Address is &moduleContext.functions[0] as uintptr.
		functionsElement0Address uintptr

		// typeIDsElement0Address holds the &ModuleInstance.TypeIDs[0] as uintptr.
		typeIDsElement0Address uintptr

		// dataInstancesElement0Address holds the &ModuleInstance.DataInstances[0] as uintptr.
		dataInstancesElement0Address uintptr

		// elementInstancesElement0Address holds the &ModuleInstance.ElementInstances[0] as uintptr.
		elementInstancesElement0Address uintptr
	}

	// stackContext stores the data to access engine.stack.
	stackContext struct {
		// See note at top of file before modifying this struct.

		// stackPointer on .stack field which is accessed by stack[stackBasePointer+stackBasePointerInBytes*8].
		//
		// Note: stackPointer is not used in assembly since the native code knows exact position of
		// each variable in the value stack from the info from compilation.
		// Therefore, only updated when native code exit from the Compiler world and go back to the Go function.
		stackPointer uint64

		// stackBasePointerInBytes is updated whenever we make function calls.
		// Background: Functions might be compiled as if they use the stack from the bottom.
		// However, in reality, they have to use it from the middle of the stack depending on
		// when these function calls are made. So instead of accessing stack via stackPointer alone,
		// functions are compiled, so they access the stack via [stackBasePointer](fixed for entire function) + [stackPointer].
		// More precisely, stackBasePointer is set to [callee's stack pointer] + [callee's stack base pointer] - [caller's params].
		// This way, compiled functions can be independent of the timing of functions calls made against them.
		stackBasePointerInBytes uint64

		// stackElement0Address is &engine.stack[0] as uintptr.
		// Note: this is updated when growing the stack in builtinFunctionGrowStack.
		stackElement0Address uintptr

		// stackLenInBytes is len(engine.stack[0]) * 8 (bytes).
		// Note: this is updated when growing the stack in builtinFunctionGrowStack.
		stackLenInBytes uint64
	}

	// exitContext will be manipulated whenever compiled native code returns into the Go function.
	exitContext struct {
		// See note at top of file before modifying this struct.

		// Where we store the status code of Compiler execution.
		statusCode nativeCallStatusCode

		// Set when statusCode == compilerStatusCallBuiltInFunction
		// Indicating the function call index.
		builtinFunctionCallIndex wasm.Index

		// returnAddress is the return address which the engine jumps into
		// after executing a builtin function or host function.
		returnAddress uintptr

		// callerModuleInstance holds the caller's wasm.ModuleInstance, and is only valid if currently executing a host function.
		callerModuleInstance *wasm.ModuleInstance
	}

	// callFrame holds the information to which the caller function can return.
	// This is mixed in callEngine.stack with other Wasm values just like any other
	// native program (where the stack is the system stack though), and we retrieve the struct
	// with unsafe pointer casts.
	callFrame struct {
		// See note at top of file before modifying this struct.

		// returnAddress is the return address to which the engine jumps when the callee function returns.
		returnAddress uintptr
		// returnStackBasePointerInBytes is the stack base pointer to set on stackContext.stackBasePointerInBytes
		// when the callee function returns.
		returnStackBasePointerInBytes uint64
		// function is the caller *function, and is used to retrieve the stack trace.
		// Note: should be possible to revive *function from returnAddress, but might be costly.
		function *function
	}

	// Function corresponds to function instance in Wasm, and is created from `code`.
	function struct {
		// See note at top of file before modifying this struct.

		// codeInitialAddress is the pre-calculated pointer pointing to the initial byte of .codeSegment slice.
		// That mean codeInitialAddress always equals uintptr(unsafe.Pointer(&.codeSegment[0]))
		// and we cache the value (uintptr(unsafe.Pointer(&.codeSegment[0]))) to this field,
		// so we don't need to repeat the calculation on each function call.
		codeInitialAddress uintptr
		// moduleInstance holds the address of source.ModuleInstance.
		moduleInstance *wasm.ModuleInstance
		// typeID is the corresponding wasm.FunctionTypeID for funcType.
		typeID wasm.FunctionTypeID
		// index is the function Index in this module.
		index wasm.Index
		// funcType is the function type for this function. Created during compilation.
		funcType *wasm.FunctionType
		// def is the api.Function for this function. Created during compilation.
		def api.FunctionDefinition
		// parent holds code from which this is created.
		parent *code
	}

	// code corresponds to a function in a module (not instantiated one). This holds the machine code
	// compiled by wazero compiler.
	code struct {
		// See note at top of file before modifying this struct.

		// codeSegment is holding the compiled native code as a byte slice.
		codeSegment []byte
		// See the doc for codeStaticData type.
		// stackPointerCeil is the max of the stack pointer this function can reach. Lazily applied via maybeGrowStack.
		stackPointerCeil uint64

		// indexInModule is the index of this function in the module. For logging purpose.
		indexInModule wasm.Index
		// sourceModule is the module from which this function is compiled. For logging purpose.
		sourceModule *wasm.Module
		// listener holds a listener to notify when this function is called.
		listener experimental.FunctionListener
		goFunc   interface{}

		withEnsureTermination bool
		sourceOffsetMap       sourceOffsetMap
	}

	// sourceOffsetMap holds the information to retrieve the original offset in the Wasm binary from the
	// offset in the native binary.
	sourceOffsetMap struct {
		// See note at top of file before modifying this struct.

		// irOperationOffsetsInNativeBinary is index-correlated with irOperationSourceOffsetsInWasmBinary,
		// and maps each index (corresponding to each IR Operation) to the offset in the compiled native code.
		irOperationOffsetsInNativeBinary []uint64
		// irOperationSourceOffsetsInWasmBinary is index-correlated with irOperationOffsetsInNativeBinary.
		// See wazeroir.CompilationResult irOperationOffsetsInNativeBinary.
		irOperationSourceOffsetsInWasmBinary []uint64
	}
)

// Native code reads/writes Go's structs with the following constants.
// See TestVerifyOffsetValue for how to derive these values.
const (
	// Offsets for moduleEngine.functions
	moduleEngineFunctionsOffset = 0

	// Offsets for callEngine moduleContext.
	callEngineModuleContextFnOffset                              = 0
	callEngineModuleContextModuleInstanceOffset                  = 8
	callEngineModuleContextGlobalElement0AddressOffset           = 16
	callEngineModuleContextMemoryElement0AddressOffset           = 24
	callEngineModuleContextMemorySliceLenOffset                  = 32
	callEngineModuleContextMemoryInstanceOffset                  = 40
	callEngineModuleContextTablesElement0AddressOffset           = 48
	callEngineModuleContextFunctionsElement0AddressOffset        = 56
	callEngineModuleContextTypeIDsElement0AddressOffset          = 64
	callEngineModuleContextDataInstancesElement0AddressOffset    = 72
	callEngineModuleContextElementInstancesElement0AddressOffset = 80

	// Offsets for callEngine stackContext.
	callEngineStackContextStackPointerOffset            = 88
	callEngineStackContextStackBasePointerInBytesOffset = 96
	callEngineStackContextStackElement0AddressOffset    = 104
	callEngineStackContextStackLenInBytesOffset         = 112

	// Offsets for callEngine exitContext.
	callEngineExitContextNativeCallStatusCodeOffset     = 120
	callEngineExitContextBuiltinFunctionCallIndexOffset = 124
	callEngineExitContextReturnAddressOffset            = 128
	callEngineExitContextCallerModuleInstanceOffset     = 136

	// Offsets for function.
	functionCodeInitialAddressOffset = 0
	functionModuleInstanceOffset     = 8
	functionTypeIDOffset             = 16
	functionSize                     = 56

	// Offsets for wasm.ModuleInstance.
	moduleInstanceGlobalsOffset          = 32
	moduleInstanceMemoryOffset           = 56
	moduleInstanceTablesOffset           = 64
	moduleInstanceEngineOffset           = 88
	moduleInstanceTypeIDsOffset          = 104
	moduleInstanceDataInstancesOffset    = 128
	moduleInstanceElementInstancesOffset = 152

	// Offsets for wasm.TableInstance.
	tableInstanceTableOffset    = 0
	tableInstanceTableLenOffset = 8

	// Offsets for wasm.MemoryInstance.
	memoryInstanceBufferOffset    = 0
	memoryInstanceBufferLenOffset = 8

	// Offsets for wasm.GlobalInstance.
	globalInstanceValueOffset = 8

	// Offsets for Go's interface.
	// https://research.swtch.com/interfaces
	// https://github.com/golang/go/blob/release-branch.go1.20/src/runtime/runtime2.go#L207-L210
	interfaceDataOffset = 8

	// Consts for wasm.DataInstance.
	dataInstanceStructSize = 24

	// Consts for wasm.ElementInstance.
	elementInstanceStructSize = 32

	// pointerSizeLog2 satisfies: 1 << pointerSizeLog2 = sizeOf(uintptr)
	pointerSizeLog2 = 3

	// callFrameDataSizeInUint64 is the size of callFrame struct per 8 bytes (= size of uint64).
	callFrameDataSizeInUint64 = 24 / 8
)

// nativeCallStatusCode represents the result of `nativecall`.
// This is set by the native code.
type nativeCallStatusCode uint32

const (
	// nativeCallStatusCodeReturned means the nativecall reaches the end of function, and returns successfully.
	nativeCallStatusCodeReturned nativeCallStatusCode = iota
	// nativeCallStatusCodeCallGoHostFunction means the nativecall returns to make a host function call.
	nativeCallStatusCodeCallGoHostFunction
	// nativeCallStatusCodeCallBuiltInFunction means the nativecall returns to make a builtin function call.
	nativeCallStatusCodeCallBuiltInFunction
	// nativeCallStatusCodeUnreachable means the function invocation reaches "unreachable" instruction.
	nativeCallStatusCodeUnreachable
	// nativeCallStatusCodeInvalidFloatToIntConversion means an invalid conversion of integer to floats happened.
	nativeCallStatusCodeInvalidFloatToIntConversion
	// nativeCallStatusCodeMemoryOutOfBounds means an out-of-bounds memory access happened.
	nativeCallStatusCodeMemoryOutOfBounds
	// nativeCallStatusCodeInvalidTableAccess means either offset to the table was out of bounds of table, or
	// the target element in the table was uninitialized during call_indirect instruction.
	nativeCallStatusCodeInvalidTableAccess
	// nativeCallStatusCodeTypeMismatchOnIndirectCall means the type check failed during call_indirect.
	nativeCallStatusCodeTypeMismatchOnIndirectCall
	nativeCallStatusIntegerOverflow
	nativeCallStatusIntegerDivisionByZero
	nativeCallStatusModuleClosed
)

// causePanic causes a panic with the corresponding error to the nativeCallStatusCode.
func (s nativeCallStatusCode) causePanic() {
	var err error
	switch s {
	case nativeCallStatusIntegerOverflow:
		err = wasmruntime.ErrRuntimeIntegerOverflow
	case nativeCallStatusIntegerDivisionByZero:
		err = wasmruntime.ErrRuntimeIntegerDivideByZero
	case nativeCallStatusCodeInvalidFloatToIntConversion:
		err = wasmruntime.ErrRuntimeInvalidConversionToInteger
	case nativeCallStatusCodeUnreachable:
		err = wasmruntime.ErrRuntimeUnreachable
	case nativeCallStatusCodeMemoryOutOfBounds:
		err = wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess
	case nativeCallStatusCodeInvalidTableAccess:
		err = wasmruntime.ErrRuntimeInvalidTableAccess
	case nativeCallStatusCodeTypeMismatchOnIndirectCall:
		err = wasmruntime.ErrRuntimeIndirectCallTypeMismatch
	}
	panic(err)
}

func (s nativeCallStatusCode) String() (ret string) {
	switch s {
	case nativeCallStatusCodeReturned:
		ret = "returned"
	case nativeCallStatusCodeCallGoHostFunction:
		ret = "call_host_function"
	case nativeCallStatusCodeCallBuiltInFunction:
		ret = "call_builtin_function"
	case nativeCallStatusCodeUnreachable:
		ret = "unreachable"
	case nativeCallStatusCodeInvalidFloatToIntConversion:
		ret = "invalid float to int conversion"
	case nativeCallStatusCodeMemoryOutOfBounds:
		ret = "memory out of bounds"
	case nativeCallStatusCodeInvalidTableAccess:
		ret = "invalid table access"
	case nativeCallStatusCodeTypeMismatchOnIndirectCall:
		ret = "type mismatch on indirect call"
	case nativeCallStatusIntegerOverflow:
		ret = "integer overflow"
	case nativeCallStatusIntegerDivisionByZero:
		ret = "integer division by zero"
	case nativeCallStatusModuleClosed:
		ret = "module closed"
	default:
		panic("BUG")
	}
	return
}

// releaseCode is a runtime.SetFinalizer function that munmaps the code.codeSegment.
func releaseCode(compiledFn *code) {
	codeSegment := compiledFn.codeSegment
	if codeSegment == nil {
		return // already released
	}

	// Setting this to nil allows tests to know the correct finalizer function was called.
	compiledFn.codeSegment = nil
	if err := platform.MunmapCodeSegment(codeSegment); err != nil {
		// munmap failure cannot recover, and happen asynchronously on the finalizer thread. While finalizer
		// functions can return errors, they are ignored. To make these visible for troubleshooting, we panic
		// with additional context. module+funcidx should be enough, but if not, we can add more later.
		panic(fmt.Errorf("compiler: failed to munmap code segment for %s.function[%d]: %w", compiledFn.sourceModule.NameSection.ModuleName,
			compiledFn.indexInModule, err))
	}
}

// CompiledModuleCount implements the same method as documented on wasm.Engine.
func (e *engine) CompiledModuleCount() uint32 {
	return uint32(len(e.codes))
}

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *engine) DeleteCompiledModule(module *wasm.Module) {
	e.deleteCodes(module)
}

// Close implements the same method as documented on wasm.Engine.
func (e *engine) Close() (err error) {
	e.mux.Lock()
	defer e.mux.Unlock()
	// Releasing the references to compiled codes including the memory-mapped machine codes.
	e.codes = nil
	return
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *engine) CompileModule(_ context.Context, module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) error {
	if _, ok, err := e.getCodes(module); ok { // cache hit!
		return nil
	} else if err != nil {
		return err
	}

	irCompiler, err := wazeroir.NewCompiler(e.enabledFeatures, callFrameDataSizeInUint64, module, ensureTermination)
	if err != nil {
		return err
	}

	var withGoFunc bool
	importedFuncs := module.ImportFunctionCount
	funcs := make([]*code, len(module.FunctionSection))
	ln := len(listeners)
	cmp := newCompiler()
	for i := range module.CodeSection {
		typ := &module.TypeSection[module.FunctionSection[i]]
		var lsn experimental.FunctionListener
		if i < ln {
			lsn = listeners[i]
		}
		funcIndex := wasm.Index(i)
		var compiled *code
		if codeSeg := &module.CodeSection[i]; codeSeg.GoFunc != nil {
			cmp.Init(typ, nil, lsn != nil)
			withGoFunc = true
			if compiled, err = compileGoDefinedHostFunction(cmp); err != nil {
				def := module.FunctionDefinitionSection[funcIndex+importedFuncs]
				return fmt.Errorf("error compiling host go func[%s]: %w", def.DebugName(), err)
			}
			compiled.goFunc = codeSeg.GoFunc
		} else {
			ir, err := irCompiler.Next()
			if err != nil {
				return fmt.Errorf("failed to lower func[%d]: %v", i, err)
			}
			cmp.Init(typ, ir, lsn != nil)
			if compiled, err = compileWasmFunction(cmp, ir); err != nil {
				def := module.FunctionDefinitionSection[funcIndex+importedFuncs]
				return fmt.Errorf("error compiling wasm func[%s]: %w", def.DebugName(), err)
			}
		}

		// As this uses mmap, we need to munmap on the compiled machine code when it's GCed.
		e.setFinalizer(compiled, releaseCode)

		compiled.listener = lsn
		compiled.indexInModule = funcIndex
		compiled.sourceModule = module
		compiled.withEnsureTermination = ensureTermination
		funcs[funcIndex] = compiled
	}
	return e.addCodes(module, funcs, withGoFunc)
}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *engine) NewModuleEngine(module *wasm.Module, instance *wasm.ModuleInstance) (wasm.ModuleEngine, error) {
	me := &moduleEngine{
		functions: make([]function, len(module.FunctionSection)+int(module.ImportFunctionCount)),
	}

	// Note: imported functions are resolved in moduleEngine.ResolveImportedFunction.

	codes, ok, err := e.getCodes(module)
	if !ok {
		return nil, errors.New("source module must be compiled before instantiation")
	} else if err != nil {
		return nil, err
	}

	for i, c := range codes {
		offset := int(module.ImportFunctionCount) + i
		typeIndex := module.FunctionSection[i]
		me.functions[offset] = function{
			codeInitialAddress: uintptr(unsafe.Pointer(&c.codeSegment[0])),
			moduleInstance:     instance,
			index:              wasm.Index(offset),
			typeID:             instance.TypeIDs[typeIndex],
			funcType:           &module.TypeSection[typeIndex],
			def:                &module.FunctionDefinitionSection[offset],
			parent:             c,
		}
	}
	return me, nil
}

// ResolveImportedFunction implements wasm.ModuleEngine.
func (e *moduleEngine) ResolveImportedFunction(index, indexInImportedModule wasm.Index, importedModuleEngine wasm.ModuleEngine) {
	imported := importedModuleEngine.(*moduleEngine)
	// Copies the content from the import target moduleEngine.
	e.functions[index] = imported.functions[indexInImportedModule]
	// Update the .index field to the value in this Module.
	e.functions[index].index = index
}

// FunctionInstanceReference implements the same method as documented on wasm.ModuleEngine.
func (e *moduleEngine) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference {
	return uintptr(unsafe.Pointer(&e.functions[funcIndex]))
}

func (e *moduleEngine) NewFunction(index wasm.Index) api.Function {
	// Note: The input parameters are pre-validated, so a compiled function is only absent on close. Updates to
	// code on close aren't locked, neither is this read.
	compiled := &e.functions[index]

	initStackSize := initialStackSize
	if initialStackSize < compiled.parent.stackPointerCeil {
		initStackSize = compiled.parent.stackPointerCeil * 2
	}
	return e.newCallEngine(initStackSize, compiled)
}

// LookupFunction implements the same method as documented on wasm.ModuleEngine.
func (e *moduleEngine) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (idx wasm.Index, err error) {
	if tableOffset >= uint32(len(t.References)) || t.Type != wasm.RefTypeFuncref {
		err = wasmruntime.ErrRuntimeInvalidTableAccess
		return
	}
	rawPtr := t.References[tableOffset]
	if rawPtr == 0 {
		err = wasmruntime.ErrRuntimeInvalidTableAccess
		return
	}

	tf := functionFromUintptr(rawPtr)
	if tf.typeID != typeId {
		err = wasmruntime.ErrRuntimeIndirectCallTypeMismatch
		return
	}
	idx = tf.index

	return
}

// functionFromUintptr resurrects the original *function from the given uintptr
// which comes from either funcref table or OpcodeRefFunc instruction.
func functionFromUintptr(ptr uintptr) *function {
	// Wraps ptrs as the double pointer in order to avoid the unsafe access as detected by race detector.
	//
	// For example, if we have (*function)(unsafe.Pointer(ptr)) instead, then the race detector's "checkptr"
	// subroutine wanrs as "checkptr: pointer arithmetic result points to invalid allocation"
	// https://github.com/golang/go/blob/1ce7fcf139417d618c2730010ede2afb41664211/src/runtime/checkptr.go#L69
	var wrapped *uintptr = &ptr
	return *(**function)(unsafe.Pointer(wrapped))
}

// Definition implements the same method as documented on wasm.ModuleEngine.
func (ce *callEngine) Definition() api.FunctionDefinition {
	return ce.initialFn.def
}

// Call implements the same method as documented on wasm.ModuleEngine.
func (ce *callEngine) Call(ctx context.Context, params ...uint64) (results []uint64, err error) {
	m := ce.initialFn.moduleInstance
	if ce.fn.parent.withEnsureTermination {
		select {
		case <-ctx.Done():
			// If the provided context is already done, close the call context
			// and return the error.
			m.CloseWithCtxErr(ctx)
			return nil, m.FailIfClosed()
		default:
		}
	}

	tp := ce.initialFn.funcType

	paramCount := len(params)
	if tp.ParamNumInUint64 != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", ce.initialFn.funcType.ParamNumInUint64, paramCount)
	}

	// We ensure that this Call method never panics as
	// this Call method is indirectly invoked by embedders via store.CallFunction,
	// and we have to make sure that all the runtime errors, including the one happening inside
	// host functions, will be captured as errors, not panics.
	defer func() {
		err = ce.deferredOnCall(recover())
		if err == nil {
			// If the module closed during the call, and the call didn't err for another reason, set an ExitError.
			err = m.FailIfClosed()
		}
	}()

	ce.initializeStack(tp, params)

	if ce.fn.parent.withEnsureTermination {
		done := m.CloseModuleOnCanceledOrTimeout(ctx)
		defer done()
	}

	ce.execWasmFunction(ctx, m)

	// This returns a safe copy of the results, instead of a slice view. If we
	// returned a re-slice, the caller could accidentally or purposefully
	// corrupt the stack of subsequent calls
	if resultCount := tp.ResultNumInUint64; resultCount > 0 {
		results = make([]uint64, resultCount)
		copy(results, ce.stack[:resultCount])
	}
	return
}

// initializeStack initializes callEngine.stack before entering native code.
//
// The stack must look like, if len(params) < len(results):
//
//	[arg0, arg1, ..., argN, 0, 0, 0, ...
//	                       {       } ^
//	                       callFrame |
//	                                 |
//	                            stackPointer
//
// else:
//
//	[arg0, arg1, ..., argN, _, _, _,  0, 0, 0, ...
//	                      |        | {       }  ^
//	                      |reserved| callFrame  |
//	                      |        |            |
//	                      |-------->       stackPointer
//	                 len(results)-len(params)
//
//		 where we reserve the slots below the callframe with the length len(results)-len(params).
//
// Note: callFrame {  } is zeroed to indicate that the initial "caller" is this callEngine, not the Wasm function.
//
// See callEngine.stack as well.
func (ce *callEngine) initializeStack(tp *wasm.FunctionType, args []uint64) {
	for _, v := range args {
		ce.pushValue(v)
	}

	ce.stackPointer = uint64(callFrameOffset(tp))

	for i := 0; i < callFrameDataSizeInUint64; i++ {
		ce.stack[ce.stackPointer] = 0
		ce.stackPointer++
	}
}

// callFrameOffset returns the offset of the call frame from the stack base pointer.
//
// See the diagram in callEngine.stack.
func callFrameOffset(funcType *wasm.FunctionType) (ret int) {
	ret = funcType.ResultNumInUint64
	if ret < funcType.ParamNumInUint64 {
		ret = funcType.ParamNumInUint64
	}
	return
}

// deferredOnCall takes the recovered value `recovered`, and wraps it
// with the call frame stack traces when not nil. This also resets
// the state of callEngine so that it can be used for the subsequent calls.
//
// This is defined for testability.
func (ce *callEngine) deferredOnCall(recovered interface{}) (err error) {
	if recovered != nil {
		builder := wasmdebug.NewErrorBuilder()

		// Unwinds call frames from the values stack, starting from the
		// current function `ce.fn`, and the current stack base pointer `ce.stackBasePointerInBytes`.
		fn := ce.fn
		pc := uint64(ce.returnAddress)
		stackBasePointer := int(ce.stackBasePointerInBytes >> 3)
		for {
			def := fn.def

			// sourceInfo holds the source code information corresponding to the frame.
			// It is not empty only when the DWARF is enabled.
			var sources []string
			if p := fn.parent; p.codeSegment != nil {
				if len(fn.parent.sourceOffsetMap.irOperationSourceOffsetsInWasmBinary) != 0 {
					offset := fn.getSourceOffsetInWasmBinary(pc)
					sources = p.sourceModule.DWARFLines.Line(offset)
				}
			}
			builder.AddFrame(def.DebugName(), def.ParamTypes(), def.ResultTypes(), sources)

			callFrameOffset := callFrameOffset(fn.funcType)
			if stackBasePointer != 0 {
				frame := *(*callFrame)(unsafe.Pointer(&ce.stack[stackBasePointer+callFrameOffset]))
				fn = frame.function
				pc = uint64(frame.returnAddress)
				stackBasePointer = int(frame.returnStackBasePointerInBytes >> 3)
			} else { // base == 0 means that this was the last call frame stacked.
				break
			}
		}
		err = builder.FromRecovered(recovered)
	}

	// Allows the reuse of CallEngine.
	ce.stackBasePointerInBytes, ce.stackPointer, ce.moduleInstance = 0, 0, nil
	ce.moduleContext.fn = ce.initialFn
	return
}

// getSourceOffsetInWasmBinary returns the corresponding offset in the original Wasm binary's code section
// for the given pc (which is an absolute address in the memory).
// If needPreviousInstr equals true, this returns the previous instruction's offset for the given pc.
func (f *function) getSourceOffsetInWasmBinary(pc uint64) uint64 {
	srcMap := &f.parent.sourceOffsetMap
	n := len(srcMap.irOperationOffsetsInNativeBinary) + 1

	// Calculate the offset in the compiled native binary.
	pcOffsetInNativeBinary := pc - uint64(f.codeInitialAddress)

	// Then, do the binary search on the list of offsets in the native binary for all the IR operations.
	// This returns the index of the *next* IR operation of the one corresponding to the origin of this pc.
	// See sort.Search.
	index := sort.Search(n, func(i int) bool {
		if i == n-1 {
			return true
		}
		return srcMap.irOperationOffsetsInNativeBinary[i] >= pcOffsetInNativeBinary
	})

	if index == n || index == 0 { // This case, somehow pc is not found in the source offset map.
		return 0
	} else {
		return srcMap.irOperationSourceOffsetsInWasmBinary[index-1]
	}
}

func NewEngine(_ context.Context, enabledFeatures api.CoreFeatures, fileCache filecache.Cache) wasm.Engine {
	return newEngine(enabledFeatures, fileCache)
}

func newEngine(enabledFeatures api.CoreFeatures, fileCache filecache.Cache) *engine {
	return &engine{
		enabledFeatures: enabledFeatures,
		codes:           map[wasm.ModuleID][]*code{},
		setFinalizer:    runtime.SetFinalizer,
		fileCache:       fileCache,
		wazeroVersion:   version.GetWazeroVersion(),
	}
}

// Do not make this variable as constant, otherwise there would be
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
// as these stack's address might be changed by Goroutine which we cannot
// detect.
//
// By declaring these values as `var`, slices created via `make([]..., var)`
// will never be allocated on stack [1]. This means accessing these slices via
// raw pointers is safe: As of version 1.18, Go's garbage collector never relocates
// heap-allocated objects (aka no compaction of memory [2]).
//
// On Go upgrades, re-validate heap-allocation via `go build -gcflags='-m' ./internal/engine/compiler/...`.
//
//	[1] https://github.com/golang/go/blob/68ecdc2c70544c303aa923139a5f16caf107d955/src/cmd/compile/internal/escape/utils.go#L206-L208
//	[2] https://github.com/golang/go/blob/68ecdc2c70544c303aa923139a5f16caf107d955/src/runtime/mgc.go#L9
//	[3] https://mayurwadekar2.medium.com/escape-analysis-in-golang-ee40a1c064c1
//	[4] https://medium.com/@yulang.chu/go-stack-or-heap-2-slices-which-keep-in-stack-have-limitation-of-size-b3f3adfd6190
var initialStackSize uint64 = 512

func (e *moduleEngine) newCallEngine(stackSize uint64, fn *function) *callEngine {
	ce := &callEngine{
		stack:         make([]uint64, stackSize),
		archContext:   newArchContext(),
		initialFn:     fn,
		moduleContext: moduleContext{fn: fn},
	}

	stackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&ce.stack))
	ce.stackContext = stackContext{
		stackElement0Address: stackHeader.Data,
		stackLenInBytes:      uint64(stackHeader.Len) << 3,
	}
	return ce
}

func (ce *callEngine) popValue() (ret uint64) {
	ce.stackContext.stackPointer--
	ret = ce.stack[ce.stackTopIndex()]
	return
}

func (ce *callEngine) pushValue(v uint64) {
	ce.stack[ce.stackTopIndex()] = v
	ce.stackContext.stackPointer++
}

func (ce *callEngine) stackTopIndex() uint64 {
	return ce.stackContext.stackPointer + (ce.stackContext.stackBasePointerInBytes >> 3)
}

const (
	builtinFunctionIndexMemoryGrow wasm.Index = iota
	builtinFunctionIndexGrowStack
	builtinFunctionIndexTableGrow
	builtinFunctionIndexFunctionListenerBefore
	builtinFunctionIndexFunctionListenerAfter
	builtinFunctionIndexCheckExitCode
	// builtinFunctionIndexBreakPoint is internal (only for wazero developers). Disabled by default.
	builtinFunctionIndexBreakPoint
)

func (ce *callEngine) execWasmFunction(ctx context.Context, m *wasm.ModuleInstance) {
	codeAddr := ce.initialFn.codeInitialAddress
	modAddr := ce.initialFn.moduleInstance
	ce.ctx = ctx

entry:
	{
		// Call into the native code.
		nativecall(codeAddr, uintptr(unsafe.Pointer(ce)), modAddr)

		// Check the status code from Compiler code.
		switch status := ce.exitContext.statusCode; status {
		case nativeCallStatusCodeReturned:
		case nativeCallStatusCodeCallGoHostFunction:
			calleeHostFunction := ce.moduleContext.fn
			base := int(ce.stackBasePointerInBytes >> 3)

			// In the compiler engine, ce.stack has enough capacity for the
			// max of param or result length, so we don't need to grow when
			// there are more results than parameters.
			stackLen := calleeHostFunction.funcType.ParamNumInUint64
			if resultLen := calleeHostFunction.funcType.ResultNumInUint64; resultLen > stackLen {
				stackLen = resultLen
			}
			stack := ce.stack[base : base+stackLen]

			fn := calleeHostFunction.parent.goFunc
			switch fn := fn.(type) {
			case api.GoModuleFunction:
				fn.Call(ce.ctx, ce.callerModuleInstance, stack)
			case api.GoFunction:
				fn.Call(ce.ctx, stack)
			}

			codeAddr, modAddr = ce.returnAddress, ce.moduleInstance
			goto entry
		case nativeCallStatusCodeCallBuiltInFunction:
			caller := ce.moduleContext.fn
			switch ce.exitContext.builtinFunctionCallIndex {
			case builtinFunctionIndexMemoryGrow:
				ce.builtinFunctionMemoryGrow(caller.moduleInstance.MemoryInstance)
			case builtinFunctionIndexGrowStack:
				ce.builtinFunctionGrowStack(caller.parent.stackPointerCeil)
			case builtinFunctionIndexTableGrow:
				ce.builtinFunctionTableGrow(caller.moduleInstance.Tables)
			case builtinFunctionIndexFunctionListenerBefore:
				ce.builtinFunctionFunctionListenerBefore(ce.ctx, m, caller)
			case builtinFunctionIndexFunctionListenerAfter:
				ce.builtinFunctionFunctionListenerAfter(ce.ctx, m, caller)
			case builtinFunctionIndexCheckExitCode:
				// Note: this operation must be done in Go, not native code. The reason is that
				// native code cannot be preempted and that means it can block forever if there are not
				// enough OS threads (which we don't have control over).
				if err := m.FailIfClosed(); err != nil {
					panic(err)
				}
			}
			if false {
				if ce.exitContext.builtinFunctionCallIndex == builtinFunctionIndexBreakPoint {
					runtime.Breakpoint()
				}
			}

			codeAddr, modAddr = ce.returnAddress, ce.moduleInstance
			goto entry
		default:
			status.causePanic()
		}
	}
}

// callStackCeiling is the maximum WebAssembly call frame stack height. This allows wazero to raise
// wasm.ErrCallStackOverflow instead of overflowing the Go runtime.
//
// The default value should suffice for most use cases. Those wishing to change this can via `go build -ldflags`.
//
// TODO: allows to configure this via context?
var callStackCeiling = uint64(5000000) // in uint64 (8 bytes) == 40000000 bytes in total == 40mb.

func (ce *callEngine) builtinFunctionGrowStack(stackPointerCeil uint64) {
	oldLen := uint64(len(ce.stack))
	if callStackCeiling < oldLen {
		panic(wasmruntime.ErrRuntimeStackOverflow)
	}

	// Extends the stack's length to oldLen*2+stackPointerCeil.
	newLen := oldLen<<1 + (stackPointerCeil)
	newStack := make([]uint64, newLen)
	top := ce.stackTopIndex()
	copy(newStack[:top], ce.stack[:top])
	ce.stack = newStack
	stackHeader := (*reflect.SliceHeader)(unsafe.Pointer(&ce.stack))
	ce.stackContext.stackElement0Address = stackHeader.Data
	ce.stackContext.stackLenInBytes = newLen << 3
}

func (ce *callEngine) builtinFunctionMemoryGrow(mem *wasm.MemoryInstance) {
	newPages := ce.popValue()

	if res, ok := mem.Grow(uint32(newPages)); !ok {
		ce.pushValue(uint64(0xffffffff)) // = -1 in signed 32-bit integer.
	} else {
		ce.pushValue(uint64(res))
	}

	// Update the moduleContext fields as they become stale after the update ^^.
	bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mem.Buffer))
	ce.moduleContext.memorySliceLen = uint64(bufSliceHeader.Len)
	ce.moduleContext.memoryElement0Address = bufSliceHeader.Data
}

func (ce *callEngine) builtinFunctionTableGrow(tables []*wasm.TableInstance) {
	tableIndex := uint32(ce.popValue())
	table := tables[tableIndex] // verified not to be out of range by the func validation at compilation phase.
	num := ce.popValue()
	ref := ce.popValue()
	res := table.Grow(uint32(num), uintptr(ref))
	ce.pushValue(uint64(res))
}

// stackIterator implements experimental.StackIterator.
type stackIterator struct {
	stack   []uint64
	fn      *function
	base    int
	started bool
}

func (si *stackIterator) reset(stack []uint64, fn *function, base int) {
	si.stack = stack
	si.fn = fn
	si.base = base
	si.started = false
}

func (si *stackIterator) clear() {
	si.stack = nil
	si.fn = nil
	si.base = 0
	si.started = false
}

// Next implements experimental.StackIterator.
func (si *stackIterator) Next() bool {
	if !si.started {
		si.started = true
		return true
	}

	if si.fn == nil || si.base == 0 {
		return false
	}

	frame := si.base + callFrameOffset(si.fn.funcType)
	si.base = int(si.stack[frame+1] >> 3)
	// *function lives in the third field of callFrame struct. This must be
	// aligned with the definition of callFrame struct.
	si.fn = *(**function)(unsafe.Pointer(&si.stack[frame+2]))
	return si.fn != nil
}

// FunctionDefinition implements experimental.StackIterator.
func (si *stackIterator) FunctionDefinition() api.FunctionDefinition {
	return si.fn.def
}

// Args implements experimental.StackIterator.
func (si *stackIterator) Parameters() []uint64 {
	return si.stack[si.base : si.base+si.fn.funcType.ParamNumInUint64]
}

func (ce *callEngine) builtinFunctionFunctionListenerBefore(ctx context.Context, mod api.Module, fn *function) {
	base := int(ce.stackBasePointerInBytes >> 3)
	ce.stackIterator.reset(ce.stack, fn, base)

	listerCtx := fn.parent.listener.Before(ctx, mod, fn.def, ce.stack[base:base+fn.funcType.ParamNumInUint64], &ce.stackIterator)
	prevStackTop := ce.contextStack
	ce.contextStack = &contextStack{self: ctx, prev: prevStackTop}

	ce.ctx = listerCtx
	ce.stackIterator.clear()
}

func (ce *callEngine) builtinFunctionFunctionListenerAfter(ctx context.Context, mod api.Module, fn *function) {
	base := int(ce.stackBasePointerInBytes >> 3)
	fn.parent.listener.After(ctx, mod, fn.def, nil, ce.stack[base:base+fn.funcType.ResultNumInUint64])
	ce.ctx = ce.contextStack.self
	ce.contextStack = ce.contextStack.prev
}

func compileGoDefinedHostFunction(cmp compiler) (*code, error) {
	if err := cmp.compileGoDefinedHostFunction(); err != nil {
		return nil, err
	}

	c, _, err := cmp.compile()
	if err != nil {
		return nil, err
	}

	return &code{codeSegment: c}, nil
}

func compileWasmFunction(cmp compiler, ir *wazeroir.CompilationResult) (*code, error) {
	if err := cmp.compilePreamble(); err != nil {
		return nil, fmt.Errorf("failed to emit preamble: %w", err)
	}

	needSourceOffsets := len(ir.IROperationSourceOffsetsInWasmBinary) > 0
	var irOpBegins []asm.Node
	if needSourceOffsets {
		irOpBegins = make([]asm.Node, len(ir.Operations))
	}

	var skip bool
	for i := range ir.Operations {
		op := &ir.Operations[i]
		if needSourceOffsets {
			// If this compilation requires source offsets for DWARF based back trace,
			// we emit a NOP node at the beginning of each IR operation to get the
			// binary offset of the beginning of the corresponding compiled native code.
			irOpBegins[i] = cmp.compileNOP()
		}

		// Compiler determines whether skip the entire label.
		// For example, if the label doesn't have any caller,
		// we don't need to generate native code at all as we never reach the region.
		if op.Kind == wazeroir.OperationKindLabel {
			skip = cmp.compileLabel(op)
		}
		if skip {
			continue
		}

		if false {
			fmt.Printf("compiling op=%s: %s\n", op.Kind, cmp)
		}
		var err error
		switch op.Kind {
		case wazeroir.OperationKindUnreachable:
			err = cmp.compileUnreachable()
		case wazeroir.OperationKindLabel:
		// label op is already handled ^^.
		case wazeroir.OperationKindBr:
			err = cmp.compileBr(op)
		case wazeroir.OperationKindBrIf:
			err = cmp.compileBrIf(op)
		case wazeroir.OperationKindBrTable:
			err = cmp.compileBrTable(op)
		case wazeroir.OperationKindCall:
			err = cmp.compileCall(op)
		case wazeroir.OperationKindCallIndirect:
			err = cmp.compileCallIndirect(op)
		case wazeroir.OperationKindDrop:
			err = cmp.compileDrop(op)
		case wazeroir.OperationKindSelect:
			err = cmp.compileSelect(op)
		case wazeroir.OperationKindPick:
			err = cmp.compilePick(op)
		case wazeroir.OperationKindSet:
			err = cmp.compileSet(op)
		case wazeroir.OperationKindGlobalGet:
			err = cmp.compileGlobalGet(op)
		case wazeroir.OperationKindGlobalSet:
			err = cmp.compileGlobalSet(op)
		case wazeroir.OperationKindLoad:
			err = cmp.compileLoad(op)
		case wazeroir.OperationKindLoad8:
			err = cmp.compileLoad8(op)
		case wazeroir.OperationKindLoad16:
			err = cmp.compileLoad16(op)
		case wazeroir.OperationKindLoad32:
			err = cmp.compileLoad32(op)
		case wazeroir.OperationKindStore:
			err = cmp.compileStore(op)
		case wazeroir.OperationKindStore8:
			err = cmp.compileStore8(op)
		case wazeroir.OperationKindStore16:
			err = cmp.compileStore16(op)
		case wazeroir.OperationKindStore32:
			err = cmp.compileStore32(op)
		case wazeroir.OperationKindMemorySize:
			err = cmp.compileMemorySize()
		case wazeroir.OperationKindMemoryGrow:
			err = cmp.compileMemoryGrow()
		case wazeroir.OperationKindConstI32:
			err = cmp.compileConstI32(op)
		case wazeroir.OperationKindConstI64:
			err = cmp.compileConstI64(op)
		case wazeroir.OperationKindConstF32:
			err = cmp.compileConstF32(op)
		case wazeroir.OperationKindConstF64:
			err = cmp.compileConstF64(op)
		case wazeroir.OperationKindEq:
			err = cmp.compileEq(op)
		case wazeroir.OperationKindNe:
			err = cmp.compileNe(op)
		case wazeroir.OperationKindEqz:
			err = cmp.compileEqz(op)
		case wazeroir.OperationKindLt:
			err = cmp.compileLt(op)
		case wazeroir.OperationKindGt:
			err = cmp.compileGt(op)
		case wazeroir.OperationKindLe:
			err = cmp.compileLe(op)
		case wazeroir.OperationKindGe:
			err = cmp.compileGe(op)
		case wazeroir.OperationKindAdd:
			err = cmp.compileAdd(op)
		case wazeroir.OperationKindSub:
			err = cmp.compileSub(op)
		case wazeroir.OperationKindMul:
			err = cmp.compileMul(op)
		case wazeroir.OperationKindClz:
			err = cmp.compileClz(op)
		case wazeroir.OperationKindCtz:
			err = cmp.compileCtz(op)
		case wazeroir.OperationKindPopcnt:
			err = cmp.compilePopcnt(op)
		case wazeroir.OperationKindDiv:
			err = cmp.compileDiv(op)
		case wazeroir.OperationKindRem:
			err = cmp.compileRem(op)
		case wazeroir.OperationKindAnd:
			err = cmp.compileAnd(op)
		case wazeroir.OperationKindOr:
			err = cmp.compileOr(op)
		case wazeroir.OperationKindXor:
			err = cmp.compileXor(op)
		case wazeroir.OperationKindShl:
			err = cmp.compileShl(op)
		case wazeroir.OperationKindShr:
			err = cmp.compileShr(op)
		case wazeroir.OperationKindRotl:
			err = cmp.compileRotl(op)
		case wazeroir.OperationKindRotr:
			err = cmp.compileRotr(op)
		case wazeroir.OperationKindAbs:
			err = cmp.compileAbs(op)
		case wazeroir.OperationKindNeg:
			err = cmp.compileNeg(op)
		case wazeroir.OperationKindCeil:
			err = cmp.compileCeil(op)
		case wazeroir.OperationKindFloor:
			err = cmp.compileFloor(op)
		case wazeroir.OperationKindTrunc:
			err = cmp.compileTrunc(op)
		case wazeroir.OperationKindNearest:
			err = cmp.compileNearest(op)
		case wazeroir.OperationKindSqrt:
			err = cmp.compileSqrt(op)
		case wazeroir.OperationKindMin:
			err = cmp.compileMin(op)
		case wazeroir.OperationKindMax:
			err = cmp.compileMax(op)
		case wazeroir.OperationKindCopysign:
			err = cmp.compileCopysign(op)
		case wazeroir.OperationKindI32WrapFromI64:
			err = cmp.compileI32WrapFromI64()
		case wazeroir.OperationKindITruncFromF:
			err = cmp.compileITruncFromF(op)
		case wazeroir.OperationKindFConvertFromI:
			err = cmp.compileFConvertFromI(op)
		case wazeroir.OperationKindF32DemoteFromF64:
			err = cmp.compileF32DemoteFromF64()
		case wazeroir.OperationKindF64PromoteFromF32:
			err = cmp.compileF64PromoteFromF32()
		case wazeroir.OperationKindI32ReinterpretFromF32:
			err = cmp.compileI32ReinterpretFromF32()
		case wazeroir.OperationKindI64ReinterpretFromF64:
			err = cmp.compileI64ReinterpretFromF64()
		case wazeroir.OperationKindF32ReinterpretFromI32:
			err = cmp.compileF32ReinterpretFromI32()
		case wazeroir.OperationKindF64ReinterpretFromI64:
			err = cmp.compileF64ReinterpretFromI64()
		case wazeroir.OperationKindExtend:
			err = cmp.compileExtend(op)
		case wazeroir.OperationKindSignExtend32From8:
			err = cmp.compileSignExtend32From8()
		case wazeroir.OperationKindSignExtend32From16:
			err = cmp.compileSignExtend32From16()
		case wazeroir.OperationKindSignExtend64From8:
			err = cmp.compileSignExtend64From8()
		case wazeroir.OperationKindSignExtend64From16:
			err = cmp.compileSignExtend64From16()
		case wazeroir.OperationKindSignExtend64From32:
			err = cmp.compileSignExtend64From32()
		case wazeroir.OperationKindMemoryInit:
			err = cmp.compileMemoryInit(op)
		case wazeroir.OperationKindDataDrop:
			err = cmp.compileDataDrop(op)
		case wazeroir.OperationKindMemoryCopy:
			err = cmp.compileMemoryCopy()
		case wazeroir.OperationKindMemoryFill:
			err = cmp.compileMemoryFill()
		case wazeroir.OperationKindTableInit:
			err = cmp.compileTableInit(op)
		case wazeroir.OperationKindElemDrop:
			err = cmp.compileElemDrop(op)
		case wazeroir.OperationKindTableCopy:
			err = cmp.compileTableCopy(op)
		case wazeroir.OperationKindRefFunc:
			err = cmp.compileRefFunc(op)
		case wazeroir.OperationKindTableGet:
			err = cmp.compileTableGet(op)
		case wazeroir.OperationKindTableSet:
			err = cmp.compileTableSet(op)
		case wazeroir.OperationKindTableGrow:
			err = cmp.compileTableGrow(op)
		case wazeroir.OperationKindTableSize:
			err = cmp.compileTableSize(op)
		case wazeroir.OperationKindTableFill:
			err = cmp.compileTableFill(op)
		case wazeroir.OperationKindV128Const:
			err = cmp.compileV128Const(op)
		case wazeroir.OperationKindV128Add:
			err = cmp.compileV128Add(op)
		case wazeroir.OperationKindV128Sub:
			err = cmp.compileV128Sub(op)
		case wazeroir.OperationKindV128Load:
			err = cmp.compileV128Load(op)
		case wazeroir.OperationKindV128LoadLane:
			err = cmp.compileV128LoadLane(op)
		case wazeroir.OperationKindV128Store:
			err = cmp.compileV128Store(op)
		case wazeroir.OperationKindV128StoreLane:
			err = cmp.compileV128StoreLane(op)
		case wazeroir.OperationKindV128ExtractLane:
			err = cmp.compileV128ExtractLane(op)
		case wazeroir.OperationKindV128ReplaceLane:
			err = cmp.compileV128ReplaceLane(op)
		case wazeroir.OperationKindV128Splat:
			err = cmp.compileV128Splat(op)
		case wazeroir.OperationKindV128Shuffle:
			err = cmp.compileV128Shuffle(op)
		case wazeroir.OperationKindV128Swizzle:
			err = cmp.compileV128Swizzle(op)
		case wazeroir.OperationKindV128AnyTrue:
			err = cmp.compileV128AnyTrue(op)
		case wazeroir.OperationKindV128AllTrue:
			err = cmp.compileV128AllTrue(op)
		case wazeroir.OperationKindV128BitMask:
			err = cmp.compileV128BitMask(op)
		case wazeroir.OperationKindV128And:
			err = cmp.compileV128And(op)
		case wazeroir.OperationKindV128Not:
			err = cmp.compileV128Not(op)
		case wazeroir.OperationKindV128Or:
			err = cmp.compileV128Or(op)
		case wazeroir.OperationKindV128Xor:
			err = cmp.compileV128Xor(op)
		case wazeroir.OperationKindV128Bitselect:
			err = cmp.compileV128Bitselect(op)
		case wazeroir.OperationKindV128AndNot:
			err = cmp.compileV128AndNot(op)
		case wazeroir.OperationKindV128Shl:
			err = cmp.compileV128Shl(op)
		case wazeroir.OperationKindV128Shr:
			err = cmp.compileV128Shr(op)
		case wazeroir.OperationKindV128Cmp:
			err = cmp.compileV128Cmp(op)
		case wazeroir.OperationKindV128AddSat:
			err = cmp.compileV128AddSat(op)
		case wazeroir.OperationKindV128SubSat:
			err = cmp.compileV128SubSat(op)
		case wazeroir.OperationKindV128Mul:
			err = cmp.compileV128Mul(op)
		case wazeroir.OperationKindV128Div:
			err = cmp.compileV128Div(op)
		case wazeroir.OperationKindV128Neg:
			err = cmp.compileV128Neg(op)
		case wazeroir.OperationKindV128Sqrt:
			err = cmp.compileV128Sqrt(op)
		case wazeroir.OperationKindV128Abs:
			err = cmp.compileV128Abs(op)
		case wazeroir.OperationKindV128Popcnt:
			err = cmp.compileV128Popcnt(op)
		case wazeroir.OperationKindV128Min:
			err = cmp.compileV128Min(op)
		case wazeroir.OperationKindV128Max:
			err = cmp.compileV128Max(op)
		case wazeroir.OperationKindV128AvgrU:
			err = cmp.compileV128AvgrU(op)
		case wazeroir.OperationKindV128Pmin:
			err = cmp.compileV128Pmin(op)
		case wazeroir.OperationKindV128Pmax:
			err = cmp.compileV128Pmax(op)
		case wazeroir.OperationKindV128Ceil:
			err = cmp.compileV128Ceil(op)
		case wazeroir.OperationKindV128Floor:
			err = cmp.compileV128Floor(op)
		case wazeroir.OperationKindV128Trunc:
			err = cmp.compileV128Trunc(op)
		case wazeroir.OperationKindV128Nearest:
			err = cmp.compileV128Nearest(op)
		case wazeroir.OperationKindV128Extend:
			err = cmp.compileV128Extend(op)
		case wazeroir.OperationKindV128ExtMul:
			err = cmp.compileV128ExtMul(op)
		case wazeroir.OperationKindV128Q15mulrSatS:
			err = cmp.compileV128Q15mulrSatS(op)
		case wazeroir.OperationKindV128ExtAddPairwise:
			err = cmp.compileV128ExtAddPairwise(op)
		case wazeroir.OperationKindV128FloatPromote:
			err = cmp.compileV128FloatPromote(op)
		case wazeroir.OperationKindV128FloatDemote:
			err = cmp.compileV128FloatDemote(op)
		case wazeroir.OperationKindV128FConvertFromI:
			err = cmp.compileV128FConvertFromI(op)
		case wazeroir.OperationKindV128Dot:
			err = cmp.compileV128Dot(op)
		case wazeroir.OperationKindV128Narrow:
			err = cmp.compileV128Narrow(op)
		case wazeroir.OperationKindV128ITruncSatFromF:
			err = cmp.compileV128ITruncSatFromF(op)
		case wazeroir.OperationKindBuiltinFunctionCheckExitCode:
			err = cmp.compileBuiltinFunctionCheckExitCode()
		default:
			err = errors.New("unsupported")
		}
		if err != nil {
			return nil, fmt.Errorf("operation %s: %w", op.Kind.String(), err)
		}
	}

	c, stackPointerCeil, err := cmp.compile()
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}

	ret := &code{codeSegment: c, stackPointerCeil: stackPointerCeil}
	if needSourceOffsets {
		offsetInNativeBin := make([]uint64, len(irOpBegins))
		for i, nop := range irOpBegins {
			offsetInNativeBin[i] = nop.OffsetInBinary()
		}
		sm := &ret.sourceOffsetMap
		sm.irOperationOffsetsInNativeBinary = offsetInNativeBin
		sm.irOperationSourceOffsetsInWasmBinary = make([]uint64, len(ir.IROperationSourceOffsetsInWasmBinary))
		copy(sm.irOperationSourceOffsetsInWasmBinary, ir.IROperationSourceOffsetsInWasmBinary)
	}
	return ret, nil
}
