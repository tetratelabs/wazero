package compiler

import (
	"fmt"
	"math"
	"os"
	"runtime"
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
	// Offsets for callEngine.moduleContext.
	requireEqual(int(unsafe.Offsetof(ce.fn)), callEngineModuleContextFnOffset, "callEngineModuleContextFnOffset")
	requireEqual(int(unsafe.Offsetof(ce.moduleInstance)), callEngineModuleContextModuleInstanceOffset, "callEngineModuleContextModuleInstanceOffset")
	requireEqual(int(unsafe.Offsetof(ce.globalElement0Address)), callEngineModuleContextGlobalElement0AddressOffset, "callEngineModuleContextGlobalElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.memoryElement0Address)), callEngineModuleContextMemoryElement0AddressOffset, "callEngineModuleContextMemoryElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.memorySliceLen)), callEngineModuleContextMemorySliceLenOffset, "callEngineModuleContextMemorySliceLenOffset")
	requireEqual(int(unsafe.Offsetof(ce.memoryInstance)), callEngineModuleContextMemoryInstanceOffset, "callEngineModuleContextMemoryInstanceOffset")
	requireEqual(int(unsafe.Offsetof(ce.tablesElement0Address)), callEngineModuleContextTablesElement0AddressOffset, "callEngineModuleContextTablesElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.functionsElement0Address)), callEngineModuleContextFunctionsElement0AddressOffset, "callEngineModuleContextFunctionsElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.typeIDsElement0Address)), callEngineModuleContextTypeIDsElement0AddressOffset, "callEngineModuleContextTypeIDsElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.dataInstancesElement0Address)), callEngineModuleContextDataInstancesElement0AddressOffset, "callEngineModuleContextDataInstancesElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.elementInstancesElement0Address)), callEngineModuleContextElementInstancesElement0AddressOffset, "callEngineModuleContextElementInstancesElement0AddressOffset")

	// Offsets for callEngine.stackContext
	requireEqual(int(unsafe.Offsetof(ce.stackPointer)), callEngineStackContextStackPointerOffset, "callEngineStackContextStackPointerOffset")
	requireEqual(int(unsafe.Offsetof(ce.stackBasePointerInBytes)), callEngineStackContextStackBasePointerInBytesOffset, "callEngineStackContextStackBasePointerInBytesOffset")
	requireEqual(int(unsafe.Offsetof(ce.stackElement0Address)), callEngineStackContextStackElement0AddressOffset, "callEngineStackContextStackElement0AddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.stackLenInBytes)), callEngineStackContextStackLenInBytesOffset, "callEngineStackContextStackLenInBytesOffset")

	// Offsets for callEngine.exitContext.
	requireEqual(int(unsafe.Offsetof(ce.statusCode)), callEngineExitContextNativeCallStatusCodeOffset, "callEngineExitContextNativeCallStatusCodeOffset")
	requireEqual(int(unsafe.Offsetof(ce.builtinFunctionCallIndex)), callEngineExitContextBuiltinFunctionCallIndexOffset, "callEngineExitContextBuiltinFunctionCallIndexOffset")
	requireEqual(int(unsafe.Offsetof(ce.returnAddress)), callEngineExitContextReturnAddressOffset, "callEngineExitContextReturnAddressOffset")
	requireEqual(int(unsafe.Offsetof(ce.callerModuleInstance)), callEngineExitContextCallerModuleInstanceOffset, "callEngineExitContextCallerModuleInstanceOffset")

	// Size and offsets for callFrame.
	var frame callFrame
	requireEqual(int(unsafe.Sizeof(frame))/8, callFrameDataSizeInUint64, "callFrameDataSize")

	// Offsets for code.
	var f function
	requireEqual(int(unsafe.Offsetof(f.codeInitialAddress)), functionCodeInitialAddressOffset, "functionCodeInitialAddressOffset")
	requireEqual(int(unsafe.Offsetof(f.moduleInstance)), functionModuleInstanceOffset, "functionModuleInstanceOffset")
	requireEqual(int(unsafe.Offsetof(f.typeID)), functionTypeIDOffset, "functionTypeIDOffset")
	requireEqual(int(unsafe.Sizeof(f)), functionSize, "functionModuleInstanceOffset")

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	requireEqual(int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset, "moduleInstanceGlobalsOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.MemoryInstance)), moduleInstanceMemoryOffset, "moduleInstanceMemoryOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.Tables)), moduleInstanceTablesOffset, "moduleInstanceTablesOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.Engine)), moduleInstanceEngineOffset, "moduleInstanceEngineOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.TypeIDs)), moduleInstanceTypeIDsOffset, "moduleInstanceTypeIDsOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.DataInstances)), moduleInstanceDataInstancesOffset, "moduleInstanceDataInstancesOffset")
	requireEqual(int(unsafe.Offsetof(moduleInstance.ElementInstances)), moduleInstanceElementInstancesOffset, "moduleInstanceElementInstancesOffset")

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
	return uint32(j.stack()[j.ce.stackContext.stackPointer-1])
}

func (j *compilerEnv) stackTopAsInt32() int32 {
	return int32(j.stack()[j.ce.stackContext.stackPointer-1])
}

func (j *compilerEnv) stackTopAsUint64() uint64 {
	return j.stack()[j.ce.stackContext.stackPointer-1]
}

func (j *compilerEnv) stackTopAsInt64() int64 {
	return int64(j.stack()[j.ce.stackContext.stackPointer-1])
}

func (j *compilerEnv) stackTopAsFloat32() float32 {
	return math.Float32frombits(uint32(j.stack()[j.ce.stackContext.stackPointer-1]))
}

func (j *compilerEnv) stackTopAsFloat64() float64 {
	return math.Float64frombits(j.stack()[j.ce.stackContext.stackPointer-1])
}

func (j *compilerEnv) stackTopAsV128() (lo, hi uint64) {
	st := j.stack()
	return st[j.ce.stackContext.stackPointer-2], st[j.ce.stackContext.stackPointer-1]
}

func (j *compilerEnv) memory() []byte {
	return j.moduleInstance.MemoryInstance.Buffer
}

func (j *compilerEnv) stack() []uint64 {
	return j.ce.stack
}

func (j *compilerEnv) compilerStatus() nativeCallStatusCode {
	return j.ce.exitContext.statusCode
}

func (j *compilerEnv) builtinFunctionCallAddress() wasm.Index {
	return j.ce.exitContext.builtinFunctionCallIndex
}

// stackPointer returns the stack pointer minus the call frame.
func (j *compilerEnv) stackPointer() uint64 {
	return j.ce.stackContext.stackPointer - callFrameDataSizeInUint64
}

func (j *compilerEnv) stackBasePointer() uint64 {
	return j.ce.stackContext.stackBasePointerInBytes >> 3
}

func (j *compilerEnv) setStackPointer(sp uint64) {
	j.ce.stackContext.stackPointer = sp
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

func (j *compilerEnv) setStackBasePointer(sp uint64) {
	j.ce.stackContext.stackBasePointerInBytes = sp << 3
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

func (j *compilerEnv) exec(machineCode []byte) {
	cm := new(compiledModule)
	if err := cm.executable.Map(len(machineCode)); err != nil {
		panic(err)
	}
	executable := cm.executable.Bytes()
	copy(executable, machineCode)
	makeExecutable(executable)

	f := &function{
		parent:             &compiledFunction{parent: cm},
		codeInitialAddress: uintptr(unsafe.Pointer(&executable[0])),
		moduleInstance:     j.moduleInstance,
	}
	j.ce.initialFn = f
	j.ce.fn = f

	nativecall(
		uintptr(unsafe.Pointer(&executable[0])),
		j.ce, j.moduleInstance,
	)
}

func (j *compilerEnv) requireNewCompiler(t *testing.T, functionType *wasm.FunctionType, fn func() compiler, ir *wazeroir.CompilationResult) compilerImpl {
	requireSupportedOSArch(t)

	if ir == nil {
		ir = &wazeroir.CompilationResult{
			LabelCallers: map[wazeroir.Label]uint32{},
		}
	}

	c := fn()
	c.Init(functionType, ir, false)

	ret, ok := c.(compilerImpl)
	require.True(t, ok)
	return ret
}

// compilerImpl is the interface used for architecture-independent unit tests in this pkg.
// This is currently implemented by amd64 and arm64.
type compilerImpl interface {
	compiler
	compileExitFromNativeCode(nativeCallStatusCode)
	compileMaybeGrowStack() error
	compileReturnFunction() error
	assignStackPointerCeil(uint64)
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
			MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize*defaultMemoryPageNumInTest)},
			Tables:         []*wasm.TableInstance{},
			Globals:        []*wasm.GlobalInstance{},
			Engine:         me,
		},
		ce: me.newCallEngine(initialStackSize, &function{parent: &compiledFunction{parent: &compiledModule{}}}),
	}
}

// requireRuntimeLocationStackPointerEqual ensures that the compiler's runtimeValueLocationStack has
// the expected stack pointer value relative to the call frame.
func requireRuntimeLocationStackPointerEqual(t *testing.T, expSP uint64, c compiler) {
	require.Equal(t, expSP, c.runtimeValueLocationStack().sp-callFrameDataSizeInUint64)
}

// TestCompileI32WrapFromI64 is the regression test for https://github.com/tetratelabs/wazero/issues/1008
func TestCompileI32WrapFromI64(t *testing.T) {
	c := newCompiler()
	c.Init(&wasm.FunctionType{}, nil, false)

	// Push the original i64 value.
	loc := c.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
	loc.valueType = runtimeValueTypeI64
	// Wrap it as the i32, and this should result in having runtimeValueTypeI32 on top of the stack.
	err := c.compileI32WrapFromI64()
	require.NoError(t, err)
	require.Equal(t, runtimeValueTypeI32, loc.valueType)
}

func operationPtr(operation wazeroir.UnionOperation) *wazeroir.UnionOperation {
	return &operation
}

func requireExecutable(original []byte) (executable []byte) {
	executable, err := platform.MmapCodeSegment(len(original))
	if err != nil {
		panic(err)
	}
	copy(executable, original)
	makeExecutable(executable)
	return executable
}

func makeExecutable(executable []byte) {
	if runtime.GOARCH == "arm64" {
		if err := platform.MprotectRX(executable); err != nil {
			panic(err)
		}
	}
}
