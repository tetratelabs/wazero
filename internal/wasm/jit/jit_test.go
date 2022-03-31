package jit

import (
	"math"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestMain(m *testing.M) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		// JIT is currently implemented only for amd64 or arm64.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type jitEnv struct {
	me             *moduleEngine
	ce             *callEngine
	moduleInstance *wasm.ModuleInstance
}

func (j *jitEnv) stackTopAsUint32() uint32 {
	return uint32(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) stackTopAsInt32() int32 {
	return int32(j.stack()[j.stackPointer()-1])
}
func (j *jitEnv) stackTopAsUint64() uint64 {
	return j.stack()[j.stackPointer()-1]
}

func (j *jitEnv) stackTopAsInt64() int64 {
	return int64(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) stackTopAsFloat32() float32 {
	return math.Float32frombits(uint32(j.stack()[j.stackPointer()-1]))
}

func (j *jitEnv) stackTopAsFloat64() float64 {
	return math.Float64frombits(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) memory() []byte {
	return j.moduleInstance.Memory.Buffer
}

func (j *jitEnv) stack() []uint64 {
	return j.ce.valueStack
}

func (j *jitEnv) jitStatus() jitCallStatusCode {
	return j.ce.exitContext.statusCode
}

func (j *jitEnv) builtinFunctionCallAddress() wasm.Index {
	return j.ce.exitContext.builtinFunctionCallIndex
}

func (j *jitEnv) stackPointer() uint64 {
	return j.ce.valueStackContext.stackPointer
}

func (j *jitEnv) stackBasePointer() uint64 {
	return j.ce.valueStackContext.stackBasePointer
}

func (j *jitEnv) setStackPointer(sp uint64) {
	j.ce.valueStackContext.stackPointer = sp
}

func (j *jitEnv) addGlobals(g ...*wasm.GlobalInstance) {
	j.moduleInstance.Globals = append(j.moduleInstance.Globals, g...)
}

func (j *jitEnv) getGlobal(index uint32) uint64 {
	return j.moduleInstance.Globals[index].Val
}

func (j *jitEnv) setTable(table []interface{}) {
	j.moduleInstance.Table = &wasm.TableInstance{Table: table}
}

func (j *jitEnv) callFrameStackPeek() *callFrame {
	return &j.ce.callFrameStack[j.ce.globalContext.callFrameStackPointer-1]
}

func (j *jitEnv) callFrameStackPointer() uint64 {
	return j.ce.globalContext.callFrameStackPointer
}

func (j *jitEnv) setValueStackBasePointer(sp uint64) {
	j.ce.valueStackContext.stackBasePointer = sp
}

func (j *jitEnv) setCallFrameStackPointerLen(l uint64) {
	j.ce.callFrameStackLen = l
}

func (j *jitEnv) module() *wasm.ModuleInstance {
	return j.moduleInstance
}

func (j *jitEnv) moduleEngine() *moduleEngine {
	return j.me
}

func (j *jitEnv) callEngine() *callEngine {
	return j.ce
}

func (j *jitEnv) exec(code []byte) {
	compiledFunction := &compiledFunction{
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		source: &wasm.FunctionInstance{
			Kind:   wasm.FunctionKindWasm,
			Type:   &wasm.FunctionType{},
			Module: j.moduleInstance,
		},
	}

	j.ce.pushCallFrame(compiledFunction)

	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(j.ce)),
	)
}

// newTestCompiler allows us to test a different architecture than the current one.
type newTestCompiler func(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error)

func (j *jitEnv) requireNewCompiler(t *testing.T, fn newTestCompiler, functype *wasm.FunctionType) compilerImpl {
	// golang-asm is not goroutine-safe so we take lock until we complete the compilation.
	// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
	assemblerMutex.Lock()

	requireSupportedOSArch(t)
	c, err := fn(
		&wasm.FunctionInstance{Module: j.moduleInstance, Kind: wasm.FunctionKindWasm, Type: functype},
		&wazeroir.CompilationResult{LabelCallers: map[string]uint32{}},
	)
	t.Cleanup(func() { assemblerMutex.Unlock() })
	require.NoError(t, err)

	ret, ok := c.(compilerImpl)
	require.True(t, ok)
	return ret
}

// CompilerImpl is the interface used for architecture-independent unit tests in this file.
// This is currently implemented by amd64 and arm64.
type compilerImpl interface {
	compiler
	compileExitFromNativeCode(jitCallStatusCode)
	compileMaybeGrowValueStack() error
	compileReturnFunction() error
	getOnStackPointerCeilDeterminedCallBack() func(uint64)
	setStackPointerCeil(uint64)
	compileReleaseRegisterToStack(loc *valueLocation)
	valueLocationStack() *valueLocationStack
	setValueLocationStack(*valueLocationStack)
	compileEnsureOnGeneralPurposeRegister(loc *valueLocation) error
	compileModuleContextInitialization() error
}

// compile implements compilerImpl.valueLocationStack for the amd64 architecture.
func (c *arm64Compiler) valueLocationStack() *valueLocationStack {
	return c.locationStack
}

// compile implements compilerImpl.getOnStackPointerCeilDeterminedCallBack for the amd64 architecture.
func (c *arm64Compiler) getOnStackPointerCeilDeterminedCallBack() func(uint64) {
	return c.onStackPointerCeilDeterminedCallBack
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *arm64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setValueLocationStack for the amd64 architecture.
func (c *arm64Compiler) setValueLocationStack(s *valueLocationStack) {
	c.locationStack = s
}

// compile implements compilerImpl.valueLocationStack for the amd64 architecture.
func (c *amd64Compiler) valueLocationStack() *valueLocationStack {
	return c.locationStack
}

// compile implements compilerImpl.getOnStackPointerCeilDeterminedCallBack for the amd64 architecture.
func (c *amd64Compiler) getOnStackPointerCeilDeterminedCallBack() func(uint64) {
	return c.onStackPointerCeilDeterminedCallBack
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *amd64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setValueLocationStack for the amd64 architecture.
func (c *amd64Compiler) setValueLocationStack(s *valueLocationStack) {
	c.locationStack = s
}

const defaultMemoryPageNumInTest = 1

func newJITEnvironment() *jitEnv {
	me := &moduleEngine{}
	return &jitEnv{
		me: me,
		moduleInstance: &wasm.ModuleInstance{
			Memory:  &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize*defaultMemoryPageNumInTest)},
			Table:   &wasm.TableInstance{},
			Globals: []*wasm.GlobalInstance{},
			Engine:  me,
		},
		ce: me.newCallEngine(),
	}
}
