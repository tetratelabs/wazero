package jit

import (
	"math"
	"os"
	"runtime"
	"testing"
	"unsafe"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

type jitEnv struct {
	eng            *engine
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
	return j.moduleInstance.MemoryInstance.Buffer
}

func (j *jitEnv) stack() []uint64 {
	return j.ce.valueStack
}

func (j *jitEnv) jitStatus() jitCallStatusCode {
	return j.ce.exitContext.statusCode
}

func (j *jitEnv) functionCallAddress() wasm.FunctionIndex {
	return j.ce.exitContext.functionCallAddress
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

func (j *jitEnv) setTable(table []wasm.TableElement) {
	j.moduleInstance.TableInstance = &wasm.TableInstance{Table: table}
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

func (j *jitEnv) engine() *engine {
	return j.eng
}

func (j *jitEnv) callEngine() *callEngine {
	return j.ce
}

func (j *jitEnv) exec(code []byte) {
	compiledFunction := &compiledFunction{
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		source: &wasm.FunctionInstance{
			Kind: wasm.FunctionKindWasm,
			Type: &wasm.FunctionType{},
		},
	}

	j.ce.pushCallFrame(compiledFunction)

	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(j.ce)),
	)
}

const defaultMemoryPageNumInTest = 2

func newJITEnvironment() *jitEnv {
	eng := newEngine()
	return &jitEnv{
		eng: eng,
		moduleInstance: &wasm.ModuleInstance{
			MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize*defaultMemoryPageNumInTest)},
			TableInstance:  &wasm.TableInstance{},
			Globals:        []*wasm.GlobalInstance{},
		},
		ce: eng.newCallEngine(),
	}
}

func TestMain(m *testing.M) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		// JIT is currently implemented only for amd64 or arm64.
		os.Exit(0)
	}
	os.Exit(m.Run())
}
