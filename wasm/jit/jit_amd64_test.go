//go:build amd64
// +build amd64

package jit

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"reflect"
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

func newMemoryInst() *wasm.MemoryInstance {
	return &wasm.MemoryInstance{Buffer: make([]byte, 1024)}
}

func requireNewCompiler(t *testing.T) *amd64Compiler {
	b, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	return &amd64Compiler{eng: nil, builder: b,
		locationStack:            newValueLocationStack(),
		onLabelStartCallbacks:    map[string][]func(*obj.Prog){},
		labelInitialInstructions: map[string]*obj.Prog{},
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
		ParamTypes: []wasm.ValueType{wasm.ValueTypeF64, wasm.ValueTypeI32},
	}}
	compiler := &amd64Compiler{locationStack: newValueLocationStack(), f: f}
	compiler.pushFunctionParams()
	require.Equal(t, uint64(len(f.Signature.ParamTypes)), compiler.locationStack.sp)
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
	// Compile.
	code, _, err := compiler.compile()
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

			// Compile.
			code, _, err := compiler.compile()
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
			// Compile.
			code, _, err := compiler.compile()
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
		// Compile.
		code, _, err := compiler.compile()
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
		// Compile.
		code, _, err := compiler.compile()
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
	// Compile.
	code, _, err := compiler.compile()
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
		// Compile.
		code, _, err := compiler.compile()
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
		// Compile.
		code, _, err := compiler.compile()
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
		// Compile.
		code, _, err := compiler.compile()
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
		// Compile.
		code, _, err := compiler.compile()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		hostFunc := reflect.ValueOf(func(ctx *wasm.HostFunctionCallContext, _, in uint64) uint64 {
			return in * 200
		})
		hostFunctionInstance := &wasm.FunctionInstance{
			HostFunction: &hostFunc,
			Signature: &wasm.FunctionType{
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI64},
				ResultTypes: []wasm.ValueType{wasm.ValueTypeI64},
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
	// Compile.
	code, _, err := compiler.compile()
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

	// Assemble.
	code, _, err := compiler.compile()
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

		// Assemble.
		code, _, err := compiler.compile()
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

	var called bool
	compiler.onLabelStartCallbacks[label.String()] = append(compiler.onLabelStartCallbacks[label.String()],
		func(p *obj.Prog) { called = true },
	)
	err := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
	require.NoError(t, err)
	require.Len(t, compiler.onLabelStartCallbacks, 0)
	require.Contains(t, compiler.labelInitialInstructions, label.String())
	require.True(t, called)

	// Assemble.
	compiler.returnFunction()
	code, _, err := compiler.compile()
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

		// Assemble.
		code, _, err := compiler.compile()
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

		// Assemble.
		code, _, err := compiler.compile()
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

			// Assemble.
			code, _, err := compiler.compile()
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

			// Assemble.
			code, _, err := compiler.compile()
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

			// Assemble.
			code, _, err := compiler.compile()
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
			require.Equal(t, o.Value*2, math.Float32frombits(uint32(eng.stack[eng.stackPointer-1])))
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

			// Assemble.
			code, _, err := compiler.compile()
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
			require.Equal(t, o.Value*2, math.Float64frombits(eng.stack[eng.stackPointer-1]))
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

		// Assemble.
		code, _, err := compiler.compile()
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

		// Assemble.
		code, _, err := compiler.compile()
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

				// Assemble.
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.v1+tc.v2, math.Float32frombits(uint32(eng.stack[eng.stackPointer-1])))
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

				// Assemble.
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.v1+tc.v2, math.Float64frombits(eng.stack[eng.stackPointer-1]))
			})
		}
	})
}

func TestAmd64Compiler_compileLe(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, tc := range []struct {
			x1, x2 int32
			signed bool
		}{
			{x1: 100, x2: -1, signed: false},
			{x1: -1, x2: -1, signed: false},
			{x1: -1, x2: 100, signed: false},
			{x1: 100, x2: 200, signed: true},
			{x1: 100, x2: 100, signed: true},
			{x1: 200, x2: 100, signed: true},
		} {
			var o *wazeroir.OperationLe
			if tc.signed {
				o = &wazeroir.OperationLe{Type: wazeroir.SignedTypeInt32}
			} else {
				o = &wazeroir.OperationLe{Type: wazeroir.SignedTypeUint32}
			}
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			x1Reg := int16(x86.REG_R9)
			x2Reg := int16(x86.REG_R10)
			compiler.locationStack.pushValueOnRegister(x1Reg)
			compiler.locationStack.pushValueOnRegister(x2Reg)
			compiler.movIntConstToRegister(int64(tc.x1), x1Reg)
			compiler.movIntConstToRegister(int64(tc.x2), x2Reg)
			err := compiler.compileLe(o)
			require.NoError(t, err)

			require.NotContains(t, compiler.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, compiler.locationStack.usedRegisters, x2Reg)
			// To verify the behavior, we push the flag value
			// to the stack.
			top := compiler.locationStack.peek()
			require.True(t, top.onConditionalRegister() && !top.onRegister())
			err = compiler.moveConditionalToGeneralPurposeRegister(top)
			require.NoError(t, err)
			require.True(t, !top.onConditionalRegister() && top.onRegister())
			compiler.releaseRegisterToStack(top)
			compiler.returnFunction()

			// Assemble.
			code, _, err := compiler.compile()
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
			if tc.signed {
				require.Equal(t, int32(tc.x1) <= int32(tc.x2), eng.stack[eng.stackPointer-1] == 1)
			} else {
				require.Equal(t, uint32(tc.x1) <= uint32(tc.x2), eng.stack[eng.stackPointer-1] == 1)
			}
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, tc := range []struct {
			x1, x2 int64
			signed bool
		}{
			{x1: 100, x2: -1, signed: false},
			{x1: -1, x2: 100, signed: false},
			{x1: 100, x2: 200, signed: true},
			{x1: 200, x2: 100, signed: true},
			{x1: 1 << 56, x2: 100, signed: true},
			{x1: 1 << 56, x2: 1 << 61, signed: true},
			{x1: math.MaxInt64, x2: 100, signed: true},
			{x1: math.MinInt64, x2: 100, signed: true},
		} {
			var o *wazeroir.OperationLe
			if tc.signed {
				o = &wazeroir.OperationLe{Type: wazeroir.SignedTypeInt64}
			} else {
				o = &wazeroir.OperationLe{Type: wazeroir.SignedTypeUint64}
			}
			compiler := requireNewCompiler(t)
			compiler.initializeReservedRegisters()
			x1Reg := int16(x86.REG_R9)
			x2Reg := int16(x86.REG_R10)
			compiler.locationStack.pushValueOnRegister(x1Reg)
			compiler.locationStack.pushValueOnRegister(x2Reg)
			compiler.movIntConstToRegister(tc.x1, x1Reg)
			compiler.movIntConstToRegister(tc.x2, x2Reg)
			err := compiler.compileLe(o)
			require.NoError(t, err)
			require.NotContains(t, compiler.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, compiler.locationStack.usedRegisters, x2Reg)
			// To verify the behavior, we push the flag value
			// to the stack.
			top := compiler.locationStack.peek()
			require.True(t, top.onConditionalRegister() && !top.onRegister())
			err = compiler.moveConditionalToGeneralPurposeRegister(top)
			require.NoError(t, err)
			require.True(t, !top.onConditionalRegister() && top.onRegister())
			compiler.releaseRegisterToStack(top)
			compiler.returnFunction()

			// Assemble.
			code, _, err := compiler.compile()
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
			if tc.signed {
				require.Equal(t, tc.x1 <= tc.x2, eng.stack[eng.stackPointer-1] == 1)
			} else {
				require.Equal(t, uint64(tc.x1) <= uint64(tc.x2), eng.stack[eng.stackPointer-1] == 1)
			}
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

				// Emit the Le instructions,
				err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat32})
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
				err = compiler.moveConditionalToGeneralPurposeRegister(flag)
				require.NoError(t, err)
				require.True(t, !flag.onConditionalRegister() && flag.onRegister())
				compiler.releaseRegisterToStack(flag)
				compiler.returnFunction()

				// Assemble.
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.x1 <= tc.x2, eng.stack[eng.stackPointer-1] == 1)
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

				// Emit the Le instructions,
				err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat64})
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
				err = compiler.moveConditionalToGeneralPurposeRegister(flag)
				require.NoError(t, err)
				require.True(t, !flag.onConditionalRegister() && flag.onRegister())
				compiler.releaseRegisterToStack(flag)
				compiler.returnFunction()

				// Assemble.
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.x1 <= tc.x2, eng.stack[eng.stackPointer-1] == 1)
			})
		}
	})
}

func TestAmd64Compiler_compileGe(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 int32
			signed bool
		}{
			{x1: 100, x2: -1, signed: false},
			{x1: -1, x2: -1, signed: false},
			{x1: -1, x2: 100, signed: false},
			{x1: 100, x2: 200, signed: true},
			{x1: 100, x2: 100, signed: true},
			{x1: 200, x2: 100, signed: true},
		} {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := requireNewCompiler(t)
				compiler.initializeReservedRegisters()
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()
				var o *wazeroir.OperationGe
				if tc.signed {
					o = &wazeroir.OperationGe{Type: wazeroir.SignedTypeInt64}
				} else {
					o = &wazeroir.OperationGe{Type: wazeroir.SignedTypeUint64}
				}
				err = compiler.compileGe(o)
				require.NoError(t, err)

				require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
				// To verify the behavior, we push the flag value
				// to the stack.
				top := compiler.locationStack.peek()
				require.True(t, top.onConditionalRegister() && !top.onRegister())
				err = compiler.moveConditionalToGeneralPurposeRegister(top)
				require.NoError(t, err)
				require.True(t, !top.onConditionalRegister() && top.onRegister())
				compiler.releaseRegisterToStack(top)
				compiler.returnFunction()

				// Assemble.
				code, _, err := compiler.compile()
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
				if tc.signed {
					require.Equal(t, int32(tc.x1) >= int32(tc.x2), eng.stack[eng.stackPointer-1] == 1)
				} else {
					require.Equal(t, uint32(tc.x1) >= uint32(tc.x2), eng.stack[eng.stackPointer-1] == 1)
				}
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 int64
			signed bool
		}{
			{x1: 100, x2: -1, signed: false},
			{x1: -1, x2: 100, signed: false},
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
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()
				var o *wazeroir.OperationGe
				if tc.signed {
					o = &wazeroir.OperationGe{Type: wazeroir.SignedTypeInt64}
				} else {
					o = &wazeroir.OperationGe{Type: wazeroir.SignedTypeUint64}
				}
				err = compiler.compileGe(o)
				require.NoError(t, err)
				require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
				// To verify the behavior, we push the flag value
				// to the stack.
				top := compiler.locationStack.peek()
				require.True(t, top.onConditionalRegister() && !top.onRegister())
				err = compiler.moveConditionalToGeneralPurposeRegister(top)
				require.NoError(t, err)
				require.True(t, !top.onConditionalRegister() && top.onRegister())
				compiler.releaseRegisterToStack(top)
				compiler.returnFunction()

				// Assemble.
				code, _, err := compiler.compile()
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
				if tc.signed {
					require.Equal(t, tc.x1 >= tc.x2, eng.stack[eng.stackPointer-1] == 1)
				} else {
					require.Equal(t, uint64(tc.x1) >= uint64(tc.x2), eng.stack[eng.stackPointer-1] == 1)
				}
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

				// Emit the Ge instructions,
				err = compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeFloat32})
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
				err = compiler.moveConditionalToGeneralPurposeRegister(flag)
				require.NoError(t, err)
				require.True(t, !flag.onConditionalRegister() && flag.onRegister())
				compiler.releaseRegisterToStack(flag)
				compiler.returnFunction()

				// Generate the code under test (constants declaration and comparison)
				// and the verification code (moving the result to the stack so we can assert against it)
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.x1 >= tc.x2, eng.stack[eng.stackPointer-1] == 1)
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

				// Emit the Ge instructions,
				err = compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeFloat64})
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
				err = compiler.moveConditionalToGeneralPurposeRegister(flag)
				require.NoError(t, err)
				require.True(t, !flag.onConditionalRegister() && flag.onRegister())
				compiler.releaseRegisterToStack(flag)
				compiler.returnFunction()

				// Generate the code under test (constants declaration and comparison)
				// and the verification code (moving the result to the stack so we can assert against it)
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.x1 >= tc.x2, eng.stack[eng.stackPointer-1] == 1)
			})
		}
	})
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

		// Assemble.
		code, _, err := compiler.compile()
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

		// Assemble.
		code, _, err := compiler.compile()
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

				// Assemble.
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.v1-tc.v2, math.Float32frombits(uint32(eng.stack[eng.stackPointer-1])))
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

				// Assemble.
				code, _, err := compiler.compile()
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
				require.Equal(t, tc.v1-tc.v2, math.Float64frombits(eng.stack[eng.stackPointer-1]))
			})
		}
	})
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
		hostFuncInstance := &wasm.FunctionInstance{HostFunction: &hostFuncRefValue}
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

		// Compile.
		code, _, err := compiler.compile()
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
		wasmFuncInstance := &wasm.FunctionInstance{}
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

		// Compile.
		code, _, err := compiler.compile()
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

			// Compile.
			compiler.returnFunction()
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
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

			// Compile.
			compiler.returnFunction()
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
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

			// Compile.
			compiler.returnFunction()
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
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

	// Compile.
	compiler.returnFunction()
	code, _, err := compiler.compile()
	require.NoError(t, err)

	// Place the load target value to the memory.
	mem := newMemoryInst()
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

			// Compile.
			compiler.returnFunction()
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			mem := newMemoryInst()
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

	// Compile.
	compiler.returnFunction()
	code, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	mem := newMemoryInst()
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

	// Compile.
	compiler.returnFunction()
	code, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	mem := newMemoryInst()
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

	// Compile.
	compiler.returnFunction()
	code, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	mem := newMemoryInst()
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

	// Compile.
	code, _, err := compiler.compile()
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

	// Compile.
	code, _, err := compiler.compile()
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
			Range: &wazeroir.InclusiveRange{Start: 0, End: numReg},
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
			// Assemble.
			code, _, err := compiler.compile()
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

	// Assemble.
	code, _, err := compiler.compile()
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
	code, _, err := compiler.compile()
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

	// Assemble.
	code, _, err := compiler.compile()
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
			code, _, err := compiler.compile()
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

			// Assemble.
			code, _, err := compiler.compile()
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

			// Assemble.
			code, _, err := compiler.compile()
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

			// Assemble.
			code, _, err := compiler.compile()
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
