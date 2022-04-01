package jit

import (
	"fmt"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
	"github.com/tetratelabs/wazero/sys"
)

type (
	// engine is an JIT implementation of wasm.Engine
	engine struct {
		compiledFunctions map[*wasm.FunctionInstance]*compiledFunction // guarded by mutex.
		mux               sync.RWMutex
		// setFinalizer defaults to runtime.SetFinalizer, but overridable for tests.
		setFinalizer func(obj interface{}, finalizer interface{})
	}

	// moduleEngine implements wasm.ModuleEngine
	moduleEngine struct {
		// name is the name the module was instantiated with used for error handling.
		name string

		// compiledFunctions are the compiled functions in a module instances.
		// The index is module instance-scoped. We intentionally avoid using map
		// as the underlying memory region is accessed by assembly directly by using
		// compiledFunctionsElement0Address.
		compiledFunctions []*compiledFunction

		// parentEngine holds *engine from which this module engine is created from.
		parentEngine          *engine
		importedFunctionCount uint32

		// closed is the pointer used both to guard moduleEngine.CloseWithExitCode and to store the exit code.
		//
		// The update value is 1 + exitCode << 32. This ensures an exit code of zero isn't mistaken for never closed.
		//
		// Note: Exclusively reading and updating this with atomics guarantees cross-goroutine observations.
		// See /RATIONALE.md
		closed uint64
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

		// compiledFunctionsElement0Address is &moduleContext.engine.compiledFunctions[0] as uintptr.
		compiledFunctionsElement0Address uintptr
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
		compiledFunction *compiledFunction
		// _ is a necessary padding to make the size of callFrame struct a power of 2.
		_ [8]byte
	}

	compiledFunction struct {
		// The following fields are accessed by JITed code.

		// Pre-calculated pointer pointing to the initial byte of .codeSegment slice.
		// That mean codeInitialAddress always equals uintptr(unsafe.Pointer(&.codeSegment[0]))
		// and we cache the value (uintptr(unsafe.Pointer(&.codeSegment[0]))) to this field,
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
	// The primary index is the logical separation of multiple data, for example data[0] and data[1]
	// correspond to different jump tables for different br_table instructions.
	compiledFunctionStaticData = [][]byte
)

// Native code reads/writes Go's structs with the following constants.
// See TestVerifyOffsetValue for how to derive these values.
const (
	// Offsets for moduleEngine.compiledFunctions
	moduleEngineCompiledFunctionsOffset = 16

	// Offsets for callEngine globalContext.
	callEngineGlobalContextValueStackElement0AddressOffset     = 0
	callEngineGlobalContextValueStackLenOffset                 = 8
	callEngineGlobalContextCallFrameStackElement0AddressOffset = 16
	callEngineGlobalContextCallFrameStackLenOffset             = 24
	callEngineGlobalContextCallFrameStackPointerOffset         = 32

	// Offsets for callEngine moduleContext.
	callEngineModuleContextModuleInstanceAddressOffset            = 40
	callEngineModuleContextGlobalElement0AddressOffset            = 48
	callEngineModuleContextMemoryElement0AddressOffset            = 56
	callEngineModuleContextMemorySliceLenOffset                   = 64
	callEngineModuleContextTableElement0AddressOffset             = 72
	callEngineModuleContextTableSliceLenOffset                    = 80
	callEngineModuleContextCompiledFunctionsElement0AddressOffset = 88

	// Offsets for callEngine valueStackContext.
	callEngineValueStackContextStackPointerOffset     = 96
	callEngineValueStackContextStackBasePointerOffset = 104

	// Offsets for callEngine exitContext.
	callEngineExitContextJITCallStatusCodeOffset          = 112
	callEngineExitContextBuiltinFunctionCallAddressOffset = 116

	// Offsets for callFrame.
	callFrameDataSize                      = 32
	callFrameDataSizeMostSignificantSetBit = 5
	callFrameReturnAddressOffset           = 0
	callFrameReturnStackBasePointerOffset  = 8
	callFrameCompiledFunctionOffset        = 16

	// Offsets for compiledFunction.
	compiledFunctionCodeInitialAddressOffset = 0
	compiledFunctionStackPointerCeilOffset   = 8
	compiledFunctionSourceOffset             = 16

	// Offsets for wasm.ModuleInstance.
	moduleInstanceGlobalsOffset = 48
	moduleInstanceMemoryOffset  = 72
	moduleInstanceTableOffset   = 80
	moduleInstanceEngineOffset  = 120

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

// String implements fmt.Stringer
func (c *callFrame) String() string {
	return fmt.Sprintf(
		"[%s: return address=0x%x, return stack base pointer=%d]",
		c.compiledFunction.source.Name, c.returnAddress, c.returnStackBasePointer,
	)
}

// releaseCompiledFunction is a runtime.SetFinalizer function that munmaps the compiledFunction.codeSegment.
func releaseCompiledFunction(compiledFn *compiledFunction) {
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
		panic(fmt.Errorf("jit: failed to munmap code segment for %s.function[%d]: %w", compiledFn.source.Module.Name, compiledFn.source.Index, err))
	}
}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *engine) NewModuleEngine(name string, importedFunctions, moduleFunctions []*wasm.FunctionInstance, table *wasm.TableInstance, tableInit map[wasm.Index]wasm.Index) (wasm.ModuleEngine, error) {
	imported := uint32(len(importedFunctions))
	me := &moduleEngine{
		name:                  name,
		compiledFunctions:     make([]*compiledFunction, 0, imported+uint32(len(moduleFunctions))),
		parentEngine:          e,
		importedFunctionCount: imported,
	}

	for idx, f := range importedFunctions {
		cf, ok := e.getCompiledFunction(f)
		if !ok {
			return nil, fmt.Errorf("import[%d] func[%s.%s]: uncompiled", idx, f.Module.Name, f.Name)
		}
		me.compiledFunctions = append(me.compiledFunctions, cf)
	}

	for i, f := range moduleFunctions {
		var compiled *compiledFunction
		var err error
		if f.Kind == wasm.FunctionKindWasm {
			compiled, err = compileWasmFunction(f)
		} else {
			compiled, err = compileHostFunction(f)
		}
		if err != nil {
			me.doClose() // safe because the reference to me was never leaked.
			return nil, fmt.Errorf("function[%d/%d] %w", i, len(moduleFunctions)-1, err)
		}

		// As this uses mmap, we need a finalizer in case moduleEngine.Close was never called. Regardless, we need a
		// finalizer due to how moduleEngine.doClose is implemented.
		e.setFinalizer(compiled, releaseCompiledFunction)

		me.compiledFunctions = append(me.compiledFunctions, compiled)

		// Add the compiled function to the store-wide engine as well so that
		// the future importing module can refer the function instance.
		e.addCompiledFunction(f, compiled)
	}

	for elemIdx, funcidx := range tableInit { // Initialize any elements with compiled functions
		table.Table[elemIdx] = me.compiledFunctions[funcidx]
	}
	return me, nil
}

// doClose is guarded by the caller with CAS, which means it happens only once. However, there is a race-condition
// inside the critical section: functions are removed from the parent engine, but there's no guard to prevent this
// moduleInstance from making new calls. This means at inside the critical section there could be in-flight calls,
// and even after it new calls can be made, given a reference to this moduleEngine.
//
// To ensure neither in-flight, nor new calls segfault due to missing code segment, memory isn't unmapped here. So, this
// relies on the fact that NewModuleEngine already added a finalizer for each compiledFunction,
//
// Note that the finalizer is a queue of work to be done at some point (perhaps never). In worst case, the finalizer
// doesn't run and functions in already closed modules retain memory until exhaustion.
//
// Potential future design (possibly faulty, so expect impl to be more complete or better):
//  * Change this to implement io.Closer and document this is blocking
//    * This implies adding docs can suggest this is run in a goroutine
//    * io.Closer allows an error return we can use in case an unrecoverable error happens
//  * Continue to guard with CAS so that close is only executed once
//  * Once in the critical section, write a status bit to a fixed memory location.
//    * End new calls with a Closed Error if this is read.
//    * This guard allows Close to eventually complete.
//  * Block exiting the critical section until all in-flight calls complete.
//    * Knowing which in-flight calls from other modules, that can use this module may be tricky
//    * Pure wasm functions can be left to complete.
//    * Host functions are the only unknowns (ex can do I/O) so they may need to be tracked.
func (me *moduleEngine) doClose() {
	// Release all the function instances declared in this module.
	for _, cf := range me.compiledFunctions[me.importedFunctionCount:] {
		// NOTE: we still rely on the finalizer of cf until the notes on this function are addressed.
		me.parentEngine.deleteCompiledFunction(cf.source)
	}
}

func (e *engine) deleteCompiledFunction(f *wasm.FunctionInstance) {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.compiledFunctions, f)
}

func (e *engine) addCompiledFunction(f *wasm.FunctionInstance, cf *compiledFunction) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.compiledFunctions[f] = cf
}

func (e *engine) getCompiledFunction(f *wasm.FunctionInstance) (cf *compiledFunction, ok bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()
	cf, ok = e.compiledFunctions[f]
	return
}

// Name implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Name() string {
	return me.name
}

// Call implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Call(ctx *wasm.ModuleContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	// Note: The input parameters are pre-validated, so a compiled function is only absent on close. Updates to
	// compiledFunctions on close aren't locked, neither is this read.
	compiled := me.compiledFunctions[f.Index]
	if compiled == nil { // Lazy check the cause as it could be because the module was already closed.
		if err = failIfClosed(me); err == nil {
			panic(fmt.Errorf("BUG: %s.compiledFunctions[%d] was nil before close", me.name, f.Index))
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
			err = failIfClosed(me)
		}
		// TODO: ^^ Will not fail if the function was imported from a closed module.

		if v := recover(); v != nil {
			if buildoptions.IsDebugMode {
				debug.PrintStack()
			}

			var frames []string
			for i := uint64(0); i < ce.globalContext.callFrameStackPointer; i++ {
				f := ce.callFrameStack[ce.globalContext.callFrameStackPointer-1-i].compiledFunction
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

	for _, v := range params {
		ce.pushValue(v)
	}

	if f.Kind == wasm.FunctionKindWasm {
		ce.execWasmFunction(ctx, compiled)
	} else {
		ce.execHostFunction(f.Kind, compiled.source.GoFunc, ctx)
	}

	// Note the top value is the tail of the results,
	// so we assign them in reverse order.
	results = make([]uint64, len(f.Type.Results))
	for i := range results {
		results[len(results)-1-i] = ce.popValue()
	}
	return
}

// failIfClosed returns a sys.ExitError if moduleEngine.CloseWithExitCode was called.
func failIfClosed(me *moduleEngine) error {
	if closed := atomic.LoadUint64(&me.closed); closed != 0 {
		return sys.NewExitError(me.name, uint32(closed>>32)) // Unpack the high order bits as the exit code.
	}
	return nil
}

// CloseWithExitCode implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) CloseWithExitCode(exitCode uint32) (bool, error) {
	closed := uint64(1) + uint64(exitCode)<<32 // Store exitCode as high-order bits.
	if !atomic.CompareAndSwapUint64(&me.closed, 0, closed) {
		return false, nil
	}
	me.doClose()
	return true, nil
}

func NewEngine() wasm.Engine {
	return newEngine()
}

func newEngine() *engine {
	return &engine{
		compiledFunctions: map[*wasm.FunctionInstance]*compiledFunction{},
		setFinalizer:      runtime.SetFinalizer,
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
	return &ce.callFrameStack[ce.globalContext.callFrameStackPointer-1-depth]
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

func (ce *callEngine) execWasmFunction(ctx *wasm.ModuleContext, f *compiledFunction) {
	ce.pushCallFrame(f)

jitentry:
	{
		frame := ce.callFrameTop()
		if buildoptions.IsDebugMode {
			fmt.Printf("callframe=%s, stackBasePointer: %d, stackPointer: %d\n",
				frame.String(), ce.valueStackContext.stackBasePointer, ce.valueStackContext.stackPointer)
		}

		// Call into the JIT code.
		jitcall(frame.returnAddress, uintptr(unsafe.Pointer(ce)))

		// Check the status code from JIT code.
		switch status := ce.exitContext.statusCode; status {
		case jitCallStatusCodeReturned:
			// Meaning that all the function frames above the previous call frame stack pointer are executed.
		case jitCallStatusCodeCallHostFunction:
			calleeHostFunction := ce.callFrameTop().compiledFunction.source
			// Not "callFrameTop" but take the below of peek with "callFrameAt(1)" as the top frame is for host function,
			// but when making host function calls, we need to pass the memory instance of host function caller.
			callerCompiledFunction := ce.callFrameAt(1).compiledFunction
			// A host function is invoked with the calling frame's memory, which may be different if in another module.
			ce.execHostFunction(calleeHostFunction.Kind, calleeHostFunction.GoFunc,
				ctx.WithMemory(callerCompiledFunction.source.Module.Memory),
			)
			goto jitentry
		case jitCallStatusCodeCallBuiltInFunction:
			switch ce.exitContext.builtinFunctionCallIndex {
			case builtinFunctionIndexMemoryGrow:
				callerCompiledFunction := ce.callFrameTop().compiledFunction
				ce.builtinFunctionMemoryGrow(callerCompiledFunction.source.Module.Memory)
			case builtinFunctionIndexGrowValueStack:
				callerCompiledFunction := ce.callFrameTop().compiledFunction
				ce.builtinFunctionGrowValueStack(callerCompiledFunction.stackPointerCeil)
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

// pushInitialFrame is implemented in assembly as well, but this Go version is used BEFORE jit entry.
func (ce *callEngine) pushCallFrame(f *compiledFunction) {
	// Push the new frame to the top of stack.
	ce.callFrameStack[ce.globalContext.callFrameStackPointer] = callFrame{returnAddress: f.codeInitialAddress, compiledFunction: f}
	ce.globalContext.callFrameStackPointer++

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
	// That means the next stack base pointer is calculated as follows (note stack pointer is relative to base):
	ce.valueStackContext.stackBasePointer =
		ce.valueStackContext.stackBasePointer + ce.valueStackContext.stackPointer - uint64(len(f.source.Type.Params))
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
		panic(wasm.ErrRuntimeCallStackOverflow)
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

// golang-asm is not goroutine-safe so we take lock until we complete the compilation.
// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
var assemblerMutex = &sync.Mutex{}

func compileHostFunction(f *wasm.FunctionInstance) (*compiledFunction, error) {
	assemblerMutex.Lock()
	defer assemblerMutex.Unlock()

	compiler, err := newCompiler(f, nil)
	if err != nil {
		return nil, err
	}

	if err = compiler.compileHostFunction(); err != nil {
		return nil, err
	}

	code, _, _, err := compiler.compile()
	if err != nil {
		return nil, err
	}

	stackPointerCeil := uint64(len(f.Type.Params))
	if res := uint64(len(f.Type.Results)); stackPointerCeil < res {
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
	assemblerMutex.Lock()
	defer assemblerMutex.Unlock()

	ir, err := wazeroir.Compile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to lower to wazeroir: %w", err)
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
