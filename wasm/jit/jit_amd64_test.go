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

func requireNewBuilder(t *testing.T) *amd64Builder {
	b, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	return &amd64Builder{eng: nil, builder: b,
		locationStack:            newValueLocationStack(),
		onLabelStartCallbacks:    map[string][]func(*obj.Prog){},
		labelInitialInstructions: map[string]*obj.Prog{},
	}
}

func (b *amd64Builder) movIntConstToRegister(val int64, targetRegister int16) *obj.Prog {
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = val
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	b.addInstruction(prog)
	return prog
}

func TestAmd64Builder_pushFunctionInputs(t *testing.T) {
	f := &wasm.FunctionInstance{Signature: &wasm.FunctionType{
		InputTypes: []wasm.ValueType{wasm.ValueTypeF64, wasm.ValueTypeI32},
	}}
	builder := &amd64Builder{locationStack: newValueLocationStack(), f: f}
	builder.pushFunctionInputs()
	require.Equal(t, uint64(len(f.Signature.InputTypes)), builder.locationStack.sp)
	loc := builder.locationStack.pop()
	require.Equal(t, uint64(1), loc.stackPointer)
	loc = builder.locationStack.pop()
	require.Equal(t, uint64(0), loc.stackPointer)
}

// Test engine.exec method on the resursive function calls.
func TestRecursiveFunctionCalls(t *testing.T) {
	eng := newEngine()
	const tmpReg = x86.REG_AX
	// Build a function that decrements top of stack,
	// and recursively call itself until the top value becomes zero,
	// and if the value becomes zero, add 5 onto the top and return.
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	// Setup the initial value.
	eng.stack[0] = 10 // We call recursively 10 times.
	loc := builder.locationStack.pushValueOnStack()
	builder.assignRegisterToValue(loc, tmpReg)
	require.Contains(t, builder.locationStack.usedRegisters, loc.register)
	// Decrement tha value.
	prog := builder.builder.NewProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = loc.register
	builder.addInstruction(prog)
	// Check if the value equals zero
	prog = builder.newProg()
	prog.As = x86.ACMPQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = loc.register
	prog.To.Type = obj.TYPE_CONST
	prog.To.Offset = 0
	builder.addInstruction(prog)
	// If zero, jump to ::End
	jmp := builder.newProg()
	jmp.As = x86.AJEQ
	jmp.To.Type = obj.TYPE_BRANCH
	builder.addInstruction(jmp)
	// If not zero, we call push back the value to the stack
	// and call itself recursively.
	builder.releaseRegisterToStack(loc)
	require.NotContains(t, builder.locationStack.usedRegisters, loc.register)
	builder.callFunctionFromConstIndex(0)
	// ::End
	// If zero, we return from this function after pushing 5.
	builder.assignRegisterToValue(loc, tmpReg)
	prog = builder.movIntConstToRegister(5, loc.register)
	jmp.To.SetTarget(prog) // the above mov instruction is the jump target of the JEQ.
	builder.releaseRegisterToStack(loc)
	builder.setJITStatus(jitCallStatusCodeReturned)
	builder.returnFunction()
	// Compile.
	code, err := builder.compile()
	require.NoError(t, err)
	// Setup engine.
	mem := newMemoryInst()
	compiledFunc := &compiledWasmFunction{codeSegment: code, memory: mem, inputs: 1, returns: 1}
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
			compiledFunc := &compiledWasmFunction{codeSegment: code, memory: mem, inputs: 1, returns: 1}
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
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			// Push consts to pushTargetRegister.
			_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
			loc := builder.locationStack.pushValueOnRegister(pushTargetRegister)
			builder.movIntConstToRegister(int64(targetValue), pushTargetRegister)
			// Push pushTargetRegister into the engine.stack[engine.sp].
			builder.releaseRegisterToStack(loc)
			// Finally increment the stack pointer and write it back to the eng.sp
			builder.returnFunction()

			// Compile.
			code, err := builder.compile()
			require.NoError(t, err)

			eng := newEngine()
			mem := newMemoryInst()

			f := &compiledWasmFunction{codeSegment: code, memory: mem}
			f.codeInitialAddress = uintptr(unsafe.Pointer(&f.codeSegment[0]))

			// Call into the function
			eng.exec(f)

			// Because we pushed the value, eng.sp must be incremented by 1
			if eng.currentStackPointer != 2 {
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
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			builder.setJITStatus(s)
			builder.returnFunction()
			// Compile.
			code, err := builder.compile()
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
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.setFunctionCallIndexFromConst(index)
		builder.returnFunction()
		// Compile.
		code, err := builder.compile()
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
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.movIntConstToRegister(index, reg)
		builder.setFunctionCallIndexFromRegister(reg)
		builder.returnFunction()
		// Compile.
		code, err := builder.compile()
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
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	builder.setContinuationOffsetAtNextInstructionAndReturn()
	exp := uintptr(len(builder.builder.Assemble()))
	// On the continuation, we have to setup the registers again.
	builder.initializeReservedRegisters()
	// The continuation after function calls.
	_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
	loc := builder.locationStack.pushValueOnRegister(tmpReg)
	builder.movIntConstToRegister(int64(50), tmpReg)
	builder.releaseRegisterToStack(loc)
	require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
	builder.setJITStatus(jitCallStatusCodeCallWasmFunction)
	builder.returnFunction()
	// Compile.
	code, err := builder.compile()
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
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.callFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(tmpReg)
		builder.movIntConstToRegister(int64(50), tmpReg)
		builder.releaseRegisterToStack(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		builder.returnFunction()
		// Compile.
		code, err := builder.compile()
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
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.movIntConstToRegister(functionIndex, tmpReg)
		builder.callFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(tmpReg)
		builder.movIntConstToRegister(int64(50), tmpReg)
		builder.releaseRegisterToStack(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		builder.returnFunction()
		// Compile.
		code, err := builder.compile()
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
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// Push the value onto stack.
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(tmpReg)
		builder.movIntConstToRegister(int64(50), tmpReg)
		builder.releaseRegisterToStack(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		builder.callHostFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		builder.setJITStatus(jitCallStatusCodeReturned)
		builder.returnFunction()
		// Compile.
		code, err := builder.compile()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		hostFunction := &compiledHostFunction{
			f: func(ctx *wasm.HostFunctionCallContext) {
				eng.stack[eng.currentStackPointer-1] *= 100
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
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// Push the value onto stack.
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(x86.REG_AX)
		builder.movIntConstToRegister(int64(50), tmpReg)
		builder.releaseRegisterToStack(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		// Set the function index
		builder.movIntConstToRegister(int64(1), tmpReg)
		builder.callHostFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		builder.setJITStatus(jitCallStatusCodeReturned)
		builder.returnFunction()
		// Compile.
		code, err := builder.compile()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		hostFunc := reflect.ValueOf(func(ctx *wasm.HostFunctionCallContext, _, in uint64) uint64 {
			return in * 200
		})
		hostFunctionInstance := &wasm.FunctionInstance{
			HostFunction: &hostFunc,
			Signature: &wasm.FunctionType{
				InputTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI64},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
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
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	// Create three values on the stack.
	builder.locationStack.pushValueOnStack()
	loc1 := builder.locationStack.pushValueOnRegister(targetRegister1)
	loc2 := builder.locationStack.pushValueOnRegister(targetRegister2)
	builder.assignRegisterToValue(loc1, targetRegister1)
	builder.assignRegisterToValue(loc2, targetRegister2)
	// Increment the popped value on the register.
	prog := builder.newProg()
	prog.As = x86.AADDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister1
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = targetRegister2
	// Now we used the two values so pop twice.
	builder.locationStack.pop()
	builder.locationStack.pop()
	// Ready to push the result location.
	result := builder.locationStack.pushValueOnRegister(targetRegister1)
	builder.addInstruction(prog)
	// Push it back to the stack.
	builder.releaseRegisterToStack(result)
	builder.returnFunction()
	// Compile.
	code, err := builder.compile()
	require.NoError(t, err)

	// Call in.
	eng := newEngine()
	eng.currentStackBasePointer = 1
	eng.stack[eng.currentStackBasePointer+2] = 10000
	eng.stack[eng.currentStackBasePointer+1] = 20000
	mem := newMemoryInst()
	require.Equal(t, []uint64{0, 20000, 10000}, eng.stack[eng.currentStackBasePointer:eng.currentStackBasePointer+3])
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the sp and value.
	require.Equal(t, uint64(2), eng.currentStackPointer)
	require.Equal(t, []uint64{0, 30000}, eng.stack[eng.currentStackBasePointer:eng.currentStackBasePointer+eng.currentStackPointer])
}

func TestAmd64Builder_initializeReservedRegisters(t *testing.T) {
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	builder.returnFunction()

	// Assemble.
	code, err := builder.compile()
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

func TestAmd64Builder_allocateRegister(t *testing.T) {
	t.Run("free", func(t *testing.T) {
		builder := requireNewBuilder(t)
		reg, err := builder.allocateRegister(generalPurposeRegisterTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		reg, err = builder.allocateRegister(generalPurposeRegisterTypeFloat)
		require.NoError(t, err)
		require.True(t, isFloatRegister(reg))
	})
	t.Run("steal", func(t *testing.T) {
		const stealTarget = x86.REG_AX
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// Use up all the Int regs.
		for _, r := range unreservedGeneralPurposeIntRegisters {
			builder.locationStack.markRegisterUsed(r)
		}
		stealTargetLocation := builder.locationStack.pushValueOnRegister(stealTarget)
		builder.movIntConstToRegister(int64(50), stealTargetLocation.register)
		require.Equal(t, int16(stealTarget), stealTargetLocation.register)
		require.True(t, stealTargetLocation.onRegister())
		reg, err := builder.allocateRegister(generalPurposeRegisterTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		require.False(t, stealTargetLocation.onRegister())

		// Create new value using the stolen register.
		loc := builder.locationStack.pushValueOnRegister(reg)
		builder.movIntConstToRegister(int64(2000), loc.register)
		builder.releaseRegisterToStack(loc)
		builder.returnFunction()

		// Assemble.
		code, err := builder.compile()
		require.NoError(t, err)

		// Run code.
		eng := newEngine()
		eng.currentStackBasePointer = 10
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)

		// Check the sp and value.
		require.Equal(t, uint64(2), eng.currentStackPointer)
		require.Equal(t, []uint64{50, 2000}, eng.stack[eng.currentStackBasePointer:eng.currentStackBasePointer+eng.currentStackPointer])
	})
}

func TestAmd64Builder_handleLabel(t *testing.T) {
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	label := &wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}

	var called bool
	builder.onLabelStartCallbacks[label.String()] = append(builder.onLabelStartCallbacks[label.String()],
		func(p *obj.Prog) { called = true },
	)
	err := builder.handleLabel(&wazeroir.OperationLabel{Label: label})
	require.NoError(t, err)
	require.Len(t, builder.onLabelStartCallbacks, 0)
	require.Contains(t, builder.labelInitialInstructions, label.String())
	require.True(t, called)

	// Assemble.
	builder.returnFunction()
	code, err := builder.compile()
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

func TestAmd64Builder_handlePick(t *testing.T) {
	o := &wazeroir.OperationPick{Depth: 1}
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	// The case when the original value is already in register.
	t.Run("on reg", func(t *testing.T) {
		// Set up the pick target original value.
		pickTargetLocation := builder.locationStack.pushValueOnRegister(int16(x86.REG_R10))
		pickTargetLocation.setValueType(wazeroir.SignLessTypeI32)
		builder.locationStack.pushValueOnStack() // Dummy value!
		builder.movIntConstToRegister(100, pickTargetLocation.register)
		// Now insert pick code.
		err := builder.handlePick(o)
		require.NoError(t, err)
		// Increment the picked value.
		pickedLocation := builder.locationStack.peek()
		require.True(t, pickedLocation.onRegister())
		require.NotEqual(t, pickedLocation.register, pickTargetLocation.register)
		prog := builder.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = pickedLocation.register
		builder.addInstruction(prog)
		// To verify the behavior, we push the incremented picked value
		// to the stack.
		builder.releaseRegisterToStack(pickedLocation)
		// Also write the original location back to the stack.
		builder.releaseRegisterToStack(pickTargetLocation)
		builder.returnFunction()

		// Assemble.
		code, err := builder.compile()
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
		require.Equal(t, uint64(3), eng.currentStackPointer)
		require.Equal(t, uint64(101), eng.stack[eng.currentStackPointer-1])
		require.Equal(t, uint64(100), eng.stack[eng.currentStackPointer-3])
	})
	// The case when the original value is in stack.
	t.Run("on stack", func(t *testing.T) {
		eng := newEngine()

		// Setup the original value.
		builder.locationStack.pushValueOnStack() // Dummy value!
		pickTargetLocation := builder.locationStack.pushValueOnStack()
		builder.locationStack.pushValueOnStack() // Dummy value!
		eng.currentStackPointer = 5
		eng.currentStackBasePointer = 1
		eng.stack[eng.currentStackBasePointer+pickTargetLocation.stackPointer] = 100

		// Now insert pick code.
		err := builder.handlePick(o)
		require.NoError(t, err)

		// Increment the picked value.
		pickedLocation := builder.locationStack.peek()
		prog := builder.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = pickedLocation.register
		builder.addInstruction(prog)

		// To verify the behavior, we push the incremented picked value
		// to the stack.
		builder.releaseRegisterToStack(pickedLocation)
		builder.returnFunction()

		// Assemble.
		code, err := builder.compile()
		require.NoError(t, err)
		// Run code.
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(100), eng.stack[eng.currentStackBasePointer+pickTargetLocation.stackPointer]) // Original value shouldn't be affected.
		require.Equal(t, uint64(3), eng.currentStackPointer)
		require.Equal(t, uint64(101), eng.stack[eng.currentStackBasePointer+eng.currentStackPointer-1])
	})
}

func TestAmd64Builder_handleConstI32(t *testing.T) {
	for _, v := range []uint32{1, 1 << 5, 1 << 31} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstI32{Value: v}
			err := builder.handleConstI32(o)
			require.NoError(t, err)

			// To verify the behavior, we increment and push the const value
			// to the stack.
			loc := builder.locationStack.peek()
			require.Equal(t, wazeroir.SignLessTypeI32, loc.valueType)
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			builder.addInstruction(prog)
			builder.releaseRegisterToStack(loc)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			// Check the value of the top on the stack equals the const plus one.
			require.Equal(t, uint64(o.Value)+1, eng.stack[eng.currentStackPointer-1])
		})
	}
}

func TestAmd64Builder_handleConstI64(t *testing.T) {
	for _, v := range []uint64{1, 1 << 5, 1 << 35, 1 << 63} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstI64{Value: v}
			err := builder.handleConstI64(o)
			require.NoError(t, err)

			// To verify the behavior, we increment and push the const value
			// to the stack.
			loc := builder.locationStack.peek()
			require.Equal(t, wazeroir.SignLessTypeI64, loc.valueType)
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			builder.addInstruction(prog)
			builder.releaseRegisterToStack(loc)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			// Check the value of the top on the stack equals the const plus one.
			require.Equal(t, o.Value+1, eng.stack[eng.currentStackPointer-1])
		})
	}
}

func TestAmd64Builder_handleConstF32(t *testing.T) {
	for _, v := range []float32{1, -3.23, 100.123} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstF32{Value: v}
			err := builder.handleConstF32(o)
			require.NoError(t, err)

			// To verify the behavior, we double and push the const value
			// to the stack.
			loc := builder.locationStack.peek()
			require.Equal(t, wazeroir.SignLessTypeF32, loc.valueType)
			prog := builder.newProg()
			prog.As = x86.AADDSS
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = loc.register
			builder.addInstruction(prog)
			builder.releaseRegisterToStack(loc)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			// Check the value of the top on the stack equals the squared const.
			require.Equal(t, o.Value*2, math.Float32frombits(uint32(eng.stack[eng.currentStackPointer-1])))
		})
	}
}

func TestAmd64Builder_handleConstF64(t *testing.T) {
	for _, v := range []float64{1, -3.23, 100.123} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()

			// Now emit the const instruction.
			o := &wazeroir.OperationConstF64{Value: v}
			err := builder.handleConstF64(o)
			require.NoError(t, err)

			// To verify the behavior, we double and push the const value
			// to the stack.
			loc := builder.locationStack.peek()
			require.Equal(t, wazeroir.SignLessTypeF64, loc.valueType)
			prog := builder.newProg()
			prog.As = x86.AADDSD
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = loc.register
			builder.addInstruction(prog)
			builder.releaseRegisterToStack(loc)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			// Check the value of the top on the stack equals the squared const.
			require.Equal(t, o.Value*2, math.Float64frombits(eng.stack[eng.currentStackPointer-1]))
		})
	}
}

func TestAmd64Builder_handleAdd(t *testing.T) {
	t.Run("int64", func(t *testing.T) {
		o := &wazeroir.OperationAdd{Type: wazeroir.SignLessTypeI64}
		t.Run("x1:reg,x2:reg", func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			x1Reg := int16(x86.REG_AX)
			x2Reg := int16(x86.REG_R10)
			x1Location := builder.locationStack.pushValueOnRegister(x1Reg)
			builder.locationStack.pushValueOnRegister(x2Reg)
			builder.movIntConstToRegister(100, x1Reg)
			builder.movIntConstToRegister(300, x2Reg)
			err := builder.handleAdd(o)
			require.NoError(t, err)
			require.Contains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(x1Location)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			require.Equal(t, uint64(400), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("x1:stack,x2:reg", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			_ = builder.locationStack.pushValueOnStack() // dummy value!
			x1Location := builder.locationStack.pushValueOnStack()
			x2Location := builder.locationStack.pushValueOnRegister(x86.REG_R10)
			eng.stack[x1Location.stackPointer] = 5000
			builder.movIntConstToRegister(300, x2Location.register)
			err := builder.handleAdd(o)
			require.NoError(t, err)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, uint64(5300), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("x1:stack,x2:stack", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			_ = builder.locationStack.pushValueOnStack() // dummy value!
			x1Location := builder.locationStack.pushValueOnStack()
			x2Location := builder.locationStack.pushValueOnStack()
			eng.stack[x1Location.stackPointer] = 5000
			eng.stack[x2Location.stackPointer] = 13
			err := builder.handleAdd(o)
			require.NoError(t, err)
			require.True(t, x1Location.onRegister())
			require.Contains(t, builder.locationStack.usedRegisters, x1Location.register)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Location.register)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, uint64(5013), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("x1:reg,x2:stack", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			_ = builder.locationStack.pushValueOnStack() // dummy value!
			x1Location := builder.locationStack.pushValueOnRegister(x86.REG_R10)
			x2Location := builder.locationStack.pushValueOnStack()
			eng.stack[x2Location.stackPointer] = 5000
			builder.movIntConstToRegister(132, x1Location.register)
			err := builder.handleAdd(o)
			require.NoError(t, err)
			require.True(t, x1Location.onRegister())
			require.Contains(t, builder.locationStack.usedRegisters, x1Location.register)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Location.register)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, uint64(5132), eng.stack[eng.currentStackPointer-1])
		})
	})
}

func TestAmd64Builder_handleLe(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, tc := range []struct {
			x1, x2      int32
			signed, exp bool
		}{
			{x1: 100, x2: -1, signed: false, exp: true},
			{x1: -1, x2: -1, signed: false, exp: true},
			{x1: -1, x2: 100, signed: false, exp: false},
			{x1: 100, x2: 200, signed: true, exp: true},
			{x1: 100, x2: 100, signed: true, exp: true},
			{x1: 200, x2: 100, signed: true, exp: false},
		} {
			var o *wazeroir.OperationLe
			if tc.signed {
				o = &wazeroir.OperationLe{Type: wazeroir.SignFulTypeInt32}
			} else {
				o = &wazeroir.OperationLe{Type: wazeroir.SignFulTypeUint32}
			}
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			x1Reg := int16(x86.REG_R9)
			x2Reg := int16(x86.REG_R10)
			builder.locationStack.pushValueOnRegister(x1Reg)
			builder.locationStack.pushValueOnRegister(x2Reg)
			builder.movIntConstToRegister(int64(tc.x1), x1Reg)
			builder.movIntConstToRegister(int64(tc.x2), x2Reg)
			err := builder.handleLe(o)
			require.NoError(t, err)

			require.NotContains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)
			// To verify the behavior, we push the flag value
			// to the stack.
			top := builder.locationStack.peek()
			require.True(t, top.onConditionalRegister() && !top.onRegister())
			err = builder.moveConditionalToGeneralPurposeRegister(top)
			require.NoError(t, err)
			require.True(t, !top.onConditionalRegister() && top.onRegister())
			builder.releaseRegisterToStack(top)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			if tc.exp {
				require.Equal(t, uint64(1), eng.stack[eng.currentStackPointer-1])
			} else {
				require.Equal(t, uint64(0), eng.stack[eng.currentStackPointer-1])
			}
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, tc := range []struct {
			x1, x2      int64
			signed, exp bool
		}{
			{x1: 100, x2: -1, signed: false, exp: true},
			{x1: -1, x2: 100, signed: false, exp: false},
			{x1: 100, x2: 200, signed: true, exp: true},
			{x1: 200, x2: 100, signed: true, exp: false},
		} {
			var o *wazeroir.OperationLe
			if tc.signed {
				o = &wazeroir.OperationLe{Type: wazeroir.SignFulTypeInt64}
			} else {
				o = &wazeroir.OperationLe{Type: wazeroir.SignFulTypeUint64}
			}
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			x1Reg := int16(x86.REG_R9)
			x2Reg := int16(x86.REG_R10)
			builder.locationStack.pushValueOnRegister(x1Reg)
			builder.locationStack.pushValueOnRegister(x2Reg)
			builder.movIntConstToRegister(tc.x1, x1Reg)
			builder.movIntConstToRegister(tc.x2, x2Reg)
			err := builder.handleLe(o)
			require.NoError(t, err)
			require.NotContains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)
			// To verify the behavior, we push the flag value
			// to the stack.
			top := builder.locationStack.peek()
			require.True(t, top.onConditionalRegister() && !top.onRegister())
			err = builder.moveConditionalToGeneralPurposeRegister(top)
			require.NoError(t, err)
			require.True(t, !top.onConditionalRegister() && top.onRegister())
			builder.releaseRegisterToStack(top)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			if tc.exp {
				require.Equal(t, uint64(1), eng.stack[eng.currentStackPointer-1])
			} else {
				require.Equal(t, uint64(0), eng.stack[eng.currentStackPointer-1])
			}
		}
	})
}

func TestAmd64Builder_handleSub(t *testing.T) {
	t.Run("int64", func(t *testing.T) {
		o := &wazeroir.OperationSub{Type: wazeroir.SignLessTypeI64}
		t.Run("x1:reg,x2:reg", func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			x1Reg := int16(x86.REG_AX)
			x2Reg := int16(x86.REG_R10)
			x1Location := builder.locationStack.pushValueOnRegister(x1Reg)
			builder.locationStack.pushValueOnRegister(x2Reg)
			builder.movIntConstToRegister(300, x1Reg)
			builder.movIntConstToRegister(51, x2Reg)
			err := builder.handleSub(o)
			require.NoError(t, err)
			require.Contains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(x1Location)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			require.Equal(t, uint64(249), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("x1:stack,x2:reg", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			_ = builder.locationStack.pushValueOnStack() // dummy value!
			x1Location := builder.locationStack.pushValueOnStack()
			x2Location := builder.locationStack.pushValueOnRegister(x86.REG_R10)
			eng.stack[x1Location.stackPointer] = 5000
			builder.movIntConstToRegister(300, x2Location.register)
			err := builder.handleSub(o)
			require.NoError(t, err)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, uint64(4700), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("x1:stack,x2:stack", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			_ = builder.locationStack.pushValueOnStack() // dummy value!
			x1Location := builder.locationStack.pushValueOnStack()
			x2Location := builder.locationStack.pushValueOnStack()
			eng.stack[x1Location.stackPointer] = 5000
			eng.stack[x2Location.stackPointer] = 13
			err := builder.handleSub(o)
			require.NoError(t, err)
			require.True(t, x1Location.onRegister())
			require.Contains(t, builder.locationStack.usedRegisters, x1Location.register)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Location.register)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, uint64(4987), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("x1:reg,x2:stack", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			_ = builder.locationStack.pushValueOnStack() // dummy value!
			x1Location := builder.locationStack.pushValueOnRegister(x86.REG_R10)
			x2Location := builder.locationStack.pushValueOnStack()
			eng.stack[x2Location.stackPointer] = 132
			builder.movIntConstToRegister(5000, x1Location.register)
			err := builder.handleSub(o)
			require.NoError(t, err)
			require.True(t, x1Location.onRegister())
			require.Contains(t, builder.locationStack.usedRegisters, x1Location.register)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Location.register)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegisterToStack(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Check the stack.
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, uint64(4868), eng.stack[eng.currentStackPointer-1])
		})
	})
}

func TestAmd64Builder_handleCall(t *testing.T) {
	t.Run("host function", func(t *testing.T) {
		const functionIndex = 5
		builder := requireNewBuilder(t)
		builder.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{}}

		// Setup.
		eng := newEngine()
		builder.eng = eng
		hostFuncRefValue := reflect.ValueOf(func() {})
		hostFuncInstance := &wasm.FunctionInstance{HostFunction: &hostFuncRefValue}
		builder.f.ModuleInstance.Functions = make([]*wasm.FunctionInstance, functionIndex+1)
		builder.f.ModuleInstance.Functions[functionIndex] = hostFuncInstance
		eng.compiledHostFunctionIndex[hostFuncInstance] = functionIndex

		// Build codes.
		builder.initializeReservedRegisters()
		// Push the value onto stack.
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(x86.REG_AX)
		builder.movIntConstToRegister(int64(50), loc.register)
		err := builder.handleCall(&wazeroir.OperationCall{FunctionIndex: functionIndex})
		require.NoError(t, err)

		// Compile.
		code, err := builder.compile()
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
		require.Equal(t, uint64(2), eng.currentStackPointer)
		require.Equal(t, uint64(50), eng.stack[eng.currentStackPointer-1])
	})
	t.Run("wasm function", func(t *testing.T) {
		const functionIndex = 20
		builder := requireNewBuilder(t)
		builder.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{}}

		// Setup.
		eng := newEngine()
		builder.eng = eng
		wasmFuncInstance := &wasm.FunctionInstance{}
		builder.f.ModuleInstance.Functions = make([]*wasm.FunctionInstance, functionIndex+1)
		builder.f.ModuleInstance.Functions[functionIndex] = wasmFuncInstance
		eng.compiledWasmFunctionIndex[wasmFuncInstance] = functionIndex

		// Build codes.
		builder.initializeReservedRegisters()
		// Push the value onto stack.
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(x86.REG_AX)
		builder.movIntConstToRegister(int64(50), loc.register)
		err := builder.handleCall(&wazeroir.OperationCall{FunctionIndex: functionIndex})
		require.NoError(t, err)

		// Compile.
		code, err := builder.compile()
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
		require.Equal(t, uint64(3), eng.currentStackPointer)
		require.Equal(t, uint64(50), eng.stack[eng.currentStackPointer-1])
	})
}

func TestAmd64Builder_handleLoad(t *testing.T) {
	for i, tp := range []wazeroir.SignLessType{
		wazeroir.SignLessTypeI32,
		wazeroir.SignLessTypeI64,
		wazeroir.SignLessTypeF32,
		wazeroir.SignLessTypeF64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := builder.locationStack.pushValueOnStack()
			eng.stack[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad{Type: tp, Arg: &wazeroir.MemoryImmediate{Offest: 361}}
			err := builder.handleLoad(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := builder.locationStack.peek()
			require.Equal(t, o.Type, loadedValue.valueType)
			require.True(t, loadedValue.onRegister())

			// Double the loaded value in order to verify the behavior.
			var addInst obj.As
			switch tp {
			case wazeroir.SignLessTypeI32:
				require.True(t, isIntRegister(loadedValue.register))
				addInst = x86.AADDL
			case wazeroir.SignLessTypeI64:
				require.True(t, isIntRegister(loadedValue.register))
				addInst = x86.AADDQ
			case wazeroir.SignLessTypeF32:
				require.True(t, isFloatRegister(loadedValue.register))
				addInst = x86.AADDSS
			case wazeroir.SignLessTypeF64:
				require.True(t, isFloatRegister(loadedValue.register))
				addInst = x86.AADDSD
			}
			doubleLoadedValue := builder.newProg()
			doubleLoadedValue.As = addInst
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			doubleLoadedValue.From.Type = obj.TYPE_REG
			doubleLoadedValue.From.Reg = loadedValue.register
			builder.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			builder.releaseRegisterToStack(loadedValue)

			// Compile.
			builder.returnFunction()
			code, err := builder.compile()
			require.NoError(t, err)

			// Place the load target value to the memory.
			mem := newMemoryInst()
			targetRegion := mem.Buffer[baseOffset+o.Arg.Offest:]
			var expValue uint64
			switch tp {
			case wazeroir.SignLessTypeI32:
				original := uint32(100)
				binary.LittleEndian.PutUint32(targetRegion, original)
				expValue = uint64(original * 2)
			case wazeroir.SignLessTypeI64:
				original := uint64(math.MaxUint32 + 123) // The value exceeds 32-bit.
				binary.LittleEndian.PutUint64(targetRegion, original)
				expValue = original * 2
			case wazeroir.SignLessTypeF32:
				original := float32(1.234)
				binary.LittleEndian.PutUint32(targetRegion, math.Float32bits(original))
				expValue = uint64(math.Float32bits(original * 2))
			case wazeroir.SignLessTypeF64:
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
			require.Equal(t, uint64(1), eng.currentStackPointer)
			require.Equal(t, expValue, eng.stack[eng.currentStackPointer-1])
		})
	}
}

func TestAmd64Builder_handleMemoryGrow(t *testing.T) {
	builder := requireNewBuilder(t)

	builder.initializeReservedRegisters()
	// Emit memory.grow instructions.
	builder.handleMemoryGrow()

	// Compile.
	code, err := builder.compile()
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

func TestAmd64Builder_handleMemorySize(t *testing.T) {
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	// Emit memory.size instructions.
	builder.handleMemorySize()
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), builder.locationStack.sp)
	require.Equal(t, wazeroir.SignLessTypeI32, builder.locationStack.peek().valueType)

	// Compile.
	code, err := builder.compile()
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

func TestAmd64Builder_handleDrop(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		builder := requireNewBuilder(t)
		err := builder.handleDrop(&wazeroir.OperationDrop{})
		require.NoError(t, err)
	})
	t.Run("zero start", func(t *testing.T) {
		builder := requireNewBuilder(t)
		shouldPeek := builder.locationStack.pushValueOnStack()
		const numReg = 10
		for i := int16(0); i < numReg; i++ {
			builder.locationStack.pushValueOnRegister(i)
		}
		err := builder.handleDrop(&wazeroir.OperationDrop{
			Range: &wazeroir.InclusiveRange{Start: 0, End: numReg},
		})
		require.NoError(t, err)
		for i := int16(0); i < numReg; i++ {
			require.NotContains(t, builder.locationStack.usedRegisters, i)
		}
		actualPeek := builder.locationStack.peek()
		require.Equal(t, shouldPeek, actualPeek)
	})
	t.Run("live all on register", func(t *testing.T) {
		const (
			numLive = 3
			dropNum = 5
		)
		builder := requireNewBuilder(t)
		shouldBottom := builder.locationStack.pushValueOnStack()
		for i := int16(0); i < dropNum; i++ {
			builder.locationStack.pushValueOnRegister(i)
		}
		for i := int16(dropNum); i < numLive+dropNum; i++ {
			builder.locationStack.pushValueOnRegister(i)
		}
		err := builder.handleDrop(&wazeroir.OperationDrop{
			Range: &wazeroir.InclusiveRange{Start: numLive, End: numLive + dropNum - 1},
		})
		require.NoError(t, err)
		for i := int16(0); i < dropNum; i++ {
			require.NotContains(t, builder.locationStack.usedRegisters, i)
		}
		for i := int16(dropNum); i < numLive+dropNum; i++ {
			require.Contains(t, builder.locationStack.usedRegisters, i)
		}
		for i := int16(0); i < numLive; i++ {
			actual := builder.locationStack.pop()
			require.True(t, actual.onRegister())
			require.Equal(t, numLive+dropNum-1-i, actual.register)
		}
		require.Equal(t, uint64(1), builder.locationStack.sp)
		require.Equal(t, shouldBottom, builder.locationStack.pop())
	})
	t.Run("live on stack", func(t *testing.T) {
		// This is for testing all the edge cases with fake registers.
		t.Run("fake registers", func(t *testing.T) {
			const (
				numLive        = 3
				dropNum        = 5
				liveRegisterID = 10
			)
			builder := requireNewBuilder(t)
			bottom := builder.locationStack.pushValueOnStack()
			for i := int16(0); i < dropNum; i++ {
				builder.locationStack.pushValueOnRegister(i)
			}
			// The bottom live value is on the stack.
			bottomLive := builder.locationStack.pushValueOnStack()
			// The second live value is on the register.
			LiveRegister := builder.locationStack.pushValueOnRegister(liveRegisterID)
			// The top live value is on the conditional.
			topLive := builder.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateAE)
			require.True(t, topLive.onConditionalRegister())
			err := builder.handleDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: numLive, End: numLive + dropNum - 1},
			})
			require.NoError(t, err)
			require.Equal(t, uint64(4), builder.locationStack.sp)
			for i := int16(0); i < dropNum; i++ {
				require.NotContains(t, builder.locationStack.usedRegisters, i)
			}
			// Top value should be on the register.
			actualTopLive := builder.locationStack.pop()
			require.True(t, actualTopLive.onRegister() && !actualTopLive.onConditionalRegister())
			require.Equal(t, topLive, actualTopLive)
			// Second one should be on the same register.
			actualLiveRegister := builder.locationStack.pop()
			require.Equal(t, LiveRegister, actualLiveRegister)
			// The bottom live value should be moved onto the stack.
			actualBottomLive := builder.locationStack.pop()
			require.Equal(t, bottomLive, actualBottomLive)
			require.True(t, actualBottomLive.onRegister() && !actualBottomLive.onStack())
			// The bottom after drop should stay on stack.
			actualBottom := builder.locationStack.pop()
			require.Equal(t, bottom, actualBottom)
			require.True(t, bottom.onStack())
		})
		t.Run("real", func(t *testing.T) {
			eng := newEngine()
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			bottom := builder.locationStack.pushValueOnRegister(x86.REG_R10)
			builder.locationStack.pushValueOnRegister(x86.REG_R9)
			top := builder.locationStack.pushValueOnStack()
			eng.stack[top.stackPointer] = 5000
			builder.movIntConstToRegister(300, bottom.register)
			err := builder.handleDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: 1, End: 1},
			})
			require.NoError(t, err)
			builder.releaseRegisterToStack(bottom)
			builder.releaseRegisterToStack(top)
			builder.returnFunction()
			// Assemble.
			code, err := builder.compile()
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
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, []uint64{
				300,
				5000, // top value should be moved to the dropped position.
			}, eng.stack[:eng.currentStackPointer])
		})
	})
}

func TestAmd64Builder_releaseAllRegistersToStack(t *testing.T) {
	eng := newEngine()
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	x1Reg := int16(x86.REG_AX)
	x2Reg := int16(x86.REG_R10)
	_ = builder.locationStack.pushValueOnStack()
	eng.stack[0] = 100
	builder.locationStack.pushValueOnRegister(x1Reg)
	builder.locationStack.pushValueOnRegister(x2Reg)
	_ = builder.locationStack.pushValueOnStack()
	eng.stack[3] = 123
	require.Len(t, builder.locationStack.usedRegisters, 2)

	// Set the values supposed to be released to stack memory space.
	builder.movIntConstToRegister(300, x1Reg)
	builder.movIntConstToRegister(51, x2Reg)
	builder.releaseAllRegistersToStack()
	require.Len(t, builder.locationStack.usedRegisters, 0)
	builder.returnFunction()

	// Assemble.
	code, err := builder.compile()
	require.NoError(t, err)
	// Run code.
	mem := newMemoryInst()
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the stack.
	require.Equal(t, uint64(4), eng.currentStackPointer)
	require.Equal(t, uint64(123), eng.stack[eng.currentStackPointer-1])
	require.Equal(t, uint64(51), eng.stack[eng.currentStackPointer-2])
	require.Equal(t, uint64(300), eng.stack[eng.currentStackPointer-3])
	require.Equal(t, uint64(100), eng.stack[eng.currentStackPointer-4])
}

func TestAmd64Builder_assemble(t *testing.T) {
	builder := requireNewBuilder(t)
	builder.setContinuationOffsetAtNextInstructionAndReturn()
	prog := builder.newProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x86.REG_R10
	builder.addInstruction(prog)
	code, err := builder.compile()
	require.NoError(t, err)
	actual := binary.LittleEndian.Uint64(code[2:10])
	require.Equal(t, uint64(prog.Pc), actual)
}

func TestAmd64Builder_handleUnreachable(t *testing.T) {
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	x1Reg := int16(x86.REG_AX)
	x2Reg := int16(x86.REG_R10)
	builder.locationStack.pushValueOnRegister(x1Reg)
	builder.locationStack.pushValueOnRegister(x2Reg)
	builder.movIntConstToRegister(300, x1Reg)
	builder.movIntConstToRegister(51, x2Reg)
	builder.handleUnreachable()

	// Assemble.
	code, err := builder.compile()
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

func TestAmd64Builder_handleSelect(t *testing.T) {
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
			builder := requireNewBuilder(t)
			eng := newEngine()
			builder.initializeReservedRegisters()
			var x1, x2, c *valueLocation
			if tc.x1OnRegister {
				x1 = builder.locationStack.pushValueOnRegister(x86.REG_AX)
				builder.movIntConstToRegister(x1Value, x1.register)
			} else {
				x1 = builder.locationStack.pushValueOnStack()
				eng.stack[x1.stackPointer] = x1Value
			}
			if tc.x2OnRegister {
				x2 = builder.locationStack.pushValueOnRegister(x86.REG_R10)
				builder.movIntConstToRegister(x2Value, x2.register)
			} else {
				x2 = builder.locationStack.pushValueOnStack()
				eng.stack[x2.stackPointer] = x2Value
			}
			if tc.condlValueOnStack {
				c = builder.locationStack.pushValueOnStack()
				if tc.selectX1 {
					eng.stack[c.stackPointer] = 1
				} else {
					eng.stack[c.stackPointer] = 0
				}
			} else if tc.condValueOnGPRegister {
				c = builder.locationStack.pushValueOnRegister(x86.REG_R9)
				if tc.selectX1 {
					builder.movIntConstToRegister(1, c.register)
				} else {
					builder.movIntConstToRegister(0, c.register)
				}
			} else if tc.condValueOnCondRegister {
				builder.movIntConstToRegister(0, x86.REG_CX)
				cmp := builder.newProg()
				cmp.As = x86.ACMPQ
				cmp.From.Type = obj.TYPE_REG
				cmp.From.Reg = x86.REG_CX
				cmp.To.Type = obj.TYPE_CONST
				if tc.selectX1 {
					cmp.To.Offset = 0
				} else {
					cmp.To.Offset = 1
				}
				builder.addInstruction(cmp)
				builder.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateE)
			}

			// Now emit code for select.
			err := builder.handleSelect()
			require.NoError(t, err)
			// The code generation should not affect the x1's placement in any case.
			require.Equal(t, tc.x1OnRegister, x1.onRegister())
			// Plus x1 is top of the stack.
			require.Equal(t, x1, builder.locationStack.peek())

			// Now write back the x1 to the memory if it is on a register.
			if tc.x1OnRegister {
				builder.releaseRegisterToStack(x1)
			}
			builder.returnFunction()

			// Run code.
			code, err := builder.compile()
			require.NoError(t, err)
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Check the selected value.
			require.Equal(t, uint64(1), eng.currentStackPointer)
			if tc.selectX1 {
				require.Equal(t, eng.stack[x1.stackPointer], uint64(x1Value))
			} else {
				require.Equal(t, eng.stack[x1.stackPointer], uint64(x2Value))
			}
		})
	}
}

func TestAmd64Builder_handleSwap(t *testing.T) {
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
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			if tc.x2OnRegister {
				x2 := builder.locationStack.pushValueOnRegister(x86.REG_R10)
				builder.movIntConstToRegister(x2Value, x2.register)
			} else {
				x2 := builder.locationStack.pushValueOnStack()
				eng.stack[x2.stackPointer] = uint64(x2Value)
			}
			_ = builder.locationStack.pushValueOnStack() // Dummy value!
			if tc.x1OnRegister && !tc.x1OnConditionalRegister {
				x1 := builder.locationStack.pushValueOnRegister(x86.REG_AX)
				builder.movIntConstToRegister(x1Value, x1.register)
			} else if !tc.x1OnConditionalRegister {
				x1 := builder.locationStack.pushValueOnStack()
				eng.stack[x1.stackPointer] = uint64(x1Value)
			} else {
				builder.movIntConstToRegister(0, x86.REG_AX)
				cmp := builder.newProg()
				cmp.As = x86.ACMPQ
				cmp.From.Type = obj.TYPE_REG
				cmp.From.Reg = x86.REG_AX
				cmp.To.Type = obj.TYPE_CONST
				cmp.To.Offset = 0
				cmp.To.Offset = 0
				builder.addInstruction(cmp)
				builder.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateE)
				x1Value = 1
			}

			// Swap x1 and x2.
			err := builder.handleSwap(&wazeroir.OperationSwap{Depth: 2})
			require.NoError(t, err)
			// To verify the behavior, we release all the registers to stack locations.
			builder.releaseAllRegistersToStack()
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			require.Equal(t, uint64(3), eng.currentStackPointer)
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

func TestAmd64Builder_handleGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			// Setup the globals.
			builder := requireNewBuilder(t)
			globals := []*wasm.GlobalInstance{nil, {Val: globalValue, Type: &wasm.GlobalType{ValType: tp}}, nil}
			builder.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{Globals: globals}}
			// Emit the code.
			builder.initializeReservedRegisters()
			op := &wazeroir.OperationGlobalGet{Index: 1}
			err := builder.handleGlobalGet(op)
			require.NoError(t, err)

			// At this point, the top of stack must be the retrieved global on a register.
			global := builder.locationStack.peek()
			require.True(t, global.onRegister())
			require.Len(t, builder.locationStack.usedRegisters, 1)
			switch tp {
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				require.True(t, isFloatRegister(global.register))
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				require.True(t, isIntRegister(global.register))
			}
			builder.releaseAllRegistersToStack()
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)

			// Run the code assembled above.
			eng := newEngine()
			eng.currentGlobalSliceAddress = uintptr(unsafe.Pointer(&globals[0]))
			mem := newMemoryInst()
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)
			// Since we call global.get, the top of the stack must be the global value.
			require.Equal(t, globalValue, eng.stack[0])
			// Plus as we push the value, the stack pointer must be incremented.
			require.Equal(t, uint64(1), eng.currentStackPointer)
		})
	}
}

func TestAmd64Builder_handleGlobalSet(t *testing.T) {
	const valueToSet uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			// Setup the globals.
			builder := requireNewBuilder(t)
			globals := []*wasm.GlobalInstance{nil, {Val: 40, Type: &wasm.GlobalType{ValType: tp}}, nil}
			builder.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{Globals: globals}}
			_ = builder.locationStack.pushValueOnStack() // where we place the set target value below.
			// Now emit the code.
			builder.initializeReservedRegisters()
			op := &wazeroir.OperationGlobalSet{Index: 1}
			err := builder.handleGlobalSet(op)
			require.NoError(t, err)
			builder.returnFunction()

			// Assemble.
			code, err := builder.compile()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			eng.currentGlobalSliceAddress = uintptr(unsafe.Pointer(&globals[0]))
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
			require.Equal(t, uint64(0), eng.currentStackPointer)
		})
	}
}
