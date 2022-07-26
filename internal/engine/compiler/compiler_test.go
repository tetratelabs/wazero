package compiler

import (
	"fmt"
	"math"
	"os"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestMain(m *testing.M) {
	if !platform.CompilerSupported() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Ensures that the offset consts do not drift when we manipulate the target
// structs.
//
// Note: This is a package initializer as many tests could fail if these
// constants are misaligned, hiding the root cause.
func init() {
	var me moduleEngine
	requireEqual := func(expected, actual int, name string) {
		if expected != actual {
			panic(fmt.Sprintf("%s: expected %d, but was %d", name, expected, actual))
		}
	}
	requireEqual(int(unsafe.Offsetof(me.functions)), moduleEngineFunctionsOffset, "moduleEngineFunctionsOffset")

	var ce callEngine
	// Offsets for callEngine.globalContext.
	requireEqual(int(unsafe.Offsetof(ce.valueStackElement0Address)), callEngineGlobalContextValueStackElement0AddressOffset, "callEngineGlobalContextValueStackElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.valueStackLen)), callEngineGlobalContextValueStackLenOffset, "callEngineGlobalContextValueStackLenOffset")
	requireEqual(int(unsafe.Offsetof(ce.callFrameStackElementZeroAddress)), callEngineGlobalContextCallFrameStackElement0AddressOffset, "callEngineGlobalContextCallFrameStackElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.callFrameStackLen)), callEngineGlobalContextCallFrameStackLenOffset, "callEngineGlobalContextCallFrameStackLenOffset")
	requireEqual(int(unsafe.Offsetof(ce.callFrameStackPointer)), callEngineGlobalContextCallFrameStackPointerOffset, "callEngineGlobalContextCallFrameStackPointerOffset")

	// Offsets for callEngine.moduleContext.
	requireEqual(int(unsafe.Offsetof(ce.moduleInstanceAddress)), callEngineModuleContextModuleInstanceAddressOffset, "callEngineModuleContextModuleInstanceAddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.globalElement0Address)), callEngineModuleContextGlobalElement0AddressOffset, "callEngineModuleContextGlobalElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.memoryElement0Address)), callEngineModuleContextMemoryElement0AddressOffset, "callEngineModuleContextMemoryElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.memorySliceLen)), callEngineModuleContextMemorySliceLenOffset, "callEngineModuleContextMemorySliceLenOffset")
	requireEqual(int(unsafe.Offsetof(ce.tablesElement0Address)), callEngineModuleContextTablesElement0AddressOffset, "callEngineModuleContextTablesElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.functionsElement0Address)), callEngineModuleContextFunctionsElement0AddressOffset, "callEngineModuleContextFunctionsElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.typeIDsElement0Address)), callEngineModuleContextTypeIDsElement0AddressOffset, "callEngineModuleContextTypeIDsElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.dataInstancesElement0Address)), callEngineModuleContextDataInstancesElement0AddressOffset, "callEngineModuleContextDataInstancesElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.elementInstancesElement0Address)), callEngineModuleContextElementInstancesElement0AddressOffset, "callEngineModuleContextElementInstancesElement0AddressOffset")

	// Offsets for callEngine.valueStackContext
	requireEqual(int(unsafe.Offsetof(ce.stackPointer)), callEngineValueStackContextStackPointerOffset, "callEngineValueStackContextStackPointerOffset")
	requireEqual(int(unsafe.Offsetof(ce.stackBasePointer)), callEngineValueStackContextStackBasePointerOffset, "callEngineValueStackContextStackBasePointerOffset")

	// Offsets for callEngine.exitContext.
	requireEqual(int(unsafe.Offsetof(ce.statusCode)), callEngineExitContextNativeCallStatusCodeOffset, "callEngineExitContextNativeCallStatusCodeOffset")
	requireEqual(int(unsafe.Offsetof(ce.builtinFunctionCallIndex)), callEngineExitContextBuiltinFunctionCallAddressOffset, "callEngineExitContextBuiltinFunctionCallAddressOffset")

	// Size and offsets for callFrame.
	var frame callFrame
	requireEqual(int(unsafe.Sizeof(frame)), callFrameDataSize, "callFrameDataSize")
	// Sizeof call-frame must be a power of 2 as we do SHL on the index by "callFrameDataSizeMostSignificantSetBit" to obtain the offset address.
	requireEqual(0, callFrameDataSize&(callFrameDataSize-1), "callFrameDataSize&(callFrameDataSize-1)")
	requireEqual(math.Ilogb(float64(callFrameDataSize)), callFrameDataSizeMostSignificantSetBit, "callFrameDataSizeMostSignificantSetBit")
	requireEqual(int(unsafe.Offsetof(frame.returnAddress)), callFrameReturnAddressOffset, "callFrameReturnAddressOffset")
	requireEqual(int(unsafe.Offsetof(frame.returnStackBasePointer)), callFrameReturnStackBasePointerOffset, "callFrameReturnStackBasePointerOffset")
	requireEqual(int(unsafe.Offsetof(frame.function)), callFrameFunctionOffset, "callFrameFunctionOffset")

	// Offsets for code.
	var compiledFunc function
	requireEqual(int(unsafe.Offsetof(compiledFunc.codeInitialAddress)), functionCodeInitialAddressOffset, "functionCodeInitialAddressOffset")
	requireEqual(int(unsafe.Offsetof(compiledFunc.stackPointerCeil)), functionStackPointerCeilOffset, "functionStackPointerCeilOffset")
	requireEqual(int(unsafe.Offsetof(compiledFunc.source)), functionSourceOffset, "functionSourceOffset")
	requireEqual(int(unsafe.Offsetof(compiledFunc.moduleInstanceAddress)), functionModuleInstanceAddressOffset, "functionModuleInstanceAddressOffset")

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	requireEqual(int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset, "moduleInstanceGlobalsOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.Memory)), moduleInstanceMemoryOffset, "moduleInstanceMemoryOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.Tables)), moduleInstanceTablesOffset, "moduleInstanceTablesOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.Engine)), moduleInstanceEngineOffset, "moduleInstanceEngineOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.TypeIDs)), moduleInstanceTypeIDsOffset, "moduleInstanceTypeIDsOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.DataInstances)), moduleInstanceDataInstancesOffset, "moduleInstanceDataInstancesOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.ElementInstances)), moduleInstanceElementInstancesOffset, "moduleInstanceElementInstancesOffset")

	var functionInstance wasm.FunctionInstance
	requireEqual(int(unsafe.Offsetof(functionInstance.TypeID)), functionInstanceTypeIDOffset, "functionInstanceTypeIDOffset")

	// Offsets for wasm.Table.
	var tableInstance wasm.TableInstance
	requireEqual(int(unsafe.Offsetof(tableInstance.References)), tableInstanceTableOffset, "tableInstanceTableOffset")
	// We add "+8" to get the length of Tables[0].Table
	// since the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	requireEqual(int(unsafe.Offsetof(tableInstance.References)+8), tableInstanceTableLenOffset, "tableInstanceTableLenOffset")

	// Offsets for wasm.Memory
	var memoryInstance wasm.MemoryInstance
	requireEqual(int(unsafe.Offsetof(memoryInstance.Buffer)), memoryInstanceBufferOffset, "memoryInstanceBufferOffset")
	// "+8" because the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	requireEqual(int(unsafe.Offsetof(memoryInstance.Buffer)+8), memoryInstanceBufferLenOffset, "memoryInstanceBufferLenOffset")

	// Offsets for wasm.GlobalInstance
	var globalInstance wasm.GlobalInstance
	requireEqual(int(unsafe.Offsetof(globalInstance.Val)), globalInstanceValueOffset, "globalInstanceValueOffset")

	var dataInstance wasm.DataInstance
	requireEqual(int(unsafe.Sizeof(dataInstance)), dataInstanceStructSize, "dataInstanceStructSize")

	var elementInstance wasm.ElementInstance
	requireEqual(int(unsafe.Sizeof(elementInstance)), elementInstanceStructSize, "elementInstanceStructSize")

	var pointer uintptr
	requireEqual(int(unsafe.Sizeof(pointer)), 1<<pointerSizeLog2, "pointerSizeLog2")
}

type compilerEnv struct {
	me             *moduleEngine
	ce             *callEngine
	moduleInstance *wasm.ModuleInstance
}

func (j *compilerEnv) stackTopAsUint32() uint32 {
	return uint32(j.stack()[j.stackPointer()-1])
}

func (j *compilerEnv) stackTopAsInt32() int32 {
	return int32(j.stack()[j.stackPointer()-1])
}
func (j *compilerEnv) stackTopAsUint64() uint64 {
	return j.stack()[j.stackPointer()-1]
}

func (j *compilerEnv) stackTopAsInt64() int64 {
	return int64(j.stack()[j.stackPointer()-1])
}

func (j *compilerEnv) stackTopAsFloat32() float32 {
	return math.Float32frombits(uint32(j.stack()[j.stackPointer()-1]))
}

func (j *compilerEnv) stackTopAsFloat64() float64 {
	return math.Float64frombits(j.stack()[j.stackPointer()-1])
}

func (j *compilerEnv) stackTopAsV128() (lo uint64, hi uint64) {
	st := j.stack()
	return st[j.stackPointer()-2], st[j.stackPointer()-1]
}

func (j *compilerEnv) memory() []byte {
	return j.moduleInstance.Memory.Buffer
}

func (j *compilerEnv) stack() []uint64 {
	return j.ce.valueStack
}

func (j *compilerEnv) compilerStatus() nativeCallStatusCode {
	return j.ce.exitContext.statusCode
}

func (j *compilerEnv) builtinFunctionCallAddress() wasm.Index {
	return j.ce.exitContext.builtinFunctionCallIndex
}

func (j *compilerEnv) stackPointer() uint64 {
	return j.ce.valueStackContext.stackPointer
}

func (j *compilerEnv) stackBasePointer() uint64 {
	return j.ce.valueStackContext.stackBasePointer
}

func (j *compilerEnv) setStackPointer(sp uint64) {
	j.ce.valueStackContext.stackPointer = sp
}

func (j *compilerEnv) addGlobals(g ...*wasm.GlobalInstance) {
	j.moduleInstance.Globals = append(j.moduleInstance.Globals, g...)
}

func (j *compilerEnv) globals() []*wasm.GlobalInstance {
	return j.moduleInstance.Globals
}

func (j *compilerEnv) addTable(table *wasm.TableInstance) {
	j.moduleInstance.Tables = append(j.moduleInstance.Tables, table)
}

func (j *compilerEnv) callFrameStackPeek() *callFrame {
	return &j.ce.callFrameStack[j.ce.globalContext.callFrameStackPointer-1]
}

func (j *compilerEnv) callFrameStackPointer() uint64 {
	return j.ce.globalContext.callFrameStackPointer
}

func (j *compilerEnv) setValueStackBasePointer(sp uint64) {
	j.ce.valueStackContext.stackBasePointer = sp
}

func (j *compilerEnv) setCallFrameStackPointerLen(l uint64) {
	j.ce.callFrameStackLen = l
}

func (j *compilerEnv) module() *wasm.ModuleInstance {
	return j.moduleInstance
}

func (j *compilerEnv) moduleEngine() *moduleEngine {
	return j.me
}

func (j *compilerEnv) callEngine() *callEngine {
	return j.ce
}

func (j *compilerEnv) newFunctionFrame(codeSegment []byte) *function {
	return &function{
		parent:                &code{codeSegment: codeSegment},
		codeInitialAddress:    uintptr(unsafe.Pointer(&codeSegment[0])),
		moduleInstanceAddress: uintptr(unsafe.Pointer(j.moduleInstance)),
		source: &wasm.FunctionInstance{
			Kind:   wasm.FunctionKindWasm,
			Type:   &wasm.FunctionType{},
			Module: j.moduleInstance,
		},
	}
}

func (j *compilerEnv) exec(codeSegment []byte) {
	j.ce.callFrameStack[j.ce.globalContext.callFrameStackPointer] = callFrame{function: j.newFunctionFrame(codeSegment)}
	j.ce.globalContext.callFrameStackPointer++

	nativecall(
		uintptr(unsafe.Pointer(&codeSegment[0])),
		uintptr(unsafe.Pointer(j.ce)),
		uintptr(unsafe.Pointer(j.moduleInstance)),
	)
}

// newTestCompiler allows us to test a different architecture than the current one.
type newTestCompiler func(ir *wazeroir.CompilationResult) (compiler, error)

func (j *compilerEnv) requireNewCompiler(t *testing.T, fn newTestCompiler, ir *wazeroir.CompilationResult) compilerImpl {
	requireSupportedOSArch(t)

	if ir == nil {
		ir = &wazeroir.CompilationResult{
			LabelCallers: map[string]uint32{},
			Signature:    &wasm.FunctionType{},
		}
	}
	c, err := fn(ir)

	require.NoError(t, err)

	ret, ok := c.(compilerImpl)
	require.True(t, ok)
	return ret
}

// CompilerImpl is the interface used for architecture-independent unit tests in this pkg.
// This is currently implemented by amd64 and arm64.
type compilerImpl interface {
	compiler
	compileExitFromNativeCode(nativeCallStatusCode)
	compileMaybeGrowValueStack() error
	compileReturnFunction() error
	getOnStackPointerCeilDeterminedCallBack() func(uint64)
	setStackPointerCeil(uint64)
	compileReleaseRegisterToStack(loc *runtimeValueLocation)
	setRuntimeValueLocationStack(*runtimeValueLocationStack)
	compileEnsureOnRegister(loc *runtimeValueLocation) error
	compileModuleContextInitialization() error
}

const defaultMemoryPageNumInTest = 1

func newCompilerEnvironment() *compilerEnv {
	me := &moduleEngine{}
	return &compilerEnv{
		me: me,
		moduleInstance: &wasm.ModuleInstance{
			Memory:  &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize*defaultMemoryPageNumInTest)},
			Tables:  []*wasm.TableInstance{},
			Globals: []*wasm.GlobalInstance{},
			Engine:  me,
		},
		ce: me.newCallEngine(),
	}
}
