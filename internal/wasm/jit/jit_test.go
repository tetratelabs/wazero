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
	vm             *callEngine
	moduleInstance *wasm.ModuleInstance
}

func (j *jitEnv) stackTopAsByte() byte {
	return byte(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) stackTopAsUint16() uint16 {
	return uint16(j.stack()[j.stackPointer()-1])
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
	return j.vm.valueStack
}

func (j *jitEnv) jitStatus() jitCallStatusCode {
	return j.vm.exitContext.statusCode
}

func (j *jitEnv) functionCallAddress() wasm.FunctionAddress {
	return j.vm.exitContext.functionCallAddress
}

func (j *jitEnv) stackPointer() uint64 {
	return j.vm.valueStackContext.stackPointer
}

func (j *jitEnv) stackBasePointer() uint64 {
	return j.vm.valueStackContext.stackBasePointer
}

func (j *jitEnv) setStackPointer(sp uint64) {
	j.vm.valueStackContext.stackPointer = sp
}

func (j *jitEnv) addGlobals(g ...*wasm.GlobalInstance) {
	j.moduleInstance.Globals = append(j.moduleInstance.Globals, g...)
}

func (j *jitEnv) getGlobal(index uint32) uint64 {
	return j.moduleInstance.Globals[index].Val
}

func (j *jitEnv) setTable(table []wasm.TableElement) {
	j.moduleInstance.Tables[0] = &wasm.TableInstance{Table: table}
}

func (j *jitEnv) callFrameStackPeek() *callFrame {
	return &j.vm.callFrameStack[j.vm.globalContext.callFrameStackPointer-1]
}

func (j *jitEnv) callFrameStackPointer() uint64 {
	return j.vm.globalContext.callFrameStackPointer
}

func (j *jitEnv) setValueStackBasePointer(sp uint64) {
	j.vm.valueStackContext.stackBasePointer = sp
}

func (j *jitEnv) setCallFrameStackPointerLen(l uint64) {
	j.vm.callFrameStackLen = l
}

func (j *jitEnv) module() *wasm.ModuleInstance {
	return j.moduleInstance
}

func (j *jitEnv) engine() *engine {
	return j.eng
}

func (j *jitEnv) callEngine() *callEngine {
	return j.vm
}

func (j *jitEnv) exec(code []byte) {
	compiledFunction := &compiledFunction{
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		source: &wasm.FunctionInstance{
			FunctionKind: wasm.FunctionKindWasm,
			FunctionType: &wasm.TypeInstance{Type: &wasm.FunctionType{}},
		},
	}

	j.vm.pushCallFrame(compiledFunction)

	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(j.vm)),
	)
}

const defaultMemoryPageNumInTest = 2

func newJITEnvironment() *jitEnv {
	eng := newEngine()
	return &jitEnv{
		eng: eng,
		moduleInstance: &wasm.ModuleInstance{
			MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize*defaultMemoryPageNumInTest)},
			Tables:         []*wasm.TableInstance{{}},
			Globals:        []*wasm.GlobalInstance{},
		},
		vm: eng.newCallEngine(),
	}
}

func TestMain(m *testing.M) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		// JIT is currently implemented only for amd64 or arm64.
		os.Exit(0)
	}
	os.Exit(m.Run())
}
