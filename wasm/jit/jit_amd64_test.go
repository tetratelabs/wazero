//go:build amd64
// +build amd64

package jit

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"math/bits"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

// TODO: have some utility functions to reduce loc here: https://github.com/tetratelabs/wazero/issues/100

func newMemoryInst() *wasm.MemoryInstance {
	return &wasm.MemoryInstance{Buffer: make([]byte, 1024)}
}

func stackTopAsUint32(eng *engine) uint32 {
	return uint32(eng.stack[eng.stackPointer-1])
}

func stackTopAsInt32(eng *engine) int32 {
	return int32(eng.stack[eng.stackPointer-1])
}
func stackTopAsUint64(eng *engine) uint64 {
	return uint64(eng.stack[eng.stackPointer-1])
}

func stackTopAsInt64(eng *engine) int64 {
	return int64(eng.stack[eng.stackPointer-1])
}

func stackTopAsFloat32(eng *engine) float32 {
	return math.Float32frombits(uint32(eng.stack[eng.stackPointer-1]))
}

func stackTopAsFloat64(eng *engine) float64 {
	return math.Float64frombits(eng.stack[eng.stackPointer-1])
}

func requireNewCompiler(t *testing.T) *amd64Compiler {
	b, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	return &amd64Compiler{eng: nil, builder: b,
		locationStack:         newValueLocationStack(),
		onLabelStartCallbacks: map[string][]func(*obj.Prog){},
		labels:                map[string]*labelInfo{},
	}
}

func (c *amd64Compiler) movIntConstToRegister(val int64, targetRegister int16) *obj.Prog {
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = val
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	c.addInstruction(prog)
	return prog
}

func TestAmd64Compiler_pushFunctionInputs(t *testing.T) {
	f := &wasm.FunctionInstance{Signature: &wasm.FunctionType{
		Params: []wasm.ValueType{wasm.ValueTypeF64, wasm.ValueTypeI32},
	}}
	compiler := &amd64Compiler{locationStack: newValueLocationStack(), f: f}
	compiler.pushFunctionParams()
	require.Equal(t, uint64(len(f.Signature.Params)), compiler.locationStack.sp)
	loc := compiler.locationStack.pop()
	require.Equal(t, uint64(1), loc.stackPointer)
	loc = compiler.locationStack.pop()
	require.Equal(t, uint64(0), loc.stackPointer)
}

// Test engine.exec method on the resursive function calls.
func TestRecursiveFunctionCalls(t *testing.T) {
	eng := newEngine()
	const tmpReg = x86.REG_AX
	// Build a function that decrements top of stack,
	// and recursively call itself until the top value becomes zero,
	// and if the value becomes zero, add 5 onto the top and return.
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	// Setup the initial value.
	eng.stack[0] = 10 // We call recursively 10 times.
	loc := compiler.locationStack.pushValueOnStack()
	compiler.assignRegisterToValue(loc, tmpReg)
	require.Contains(t, compiler.locationStack.usedRegisters, loc.register)
	// Decrement tha value.
	prog := compiler.builder.NewProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = loc.register
	compiler.addInstruction(prog)
	// Check if the value equals zero
	prog = compiler.newProg()
	prog.As = x86.ACMPQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = loc.register
	prog.To.Type = obj.TYPE_CONST
	prog.To.Offset = 0
	compiler.addInstruction(prog)
	// If zero, jump to ::End
	jmp := compiler.newProg()
	jmp.As = x86.AJEQ
	jmp.To.Type = obj.TYPE_BRANCH
	compiler.addInstruction(jmp)
	// If not zero, we call push back the value to the stack
	// and call itself recursively.
	compiler.releaseRegisterToStack(loc)
	require.NotContains(t, compiler.locationStack.usedRegisters, loc.register)
	compiler.callFunctionFromConstIndex(0)
	// ::End
	// If zero, we return from this function after pushing 5.
	compiler.assignRegisterToValue(loc, tmpReg)
	prog = compiler.movIntConstToRegister(5, loc.register)
	jmp.To.SetTarget(prog) // the above mov instruction is the jump target of the JEQ.
	compiler.releaseRegisterToStack(loc)
	compiler.setJITStatus(jitCallStatusCodeReturned)
	compiler.returnFunction()
	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)
	// Setup engine.
	mem := newMemoryInst()
	compiledFunc := &compiledWasmFunction{codeSegment: code, memory: mem, paramCount: 1, resultCount: 1}
	compiledFunc.codeInitialAddress = uintptr(unsafe.Pointer(&compiledFunc.codeSegment[0]))
	eng.compiledWasmFunctions = []*compiledWasmFunction{compiledFunc}
	// Call into the function
	eng.exec(compiledFunc)
	require.Equal(t, []uint64{5, 0, 0, 0, 0}, eng.stack[:5])
	// And the callstack should be empty.
	require.Nil(t, eng.callFrameStack)

	// // Check the stability with busy Go runtime.
	var wg sync.WaitGroup
	const goroutines = 10000
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			if i/10 == 0 {
				// This is to kick the Go runtime to come in.
				fmt.Fprintf(io.Discard, "aaaaaaaaaaaa")
			}
			defer wg.Done()
			// Setup engine.
			mem := newMemoryInst()
			eng := newEngine()
			eng.stack[0] = 10 // We call recursively 10 times.
			compiledFunc := &compiledWasmFunction{codeSegment: code, memory: mem, paramCount: 1, resultCount: 1}
			compiledFunc.codeInitialAddress = uintptr(unsafe.Pointer(&compiledFunc.codeSegment[0]))
			eng.compiledWasmFunctions = []*compiledWasmFunction{compiledFunc}
			// Call into the function
			eng.exec(compiledFunc)
		}()
	}
}

// Test perform operations on
// pushing the const value into the Go-allocated slice
// under large amout of Goroutines.
func TestPushValueWithGoroutines(t *testing.T) {
	const (
		targetValue        uint64 = 100
		pushTargetRegister        = x86.REG_AX
		goroutines                = 10000
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			// Build native codes.
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			// Push consts to pushTargetRegister.
			_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
			loc := compiler.locationStack.pushValueOnRegister(pushTargetRegister)
			compiler.movIntConstToRegister(int64(targetValue), pushTargetRegister)
			// Push pushTargetRegister into the engine.stack[engine.sp].
			compiler.releaseRegisterToStack(loc)
			// Finally increment the stack pointer and write it back to the eng.sp
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)

			eng := newEngine()
			mem := newMemoryInst()

			f := &compiledWasmFunction{codeSegment: code, memory: mem}
			f.codeInitialAddress = uintptr(unsafe.Pointer(&f.codeSegment[0]))

			// Call into the function
			eng.exec(f)

			// Because we pushed the value, eng.sp must be incremented by 1
			if eng.stackPointer != 2 {
				panic("eng.sp must be incremented.")
			}

			// Also we push the const value to the top of slice!
			if eng.stack[1] != 100 {
				panic("eng.stack[0] must be changed to the const!")
			}
		}()
	}
	wg.Wait()
}

func Test_setJITStatus(t *testing.T) {
	for _, s := range []jitCallStatusCode{
		jitCallStatusCodeReturned,
		jitCallStatusCodeCallWasmFunction,
		jitCallStatusCodeCallBuiltInFunction,
		jitCallStatusCodeCallHostFunction,
		jitCallStatusCodeUnreachable,
	} {
		t.Run(s.String(), func(t *testing.T) {

			// Build codes.
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			compiler.setJITStatus(s)
			compiler.returnFunction()
			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run codes
			eng := newEngine()
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check status.
			require.Equal(t, s, eng.jitCallStatusCode)
		})
	}
}

func Test_setFunctionCallIndexFromConst(t *testing.T) {
	// Build codes.
	for _, index := range []int64{1, 5, 20} {
		// Build codes.
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		compiler.setFunctionCallIndexFromConst(index)
		compiler.returnFunction()
		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run codes
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check index.
		require.Equal(t, index, eng.functionCallIndex)
	}
}

func Test_setFunctionCallIndexFromRegister(t *testing.T) {
	reg := int16(x86.REG_R10)
	for _, index := range []int64{1, 5, 20} {
		// Build codes.
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		compiler.movIntConstToRegister(index, reg)
		compiler.setFunctionCallIndexFromRegister(reg)
		compiler.returnFunction()
		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run codes
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check index.
		require.Equal(t, index, eng.functionCallIndex)
	}
}

func Test_setContinuationAtNextInstruction(t *testing.T) {
	const tmpReg = x86.REG_AX
	// Build codes.
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	compiler.setContinuationOffsetAtNextInstructionAndReturn()
	exp := uintptr(len(compiler.builder.Assemble()))
	// On the continuation, we have to setup the registers again.
	compiler.initializeReservedRegisters()
	// The continuation after function calls.
	_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
	loc := compiler.locationStack.pushValueOnRegister(tmpReg)
	compiler.movIntConstToRegister(int64(50), tmpReg)
	compiler.releaseRegisterToStack(loc)
	require.NotContains(t, compiler.locationStack.usedRegisters, tmpReg)
	compiler.setJITStatus(jitCallStatusCodeCallWasmFunction)
	compiler.returnFunction()
	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run codes
	eng := newEngine()
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check offset.
	require.Equal(t, exp, eng.continuationAddressOffset)

	// Run code again on the continuation
	jitcall(
		uintptr(unsafe.Pointer(&code[0]))+eng.continuationAddressOffset,
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	require.Equal(t, jitCallStatusCodeCallWasmFunction, eng.jitCallStatusCode)
	require.Equal(t, uint64(50), eng.stack[1])
}

func Test_callFunction(t *testing.T) {
	const (
		functionIndex int64 = 10
		tmpReg              = x86.REG_AX
	)
	t.Run("from const", func(t *testing.T) {
		// Build codes.
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		compiler.callFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := compiler.locationStack.pushValueOnRegister(tmpReg)
		compiler.movIntConstToRegister(int64(50), tmpReg)
		compiler.releaseRegisterToStack(loc)
		require.NotContains(t, compiler.locationStack.usedRegisters, tmpReg)
		compiler.returnFunction()
		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		mem := newMemoryInst()

		// The first call.
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		require.Equal(t, jitCallStatusCodeCallWasmFunction, eng.jitCallStatusCode)
		require.Equal(t, functionIndex, eng.functionCallIndex)

		// Continue.
		jitcall(
			uintptr(unsafe.Pointer(&code[0]))+eng.continuationAddressOffset,
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		require.Equal(t, uint64(50), eng.stack[1])
	})
	t.Run("from reg", func(t *testing.T) {
		const functionIndex int64 = 10
		// Build codes.
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		compiler.movIntConstToRegister(functionIndex, tmpReg)
		compiler.callFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := compiler.locationStack.pushValueOnRegister(tmpReg)
		compiler.movIntConstToRegister(int64(50), tmpReg)
		compiler.releaseRegisterToStack(loc)
		require.NotContains(t, compiler.locationStack.usedRegisters, tmpReg)
		compiler.returnFunction()
		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		mem := newMemoryInst()

		// The first call.
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		require.Equal(t, jitCallStatusCodeCallWasmFunction, eng.jitCallStatusCode)
		require.Equal(t, functionIndex, eng.functionCallIndex)

		// Continue.
		jitcall(
			uintptr(unsafe.Pointer(&code[0]))+eng.continuationAddressOffset,
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		require.Equal(t, uint64(50), eng.stack[1])
	})
}

func TestEngine_exec_callHostFunction(t *testing.T) {
	t.Run("from const", func(t *testing.T) {
		const (
			functionIndex int64 = 0
			tmpReg              = x86.REG_AX
		)
		// Build codes.
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		// Push the value onto stack.
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := compiler.locationStack.pushValueOnRegister(tmpReg)
		compiler.movIntConstToRegister(int64(50), tmpReg)
		compiler.releaseRegisterToStack(loc)
		require.NotContains(t, compiler.locationStack.usedRegisters, tmpReg)
		compiler.callHostFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		compiler.setJITStatus(jitCallStatusCodeReturned)
		compiler.returnFunction()
		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		hostFunction := &compiledHostFunction{
			f: func(ctx *wasm.HostFunctionCallContext) {
				eng.stack[eng.stackPointer-1] *= 100
			},
		}
		eng.compiledHostFunctions = append(eng.compiledHostFunctions, hostFunction)
		mem := newMemoryInst()

		// Call into the function
		f := &compiledWasmFunction{codeSegment: code, memory: mem}
		f.codeInitialAddress = uintptr(unsafe.Pointer(&f.codeSegment[0]))
		eng.exec(f)
		require.Equal(t, uint64(50)*100, eng.stack[1])
	})

	t.Run("from register", func(t *testing.T) {
		const (
			functionIndex uint32 = 1
			tmpReg               = x86.REG_AX
		)
		// Build codes.
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		// Push the value onto stack.
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := compiler.locationStack.pushValueOnRegister(x86.REG_AX)
		compiler.movIntConstToRegister(int64(50), tmpReg)
		compiler.releaseRegisterToStack(loc)
		require.NotContains(t, compiler.locationStack.usedRegisters, tmpReg)
		// Set the function index
		compiler.movIntConstToRegister(int64(1), tmpReg)
		compiler.callHostFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		compiler.setJITStatus(jitCallStatusCodeReturned)
		compiler.returnFunction()
		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		hostFunc := reflect.ValueOf(func(ctx *wasm.HostFunctionCallContext, _, in uint64) uint64 {
			return in * 200
		})
		hostFunctionInstance := &wasm.FunctionInstance{
			HostFunction: &hostFunc,
			Signature: &wasm.FunctionType{
				Params:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI64},
				Results: []wasm.ValueType{wasm.ValueTypeI64},
			},
		}
		eng.compiledHostFunctionIndex[hostFunctionInstance] = 1
		eng.compiledHostFunctions = make([]*compiledHostFunction, 2)
		err = eng.Compile(hostFunctionInstance)
		require.NoError(t, err)
		mem := newMemoryInst()

		// Call into the function
		f := &compiledWasmFunction{codeSegment: code, memory: mem}
		f.codeInitialAddress = uintptr(unsafe.Pointer(&f.codeSegment[0]))
		eng.exec(f)
		require.Equal(t, uint64(50)*200, eng.stack[0])
	})
}

func Test_popFromStackToRegister(t *testing.T) {
	const (
		targetRegister1 = x86.REG_AX
		targetRegister2 = x86.REG_R9
	)
	// Build function.
	// Pop the value from the top twice, add two values,
	// and push it back to the top.
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	// Create three values on the stack.
	compiler.locationStack.pushValueOnStack()
	loc1 := compiler.locationStack.pushValueOnRegister(targetRegister1)
	loc2 := compiler.locationStack.pushValueOnRegister(targetRegister2)
	compiler.assignRegisterToValue(loc1, targetRegister1)
	compiler.assignRegisterToValue(loc2, targetRegister2)
	// Increment the popped value on the register.
	prog := compiler.newProg()
	prog.As = x86.AADDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister1
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = targetRegister2
	// Now we used the two values so pop twice.
	compiler.locationStack.pop()
	compiler.locationStack.pop()
	// Ready to push the result location.
	result := compiler.locationStack.pushValueOnRegister(targetRegister1)
	compiler.addInstruction(prog)
	// Push it back to the stack.
	compiler.releaseRegisterToStack(result)
	compiler.returnFunction()
	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Call in.
	eng := newEngine()
	eng.stackBasePointer = 1
	eng.stack[eng.stackBasePointer+2] = 10000
	eng.stack[eng.stackBasePointer+1] = 20000
	mem := newMemoryInst()
	require.Equal(t, []uint64{0, 20000, 10000}, eng.stack[eng.stackBasePointer:eng.stackBasePointer+3])
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the sp and value.
	require.Equal(t, uint64(2), eng.stackPointer)
	require.Equal(t, []uint64{0, 30000}, eng.stack[eng.stackBasePointer:eng.stackBasePointer+eng.stackPointer])
}

func TestAmd64Compiler_initializeReservedRegisters(t *testing.T) {
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	compiler.returnFunction()

	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	eng := newEngine()
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
}

func TestAmd64Compiler_allocateRegister(t *testing.T) {
	t.Run("free", func(t *testing.T) {
		compiler := requireNewCompiler(t)
		reg, err := compiler.allocateRegister(generalPurposeRegisterTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		reg, err = compiler.allocateRegister(generalPurposeRegisterTypeFloat)
		require.NoError(t, err)
		require.True(t, isFloatRegister(reg))
	})
	t.Run("steal", func(t *testing.T) {
		const stealTarget = x86.REG_AX
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		// Use up all the Int regs.
		for _, r := range unreservedGeneralPurposeIntRegisters {
			compiler.locationStack.markRegisterUsed(r)
		}
		stealTargetLocation := compiler.locationStack.pushValueOnRegister(stealTarget)
		compiler.movIntConstToRegister(int64(50), stealTargetLocation.register)
		require.Equal(t, int16(stealTarget), stealTargetLocation.register)
		require.True(t, stealTargetLocation.onRegister())
		reg, err := compiler.allocateRegister(generalPurposeRegisterTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		require.False(t, stealTargetLocation.onRegister())

		// Create new value using the stolen register.
		loc := compiler.locationStack.pushValueOnRegister(reg)
		compiler.movIntConstToRegister(int64(2000), loc.register)
		compiler.releaseRegisterToStack(loc)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Run code.
		eng := newEngine()
		eng.stackBasePointer = 10
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)

		// Check the sp and value.
		require.Equal(t, uint64(2), eng.stackPointer)
		require.Equal(t, []uint64{50, 2000}, eng.stack[eng.stackBasePointer:eng.stackBasePointer+eng.stackPointer])
	})
}

func TestAmd64Compiler_compileLabel(t *testing.T) {
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	label := &wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}
	labelKey := label.String()
	compiler.labels[labelKey] = &labelInfo{}

	var called bool
	compiler.onLabelStartCallbacks[labelKey] = append(compiler.onLabelStartCallbacks[labelKey],
		func(p *obj.Prog) { called = true },
	)
	err := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
	require.NoError(t, err)
	require.Len(t, compiler.onLabelStartCallbacks, 0)
	require.NotNil(t, compiler.labels[labelKey].initialInstruction)
	require.True(t, called)

	// Generate the code under test.
	compiler.returnFunction()
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	eng := newEngine()
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
}

func TestAmd64Compiler_compilePick(t *testing.T) {
	o := &wazeroir.OperationPick{Depth: 1}
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	// The case when the original value is already in register.
	t.Run("on reg", func(t *testing.T) {
		// Set up the pick target original value.
		pickTargetLocation := compiler.locationStack.pushValueOnRegister(int16(x86.REG_R10))
		pickTargetLocation.setRegisterType(generalPurposeRegisterTypeInt)
		compiler.locationStack.pushValueOnStack() // Dummy value!
		compiler.movIntConstToRegister(100, pickTargetLocation.register)
		// Now insert pick code.
		err := compiler.compilePick(o)
		require.NoError(t, err)
		// Increment the picked value.
		pickedLocation := compiler.locationStack.peek()
		require.True(t, pickedLocation.onRegister())
		require.NotEqual(t, pickedLocation.register, pickTargetLocation.register)
		prog := compiler.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = pickedLocation.register
		compiler.addInstruction(prog)
		// To verify the behavior, we push the incremented picked value
		// to the stack.
		compiler.releaseRegisterToStack(pickedLocation)
		// Also write the original location back to the stack.
		compiler.releaseRegisterToStack(pickTargetLocation)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run code.
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(3), eng.stackPointer)
		require.Equal(t, uint64(101), eng.stack[eng.stackPointer-1])
		require.Equal(t, uint64(100), eng.stack[eng.stackPointer-3])
	})
	// The case when the original value is in stack.
	t.Run("on stack", func(t *testing.T) {
		eng := newEngine()

		// Setup the original value.
		compiler.locationStack.pushValueOnStack() // Dummy value!
		pickTargetLocation := compiler.locationStack.pushValueOnStack()
		compiler.locationStack.pushValueOnStack() // Dummy value!
		eng.stackPointer = 5
		eng.stackBasePointer = 1
		eng.stack[eng.stackBasePointer+pickTargetLocation.stackPointer] = 100

		// Now insert pick code.
		err := compiler.compilePick(o)
		require.NoError(t, err)

		// Increment the picked value.
		pickedLocation := compiler.locationStack.peek()
		prog := compiler.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = pickedLocation.register
		compiler.addInstruction(prog)

		// To verify the behavior, we push the incremented picked value
		// to the stack.
		compiler.releaseRegisterToStack(pickedLocation)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run code.
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(100), eng.stack[eng.stackBasePointer+pickTargetLocation.stackPointer]) // Original value shouldn't be affected.
		require.Equal(t, uint64(3), eng.stackPointer)
		require.Equal(t, uint64(101), eng.stack[eng.stackBasePointer+eng.stackPointer-1])
	})
}

func TestAmd64Compiler_compileConstI32(t *testing.T) {
	for _, v := range []uint32{1, 1 << 5, 1 << 31} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstI32{Value: v}
			err := compiler.compileConstI32(o)
			require.NoError(t, err)

			// To verify the behavior, we increment and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			// Check the value of the top on the stack equals the const plus one.
			require.Equal(t, uint64(o.Value)+1, eng.stack[eng.stackPointer-1])
		})
	}
}

func TestAmd64Compiler_compileConstI64(t *testing.T) {
	for _, v := range []uint64{1, 1 << 5, 1 << 35, 1 << 63} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstI64{Value: v}
			err := compiler.compileConstI64(o)
			require.NoError(t, err)

			// To verify the behavior, we increment and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			// Check the value of the top on the stack equals the const plus one.
			require.Equal(t, o.Value+1, eng.stack[eng.stackPointer-1])
		})
	}
}

func TestAmd64Compiler_compileConstF32(t *testing.T) {
	for _, v := range []float32{1, -3.23, 100.123} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstF32{Value: v}
			err := compiler.compileConstF32(o)
			require.NoError(t, err)

			// To verify the behavior, we double and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeFloat, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AADDSS
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			// Check the value of the top on the stack equals the squared const.
			require.Equal(t, o.Value*2, stackTopAsFloat32(eng))
		})
	}
}

func TestAmd64Compiler_compileConstF64(t *testing.T) {
	for _, v := range []float64{1, -3.23, 100.123} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstF64{Value: v}
			err := compiler.compileConstF64(o)
			require.NoError(t, err)

			// To verify the behavior, we double and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeFloat, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AADDSD
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			// Check the value of the top on the stack equals the squared const.
			require.Equal(t, o.Value*2, stackTopAsFloat64(eng))
		})
	}
}

func TestAmd64Compiler_compileAdd(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		const x1Value uint32 = 113
		const x2Value uint32 = 41
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run code.
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(x1Value+x2Value), eng.stack[eng.stackPointer-1])
	})
	t.Run("int64", func(t *testing.T) {
		const x1Value uint64 = 1 << 35
		const x2Value uint64 = 41
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run code.
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(x1Value+x2Value), eng.stack[eng.stackPointer-1])
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float32
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: float32(math.Inf(1)), v2: -2.1},
			{v1: float32(math.Inf(1)), v2: 2.1},
			{v1: float32(math.Inf(-1)), v2: -2.1},
			{v1: float32(math.Inf(-1)), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.v1+tc.v2, stackTopAsFloat32(eng))
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float64
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: math.Inf(1), v2: -2.1},
			{v1: math.Inf(1), v2: 2.1},
			{v1: math.Inf(-1), v2: -2.1},
			{v1: math.Inf(-1), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.v1+tc.v2, stackTopAsFloat64(eng))
			})
		}
	})
}

func TestAmd64Compiler_emitEqOrNe(t *testing.T) {
	for _, instruction := range []struct {
		name string
		isEq bool
	}{
		{name: "eq", isEq: true},
		{name: "ne", isEq: false},
	} {
		instruction := instruction
		t.Run(instruction.name, func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 uint32
				}{
					{x1: 100, x2: math.MaxUint32},
					{x1: math.MaxUint32, x2: math.MaxUint32},
					{x1: math.MaxUint32, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100, x2: 100},
					{x1: 200, x2: 100},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the cmp target values.
						err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
						}
						require.NoError(t, err)
						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, eng.stack[eng.stackPointer-1] == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, eng.stack[eng.stackPointer-1] == 1)
						}
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 uint64
				}{
					{x1: 1, x2: math.MaxUint64},
					{x1: 100, x2: 200},
					{x1: 200, x2: 100},
					{x1: 1 << 56, x2: 100},
					{x1: 1 << 56, x2: 1 << 61},
					{x1: math.MaxUint64, x2: 100},
					{x1: 0, x2: 100},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the cmp target values.
						err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI64})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI64})
						}
						require.NoError(t, err)
						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, eng.stack[eng.stackPointer-1] == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, eng.stack[eng.stackPointer-1] == 1)
						}
					})
				}
			})
			t.Run("float32", func(t *testing.T) { // AAA
				for _, tc := range []struct {
					x1, x2 float32
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: float32(math.NaN()), x2: 1.231},
					{x1: float32(math.NaN()), x2: -1.231},
					{x1: 1.231, x2: float32(math.NaN())},
					{x1: -1.231, x2: float32(math.NaN())},
					{x1: float32(math.Inf(1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(1)), x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(-1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(-1))},
					{x1: float32(math.Inf(-1)), x2: float32(math.Inf(-1))},
				} {
					t.Run(fmt.Sprintf("x1=%f,x2=%f", tc.x1, tc.x2), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the cmp target values.
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF32})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF32})
						}
						require.NoError(t, err)
						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we release the flag value
						// to the stack.
						compiler.releaseAllRegistersToStack()
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						fmt.Println(hex.EncodeToString(code))
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, eng.stack[eng.stackPointer-1] == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, eng.stack[eng.stackPointer-1] == 1)
						}
					})
				}
			})
			t.Run("float64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 float64
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
					{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
					{x1: math.Inf(1), x2: 100},
					{x1: math.Inf(-1), x2: 100},
					{x1: math.NaN(), x2: 1.231},
					{x1: math.NaN(), x2: -1.231},
					{x1: 1.231, x2: math.NaN()},
					{x1: -1.231, x2: math.NaN()},
					{x1: math.Inf(1), x2: 100},
					{x1: 100, x2: math.Inf(1)},
					{x1: math.Inf(1), x2: math.Inf(1)},
					{x1: math.Inf(-1), x2: 100},
					{x1: 100, x2: math.Inf(-1)},
					{x1: math.Inf(-1), x2: math.Inf(-1)},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the cmp target values.
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF64})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF64})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						compiler.releaseAllRegistersToStack()
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, eng.stack[eng.stackPointer-1] == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, eng.stack[eng.stackPointer-1] == 1)
						}
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_compileEqz(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for i, v := range []uint32{
			0, 1 << 16, math.MaxUint32,
		} {
			v := v
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()

				// Push the cmp target value.
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: v})
				require.NoError(t, err)
				loc := compiler.locationStack.peek()

				// Emit the eqz instructions.
				err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// At this point, the target value must be consumed
				// so the corresponding register must be marked unused.
				require.NotContains(t, compiler.locationStack.usedRegisters, loc.register)

				// To verify the behavior, we push the flag value
				// to the stack.
				top := compiler.locationStack.peek()
				require.True(t, top.onConditionalRegister() && !top.onRegister())
				err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
				require.NoError(t, err)
				require.True(t, !top.onConditionalRegister() && top.onRegister())
				compiler.releaseRegisterToStack(top)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, v == uint32(0), eng.stack[eng.stackPointer-1] == 1)
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for i, v := range []uint64{
			0, 1 << 16, 1 << 36, math.MaxUint64,
		} {
			v := v
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()

				// Push the cmp target values.
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
				require.NoError(t, err)
				loc := compiler.locationStack.peek()

				// Emit the eqz instructions.
				err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// At this point, the target value must be consumed
				// so the corresponding register must be marked unused.
				require.NotContains(t, compiler.locationStack.usedRegisters, loc.register)

				// To verify the behavior, we push the flag value
				// to the stack.
				top := compiler.locationStack.peek()
				require.True(t, top.onConditionalRegister() && !top.onRegister())
				err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
				require.NoError(t, err)
				require.True(t, !top.onConditionalRegister() && top.onRegister())
				compiler.releaseRegisterToStack(top)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, v == uint64(0), eng.stack[eng.stackPointer-1] == 1)
			})
		}
	})
}

func TestAmd64Compiler_compileLe_or_Lt(t *testing.T) {
	for _, instruction := range []struct {
		name      string
		inclusive bool
	}{
		{name: "less_than_or_equal", inclusive: true},
		{name: "less_than", inclusive: false},
	} {
		instruction := instruction
		t.Run(instruction.name, func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int32
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint32
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint32
					{x1: 100, x2: 200, signed: true},
					{x1: 100, x2: 100, signed: true},
					{x1: 200, x2: 100, signed: true},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the target values.
						err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt32
						} else {
							targetType = wazeroir.SignedTypeUint32
						}
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: targetType})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.returnFunction()

						// Generate the code under test.
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)

						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						var exp bool
						if tc.signed {
							exp = tc.x1 < tc.x2
						} else {
							exp = uint32(tc.x1) < uint32(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int64
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint64
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint64
					{x1: 100, x2: 200, signed: true},
					{x1: 200, x2: 100, signed: true},
					{x1: 1 << 56, x2: 100, signed: true},
					{x1: 1 << 56, x2: 1 << 61, signed: true},
					{x1: math.MaxInt64, x2: 100, signed: true},
					{x1: math.MinInt64, x2: 100, signed: true},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the target values.
						err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt64
						} else {
							targetType = wazeroir.SignedTypeUint64
						}
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: targetType})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.returnFunction()

						// Generate the code under test.
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)

						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						var exp bool
						if tc.signed {
							exp = tc.x1 < tc.x2
						} else {
							exp = uint64(tc.x1) < uint64(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
			t.Run("float32", func(t *testing.T) {
				for _, tc := range []struct {
					x1, x2 float32
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: float32(math.NaN()), x2: 1.231},
					{x1: float32(math.NaN()), x2: -1.231},
					{x1: float32(math.NaN()), x2: 0},
					{x1: 0, x2: float32(math.NaN())},
					{x1: 1.231, x2: float32(math.NaN())},
					{x1: -1.231, x2: float32(math.NaN())},
					{x1: float32(math.Inf(1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(1)), x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(-1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(-1))},
					{x1: float32(math.Inf(-1)), x2: float32(math.Inf(-1))},
				} {
					t.Run(fmt.Sprintf("x1=%f,x2=%f", tc.x1, tc.x2), func(t *testing.T) {
						// Prepare operands.
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat32})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeFloat32})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.returnFunction()

						// Generate the code under test.
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						exp := tc.x1 < tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
			t.Run("float64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 float64
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
					{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
					{x1: math.Inf(1), x2: 100},
					{x1: math.Inf(-1), x2: 100},
					{x1: math.NaN(), x2: 1.231},
					{x1: math.NaN(), x2: -1.231},
					{x1: 1.231, x2: math.NaN()},
					{x1: -1.231, x2: math.NaN()},
					{x1: math.Inf(1), x2: 100},
					{x1: 100, x2: math.Inf(1)},
					{x1: math.Inf(1), x2: math.Inf(1)},
					{x1: math.Inf(-1), x2: 100},
					{x1: 100, x2: math.Inf(-1)},
					{x1: math.Inf(-1), x2: math.Inf(-1)},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						// Prepare operands.
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions,
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat64})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeFloat64})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.returnFunction()

						// Generate the code under test.
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						exp := tc.x1 < tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_compileGe_or_Gt(t *testing.T) {
	for _, instruction := range []struct {
		name      string
		inclusive bool
	}{
		{name: "greater_than_or_equal", inclusive: true},
		{name: "greater_than", inclusive: false},
	} {
		instruction := instruction
		t.Run(instruction.name, func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int32
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint32
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint32
					{x1: 100, x2: 200, signed: true},
					{x1: 100, x2: 100, signed: true},
					{x1: 200, x2: 100, signed: true},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the target values.
						err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt32
						} else {
							targetType = wazeroir.SignedTypeUint32
						}
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: targetType})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						var exp bool
						if tc.signed {
							exp = tc.x1 > tc.x2
						} else {
							exp = uint32(tc.x1) > uint32(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int64
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint64
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint64
					{x1: 100, x2: 200, signed: true},
					{x1: 200, x2: 100, signed: true},
					{x1: 1 << 56, x2: 100, signed: true},
					{x1: 1 << 56, x2: 1 << 61, signed: true},
					{x1: math.MaxInt64, x2: 100, signed: true},
					{x1: math.MinInt64, x2: 100, signed: true},
				} {

					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()

						// Push the target values.
						err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt64
						} else {
							targetType = wazeroir.SignedTypeUint64
						}
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: targetType})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)
						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						var exp bool
						if tc.signed {
							exp = tc.x1 > tc.x2
						} else {
							exp = uint64(tc.x1) > uint64(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
			t.Run("float32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 float32
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: float32(math.NaN()), x2: 0},
					{x1: float32(math.NaN()), x2: 1.231},
					{x1: float32(math.NaN()), x2: -1.231},
					{x1: 0, x2: float32(math.NaN())},
					{x1: 1.231, x2: float32(math.NaN())},
					{x1: -1.231, x2: float32(math.NaN())},
					{x1: float32(math.Inf(1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(1)), x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(-1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(-1))},
					{x1: float32(math.Inf(-1)), x2: float32(math.Inf(-1))},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						// Prepare operands.
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions,
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeFloat32})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeFloat32})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)

						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						exp := tc.x1 > tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
			t.Run("float64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 float64
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
					{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
					{x1: math.Inf(1), x2: 100},
					{x1: math.Inf(-1), x2: 100},
					{x1: math.NaN(), x2: 1.231},
					{x1: math.NaN(), x2: -1.231},
					{x1: 1.231, x2: math.NaN()},
					{x1: -1.231, x2: math.NaN()},
					{x1: math.Inf(1), x2: 100},
					{x1: 100, x2: math.Inf(1)},
					{x1: math.Inf(1), x2: math.Inf(1)},
					{x1: math.Inf(-1), x2: 100},
					{x1: 100, x2: math.Inf(-1)},
					{x1: math.Inf(-1), x2: math.Inf(-1)},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						// Prepare operands.
						compiler := requireNewCompiler(t)
						compiler.initializeReservedRegisters()
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions,
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeFloat64})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeFloat64})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.returnFunction()

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, err := compiler.generate()
						require.NoError(t, err)
						// Run code.
						eng := newEngine()
						mem := newMemoryInst()
						jitcall(
							uintptr(unsafe.Pointer(&code[0])),
							uintptr(unsafe.Pointer(eng)),
							uintptr(unsafe.Pointer(&mem.Buffer[0])),
						)

						// Check the stack.
						require.Equal(t, uint64(1), eng.stackPointer)
						exp := tc.x1 > tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, eng.stack[eng.stackPointer-1] == 1)
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_compileSub(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		const x1Value uint32 = 1 << 31
		const x2Value uint32 = 51
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeI32})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run code.
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(x1Value-x2Value), eng.stack[eng.stackPointer-1])
	})
	t.Run("int64", func(t *testing.T) {
		const x1Value uint64 = 1 << 35
		const x2Value uint64 = 51
		compiler := requireNewCompiler(t)
		compiler.initializeReservedRegisters()
		err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeI64})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)
		// Run code.
		eng := newEngine()
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, x1Value-x2Value, eng.stack[eng.stackPointer-1])
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float32
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: float32(math.Inf(1)), v2: -2.1},
			{v1: float32(math.Inf(1)), v2: 2.1},
			{v1: float32(math.Inf(-1)), v2: -2.1},
			{v1: float32(math.Inf(-1)), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.v1-tc.v2, stackTopAsFloat32(eng))
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float64
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: math.Inf(1), v2: -2.1},
			{v1: math.Inf(1), v2: 2.1},
			{v1: math.Inf(-1), v2: -2.1},
			{v1: math.Inf(-1), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.v1-tc.v2, stackTopAsFloat64(eng))
			})
		}
	})
}

func TestAmd64Compiler_compileMul(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			// Interpret -1 as stack.
			x1Reg, x2Reg int16
		}{
			{
				name:  "x1:ax,x2:random_reg",
				x1Reg: x86.REG_AX,
				x2Reg: x86.REG_R10,
			},
			{
				name:  "x1:ax,x2:stack",
				x1Reg: x86.REG_AX,
				x2Reg: -1,
			},
			{
				name:  "x1:random_reg,x2:ax",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:staack,x2:ax",
				x1Reg: -1,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:random_reg,x2:random_reg",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:stack,x2:random_reg",
				x1Reg: -1,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:random_reg,x2:stack",
				x1Reg: x86.REG_R9,
				x2Reg: -1,
			},
			{
				name:  "x1:stack,x2:stack",
				x1Reg: -1,
				x2Reg: -1,
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				const x1Value uint32 = 1 << 11
				const x2Value uint32 = 51
				const dxValue uint64 = 111111

				eng := newEngine()
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()

				// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
				// Here, we put it just before two operands as ["any value used by DX", x1, x2]
				// but in reality, it can exist in any position of stack.
				compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
				prevOnDX := compiler.locationStack.pushValueOnRegister(x86.REG_DX)

				// Setup values.
				if tc.x1Reg != -1 {
					compiler.movIntConstToRegister(int64(x1Value), tc.x1Reg)
					compiler.locationStack.pushValueOnRegister(tc.x1Reg)
				} else {
					loc := compiler.locationStack.pushValueOnStack()
					eng.stack[loc.stackPointer] = uint64(x1Value)
				}
				if tc.x2Reg != -1 {
					compiler.movIntConstToRegister(int64(x2Value), tc.x2Reg)
					compiler.locationStack.pushValueOnRegister(tc.x2Reg)
				} else {
					loc := compiler.locationStack.pushValueOnStack()
					eng.stack[loc.stackPointer] = uint64(x2Value)
				}

				err := compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
				require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
				require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
				require.Equal(t, uint64(2), compiler.locationStack.sp)
				require.Len(t, compiler.locationStack.usedRegisters, 1)
				// At this point, the previous value on the DX register is saved to the stack.
				require.True(t, prevOnDX.onStack())

				// We add the value previously on the DX with the multiplication result
				// in order to ensure that not saving existing DX value would cause
				// the failure in a subsequent instruction.
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					0,
				)

				// Verify the stack is in the form of ["any value previously used by DX" + x1 * x2]
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, uint64(x1Value*x2Value)+dxValue, eng.stack[eng.stackPointer-1])
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			// Interpret -1 as stack.
			x1Reg, x2Reg int16
		}{
			{
				name:  "x1:ax,x2:random_reg",
				x1Reg: x86.REG_AX,
				x2Reg: x86.REG_R10,
			},
			{
				name:  "x1:ax,x2:stack",
				x1Reg: x86.REG_AX,
				x2Reg: -1,
			},
			{
				name:  "x1:random_reg,x2:ax",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:stack,x2:ax",
				x1Reg: -1,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:random_reg,x2:random_reg",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:stack,x2:random_reg",
				x1Reg: -1,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:random_reg,x2:stack",
				x1Reg: x86.REG_R9,
				x2Reg: -1,
			},
			{
				name:  "x1:stack,x2:stack",
				x1Reg: -1,
				x2Reg: -1,
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				const x1Value uint64 = 1 << 35
				const x2Value uint64 = 51
				const dxValue uint64 = 111111

				eng := newEngine()
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()

				// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
				// Here, we put it just before two operands as ["any value used by DX", x1, x2]
				// but in reality, it can exist in any position of stack.
				compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
				prevOnDX := compiler.locationStack.pushValueOnRegister(x86.REG_DX)

				// Setup values.
				if tc.x1Reg != -1 {
					compiler.movIntConstToRegister(int64(x1Value), tc.x1Reg)
					compiler.locationStack.pushValueOnRegister(tc.x1Reg)
				} else {
					loc := compiler.locationStack.pushValueOnStack()
					eng.stack[loc.stackPointer] = uint64(x1Value)
				}
				if tc.x2Reg != -1 {
					compiler.movIntConstToRegister(int64(x2Value), tc.x2Reg)
					compiler.locationStack.pushValueOnRegister(tc.x2Reg)
				} else {
					loc := compiler.locationStack.pushValueOnStack()
					eng.stack[loc.stackPointer] = uint64(x2Value)
				}

				err := compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
				require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
				require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
				require.Equal(t, uint64(2), compiler.locationStack.sp)
				require.Len(t, compiler.locationStack.usedRegisters, 1)
				// At this point, the previous value on the DX register is saved to the stack.
				require.True(t, prevOnDX.onStack())

				// We add the value previously on the DX with the multiplication result
				// in order to ensure that not saving existing DX value would cause
				// the failure in a subsequent instruction.
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					0,
				)

				// Verify the stack is in the form of ["any value previously used by DX" + x1 * x2]
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, uint64(x1Value*x2Value)+dxValue, eng.stack[eng.stackPointer-1])
			})
		}
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float32
		}{
			{x1: 100, x2: -1.1},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: float32(math.Inf(1)), x2: 100},
			{x1: float32(math.Inf(-1)), x2: 100},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.x1*tc.x2, stackTopAsFloat32(eng))
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float64
		}{
			{x1: 100, x2: -1.1},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
			{x1: math.Inf(1), x2: 100},
			{x1: math.Inf(-1), x2: 100},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)
				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.x1*tc.x2, stackTopAsFloat64(eng))
			})
		}
	})
}

func TestAmd64Compiler_compilClz(t *testing.T) {
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedLeadingZeros uint32 }{
			{input: 0xff_ff_ff_ff, expectedLeadingZeros: 0},
			{input: 0xf0_00_00_00, expectedLeadingZeros: 0},
			{input: 0x00_ff_ff_ff, expectedLeadingZeros: 8},
			{input: 0, expectedLeadingZeros: 32},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%032b", tc.input), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				// Setup the target value.
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileClz(&wazeroir.OperationClz{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileClz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				// To verify the behavior, we release the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate and run the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.expectedLeadingZeros, uint32(eng.stack[eng.stackPointer-1]))
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedLeadingZeros uint64 }{
			{input: 0xf0_00_00_00_00_00_00_00, expectedLeadingZeros: 0},
			{input: 0xff_ff_ff_ff_ff_ff_ff_ff, expectedLeadingZeros: 0},
			{input: 0x00_ff_ff_ff_ff_ff_ff_ff, expectedLeadingZeros: 8},
			{input: 0x00_00_00_00_ff_ff_ff_ff, expectedLeadingZeros: 32},
			{input: 0x00_00_00_00_00_ff_ff_ff, expectedLeadingZeros: 40},
			{input: 0, expectedLeadingZeros: 64},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%064b", tc.expectedLeadingZeros), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				// Setup the target value.
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileClz(&wazeroir.OperationClz{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileClz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				// To verify the behavior, we release the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate and run the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.expectedLeadingZeros, eng.stack[eng.stackPointer-1])
			})
		}
	})
}

func TestAmd64Compiler_compilCtz(t *testing.T) {
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedTrailingZeros uint32 }{
			{input: 0xff_ff_ff_ff, expectedTrailingZeros: 0},
			{input: 0x00_00_00_01, expectedTrailingZeros: 0},
			{input: 0xff_ff_ff_00, expectedTrailingZeros: 8},
			{input: 0, expectedTrailingZeros: 32},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%032b", tc.input), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				// Setup the target value.
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileCtz(&wazeroir.OperationCtz{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileCtz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				// To verify the behavior, we release the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate and run the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.expectedTrailingZeros, uint32(eng.stack[eng.stackPointer-1]))
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedTrailingZeros uint64 }{
			{input: 0xff_ff_ff_ff_ff_ff_ff_ff, expectedTrailingZeros: 0},
			{input: 0x00_00_00_00_00_00_00_01, expectedTrailingZeros: 0},
			{input: 0xff_ff_ff_ff_ff_ff_ff_00, expectedTrailingZeros: 8},
			{input: 0xff_ff_ff_ff_00_00_00_00, expectedTrailingZeros: 32},
			{input: 0xff_ff_ff_00_00_00_00_00, expectedTrailingZeros: 40},
			{input: 0, expectedTrailingZeros: 64},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%064b", tc.input), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				// Setup the target value.
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileCtz(&wazeroir.OperationCtz{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileCtz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				// To verify the behavior, we release the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate and run the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.expectedTrailingZeros, eng.stack[eng.stackPointer-1])
			})
		}
	})
}
func TestAmd64Compiler_compilPopcnt(t *testing.T) {
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedSetBits uint32 }{
			{input: 0xff_ff_ff_ff, expectedSetBits: 32},
			{input: 0x00_00_00_01, expectedSetBits: 1},
			{input: 0x10_00_00_00, expectedSetBits: 1},
			{input: 0x00_00_10_00, expectedSetBits: 1},
			{input: 0x00_01_00_01, expectedSetBits: 2},
			{input: 0xff_ff_00_ff, expectedSetBits: 24},
			{input: 0, expectedSetBits: 0},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%032b", tc.input), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				// Setup the target value.
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compilePopcnt(&wazeroir.OperationPopcnt{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())

				// To verify the behavior, we release the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate and run the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.expectedSetBits, uint32(eng.stack[eng.stackPointer-1]))
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct{ in, exp uint64 }{
			{in: 0xff_ff_ff_ff_ff_ff_ff_ff, exp: 64},
			{in: 0x00_00_00_00_00_00_00_01, exp: 1},
			{in: 0x00_00_00_01_00_00_00_00, exp: 1},
			{in: 0x10_00_00_00_00_00_00_00, exp: 1},
			{in: 0xf0_00_00_00_00_00_01_00, exp: 5},
			{in: 0xff_ff_ff_ff_ff_ff_ff_00, exp: 56},
			{in: 0xff_ff_ff_00_ff_ff_ff_ff, exp: 56},
			{in: 0xff_ff_ff_ff_00_00_00_00, exp: 32},
			{in: 0xff_ff_ff_00_00_00_00_00, exp: 24},
			{in: 0, exp: 0},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%064b", tc.in), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				// Setup the target value.
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: tc.in})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compilePopcnt(&wazeroir.OperationPopcnt{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())

				// To verify the behavior, we release the value
				// to the stack.
				compiler.releaseAllRegistersToStack()
				compiler.returnFunction()

				// Generate and run the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the stack.
				require.Equal(t, uint64(1), eng.stackPointer)
				require.Equal(t, tc.exp, eng.stack[eng.stackPointer-1])
			})
		}
	})
}

// The division by zero error must be caught by Go's runtime via x86's exception caught by kernel.
func getDivisionByZeroErrorRecoverFunc(t *testing.T) func() {
	return func() {
		if e := recover(); e != nil {
			err, ok := e.(error)
			require.True(t, ok)
			require.Equal(t, "runtime error: integer divide by zero", err.Error())
		}
	}
}

func TestAmd64Compiler_compile_and_or_xor_shl_shr_rotl_rotr(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   wazeroir.Operation
	}{
		{name: "and-32-bit", op: &wazeroir.OperationAnd{Type: wazeroir.UnsignedInt32}},
		{name: "and-64-bit", op: &wazeroir.OperationAnd{Type: wazeroir.UnsignedInt64}},
		{name: "or-32-bit", op: &wazeroir.OperationOr{Type: wazeroir.UnsignedInt32}},
		{name: "or-64-bit", op: &wazeroir.OperationOr{Type: wazeroir.UnsignedInt64}},
		{name: "xor-32-bit", op: &wazeroir.OperationXor{Type: wazeroir.UnsignedInt32}},
		{name: "xor-64-bit", op: &wazeroir.OperationXor{Type: wazeroir.UnsignedInt64}},
		{name: "shl-32-bit", op: &wazeroir.OperationShl{Type: wazeroir.UnsignedInt32}},
		{name: "shl-64-bit", op: &wazeroir.OperationShl{Type: wazeroir.UnsignedInt64}},
		{name: "shr-signed-32-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedInt32}},
		{name: "shr-signed-64-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedInt64}},
		{name: "shr-unsigned-32-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedUint32}},
		{name: "shr-unsigned-64-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedUint64}},
		{name: "rotl-32-bit", op: &wazeroir.OperationRotl{Type: wazeroir.UnsignedInt32}},
		{name: "rotl-64-bit", op: &wazeroir.OperationRotl{Type: wazeroir.UnsignedInt64}},
		{name: "rotr-32-bit", op: &wazeroir.OperationRotr{Type: wazeroir.UnsignedInt32}},
		{name: "rotr-64-bit", op: &wazeroir.OperationRotr{Type: wazeroir.UnsignedInt64}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for i, vs := range []struct {
				x1, x2 uint64
			}{
				{x1: 0, x2: 0},
				{x1: 0, x2: 1},
				{x1: 1, x2: 0},
				{x1: 1, x2: 1},
				{x1: 1 << 31, x2: 1},
				{x1: 1, x2: 1 << 31},
				{x1: 1 << 31, x2: 1 << 31},
				{x1: 1 << 63, x2: 1},
				{x1: 1, x2: 1 << 63},
				{x1: 1 << 63, x2: 1 << 63},
			} {
				t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()

					var is32Bit bool
					var expectedValue uint64
					var compileOperationFunc func()
					switch o := tc.op.(type) {
					case *wazeroir.OperationAnd:
						compileOperationFunc = func() {
							err := compiler.compileAnd(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.UnsignedInt32
						if is32Bit {
							expectedValue = uint64(uint32(vs.x1) & uint32(vs.x2))
						} else {
							expectedValue = vs.x1 & vs.x2
						}
					case *wazeroir.OperationOr:
						compileOperationFunc = func() {
							err := compiler.compileOr(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.UnsignedInt32
						if is32Bit {
							expectedValue = uint64(uint32(vs.x1) | uint32(vs.x2))
						} else {
							expectedValue = vs.x1 | vs.x2
						}
					case *wazeroir.OperationXor:
						compileOperationFunc = func() {
							err := compiler.compileXor(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.UnsignedInt32
						if is32Bit {
							expectedValue = uint64(uint32(vs.x1) ^ uint32(vs.x2))
						} else {
							expectedValue = vs.x1 ^ vs.x2
						}
					case *wazeroir.OperationShl:
						compileOperationFunc = func() {
							err := compiler.compileShl(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.UnsignedInt32
						if is32Bit {
							expectedValue = uint64(uint32(vs.x1) << uint32(vs.x2%32))
						} else {
							expectedValue = vs.x1 << (vs.x2 % 64)
						}
					case *wazeroir.OperationShr:
						compileOperationFunc = func() {
							err := compiler.compileShr(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.SignedInt32 || o.Type == wazeroir.SignedUint32
						switch o.Type {
						case wazeroir.SignedInt32:
							expectedValue = uint64(int32(vs.x1) >> (uint32(vs.x2) % 32))
						case wazeroir.SignedInt64:
							expectedValue = uint64(int64(vs.x1) >> (vs.x2 % 64))
						case wazeroir.SignedUint32:
							expectedValue = uint64(uint32(vs.x1) >> (uint32(vs.x2) % 32))
						case wazeroir.SignedUint64:
							expectedValue = vs.x1 >> (vs.x2 % 64)
						}
					case *wazeroir.OperationRotl:
						compileOperationFunc = func() {
							err := compiler.compileRotl(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.UnsignedInt32
						if is32Bit {
							expectedValue = uint64(bits.RotateLeft32(uint32(vs.x1), int(vs.x2)))
						} else {
							expectedValue = uint64(bits.RotateLeft64(vs.x1, int(vs.x2)))
						}
					case *wazeroir.OperationRotr:
						compileOperationFunc = func() {
							err := compiler.compileRotr(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.UnsignedInt32
						if is32Bit {
							expectedValue = uint64(bits.RotateLeft32(uint32(vs.x1), -int(vs.x2)))
						} else {
							expectedValue = uint64(bits.RotateLeft64(vs.x1, -int(vs.x2)))
						}
					}

					// Setup the target values.
					if is32Bit {
						err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(vs.x1)})
						require.NoError(t, err)
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(vs.x2)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: vs.x1})
						require.NoError(t, err)
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: vs.x2})
						require.NoError(t, err)
					}

					// Compile the operation.
					compileOperationFunc()

					// To verify the behavior, we release the value
					// to the stack.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()

					// Generate and run the code under test.
					code, _, err := compiler.generate()
					require.NoError(t, err)
					eng := newEngine()
					mem := newMemoryInst()
					jitcall(
						uintptr(unsafe.Pointer(&code[0])),
						uintptr(unsafe.Pointer(eng)),
						uintptr(unsafe.Pointer(&mem.Buffer[0])),
					)

					fmt.Println(hex.EncodeToString(code))

					// Check the result.
					require.Equal(t, uint64(1), eng.stackPointer)
					if is32Bit {
						require.Equal(t, uint32(expectedValue), uint32(eng.stack[eng.stackPointer-1]))
					} else {
						require.Equal(t, expectedValue, eng.stack[eng.stackPointer-1])
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compileDiv(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{{name: "signed", signed: true}, {name: "unsigned", signed: false}} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name string
					// Interpret -1 as stack.
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: -1,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:staack,x2:ax",
						x1Reg: -1,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: -1,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: -1,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: -1,
						x2Reg: -1,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for i, vs := range []struct {
							x1Value, x2Value uint32
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 2},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 1},
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 0xfffffffe /* -2 in signed 32bit */},
						} {
							vs := vs
							t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {

								eng := newEngine()
								compiler := requireNewCompiler(t)
								compiler.initializeReservedRegisters()

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x2Value)
								}

								var err error
								if signed.signed {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeInt32})
								} else {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeUint32})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the multiplication result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
								require.NoError(t, err)

								// To verify the behavior, we push the value
								// to the stack.
								compiler.releaseAllRegistersToStack()
								compiler.returnFunction()

								// Generate the code under test.
								code, _, err := compiler.generate()
								require.NoError(t, err)
								// Run code.
								defer getDivisionByZeroErrorRecoverFunc(t)()
								jitcall(
									uintptr(unsafe.Pointer(&code[0])),
									uintptr(unsafe.Pointer(eng)),
									0,
								)

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), eng.stackPointer)
								if signed.signed {
									require.Equal(t, int32(vs.x1Value)/int32(vs.x2Value)+int32(dxValue), int32(eng.stack[eng.stackPointer-1]))
								} else {
									require.Equal(t, vs.x1Value/vs.x2Value+uint32(dxValue), uint32(eng.stack[eng.stackPointer-1]))
								}
							})
						}
					})
				}
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{
			{name: "signed", signed: true},
			{name: "unsigned", signed: false},
		} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name string
					// Interpret -1 as stack.
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: -1,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:staack,x2:ax",
						x1Reg: -1,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: -1,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: -1,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: -1,
						x2Reg: -1,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for i, vs := range []struct {
							x1Value, x2Value uint64
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 1},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 1},
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 0xfffffffffffffffe /* -2 in signed 64bit */},
						} {
							vs := vs
							t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {

								eng := newEngine()
								compiler := requireNewCompiler(t)
								compiler.initializeReservedRegisters()

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x2Value)
								}

								var err error
								if signed.signed {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeInt64})
								} else {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeUint64})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the quotiont of the division result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
								require.NoError(t, err)

								// To verify the behavior, we push the value
								// to the stack.
								compiler.releaseAllRegistersToStack()
								compiler.returnFunction()

								// Generate the code under test.
								code, _, err := compiler.generate()
								require.NoError(t, err)
								// Run code.
								defer getDivisionByZeroErrorRecoverFunc(t)()
								jitcall(
									uintptr(unsafe.Pointer(&code[0])),
									uintptr(unsafe.Pointer(eng)),
									0,
								)

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), eng.stackPointer)
								if signed.signed {
									require.Equal(t, int64(vs.x1Value)/int64(vs.x2Value)+int64(dxValue), int64(eng.stack[eng.stackPointer-1]))
								} else {
									require.Equal(t, vs.x1Value/vs.x2Value+dxValue, eng.stack[eng.stackPointer-1])
								}
							})
						}
					})
				}
			})
		}
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float32
		}{
			{x1: 100, x2: 0},
			{x1: 0, x2: 100},
			{x1: 100, x2: -1.1},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: float32(math.Inf(1)), x2: 100},
			{x1: float32(math.Inf(-1)), x2: -100},
			{x1: 100, x2: float32(math.Inf(1))},
			{x1: -100, x2: float32(math.Inf(-1))},
			{x1: float32(math.Inf(1)), x2: 0},
			{x1: float32(math.Inf(-1)), x2: 0},
			{x1: 0, x2: float32(math.Inf(1))},
			{x1: 0, x2: float32(math.Inf(-1))},
			{x1: float32(math.NaN()), x2: 0},
			{x1: 0, x2: float32(math.NaN())},
			{x1: float32(math.NaN()), x2: 12321},
			{x1: 12313, x2: float32(math.NaN())},
			{x1: float32(math.NaN()), x2: float32(math.NaN())},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeFloat32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the result.
				require.Equal(t, uint64(1), eng.stackPointer)
				exp := tc.x1 / tc.x2
				actual := stackTopAsFloat32(eng)
				if math.IsNaN(float64(exp)) {
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, tc.x1/tc.x2, actual)
				}
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float64
		}{
			{x1: 100, x2: -1.1},
			{x1: 100, x2: 0},
			{x1: 0, x2: 0},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
			{x1: math.Inf(1), x2: 100},
			{x1: math.Inf(1), x2: -100},
			{x1: 100, x2: math.Inf(1)},
			{x1: -100, x2: math.Inf(1)},
			{x1: math.Inf(-1), x2: 100},
			{x1: math.Inf(-1), x2: -100},
			{x1: 100, x2: math.Inf(-1)},
			{x1: -100, x2: math.Inf(-1)},
			{x1: math.Inf(1), x2: 0},
			{x1: math.Inf(-1), x2: 0},
			{x1: 0, x2: math.Inf(1)},
			{x1: 0, x2: math.Inf(-1)},
			{x1: math.NaN(), x2: 0},
			{x1: 0, x2: math.NaN()},
			{x1: math.NaN(), x2: 12321},
			{x1: 12313, x2: math.NaN()},
			{x1: math.NaN(), x2: math.NaN()},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeFloat64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.returnFunction()

				// Generate the code under test.
				code, _, err := compiler.generate()
				require.NoError(t, err)
				// Run code.
				eng := newEngine()
				mem := newMemoryInst()
				jitcall(
					uintptr(unsafe.Pointer(&code[0])),
					uintptr(unsafe.Pointer(eng)),
					uintptr(unsafe.Pointer(&mem.Buffer[0])),
				)

				// Check the result.
				require.Equal(t, uint64(1), eng.stackPointer)
				exp := tc.x1 / tc.x2
				actual := stackTopAsFloat64(eng)
				if math.IsNaN(exp) {
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, tc.x1/tc.x2, actual)
				}
			})
		}
	})
}

func TestAmd64Compiler_compileRem(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{
			{name: "signed", signed: true},
			// {name: "unsigned", signed: false},
		} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name string
					// Interpret -1 as stack.
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: -1,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:staack,x2:ax",
						x1Reg: -1,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: -1,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: -1,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: -1,
						x2Reg: -1,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for _, vs := range []struct {
							x1Value, x2Value uint32
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 2},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 1},
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 0xfffffffe /* -2 in signed 32bit */},
							{x1Value: math.MaxInt32, x2Value: math.MaxUint32},
							{x1Value: math.MaxInt32 + 1, x2Value: math.MaxUint32},
						} {
							vs := vs
							t.Run(fmt.Sprintf("x1=%d,x2=%d", vs.x1Value, vs.x2Value), func(t *testing.T) {

								eng := newEngine()
								compiler := requireNewCompiler(t)
								compiler.initializeReservedRegisters()

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x2Value)
								}

								var err error
								if signed.signed {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt32})
								} else {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint32})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_DX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the remainder result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
								require.NoError(t, err)

								// To verify the behavior, we push the value
								// to the stack.
								compiler.releaseAllRegistersToStack()
								compiler.returnFunction()

								// Generate the code under test.
								code, _, err := compiler.generate()
								require.NoError(t, err)
								// Run code.
								if vs.x2Value == 0 {
									defer getDivisionByZeroErrorRecoverFunc(t)()
								}
								fmt.Println(hex.EncodeToString(code))
								jitcall(
									uintptr(unsafe.Pointer(&code[0])),
									uintptr(unsafe.Pointer(eng)),
									0,
								)

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), eng.stackPointer)
								if signed.signed {
									x1Signed := int32(vs.x1Value)
									x2Signed := int32(vs.x2Value)
									require.Equal(t, x1Signed%x2Signed+int32(dxValue), int32(eng.stack[eng.stackPointer-1]))
								} else {
									require.Equal(t, vs.x1Value%vs.x2Value+uint32(dxValue), uint32(eng.stack[eng.stackPointer-1]))
								}
							})
						}
					})
				}
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{
			{name: "signed", signed: true},
			{name: "unsigned", signed: false},
		} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name string
					// Interpret -1 as stack.
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: -1,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:staack,x2:ax",
						x1Reg: -1,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: -1,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: -1,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: -1,
						x2Reg: -1,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for i, vs := range []struct {
							x1Value, x2Value uint64
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 1},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 1},
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 0xfffffffffffffffe /* -2 in signed 64bit */},
							{x1Value: math.MaxInt32, x2Value: math.MaxUint32},
							{x1Value: math.MaxInt32 + 1, x2Value: math.MaxUint32},
							{x1Value: math.MaxInt64, x2Value: math.MaxUint64},
							{x1Value: math.MaxInt64 + 1, x2Value: math.MaxUint64},
						} {
							vs := vs
							t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {

								eng := newEngine()
								compiler := requireNewCompiler(t)
								compiler.initializeReservedRegisters()

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != -1 {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueOnStack()
									eng.stack[loc.stackPointer] = uint64(vs.x2Value)
								}

								var err error
								if signed.signed {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt64})
								} else {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint64})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_DX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the quotiont of the division result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
								require.NoError(t, err)

								// To verify the behavior, we push the value
								// to the stack.
								compiler.releaseAllRegistersToStack()
								compiler.returnFunction()

								// Generate the code under test.
								code, _, err := compiler.generate()
								require.NoError(t, err)
								// Run code.
								if vs.x2Value == 0 {
									defer getDivisionByZeroErrorRecoverFunc(t)()
								}
								jitcall(
									uintptr(unsafe.Pointer(&code[0])),
									uintptr(unsafe.Pointer(eng)),
									0,
								)

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), eng.stackPointer)
								if signed.signed {
									require.Equal(t, int64(vs.x1Value)%int64(vs.x2Value)+int64(dxValue), int64(eng.stack[eng.stackPointer-1]))
								} else {
									require.Equal(t, vs.x1Value%vs.x2Value+dxValue, eng.stack[eng.stackPointer-1])
								}
							})
						}
					})
				}
			})
		}
	})
}

func TestAmd64Compiler_compileF32DemoteFromF64(t *testing.T) {
	for _, v := range []float64{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		6.8719476736e+10,  /* = 1 << 36 */
		1.37438953472e+11, /* = 1 << 37 */
		math.Inf(1), math.Inf(-1), math.NaN(),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Setup the demote target.
			err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
			require.NoError(t, err)

			err = compiler.compileF32DemoteFromF64()
			require.NoError(t, err)

			// To verify the behavior, we release the value
			// to the stack.
			compiler.releaseAllRegistersToStack()
			compiler.returnFunction()

			// Generate and run the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			eng := newEngine()
			jitcall(uintptr(unsafe.Pointer(&code[0])), uintptr(unsafe.Pointer(eng)), 0)

			// Check the result.
			require.Equal(t, uint64(1), eng.stackPointer)
			if math.IsNaN(v) {
				require.True(t, math.IsNaN(float64(stackTopAsFloat32(eng))))
			} else {
				exp := float32(v)
				actual := stackTopAsFloat32(eng)
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestAmd64Compiler_compileF64PromoteFromF32(t *testing.T) {
	for _, v := range []float32{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Setup the promote target.
			err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: v})
			require.NoError(t, err)

			err = compiler.compileF64PromoteFromF32()
			require.NoError(t, err)

			// To verify the behavior, we release the value
			// to the stack.
			compiler.releaseAllRegistersToStack()
			compiler.returnFunction()

			// Generate and run the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			eng := newEngine()
			jitcall(uintptr(unsafe.Pointer(&code[0])), uintptr(unsafe.Pointer(eng)), 0)

			// Check the result.
			require.Equal(t, uint64(1), eng.stackPointer)
			if math.IsNaN(float64(v)) {
				require.True(t, math.IsNaN(stackTopAsFloat64(eng)))
			} else {
				exp := float64(v)
				actual := stackTopAsFloat64(eng)
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestAmd64Compiler_compileReinterpret(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindF32ReinterpretFromI32,
		wazeroir.OperationKindF64ReinterpretFromI64,
		wazeroir.OperationKindI32ReinterpretFromF32,
		wazeroir.OperationKindI64ReinterpretFromF64,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, originOnStack := range []bool{false, true} {
				originOnStack := originOnStack
				t.Run(fmt.Sprintf("%v", originOnStack), func(t *testing.T) {
					for _, v := range []uint64{
						0, 1, 1 << 16, 1 << 31, 1 << 32, 1 << 63,
						math.MaxInt32, math.MaxUint32, math.MaxUint64,
					} {
						v := v
						t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
							compiler := requireNewCompiler(t)
							compiler.initializeReservedRegisters()

							eng := newEngine()
							if originOnStack {
								loc := compiler.locationStack.pushValueOnStack()
								eng.stack[loc.stackPointer] = v
								eng.stackPointer = 1
							}

							var is32Bit bool
							var err error
							switch kind {
							case wazeroir.OperationKindF32ReinterpretFromI32:
								is32Bit = true
								if !originOnStack {
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
									require.NoError(t, err)
								}
								err = compiler.compileF32ReinterpretFromI32()
								require.NoError(t, err)
							case wazeroir.OperationKindF64ReinterpretFromI64:
								if !originOnStack {
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
									require.NoError(t, err)
								}
								err = compiler.compileF64ReinterpretFromI64()
								require.NoError(t, err)
							case wazeroir.OperationKindI32ReinterpretFromF32:
								is32Bit = true
								if !originOnStack {
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
									require.NoError(t, err)
								}
								err = compiler.compileI32ReinterpretFromF32()
								require.NoError(t, err)
							case wazeroir.OperationKindI64ReinterpretFromF64:
								if !originOnStack {
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
									require.NoError(t, err)
								}
								err = compiler.compileI64ReinterpretFromF64()
								require.NoError(t, err)
							default:
								t.Fail()
							}

							// To verify the behavior, we release the value
							// to the stack.
							compiler.releaseAllRegistersToStack()
							compiler.returnFunction()

							// Generate and run the code under test.
							code, _, err := compiler.generate()
							require.NoError(t, err)
							jitcall(uintptr(unsafe.Pointer(&code[0])), uintptr(unsafe.Pointer(eng)), 0)

							// Reinterpret must preserve the bit-pattern.
							if is32Bit {
								require.Equal(t, uint32(v), stackTopAsUint32(eng))
							} else {
								require.Equal(t, v, stackTopAsUint64(eng))
							}
						})
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compileExtend(t *testing.T) {
	for _, signed := range []bool{false, true} {
		signed := signed
		t.Run(fmt.Sprintf("signed=%v", signed), func(t *testing.T) {
			for _, v := range []uint32{
				0, 1, 1 << 14, 1 << 31, math.MaxUint32, 0xFFFFFFFF, math.MaxInt32,
			} {
				v := v
				t.Run(fmt.Sprintf("%v", v), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()

					// Setup the promote target.
					err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: v})
					require.NoError(t, err)

					err = compiler.compileExtend(&wazeroir.OperationExtend{Signed: signed})
					require.NoError(t, err)

					// To verify the behavior, we release the value
					// to the stack.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()

					// Generate and run the code under test.
					code, _, err := compiler.generate()
					require.NoError(t, err)
					eng := newEngine()
					jitcall(uintptr(unsafe.Pointer(&code[0])), uintptr(unsafe.Pointer(eng)), 0)

					require.Equal(t, uint64(1), eng.stackPointer)
					if signed {
						expected := int64(int32(v))
						require.Equal(t, expected, stackTopAsInt64(eng))
					} else {
						expected := uint64(uint32(v))
						require.Equal(t, expected, stackTopAsUint64(eng))
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compileITruncFromF(t *testing.T) {
	for _, tc := range []struct {
		outputType wazeroir.SignedInt
		inputType  wazeroir.Float
	}{
		{outputType: wazeroir.SignedInt32, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedInt32, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedInt64, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedInt64, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedUint32, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedUint32, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedUint64, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedUint64, inputType: wazeroir.Float64},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s from %s", tc.outputType, tc.inputType), func(t *testing.T) {
			for _, v := range []float64{
				0, 100, -100, 1, -1,
				100.01234124, -100.01234124, 200.12315,
				6.8719476736e+10, /* = 1 << 36 */
				-6.8719476736e+10,
				1.37438953472e+11, /* = 1 << 37 */
				-1.37438953472e+11,
				-2147483649.0,
				math.MinInt32,
				math.MaxInt32,
				math.MaxUint32,
				math.MinInt64,
				math.MaxInt64,
				math.MaxUint64,
				math.MaxFloat32,
				math.SmallestNonzeroFloat32,
				math.MaxFloat64,
				math.SmallestNonzeroFloat64,
				math.Inf(1), math.Inf(-1), math.NaN(),
			} {
				if v == math.MaxInt32 {
					// Note that math.MaxInt32 is rounded up to math.MaxInt32+1 in 32-bit float representation.
					require.Equal(t, float32(2147483648.0) /* = math.MaxInt32+1 */, float32(v))
				} else if v == math.MaxUint32 {
					// Note that math.MaxUint32 is rounded up to math.MaxUint32+1 in 32-bit float representation.
					require.Equal(t, float32(4294967296 /* = math.MaxUint32+1 */), float32(v))
				} else if v == math.MaxInt64 {
					// Note that math.MaxInt64 is rounded up to math.MaxInt64+1 in 32/64-bit float representation.
					require.Equal(t, float32(9223372036854775808.0) /* = math.MaxInt64+1 */, float32(v))
					require.Equal(t, float64(9223372036854775808.0) /* = math.MaxInt64+1 */, float64(v))
				} else if v == math.MaxUint64 {
					// Note that math.MaxUint64 is rounded up to math.MaxUint64+1 in 32/64-bit float representation.
					require.Equal(t, float32(18446744073709551616.0) /* = math.MaxInt64+1 */, float32(v))
					require.Equal(t, float64(18446744073709551616.0) /* = math.MaxInt64+1 */, float64(v))
				}

				t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()

					// Setup the conversion target.
					var err error
					if tc.inputType == wazeroir.Float32 {
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(v)})
					} else {
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
					}
					require.NoError(t, err)

					err = compiler.compileITruncFromF(&wazeroir.OperationITruncFromF{
						InputType: tc.inputType, OutputType: tc.outputType,
					})
					require.NoError(t, err)

					// To verify the behavior, we release the value
					// to the stack.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()

					// Generate and run the code under test.
					code, _, err := compiler.generate()
					require.NoError(t, err)
					eng := newEngine()
					jitcall(uintptr(unsafe.Pointer(&code[0])), uintptr(unsafe.Pointer(eng)), 0)

					// Check the result.
					var shouldInvalidStatus bool
					if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedInt32 {
						f32 := float32(v)
						shouldInvalidStatus = math.IsNaN(v) || f32 < math.MinInt32 || f32 >= math.MaxInt32
						if !shouldInvalidStatus {
							require.Equal(t, int32(math.Trunc(float64(f32))), stackTopAsInt32(eng))
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedInt64 {
						f32 := float32(v)
						shouldInvalidStatus = math.IsNaN(v) || f32 < math.MinInt64 || f32 >= math.MaxInt64
						if !shouldInvalidStatus {
							require.Equal(t, int64(math.Trunc(float64(f32))), stackTopAsInt64(eng))
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedInt32 {
						shouldInvalidStatus = math.IsNaN(v) || v < math.MinInt32 || v > math.MaxInt32
						if !shouldInvalidStatus {
							require.Equal(t, int32(math.Trunc(v)), stackTopAsInt32(eng))
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedInt64 {
						shouldInvalidStatus = math.IsNaN(v) || v < math.MinInt64 || v >= math.MaxInt64
						if !shouldInvalidStatus {
							require.Equal(t, int64(math.Trunc(v)), stackTopAsInt64(eng))
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedUint32 {
						f32 := float32(v)
						shouldInvalidStatus = math.IsNaN(v) || f32 < 0 || f32 >= math.MaxUint32
						if !shouldInvalidStatus {
							require.Equal(t, uint32(math.Trunc(float64(f32))), stackTopAsUint32(eng))
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedUint32 {
						shouldInvalidStatus = math.IsNaN(v) || v < 0 || v > math.MaxUint32
						if !shouldInvalidStatus {
							require.Equal(t, uint32(math.Trunc(v)), stackTopAsUint32(eng))
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedUint64 {
						f32 := float32(v)
						shouldInvalidStatus = math.IsNaN(v) || f32 < 0 || f32 >= math.MaxUint64
						if !shouldInvalidStatus {
							require.Equal(t, uint64(math.Trunc(float64(f32))), stackTopAsUint64(eng))
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedUint64 {
						shouldInvalidStatus = math.IsNaN(v) || v < 0 || v >= math.MaxUint64
						if !shouldInvalidStatus {
							require.Equal(t, uint64(math.Trunc(v)), stackTopAsUint64(eng))
						}
					} else {
						t.Fatal()
					}

					// Check the jit status code if necessary.
					require.True(t, !shouldInvalidStatus || eng.jitCallStatusCode == jitCallStatusCodeInvalidFloatToIntConversion)
				})
			}
		})
	}
}

func TestAmd64Compiler_compileFConvertFromI(t *testing.T) {
	for _, tc := range []struct {
		inputType  wazeroir.SignedInt
		outputType wazeroir.Float
	}{
		{inputType: wazeroir.SignedInt32, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedInt32, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedInt64, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedInt64, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedUint32, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedUint32, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedUint64, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedUint64, outputType: wazeroir.Float64},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s from %s", tc.outputType, tc.inputType), func(t *testing.T) {
			for _, v := range []uint64{
				0, 1, 12345, 1 << 31, 1 << 32, 1 << 54, 1 << 63,
				math.MaxUint32, math.MaxUint64, math.MaxInt32, math.MaxInt64,
				math.Float64bits(1.23455555),
				math.Float64bits(-1.23455555),
				math.Float64bits(math.NaN()),
				math.Float64bits(math.Inf(1)),
				math.Float64bits(math.Inf(-1)),
			} {
				t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()

					// Setup the conversion target.
					var err error
					if tc.inputType == wazeroir.SignedInt32 || tc.inputType == wazeroir.SignedUint32 {
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
					} else {
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(v)})
					}
					require.NoError(t, err)

					err = compiler.compileFConvertFromI(&wazeroir.OperationFConvertFromI{
						InputType: tc.inputType, OutputType: tc.outputType,
					})
					require.NoError(t, err)

					// To verify the behavior, we release the value
					// to the stack.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()

					// Generate and run the code under test.
					code, _, err := compiler.generate()
					require.NoError(t, err)
					eng := newEngine()
					jitcall(uintptr(unsafe.Pointer(&code[0])), uintptr(unsafe.Pointer(eng)), 0)

					// Check the result.
					require.Equal(t, uint64(1), eng.stackPointer)
					actualBits := eng.stack[eng.stackPointer-1]
					if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedInt32 {
						exp := float32(int32(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedInt64 {
						exp := float32(int64(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedInt32 {
						exp := float64(int32(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedInt64 {
						exp := float64(int64(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedUint32 {
						exp := float32(uint32(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedUint32 {
						exp := float64(uint32(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedUint64 {
						exp := float32(v)
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedUint64 {
						exp := float64(v)
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else {
						t.Fatal()
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compile_abs_neg_ceil_floor(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   wazeroir.Operation
	}{
		{name: "abs-32-bit", op: &wazeroir.OperationAbs{Type: wazeroir.Float32}},
		{name: "abs-64-bit", op: &wazeroir.OperationAbs{Type: wazeroir.Float64}},
		{name: "neg-32-bit", op: &wazeroir.OperationNeg{Type: wazeroir.Float32}},
		{name: "neg-64-bit", op: &wazeroir.OperationNeg{Type: wazeroir.Float64}},
		{name: "ceil-32-bit", op: &wazeroir.OperationCeil{Type: wazeroir.Float32}},
		{name: "ceil-64-bit", op: &wazeroir.OperationCeil{Type: wazeroir.Float64}},
		{name: "floor-32-bit", op: &wazeroir.OperationFloor{Type: wazeroir.Float32}},
		{name: "floor-64-bit", op: &wazeroir.OperationFloor{Type: wazeroir.Float64}},
		{name: "trunc-32-bit", op: &wazeroir.OperationTrunc{Type: wazeroir.Float32}},
		{name: "trunc-64-bit", op: &wazeroir.OperationTrunc{Type: wazeroir.Float64}},
		{name: "sqrt-32-bit", op: &wazeroir.OperationSqrt{Type: wazeroir.Float32}},
		{name: "sqrt-64-bit", op: &wazeroir.OperationSqrt{Type: wazeroir.Float64}},
		{name: "nearest-32-bit", op: &wazeroir.OperationNearest{Type: wazeroir.Float32}},
		{name: "nearest-64-bit", op: &wazeroir.OperationNearest{Type: wazeroir.Float64}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for i, v := range []uint64{
				0,
				1 << 63,
				1<<63 | 12345,
				1 << 31,
				1<<31 | 123455,
				6.8719476736e+10,
				math.Float64bits(-4.5), // This produces the different result between math.Round and ROUND with 0x00 mode.
				1.37438953472e+11,
				math.Float64bits(-1.3),
				uint64(math.Float32bits(-1231.123)),
				math.Float64bits(1.3),
				math.Float64bits(100.3),
				math.Float64bits(-100.3),
				uint64(math.Float32bits(1231.123)),
				math.Float64bits(math.Inf(1)),
				math.Float64bits(math.Inf(-1)),
				math.Float64bits(math.NaN()),
			} {
				t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()
					eng := newEngine()

					var is32Bit bool
					var expFloat32 float32
					var expFloat64 float64
					var compileOperationFunc func()
					switch o := tc.op.(type) {
					case *wazeroir.OperationAbs:
						compileOperationFunc = func() {
							err := compiler.compileAbs(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Abs(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Abs(math.Float64frombits(v))
						}
					case *wazeroir.OperationNeg:
						compileOperationFunc = func() {
							err := compiler.compileNeg(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = -math.Float32frombits(uint32(v))
						} else {
							expFloat64 = -math.Float64frombits(v)
						}
					case *wazeroir.OperationCeil:
						compileOperationFunc = func() {
							err := compiler.compileCeil(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Ceil(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Ceil(math.Float64frombits(v))
						}
					case *wazeroir.OperationFloor:
						compileOperationFunc = func() {
							err := compiler.compileFloor(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Floor(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Floor(math.Float64frombits(v))
						}
					case *wazeroir.OperationTrunc:
						compileOperationFunc = func() {
							err := compiler.compileTrunc(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Trunc(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Trunc(math.Float64frombits(v))
						}
					case *wazeroir.OperationSqrt:
						compileOperationFunc = func() {
							err := compiler.compileSqrt(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Sqrt(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Sqrt(math.Float64frombits(v))
						}
					case *wazeroir.OperationNearest:
						compileOperationFunc = func() {
							err := compiler.compileNearest(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						// The same algorithm as in wazeroir/interpreter.go.
						if is32Bit {
							expFloat32 = math.Float32frombits(uint32(v))
							f64 := float64(expFloat32)
							if expFloat32 != -0 && expFloat32 != 0 {
								ceil := float32(math.Ceil(f64))
								floor := float32(math.Floor(f64))
								distToCeil := math.Abs(float64(expFloat32 - ceil))
								distToFloor := math.Abs(float64(expFloat32 - floor))
								h := ceil / 2.0
								if distToCeil < distToFloor {
									expFloat32 = ceil
								} else if distToCeil == distToFloor && float32(math.Floor(float64(h))) == h {
									expFloat32 = ceil
								} else {
									expFloat32 = floor
								}
							}
						} else {
							expFloat64 = math.Float64frombits(v)
							if expFloat64 != -0 && expFloat64 != 0 {
								ceil := math.Ceil(expFloat64)
								floor := math.Floor(expFloat64)
								distToCeil := math.Abs(expFloat64 - ceil)
								distToFloor := math.Abs(expFloat64 - floor)
								h := ceil / 2.0
								if distToCeil < distToFloor {
									expFloat64 = ceil
								} else if distToCeil == distToFloor && math.Floor(h) == h {
									expFloat64 = ceil
								} else {
									expFloat64 = floor
								}
							}
						}
					}

					// Setup the target values.
					var err error
					if is32Bit {
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
					} else {
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
					}
					require.NoError(t, err)

					// Compile the operation.
					compileOperationFunc()

					// To verify the behavior, we release the value
					// to the stack.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()

					// Generate and run the code under test.
					code, _, err := compiler.generate()
					require.NoError(t, err)
					mem := newMemoryInst()
					jitcall(
						uintptr(unsafe.Pointer(&code[0])),
						uintptr(unsafe.Pointer(eng)),
						uintptr(unsafe.Pointer(&mem.Buffer[0])),
					)

					// Check the result.
					require.Equal(t, uint64(1), eng.stackPointer)
					if is32Bit {
						actual := stackTopAsFloat32(eng)
						if math.IsNaN(float64(expFloat32)) {
							require.True(t, math.IsNaN(float64(actual)))
						} else {
							require.Equal(t, actual, expFloat32)
						}
					} else {
						actual := stackTopAsFloat64(eng)
						if math.IsNaN(expFloat64) {
							require.True(t, math.IsNaN(actual))
						} else {
							require.Equal(t, expFloat64, actual)
						}
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compile_min_max_copysign(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   wazeroir.Operation
	}{
		{name: "min-32-bit", op: &wazeroir.OperationMin{Type: wazeroir.Float32}},
		{name: "min-64-bit", op: &wazeroir.OperationMin{Type: wazeroir.Float64}},
		{name: "max-32-bit", op: &wazeroir.OperationMax{Type: wazeroir.Float32}},
		{name: "max-64-bit", op: &wazeroir.OperationMax{Type: wazeroir.Float64}},
		{name: "copysign-32-bit", op: &wazeroir.OperationCopysign{Type: wazeroir.Float32}},
		{name: "copysign-64-bit", op: &wazeroir.OperationCopysign{Type: wazeroir.Float64}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, vs := range []struct{ x1, x2 float64 }{
				{x1: 100, x2: -1.1},
				{x1: 100, x2: 0},
				{x1: 0, x2: 0},
				{x1: 1, x2: 1},
				{x1: -1, x2: 100},
				{x1: 100, x2: 200},
				{x1: 100.01234124, x2: 100.01234124},
				{x1: 100.01234124, x2: -100.01234124},
				{x1: 200.12315, x2: 100},
				{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
				{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
				{x1: math.Inf(1), x2: 100},
				{x1: math.Inf(1), x2: -100},
				{x1: 100, x2: math.Inf(1)},
				{x1: -100, x2: math.Inf(1)},
				{x1: math.Inf(-1), x2: 100},
				{x1: math.Inf(-1), x2: -100},
				{x1: 100, x2: math.Inf(-1)},
				{x1: -100, x2: math.Inf(-1)},
				{x1: math.Inf(1), x2: 0},
				{x1: math.Inf(-1), x2: 0},
				{x1: 0, x2: math.Inf(1)},
				{x1: 0, x2: math.Inf(-1)},
				{x1: math.NaN(), x2: 0},
				{x1: 0, x2: math.NaN()},
				{x1: math.NaN(), x2: 12321},
				{x1: 12313, x2: math.NaN()},
				{x1: math.NaN(), x2: math.NaN()},
			} {
				t.Run(fmt.Sprintf("x1=%f_x2=%f", vs.x1, vs.x2), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()
					eng := newEngine()

					var is32Bit bool
					var expFloat32 float32
					var expFloat64 float64
					var compileOperationFunc func()
					switch o := tc.op.(type) {
					case *wazeroir.OperationMin:
						compileOperationFunc = func() {
							err := compiler.compileMin(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(wazeroir.Min(float64(float32(vs.x1)), float64(float32(vs.x2))))
						} else {
							expFloat64 = wazeroir.Min(vs.x1, vs.x2)
						}
					case *wazeroir.OperationMax:
						compileOperationFunc = func() {
							err := compiler.compileMax(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(wazeroir.Max(float64(float32(vs.x1)), float64(float32(vs.x2))))
						} else {
							expFloat64 = wazeroir.Max(vs.x1, vs.x2)
						}
					case *wazeroir.OperationCopysign:
						compileOperationFunc = func() {
							err := compiler.compileCopysign(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Copysign(float64(float32(vs.x1)), float64(float32(vs.x2))))
						} else {
							expFloat64 = math.Copysign(vs.x1, vs.x2)
						}
					}

					// Setup the target values.
					if is32Bit {
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(vs.x1)})
						require.NoError(t, err)
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(vs.x2)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: vs.x1})
						require.NoError(t, err)
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: vs.x2})
						require.NoError(t, err)
					}

					// Compile the operation.
					compileOperationFunc()

					// To verify the behavior, we release the value
					// to the stack.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()

					// Generate and run the code under test.
					code, _, err := compiler.generate()
					require.NoError(t, err)
					mem := newMemoryInst()
					jitcall(
						uintptr(unsafe.Pointer(&code[0])),
						uintptr(unsafe.Pointer(eng)),
						uintptr(unsafe.Pointer(&mem.Buffer[0])),
					)

					// Check the result.
					require.Equal(t, uint64(1), eng.stackPointer)
					if is32Bit {
						actual := stackTopAsFloat32(eng)
						if math.IsNaN(float64(expFloat32)) {
							require.True(t, math.IsNaN(float64(actual)), actual)
						} else {
							require.Equal(t, expFloat32, actual)
						}
					} else {
						actual := stackTopAsFloat64(eng)
						if math.IsNaN(expFloat64) {
							require.True(t, math.IsNaN(actual), actual)
						} else {
							require.Equal(t, expFloat64, actual)
						}
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compileCall(t *testing.T) {
	t.Run("host function", func(t *testing.T) {
		const functionIndex = 5
		compiler := requireNewCompiler(t)
		compiler.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{}}

		// Setup.
		eng := newEngine()
		compiler.eng = eng
		hostFuncRefValue := reflect.ValueOf(func() {})
		hostFuncInstance := &wasm.FunctionInstance{HostFunction: &hostFuncRefValue, Signature: &wasm.FunctionType{}}
		compiler.f.ModuleInstance.Functions = make([]*wasm.FunctionInstance, functionIndex+1)
		compiler.f.ModuleInstance.Functions[functionIndex] = hostFuncInstance
		eng.compiledHostFunctionIndex[hostFuncInstance] = functionIndex

		// Build codes.
		compiler.initializeReservedRegisters()
		// Push the value onto stack.
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := compiler.locationStack.pushValueOnRegister(x86.REG_AX)
		compiler.movIntConstToRegister(int64(50), loc.register)
		err := compiler.compileCall(&wazeroir.OperationCall{FunctionIndex: functionIndex})
		require.NoError(t, err)

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Run code.
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the status.
		require.Equal(t, jitCallStatusCodeCallHostFunction, eng.jitCallStatusCode)
		require.Equal(t, int64(functionIndex), eng.functionCallIndex)
		// All the registers must be written back to stack.
		require.Equal(t, uint64(2), eng.stackPointer)
		require.Equal(t, uint64(50), eng.stack[eng.stackPointer-1])
	})
	t.Run("wasm function", func(t *testing.T) {
		const functionIndex = 20
		compiler := requireNewCompiler(t)
		compiler.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{}}

		// Setup.
		eng := newEngine()
		compiler.eng = eng
		wasmFuncInstance := &wasm.FunctionInstance{Signature: &wasm.FunctionType{}}
		compiler.f.ModuleInstance.Functions = make([]*wasm.FunctionInstance, functionIndex+1)
		compiler.f.ModuleInstance.Functions[functionIndex] = wasmFuncInstance
		eng.compiledWasmFunctionIndex[wasmFuncInstance] = functionIndex

		// Build codes.
		compiler.initializeReservedRegisters()
		// Push the value onto stack.
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		_ = compiler.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := compiler.locationStack.pushValueOnRegister(x86.REG_AX)
		compiler.movIntConstToRegister(int64(50), loc.register)
		err := compiler.compileCall(&wazeroir.OperationCall{FunctionIndex: functionIndex})
		require.NoError(t, err)

		// Generate the code under test.
		code, _, err := compiler.generate()
		require.NoError(t, err)

		// Run code.
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the status.
		require.Equal(t, jitCallStatusCodeCallWasmFunction, eng.jitCallStatusCode)
		require.Equal(t, int64(functionIndex), eng.functionCallIndex)
		// All the registers must be written back to stack.
		require.Equal(t, uint64(3), eng.stackPointer)
		require.Equal(t, uint64(50), eng.stack[eng.stackPointer-1])
	})
}

func TestAmd64Compiler_setupMemoryOffset(t *testing.T) {
	bases := []uint32{0, 1 << 5, 1 << 9, 1 << 10, 1 << 15, math.MaxUint32 - 1, math.MaxUint32}
	offsets := []uint32{0,
		1 << 10, 1 << 31,
		math.MaxInt32 - 1, math.MaxInt32 - 2, math.MaxInt32 - 3, math.MaxInt32 - 4, math.MaxInt32 - 5, math.MaxInt32 - 8, math.MaxInt32 - 9,
		math.MaxInt32, math.MaxUint32,
	}
	targetSizeInBytes := []int64{1, 2, 4, 8}
	for _, base := range bases {
		base := base
		for _, offset := range offsets {
			offset := offset
			for _, targetSizeInByte := range targetSizeInBytes {
				targetSizeInByte := targetSizeInByte
				t.Run(fmt.Sprintf("base=%d,offset=%d,targetSizeInBytes=%d", base, offset, targetSizeInByte), func(t *testing.T) {
					compiler := requireNewCompiler(t)
					compiler.initializeReservedRegisters()

					err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: base})
					require.NoError(t, err)

					reg, err := compiler.setupMemoryOffset(offset, targetSizeInByte)
					require.NoError(t, err)

					compiler.locationStack.pushValueOnRegister(reg)

					// Generate the code under test.
					compiler.releaseAllRegistersToStack()
					compiler.returnFunction()
					code, _, err := compiler.generate()
					// fmt.Println(hex.EncodeToString(code))
					require.NoError(t, err)

					// Set up and run.
					mem := newMemoryInst()
					eng := newEngine()
					eng.memroySliceLen = len(mem.Buffer)
					jitcall(
						uintptr(unsafe.Pointer(&code[0])),
						uintptr(unsafe.Pointer(eng)),
						uintptr(unsafe.Pointer(&mem.Buffer[0])),
					)

					baseOffset := int(base) + int(offset)
					if baseOffset >= math.MaxUint32 || len(mem.Buffer) < baseOffset+int(targetSizeInByte) {
						require.Equal(t, jitCallStatusCodeInvalidMemoryOutOfBounds, eng.jitCallStatusCode)
					} else {
						require.Equal(t, jitCallStatusCodeReturned, eng.jitCallStatusCode)
					}
				})
			}
		}
	}
}

func TestAmd64Compiler_compileLoad(t *testing.T) {
	for i, tp := range []wazeroir.UnsignedType{
		wazeroir.UnsignedTypeI32,
		wazeroir.UnsignedTypeI64,
		wazeroir.UnsignedTypeF32,
		wazeroir.UnsignedTypeF64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			eng := newEngine()
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueOnStack()
			eng.stack[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad{Type: tp, Arg: &wazeroir.MemoryImmediate{Offest: 361}}
			err := compiler.compileLoad(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := compiler.locationStack.peek()
			require.True(t, loadedValue.onRegister())

			// Double the loaded value in order to verify the behavior.
			var addInst obj.As
			switch tp {
			case wazeroir.UnsignedTypeI32:
				require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
				require.True(t, isIntRegister(loadedValue.register))
				addInst = x86.AADDL
			case wazeroir.UnsignedTypeI64:
				require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
				require.True(t, isIntRegister(loadedValue.register))
				addInst = x86.AADDQ
			case wazeroir.UnsignedTypeF32:
				require.Equal(t, generalPurposeRegisterTypeFloat, loadedValue.registerType())
				require.True(t, isFloatRegister(loadedValue.register))
				addInst = x86.AADDSS
			case wazeroir.UnsignedTypeF64:
				require.Equal(t, generalPurposeRegisterTypeFloat, loadedValue.registerType())
				require.True(t, isFloatRegister(loadedValue.register))
				addInst = x86.AADDSD
			}
			doubleLoadedValue := compiler.newProg()
			doubleLoadedValue.As = addInst
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			doubleLoadedValue.From.Type = obj.TYPE_REG
			doubleLoadedValue.From.Reg = loadedValue.register
			compiler.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			compiler.releaseRegisterToStack(loadedValue)

			// Generate the code under test.
			compiler.returnFunction()
			code, _, err := compiler.generate()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
			eng.memroySliceLen = len(mem.Buffer)
			targetRegion := mem.Buffer[baseOffset+o.Arg.Offest:]
			var expValue uint64
			switch tp {
			case wazeroir.UnsignedTypeI32:
				original := uint32(100)
				binary.LittleEndian.PutUint32(targetRegion, original)
				expValue = uint64(original * 2)
			case wazeroir.UnsignedTypeI64:
				original := uint64(math.MaxUint32 + 123) // The value exceeds 32-bit.
				binary.LittleEndian.PutUint64(targetRegion, original)
				expValue = original * 2
			case wazeroir.UnsignedTypeF32:
				original := float32(1.234)
				binary.LittleEndian.PutUint32(targetRegion, math.Float32bits(original))
				expValue = uint64(math.Float32bits(original * 2))
			case wazeroir.UnsignedTypeF64:
				original := float64(math.MaxFloat32 + 100.1) // The value exceeds 32-bit.
				binary.LittleEndian.PutUint64(targetRegion, math.Float64bits(original))
				expValue = math.Float64bits(original * 2)
			}

			// Run code.
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Load instruction must push the loaded value to the top of the stack,
			// so the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			require.Equal(t, expValue, eng.stack[eng.stackPointer-1])
		})
	}
}

func TestAmd64Compiler_compileLoad8(t *testing.T) {
	for i, tp := range []wazeroir.SignedInt{
		wazeroir.SignedInt32,
		wazeroir.SignedInt64,
		wazeroir.SignedUint32,
		wazeroir.SignedUint64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			eng := newEngine()
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueOnStack()
			eng.stack[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad8{Type: tp, Arg: &wazeroir.MemoryImmediate{Offest: 361}}
			err := compiler.compileLoad8(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
			require.True(t, loadedValue.onRegister())

			// Increment the loaded value in order to verify the behavior.
			doubleLoadedValue := compiler.newProg()
			doubleLoadedValue.As = x86.AINCB
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			compiler.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			compiler.releaseRegisterToStack(loadedValue)

			// Generate the code under test.
			compiler.returnFunction()
			code, _, err := compiler.generate()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
			eng.memroySliceLen = len(mem.Buffer)
			// For testing, arbitrary byte is be fine.
			original := byte(0x10)
			mem.Buffer[baseOffset+o.Arg.Offest] = byte(original)

			// Run code.
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Load instruction must push the loaded value to the top of the stack,
			// so the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			// The loaded value must be incremented via x86.AINCB.
			require.Equal(t, original+1, byte(eng.stack[eng.stackPointer-1]))
		})
	}
}

func TestAmd64Compiler_compileLoad16(t *testing.T) {
	for i, tp := range []wazeroir.SignedInt{
		wazeroir.SignedInt32,
		wazeroir.SignedInt64,
		wazeroir.SignedUint32,
		wazeroir.SignedUint64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			eng := newEngine()
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueOnStack()
			eng.stack[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad16{Type: tp, Arg: &wazeroir.MemoryImmediate{Offest: 361}}
			err := compiler.compileLoad16(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
			require.True(t, loadedValue.onRegister())

			// Increment the loaded value in order to verify the behavior.
			doubleLoadedValue := compiler.newProg()
			doubleLoadedValue.As = x86.AINCW
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			compiler.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			compiler.releaseRegisterToStack(loadedValue)

			// Generate the code under test.
			compiler.returnFunction()
			code, _, err := compiler.generate()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
			eng.memroySliceLen = len(mem.Buffer)
			// For testing, arbitrary uint16 is be fine.
			original := uint16(0xff_fe)
			binary.LittleEndian.PutUint16(mem.Buffer[baseOffset+o.Arg.Offest:], original)

			// Run code.
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Load instruction must push the loaded value to the top of the stack,
			// so the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
			// The loaded value must be incremented via x86.AINCW.
			require.Equal(t, original+1, uint16(eng.stack[eng.stackPointer-1]))
		})
	}
}

func TestAmd64Compiler_compileLoad32(t *testing.T) {
	eng := newEngine()
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	// Before load operations, we must push the base offset value.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueOnStack()
	eng.stack[base.stackPointer] = baseOffset

	// Emit the memory load instructions.
	o := &wazeroir.OperationLoad32{Arg: &wazeroir.MemoryImmediate{Offest: 361}}
	err := compiler.compileLoad32(o)
	require.NoError(t, err)

	// At this point, the loaded value must be on top of the stack, and placed on a register.
	loadedValue := compiler.locationStack.peek()
	require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
	require.True(t, loadedValue.onRegister())

	// Increment the loaded value in order to verify the behavior.
	doubleLoadedValue := compiler.newProg()
	doubleLoadedValue.As = x86.AINCL
	doubleLoadedValue.To.Type = obj.TYPE_REG
	doubleLoadedValue.To.Reg = loadedValue.register
	compiler.addInstruction(doubleLoadedValue)

	// We need to write the result back to the memory stack.
	compiler.releaseRegisterToStack(loadedValue)

	// Generate the code under test.
	compiler.returnFunction()
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Place the load target value to the memory.
	mem := newMemoryInst()
	eng.memroySliceLen = len(mem.Buffer)
	// For testing, arbitrary uint32 is be fine.
	original := uint32(0xff_ff_fe)
	binary.LittleEndian.PutUint32(mem.Buffer[baseOffset+o.Arg.Offest:], original)

	// Run code.
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)

	// Load instruction must push the loaded value to the top of the stack,
	// so the stack pointer must be incremented.
	require.Equal(t, uint64(1), eng.stackPointer)
	// The loaded value must be incremented via x86.AINCL.
	require.Equal(t, original+1, uint32(eng.stack[eng.stackPointer-1]))
}

func TestAmd64Compiler_compileStore(t *testing.T) {
	for i, tp := range []wazeroir.UnsignedType{
		wazeroir.UnsignedTypeI32,
		wazeroir.UnsignedTypeI64,
		wazeroir.UnsignedTypeF32,
		wazeroir.UnsignedTypeF64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			eng := newEngine()
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()

			// Before store operations, we must push the base offset, and the store target values.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueOnStack()
			eng.stack[base.stackPointer] = baseOffset
			storeTargetValue := uint64(math.MaxUint64)
			storeTarget := compiler.locationStack.pushValueOnStack()
			eng.stack[storeTarget.stackPointer] = storeTargetValue
			switch tp {
			case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
				storeTarget.setRegisterType(generalPurposeRegisterTypeInt)
			case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
				storeTarget.setRegisterType(generalPurposeRegisterTypeFloat)
			}

			// Emit the memory load instructions.
			o := &wazeroir.OperationStore{Type: tp, Arg: &wazeroir.MemoryImmediate{Offest: 361}}
			err := compiler.compileStore(o)
			require.NoError(t, err)

			// At this point, two values are popped so the stack pointer must be zero.
			require.Equal(t, uint64(0), compiler.locationStack.sp)
			// Plus there should be no used registers.
			require.Len(t, compiler.locationStack.usedRegisters, 0)

			// Generate the code under test.
			compiler.returnFunction()
			code, _, err := compiler.generate()
			require.NoError(t, err)

			// Run code.
			mem := newMemoryInst()
			eng.memroySliceLen = len(mem.Buffer)
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// All the values are popped, so the stack pointer must be zero.
			require.Equal(t, uint64(0), eng.stackPointer)
			// Check the stored value.
			offset := o.Arg.Offest + baseOffset
			switch o.Type {
			case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
				v := binary.LittleEndian.Uint32(mem.Buffer[offset : offset+4])
				require.Equal(t, uint32(storeTargetValue), v)
				// The trailing bytes must be intact since this is 32-bit mov.
				v = binary.LittleEndian.Uint32(mem.Buffer[offset+4 : offset+8])
				require.Equal(t, uint32(0), v)
			case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
				v := binary.LittleEndian.Uint64(mem.Buffer[offset : offset+8])
				require.Equal(t, storeTargetValue, v)
			}
		})
	}
}

func TestAmd64Compiler_compileStore8(t *testing.T) {
	eng := newEngine()
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()

	// Before store operations, we must push the base offset, and the store target values.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueOnStack()
	eng.stack[base.stackPointer] = baseOffset
	storeTargetValue := uint64(0x12_34_56_78_9a_bc_ef_01) // For testing. Arbitrary number is fine.
	storeTarget := compiler.locationStack.pushValueOnStack()
	eng.stack[storeTarget.stackPointer] = storeTargetValue
	storeTarget.setRegisterType(generalPurposeRegisterTypeInt)

	// Emit the memory load instructions.
	o := &wazeroir.OperationStore8{Arg: &wazeroir.MemoryImmediate{Offest: 361}}
	err := compiler.compileStore8(o)
	require.NoError(t, err)

	// At this point, two values are popped so the stack pointer must be zero.
	require.Equal(t, uint64(0), compiler.locationStack.sp)
	// Plus there should be no used registers.
	require.Len(t, compiler.locationStack.usedRegisters, 0)

	// Generate the code under test.
	compiler.returnFunction()
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	mem := newMemoryInst()
	eng.memroySliceLen = len(mem.Buffer)
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)

	// All the values are popped, so the stack pointer must be zero.
	require.Equal(t, uint64(0), eng.stackPointer)
	// Check the stored value.
	offset := o.Arg.Offest + baseOffset
	require.Equal(t, byte(storeTargetValue), mem.Buffer[offset])
	// The trailing bytes must be intact since this is only moving one byte.
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0}, mem.Buffer[offset+1:offset+8])
}

func TestAmd64Compiler_compileStore16(t *testing.T) {
	eng := newEngine()
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()

	// Before store operations, we must push the base offset, and the store target values.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueOnStack()
	eng.stack[base.stackPointer] = baseOffset
	storeTargetValue := uint64(0x12_34_56_78_9a_bc_ef_01) // For testing. Arbitrary number is fine.
	storeTarget := compiler.locationStack.pushValueOnStack()
	eng.stack[storeTarget.stackPointer] = storeTargetValue
	storeTarget.setRegisterType(generalPurposeRegisterTypeInt)

	// Emit the memory load instructions.
	o := &wazeroir.OperationStore16{Arg: &wazeroir.MemoryImmediate{Offest: 361}}
	err := compiler.compileStore16(o)
	require.NoError(t, err)

	// At this point, two values are popped so the stack pointer must be zero.
	require.Equal(t, uint64(0), compiler.locationStack.sp)
	// Plus there should be no used registers.
	require.Len(t, compiler.locationStack.usedRegisters, 0)

	// Generate the code under test.
	compiler.returnFunction()
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	mem := newMemoryInst()
	eng.memroySliceLen = len(mem.Buffer)
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)

	// All the values are popped, so the stack pointer must be zero.
	require.Equal(t, uint64(0), eng.stackPointer)
	// Check the stored value.
	offset := o.Arg.Offest + baseOffset
	require.Equal(t, uint16(storeTargetValue), binary.LittleEndian.Uint16(mem.Buffer[offset:]))
	// The trailing bytes must be intact since this is only moving 2 byte.
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0}, mem.Buffer[offset+2:offset+8])
}

func TestAmd64Compiler_compileStore32(t *testing.T) {
	eng := newEngine()
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()

	// Before store operations, we must push the base offset, and the store target values.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueOnStack()
	eng.stack[base.stackPointer] = baseOffset
	storeTargetValue := uint64(0x12_34_56_78_9a_bc_ef_01) // For testing. Arbitrary number is fine.
	storeTarget := compiler.locationStack.pushValueOnStack()
	eng.stack[storeTarget.stackPointer] = storeTargetValue
	storeTarget.setRegisterType(generalPurposeRegisterTypeInt)

	// Emit the memory load instructions.
	o := &wazeroir.OperationStore32{Arg: &wazeroir.MemoryImmediate{Offest: 361}}
	err := compiler.compileStore32(o)
	require.NoError(t, err)

	// At this point, two values are popped so the stack pointer must be zero.
	require.Equal(t, uint64(0), compiler.locationStack.sp)
	// Plus there should be no used registers.
	require.Len(t, compiler.locationStack.usedRegisters, 0)

	// Generate the code under test.
	compiler.returnFunction()
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	mem := newMemoryInst()
	eng.memroySliceLen = len(mem.Buffer)
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)

	// All the values are popped, so the stack pointer must be zero.
	require.Equal(t, uint64(0), eng.stackPointer)
	// Check the stored value.
	offset := o.Arg.Offest + baseOffset
	require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem.Buffer[offset:]))
	// The trailing bytes must be intact since this is only moving 4 byte.
	require.Equal(t, []byte{0, 0, 0, 0}, mem.Buffer[offset+4:offset+8])
}

func TestAmd64Compiler_compileMemoryGrow(t *testing.T) {
	compiler := requireNewCompiler(t)

	compiler.initializeReservedRegisters()
	// Emit memory.grow instructions.
	compiler.compileMemoryGrow()

	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	eng := newEngine()
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	require.Equal(t, jitCallStatusCodeCallBuiltInFunction, eng.jitCallStatusCode)
	require.Equal(t, int64(builtinFunctionIndexMemoryGrow), eng.functionCallIndex)
}

func TestAmd64Compiler_compileMemorySize(t *testing.T) {
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	// Emit memory.size instructions.
	compiler.compileMemorySize()
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), compiler.locationStack.sp)
	require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().registerType())

	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)

	// Run code.
	eng := newEngine()
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	require.Equal(t, jitCallStatusCodeCallBuiltInFunction, eng.jitCallStatusCode)
	require.Equal(t, int64(builtinFunctionIndexMemorySize), eng.functionCallIndex)
}

func TestAmd64Compiler_compileDrop(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		compiler := requireNewCompiler(t)
		err := compiler.compileDrop(&wazeroir.OperationDrop{})
		require.NoError(t, err)
	})
	t.Run("zero start", func(t *testing.T) {
		compiler := requireNewCompiler(t)
		shouldPeek := compiler.locationStack.pushValueOnStack()
		const numReg = 10
		for i := int16(0); i < numReg; i++ {
			compiler.locationStack.pushValueOnRegister(i)
		}
		err := compiler.compileDrop(&wazeroir.OperationDrop{
			Range: &wazeroir.InclusiveRange{Start: 0, End: numReg - 1},
		})
		require.NoError(t, err)
		for i := int16(0); i < numReg; i++ {
			require.NotContains(t, compiler.locationStack.usedRegisters, i)
		}
		actualPeek := compiler.locationStack.peek()
		require.Equal(t, shouldPeek, actualPeek)
	})
	t.Run("live all on register", func(t *testing.T) {
		const (
			numLive = 3
			dropNum = 5
		)
		compiler := requireNewCompiler(t)
		shouldBottom := compiler.locationStack.pushValueOnStack()
		for i := int16(0); i < dropNum; i++ {
			compiler.locationStack.pushValueOnRegister(i)
		}
		for i := int16(dropNum); i < numLive+dropNum; i++ {
			compiler.locationStack.pushValueOnRegister(i)
		}
		err := compiler.compileDrop(&wazeroir.OperationDrop{
			Range: &wazeroir.InclusiveRange{Start: numLive, End: numLive + dropNum - 1},
		})
		require.NoError(t, err)
		for i := int16(0); i < dropNum; i++ {
			require.NotContains(t, compiler.locationStack.usedRegisters, i)
		}
		for i := int16(dropNum); i < numLive+dropNum; i++ {
			require.Contains(t, compiler.locationStack.usedRegisters, i)
		}
		for i := int16(0); i < numLive; i++ {
			actual := compiler.locationStack.pop()
			require.True(t, actual.onRegister())
			require.Equal(t, numLive+dropNum-1-i, actual.register)
		}
		require.Equal(t, uint64(1), compiler.locationStack.sp)
		require.Equal(t, shouldBottom, compiler.locationStack.pop())
	})
	t.Run("live on stack", func(t *testing.T) {
		// This is for testing all the edge cases with fake registers.
		t.Run("fake registers", func(t *testing.T) {
			const (
				numLive        = 3
				dropNum        = 5
				liveRegisterID = 10
			)
			compiler := requireNewCompiler(t)
			bottom := compiler.locationStack.pushValueOnStack()
			require.Equal(t, uint64(0), compiler.locationStack.stack[0].stackPointer)
			for i := int16(0); i < dropNum; i++ {
				compiler.locationStack.pushValueOnRegister(i)
			}
			// The bottom live value is on the stack.
			bottomLive := compiler.locationStack.pushValueOnStack()
			// The second live value is on the register.
			LiveRegister := compiler.locationStack.pushValueOnRegister(liveRegisterID)
			// The top live value is on the conditional.
			topLive := compiler.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateAE)
			require.True(t, topLive.onConditionalRegister())
			err := compiler.compileDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: numLive, End: numLive + dropNum - 1},
			})
			require.Equal(t, uint64(0), compiler.locationStack.stack[0].stackPointer)
			require.NoError(t, err)
			require.Equal(t, uint64(4), compiler.locationStack.sp)
			for i := int16(0); i < dropNum; i++ {
				require.NotContains(t, compiler.locationStack.usedRegisters, i)
			}
			// Top value should be on the register.
			actualTopLive := compiler.locationStack.pop()
			require.True(t, actualTopLive.onRegister() && !actualTopLive.onConditionalRegister())
			require.Equal(t, topLive, actualTopLive)
			// Second one should be on the same register.
			actualLiveRegister := compiler.locationStack.pop()
			require.Equal(t, LiveRegister, actualLiveRegister)
			// The bottom live value should be moved onto the stack.
			actualBottomLive := compiler.locationStack.pop()
			require.Equal(t, bottomLive, actualBottomLive)
			require.True(t, actualBottomLive.onRegister() && !actualBottomLive.onStack())
			// The bottom after drop should stay on stack.
			actualBottom := compiler.locationStack.pop()
			require.Equal(t, uint64(0), compiler.locationStack.stack[0].stackPointer)
			require.Equal(t, bottom, actualBottom)
			require.True(t, bottom.onStack())
		})
		t.Run("real", func(t *testing.T) {
			eng := newEngine()
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			bottom := compiler.locationStack.pushValueOnRegister(x86.REG_R10)
			compiler.locationStack.pushValueOnRegister(x86.REG_R9)
			top := compiler.locationStack.pushValueOnStack()
			eng.stack[top.stackPointer] = 5000
			compiler.movIntConstToRegister(300, bottom.register)
			err := compiler.compileDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: 1, End: 1},
			})
			require.NoError(t, err)
			compiler.releaseRegisterToStack(bottom)
			compiler.releaseRegisterToStack(top)
			compiler.returnFunction()
			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			require.Equal(t, []uint64{0, 0, 5000}, eng.stack[:3])
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.stackPointer)
			require.Equal(t, []uint64{
				300,
				5000, // top value should be moved to the dropped position.
			}, eng.stack[:eng.stackPointer])
		})
	})
}

func TestAmd64Compiler_releaseAllRegistersToStack(t *testing.T) {
	eng := newEngine()
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	x1Reg := int16(x86.REG_AX)
	x2Reg := int16(x86.REG_R10)
	_ = compiler.locationStack.pushValueOnStack()
	eng.stack[0] = 100
	compiler.locationStack.pushValueOnRegister(x1Reg)
	compiler.locationStack.pushValueOnRegister(x2Reg)
	_ = compiler.locationStack.pushValueOnStack()
	eng.stack[3] = 123
	require.Len(t, compiler.locationStack.usedRegisters, 2)

	// Set the values supposed to be released to stack memory space.
	compiler.movIntConstToRegister(300, x1Reg)
	compiler.movIntConstToRegister(51, x2Reg)
	compiler.releaseAllRegistersToStack()
	require.Len(t, compiler.locationStack.usedRegisters, 0)
	compiler.returnFunction()

	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)
	// Run code.
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the stack.
	require.Equal(t, uint64(4), eng.stackPointer)
	require.Equal(t, uint64(123), eng.stack[eng.stackPointer-1])
	require.Equal(t, uint64(51), eng.stack[eng.stackPointer-2])
	require.Equal(t, uint64(300), eng.stack[eng.stackPointer-3])
	require.Equal(t, uint64(100), eng.stack[eng.stackPointer-4])
}

func TestAmd64Compiler_assemble(t *testing.T) {
	compiler := requireNewCompiler(t)
	compiler.setContinuationOffsetAtNextInstructionAndReturn()
	prog := compiler.newProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x86.REG_R10
	compiler.addInstruction(prog)
	code, _, err := compiler.generate()
	require.NoError(t, err)
	actual := binary.LittleEndian.Uint64(code[2:10])
	require.Equal(t, uint64(prog.Pc), actual)
}

func TestAmd64Compiler_compileUnreachable(t *testing.T) {
	compiler := requireNewCompiler(t)
	compiler.initializeReservedRegisters()
	x1Reg := int16(x86.REG_AX)
	x2Reg := int16(x86.REG_R10)
	compiler.locationStack.pushValueOnRegister(x1Reg)
	compiler.locationStack.pushValueOnRegister(x2Reg)
	compiler.movIntConstToRegister(300, x1Reg)
	compiler.movIntConstToRegister(51, x2Reg)
	compiler.compileUnreachable()

	// Generate the code under test.
	code, _, err := compiler.generate()
	require.NoError(t, err)
	// Run code.
	eng := newEngine()
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)

	// Check the jitCallStatus of engine.
	require.Equal(t, jitCallStatusCodeUnreachable, eng.jitCallStatusCode)
	// All the values on registers must be written back to stack.
	require.Equal(t, uint64(300), eng.stack[0])
	require.Equal(t, uint64(51), eng.stack[1])
}

func TestAmd64Compiler_compileSelect(t *testing.T) {
	// There are mainly 8 cases we have to test:
	// - [x1 = reg, x2 = reg] select x1
	// - [x1 = reg, x2 = reg] select x2
	// - [x1 = reg, x2 = stack] select x1
	// - [x1 = reg, x2 = stack] select x2
	// - [x1 = stack, x2 = reg] select x1
	// - [x1 = stack, x2 = reg] select x2
	// - [x1 = stack, x2 = stack] select x1
	// - [x1 = stack, x2 = stack] select x2
	// And for each case, we have to test with
	// three conditional value location: stack, gp register, conditional register.
	// So in total we have 24 cases.
	const x1Value, x2Value = 100, 200
	for i, tc := range []struct {
		x1OnRegister, x2OnRegister                                        bool
		selectX1                                                          bool
		condlValueOnStack, condValueOnGPRegister, condValueOnCondRegister bool
	}{
		// Conditional value on stack.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condlValueOnStack: true},
		// Conditional value on register.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condValueOnGPRegister: true},
		// Conditional value on conditional register.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condValueOnCondRegister: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			compiler := requireNewCompiler(t)
			eng := newEngine()
			compiler.initializeReservedRegisters()
			var x1, x2, c *valueLocation
			if tc.x1OnRegister {
				x1 = compiler.locationStack.pushValueOnRegister(x86.REG_AX)
				compiler.movIntConstToRegister(x1Value, x1.register)
			} else {
				x1 = compiler.locationStack.pushValueOnStack()
				eng.stack[x1.stackPointer] = x1Value
			}
			if tc.x2OnRegister {
				x2 = compiler.locationStack.pushValueOnRegister(x86.REG_R10)
				compiler.movIntConstToRegister(x2Value, x2.register)
			} else {
				x2 = compiler.locationStack.pushValueOnStack()
				eng.stack[x2.stackPointer] = x2Value
			}
			if tc.condlValueOnStack {
				c = compiler.locationStack.pushValueOnStack()
				if tc.selectX1 {
					eng.stack[c.stackPointer] = 1
				} else {
					eng.stack[c.stackPointer] = 0
				}
			} else if tc.condValueOnGPRegister {
				c = compiler.locationStack.pushValueOnRegister(x86.REG_R9)
				if tc.selectX1 {
					compiler.movIntConstToRegister(1, c.register)
				} else {
					compiler.movIntConstToRegister(0, c.register)
				}
			} else if tc.condValueOnCondRegister {
				compiler.movIntConstToRegister(0, x86.REG_CX)
				cmp := compiler.newProg()
				cmp.As = x86.ACMPQ
				cmp.From.Type = obj.TYPE_REG
				cmp.From.Reg = x86.REG_CX
				cmp.To.Type = obj.TYPE_CONST
				if tc.selectX1 {
					cmp.To.Offset = 0
				} else {
					cmp.To.Offset = 1
				}
				compiler.addInstruction(cmp)
				compiler.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateE)
			}

			// Now emit code for select.
			err := compiler.compileSelect()
			require.NoError(t, err)
			// The code generation should not affect the x1's placement in any case.
			require.Equal(t, tc.x1OnRegister, x1.onRegister())
			// Plus x1 is top of the stack.
			require.Equal(t, x1, compiler.locationStack.peek())

			// Now write back the x1 to the memory if it is on a register.
			if tc.x1OnRegister {
				compiler.releaseRegisterToStack(x1)
			}
			compiler.returnFunction()

			// Run code.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Check the selected value.
			require.Equal(t, uint64(1), eng.stackPointer)
			if tc.selectX1 {
				require.Equal(t, eng.stack[x1.stackPointer], uint64(x1Value))
			} else {
				require.Equal(t, eng.stack[x1.stackPointer], uint64(x2Value))
			}
		})
	}
}

func TestAmd64Compiler_compileSwap(t *testing.T) {
	var x1Value, x2Value int64 = 100, 200
	for i, tc := range []struct {
		x1OnConditionalRegister, x1OnRegister, x2OnRegister bool
	}{
		{x1OnRegister: true, x2OnRegister: true},
		{x1OnRegister: true, x2OnRegister: false},
		{x1OnRegister: false, x2OnRegister: true},
		{x1OnRegister: false, x2OnRegister: false},
		// x1 on conditional register
		{x1OnConditionalRegister: true, x2OnRegister: false},
		{x1OnConditionalRegister: true, x2OnRegister: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			eng := newEngine()
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			if tc.x2OnRegister {
				x2 := compiler.locationStack.pushValueOnRegister(x86.REG_R10)
				compiler.movIntConstToRegister(x2Value, x2.register)
			} else {
				x2 := compiler.locationStack.pushValueOnStack()
				eng.stack[x2.stackPointer] = uint64(x2Value)
			}
			_ = compiler.locationStack.pushValueOnStack() // Dummy value!
			if tc.x1OnRegister && !tc.x1OnConditionalRegister {
				x1 := compiler.locationStack.pushValueOnRegister(x86.REG_AX)
				compiler.movIntConstToRegister(x1Value, x1.register)
			} else if !tc.x1OnConditionalRegister {
				x1 := compiler.locationStack.pushValueOnStack()
				eng.stack[x1.stackPointer] = uint64(x1Value)
			} else {
				compiler.movIntConstToRegister(0, x86.REG_AX)
				cmp := compiler.newProg()
				cmp.As = x86.ACMPQ
				cmp.From.Type = obj.TYPE_REG
				cmp.From.Reg = x86.REG_AX
				cmp.To.Type = obj.TYPE_CONST
				cmp.To.Offset = 0
				cmp.To.Offset = 0
				compiler.addInstruction(cmp)
				compiler.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateE)
				x1Value = 1
			}

			// Swap x1 and x2.
			err := compiler.compileSwap(&wazeroir.OperationSwap{Depth: 2})
			require.NoError(t, err)
			// To verify the behavior, we release all the registers to stack locations.
			compiler.releaseAllRegistersToStack()
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			require.Equal(t, uint64(3), eng.stackPointer)
			// Check values are swapped.
			require.Equal(t, uint64(x1Value), eng.stack[0])
			require.Equal(t, uint64(x2Value), eng.stack[2])
		})
	}
}

// TestGlobalInstanceValueOffset ensures the globalInstanceValueOffset doesn't drift when we modify the struct (wasm.GlobalInstance).
func TestGlobalInstanceValueOffset(t *testing.T) {
	require.Equal(t, int(unsafe.Offsetof((&wasm.GlobalInstance{}).Val)), globalInstanceValueOffset)
}

func TestAmd64Compiler_compileGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			// Setup the globals.
			compiler := requireNewCompiler(t)
			globals := []*wasm.GlobalInstance{nil, {Val: globalValue, Type: &wasm.GlobalType{ValType: tp}}, nil}
			compiler.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{Globals: globals}}
			// Emit the code.
			compiler.initializeReservedRegisters()
			op := &wazeroir.OperationGlobalGet{Index: 1}
			err := compiler.compileGlobalGet(op)
			require.NoError(t, err)

			// At this point, the top of stack must be the retrieved global on a register.
			global := compiler.locationStack.peek()
			require.True(t, global.onRegister())
			require.Len(t, compiler.locationStack.usedRegisters, 1)
			switch tp {
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				require.True(t, isFloatRegister(global.register))
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				require.True(t, isIntRegister(global.register))
			}
			compiler.releaseAllRegistersToStack()
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)

			// Run the code assembled above.
			eng := newEngine()
			eng.globalSliceAddress = uintptr(unsafe.Pointer(&globals[0]))
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Since we call global.get, the top of the stack must be the global value.
			require.Equal(t, globalValue, eng.stack[0])
			// Plus as we push the value, the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.stackPointer)
		})
	}
}

func TestAmd64Compiler_compileGlobalSet(t *testing.T) {
	const valueToSet uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			// Setup the globals.
			compiler := requireNewCompiler(t)
			globals := []*wasm.GlobalInstance{nil, {Val: 40, Type: &wasm.GlobalType{ValType: tp}}, nil}
			compiler.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{Globals: globals}}
			_ = compiler.locationStack.pushValueOnStack() // where we place the set target value below.
			// Now emit the code.
			compiler.initializeReservedRegisters()
			op := &wazeroir.OperationGlobalSet{Index: 1}
			err := compiler.compileGlobalSet(op)
			require.NoError(t, err)
			compiler.returnFunction()

			// Generate the code under test.
			code, _, err := compiler.generate()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			eng.globalSliceAddress = uintptr(unsafe.Pointer(&globals[0]))
			eng.push(valueToSet)
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// The global value should be set to valueToSet.
			require.Equal(t, valueToSet, globals[op.Index].Val)
			// Plus we consumed the top of the stack, the stack pointer must be decremented.
			require.Equal(t, uint64(0), eng.stackPointer)
		})
	}
}
