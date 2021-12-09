//go:build amd64
// +build amd64

package jit

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
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
	builder.releaseRegister(loc)
	require.NotContains(t, builder.locationStack.usedRegisters, loc.register)
	builder.callFunctionFromConstIndex(0)
	// ::End
	// If zero, we return from this function after pushing 5.
	builder.assignRegisterToValue(loc, tmpReg)
	prog = builder.movConstToRegister(5, loc.register)
	jmp.To.SetTarget(prog) // the above mov instruction is the jump target of the JEQ.
	builder.releaseRegister(loc)
	builder.setJITStatus(jitCallStatusCodeReturned)
	builder.returnFunction()
	// Compile.
	code, err := builder.assemble()
	require.NoError(t, err)
	fmt.Println(hex.EncodeToString(code))
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
			builder.movConstToRegister(int64(targetValue), pushTargetRegister)
			// Push pushTargetRegister into the engine.stack[engine.sp].
			builder.releaseRegister(loc)
			// Finally increment the stack pointer and write it back to the eng.sp
			builder.returnFunction()

			// Compile.
			code, err := builder.assemble()
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
	} {
		t.Run(s.String(), func(t *testing.T) {

			// Build codes.
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			builder.setJITStatus(s)
			builder.returnFunction()
			// Compile.
			code, err := builder.assemble()
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
		code, err := builder.assemble()
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
		builder.movConstToRegister(index, reg)
		builder.setFunctionCallIndexFromRegister(reg)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
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
	builder.movConstToRegister(int64(50), tmpReg)
	builder.releaseRegister(loc)
	require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
	builder.setJITStatus(jitCallStatusCodeCallWasmFunction)
	builder.returnFunction()
	// Compile.
	code, err := builder.assemble()
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
		builder.movConstToRegister(int64(50), tmpReg)
		builder.releaseRegister(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
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
		builder.movConstToRegister(functionIndex, tmpReg)
		builder.callFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(tmpReg)
		builder.movConstToRegister(int64(50), tmpReg)
		builder.releaseRegister(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
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
		builder.movConstToRegister(int64(50), tmpReg)
		builder.releaseRegister(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		builder.callHostFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		builder.setJITStatus(jitCallStatusCodeReturned)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.hostFunctions = append(eng.hostFunctions, func(ctx *wasm.HostFunctionCallContext) {
			eng.stack[eng.currentStackPointer-1] *= 100
		})
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
		builder.movConstToRegister(int64(50), tmpReg)
		builder.releaseRegister(loc)
		require.NotContains(t, builder.locationStack.usedRegisters, tmpReg)
		// Set the function index
		builder.movConstToRegister(int64(1), tmpReg)
		builder.callHostFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		builder.setJITStatus(jitCallStatusCodeReturned)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
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
		eng.hostFunctionIndex[hostFunctionInstance] = 1
		eng.hostFunctions = make([]hostFunction, 2)
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
	builder.releaseRegister(result)
	builder.returnFunction()
	// Compile.
	code, err := builder.assemble()
	require.NoError(t, err)

	// Call in.
	eng := newEngine()
	eng.currentBaseStackPointer = 1
	eng.stack[eng.currentBaseStackPointer+2] = 10000
	eng.stack[eng.currentBaseStackPointer+1] = 20000
	mem := newMemoryInst()
	require.Equal(t, []uint64{0, 20000, 10000}, eng.stack[eng.currentBaseStackPointer:eng.currentBaseStackPointer+3])
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the sp and value.
	require.Equal(t, uint64(2), eng.currentStackPointer)
	require.Equal(t, []uint64{0, 30000}, eng.stack[eng.currentBaseStackPointer:eng.currentBaseStackPointer+eng.currentStackPointer])
}

func TestAmd64Builder_initializeReservedRegisters(t *testing.T) {
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	builder.returnFunction()

	// Assemble.
	code, err := builder.assemble()
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
		reg, err := builder.allocateRegister(gpTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		reg, err = builder.allocateRegister(gpTypeFloat)
		require.NoError(t, err)
		require.True(t, isFloatRegister(reg))
	})
	t.Run("steal", func(t *testing.T) {
		const stealTarget = x86.REG_AX
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// Use up all the Int regs.
		for _, r := range gpIntRegisters {
			builder.locationStack.markRegisterUsed(r)
		}
		stealTargetLocation := builder.locationStack.pushValueOnRegister(stealTarget)
		builder.movConstToRegister(int64(50), stealTargetLocation.register)
		require.Equal(t, int16(stealTarget), stealTargetLocation.register)
		require.True(t, stealTargetLocation.onRegister())
		reg, err := builder.allocateRegister(gpTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		require.False(t, stealTargetLocation.onRegister())

		// Create new value using the stolen register.
		loc := builder.locationStack.pushValueOnRegister(reg)
		builder.movConstToRegister(int64(2000), loc.register)
		builder.releaseRegister(loc)
		builder.returnFunction()

		// Assemble.
		code, err := builder.assemble()
		require.NoError(t, err)

		// Run code.
		eng := newEngine()
		eng.currentBaseStackPointer = 10
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)

		// Check the sp and value.
		require.Equal(t, uint64(2), eng.currentStackPointer)
		require.Equal(t, []uint64{50, 2000}, eng.stack[eng.currentBaseStackPointer:eng.currentBaseStackPointer+eng.currentStackPointer])
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
	code, err := builder.assemble()
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
		builder.movConstToRegister(100, pickTargetLocation.register)
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
		builder.releaseRegister(pickedLocation)
		// Also write the original location back to the stack.
		builder.releaseRegister(pickTargetLocation)
		builder.returnFunction()

		// Assemble.
		code, err := builder.assemble()
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
		eng.currentBaseStackPointer = 1
		eng.stack[eng.currentBaseStackPointer+pickTargetLocation.stackPointer] = 100

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
		builder.releaseRegister(pickedLocation)
		builder.returnFunction()

		// Assemble.
		code, err := builder.assemble()
		require.NoError(t, err)
		// Run code.
		mem := newMemoryInst()
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		// Check the stack.
		require.Equal(t, uint64(100), eng.stack[eng.currentBaseStackPointer+pickTargetLocation.stackPointer]) // Original value shouldn't be affected.
		require.Equal(t, uint64(3), eng.currentStackPointer)
		require.Equal(t, uint64(101), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
	})
}

func TestAmd64Builder_handleConstI64(t *testing.T) {
	for _, v := range []uint64{1, 1 << 5, 1 << 35, 1 << 63} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			o := &wazeroir.OperationConstI64{Value: v}
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			// Dummy value not used!
			_ = builder.locationStack.pushValueOnStack()

			// Now emit the const instruction.
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
			builder.releaseRegister(loc)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			require.Equal(t, uint64(2), eng.currentStackPointer)
			require.Equal(t, o.Value+1, eng.stack[eng.currentStackPointer-1])
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
			builder.movConstToRegister(100, x1Reg)
			builder.movConstToRegister(300, x2Reg)
			err := builder.handleAdd(o)
			require.NoError(t, err)
			require.Contains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegister(x1Location)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(300, x2Location.register)
			err := builder.handleAdd(o)
			require.NoError(t, err)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegister(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.releaseRegister(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(132, x1Location.register)
			err := builder.handleAdd(o)
			require.NoError(t, err)
			require.True(t, x1Location.onRegister())
			require.Contains(t, builder.locationStack.usedRegisters, x1Location.register)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Location.register)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegister(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(int64(tc.x1), x1Reg)
			builder.movConstToRegister(int64(tc.x2), x2Reg)
			err := builder.handleLe(o)
			require.NoError(t, err)

			require.NotContains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)
			// To verify the behavior, we push the flag value
			// to the stack.
			top := builder.locationStack.peek()
			require.True(t, top.onConditionalRegister() && !top.onRegister())
			err = builder.moveConditionalToGPRegister(top)
			require.NoError(t, err)
			require.True(t, !top.onConditionalRegister() && top.onRegister())
			builder.releaseRegister(top)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(tc.x1, x1Reg)
			builder.movConstToRegister(tc.x2, x2Reg)
			err := builder.handleLe(o)
			require.NoError(t, err)
			require.NotContains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)
			// To verify the behavior, we push the flag value
			// to the stack.
			top := builder.locationStack.peek()
			require.True(t, top.onConditionalRegister() && !top.onRegister())
			err = builder.moveConditionalToGPRegister(top)
			require.NoError(t, err)
			require.True(t, !top.onConditionalRegister() && top.onRegister())
			builder.releaseRegister(top)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(300, x1Reg)
			builder.movConstToRegister(51, x2Reg)
			err := builder.handleSub(o)
			require.NoError(t, err)
			require.Contains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegister(x1Location)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(300, x2Location.register)
			err := builder.handleSub(o)
			require.NoError(t, err)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegister(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.releaseRegister(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
			builder.movConstToRegister(5000, x1Location.register)
			err := builder.handleSub(o)
			require.NoError(t, err)
			require.True(t, x1Location.onRegister())
			require.Contains(t, builder.locationStack.usedRegisters, x1Location.register)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Location.register)

			// To verify the behavior, we push the value
			// to the stack.
			builder.releaseRegister(builder.locationStack.peek())
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
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
		eng.hostFunctionIndex[hostFuncInstance] = functionIndex

		// Build codes.
		builder.initializeReservedRegisters()
		// Push the value onto stack.
		_ = builder.locationStack.pushValueOnStack() // dummy value, not actually used!
		loc := builder.locationStack.pushValueOnRegister(x86.REG_AX)
		builder.movConstToRegister(int64(50), loc.register)
		err := builder.handleCall(&wazeroir.OperationCall{FunctionIndex: functionIndex})
		require.NoError(t, err)

		// Compile.
		code, err := builder.assemble()
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
		builder.movConstToRegister(int64(50), loc.register)
		err := builder.handleCall(&wazeroir.OperationCall{FunctionIndex: functionIndex})
		require.NoError(t, err)

		// Compile.
		code, err := builder.assemble()
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
			builder.movConstToRegister(300, bottom.register)
			err := builder.handleDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: 1, End: 1},
			})
			require.NoError(t, err)
			builder.releaseRegister(bottom)
			builder.releaseRegister(top)
			builder.returnFunction()
			// Assemble.
			code, err := builder.assemble()
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
	builder.movConstToRegister(300, x1Reg)
	builder.movConstToRegister(51, x2Reg)
	builder.releaseAllRegistersToStack()
	require.Len(t, builder.locationStack.usedRegisters, 0)
	builder.returnFunction()

	// Assemble.
	code, err := builder.assemble()
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
	code, err := builder.assemble()
	require.NoError(t, err)
	actual := binary.LittleEndian.Uint64(code[2:10])
	require.Equal(t, uint64(prog.Pc), actual)
}
