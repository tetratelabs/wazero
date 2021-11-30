//go:build amd64
// +build amd64

package jit

import (
	"fmt"
	"io"
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

// func Test_fibonacci(t *testing.T) {
// 	buf, err := os.ReadFile("testdata/fib.wasm")
// 	require.NoError(t, err)

// 	mod, err := wasm.DecodeModule(buf)
// 	require.NoError(t, err)

// 	store := wasm.NewStore(wazeroir.NewEngine())
// 	require.NoError(t, err)

// 	err = wasi.NewEnvironment().Register(store)
// 	require.NoError(t, err)

// 	err = store.Instantiate(mod, "test")
// 	require.NoError(t, err)

// 	m, ok := store.ModuleInstances["test"]
// 	require.True(t, ok)

// 	exp, ok := m.Exports["fib"]
// 	require.True(t, ok)

// 	f := exp.Function

// 	e := newEngine()
// 	_, err = e.compileWasmFunction(f)
// 	require.NoError(t, err)
// }

func newMemoryInst() *wasm.MemoryInstance {
	return &wasm.MemoryInstance{Buffer: make([]byte, 1024)}
}

func requireNewBuilder(t *testing.T) *amd64Builder {
	b, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	return &amd64Builder{eng: nil, builder: b,
		locationStack:         newValueLocationStack(),
		onLabelStartCallbacks: map[string][]func(*obj.Prog){},
		labelProgs:            map[string]*obj.Prog{},
	}
}

func TestAmd64Builder_pushSignatureLocals(t *testing.T) {
	f := &wasm.FunctionInstance{Signature: &wasm.FunctionType{
		InputTypes: []wasm.ValueType{wasm.ValueTypeF64, wasm.ValueTypeI32},
	}}
	builder := &amd64Builder{locationStack: newValueLocationStack(), f: f}
	builder.pushSignatureLocals()
	require.Equal(t, uint64(len(f.Signature.InputTypes)), builder.memoryStackPointer)
	require.Equal(t, 2, builder.locationStack.sp)
	loc := builder.locationStack.pop()
	require.Equal(t, wazeroir.SignLessTypeI32, loc.valueType)
	require.Equal(t, uint64(1), *loc.stackPointer)
	loc = builder.locationStack.pop()
	require.Equal(t, wazeroir.SignLessTypeF64, loc.valueType)
	require.Equal(t, uint64(0), *loc.stackPointer)
}

// Test engine.exec method on the resursive function calls.
func TestRecursiveFunctionCalls(t *testing.T) {
	const tmpReg = x86.REG_AX
	// Build a function that decrements top of stack,
	// and recursively call itself until the top value becomes zero,
	// and if the value becomes zero, add 5 onto the top and return.
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	// Pop the value from the stack
	builder.popFromStackToRegister(tmpReg)
	// Decrement tha value.
	prog := builder.builder.NewProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = tmpReg
	builder.addInstruction(prog)
	// Check if the value equals zero
	prog = builder.newProg()
	prog.As = x86.ACMPQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
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
	builder.pushRegisterToStack(tmpReg)
	builder.callFunctionFromConstIndex(0)
	builder.popFromStackToRegister(tmpReg)
	// ::End
	// If zero, we return from this function after pushing 5.
	prog = builder.movConstToRegister(5, tmpReg)
	jmp.To.SetTarget(prog) // the above mov instruction is the jump target of the JEQ.
	builder.pushRegisterToStack(tmpReg)
	builder.setJITStatus(jitStatusReturned)
	builder.returnFunction()
	// Compile.
	code, err := builder.assemble()
	require.NoError(t, err)
	// Setup engine.
	mem := newMemoryInst()
	eng := newEngine()
	eng.stack[0] = 10 // We call recursively 10 times.
	eng.currentStackPointer++
	compiledFunc := &compiledWasmFunction{codeSegment: code, memoryInst: mem, inputNum: 1, outputNum: 1}
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
				// This is to kick the Go runtime to come in
				fmt.Fprintf(io.Discard, "aaaaaaaaaaaa")
			}
			defer wg.Done()
			// Setup engine.
			mem := newMemoryInst()
			eng := newEngine()
			eng.stack[0] = 10 // We call recursively 10 times.
			eng.currentStackPointer++
			compiledFunc := &compiledWasmFunction{codeSegment: code, memoryInst: mem}
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
			builder.movConstToRegister(int64(targetValue), pushTargetRegister)
			// Push pushTargetRegister into the engine.stack[engine.sp].
			builder.pushRegisterToStack(pushTargetRegister)
			// Finally increment the stack pointer and write it back to the eng.sp
			builder.returnFunction()

			// Compile.
			code, err := builder.assemble()
			require.NoError(t, err)

			eng := newEngine()
			mem := newMemoryInst()

			f := &compiledWasmFunction{codeSegment: code, memoryInst: mem}

			// Call into the function
			eng.exec(f)

			// Because we pushed the value, eng.sp must be incremented by 1
			if eng.currentStackPointer != 1 {
				panic("eng.sp must be incremented.")
			}

			// Also we push the const value to the top of slice!
			if eng.stack[0] != 100 {
				panic("eng.stack[0] must be changed to the const!")
			}
		}()
	}
	wg.Wait()
}

func Test_setJITStatus(t *testing.T) {
	for _, s := range []jitStatusCodes{jitStatusReturned, jitStatusCallFunction, jitStatusCallBuiltInFunction} {
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
	}
}

func Test_setFunctionCallIndexFromConst(t *testing.T) {
	// Build codes.
	for _, index := range []uint32{1, 5, 20} {
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
	for _, index := range []uint32{1, 5, 20} {
		// Build codes.
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.movConstToRegister(int64(index), reg)
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
	// Build codes.
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	builder.setContinuationOffsetAtNextInstructionAndReturn()
	exp := uintptr(len(builder.builder.Assemble()))
	// On the continuation, we have to setup the registers again.
	builder.initializeReservedRegisters()
	// The continuation after function calls.
	builder.movConstToRegister(int64(50), x86.REG_AX)
	builder.pushRegisterToStack(x86.REG_AX)
	builder.setJITStatus(jitStatusCallFunction)
	builder.returnFunction()
	// Compile.
	code, err := builder.assemble()
	require.NoError(t, err)

	// Run codes
	eng := newEngine()
	eng.currentStackPointer++
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
	require.Equal(t, jitStatusCallFunction, eng.jitCallStatusCode)
	require.Equal(t, uint64(50), eng.stack[1])
}

func Test_callFunction(t *testing.T) {
	t.Run("from const", func(t *testing.T) {
		const functionIndex uint32 = 10
		// Build codes.
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.callFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		builder.movConstToRegister(int64(50), x86.REG_AX)
		builder.pushRegisterToStack(x86.REG_AX)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.currentStackPointer++
		mem := newMemoryInst()

		// The first call.
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		require.Equal(t, jitStatusCallFunction, eng.jitCallStatusCode)
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
		const functionIndex uint32 = 10
		const reg = x86.REG_AX
		// Build codes.
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		builder.movConstToRegister(int64(functionIndex), reg)
		builder.callFunctionFromRegisterIndex(reg)
		// On the continuation after function call,
		// We push the value onto stack
		builder.movConstToRegister(int64(50), x86.REG_AX)
		builder.pushRegisterToStack(x86.REG_AX)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.currentStackPointer++
		mem := newMemoryInst()

		// The first call.
		jitcall(
			uintptr(unsafe.Pointer(&code[0])),
			uintptr(unsafe.Pointer(eng)),
			uintptr(unsafe.Pointer(&mem.Buffer[0])),
		)
		require.Equal(t, jitStatusCallFunction, eng.jitCallStatusCode)
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

func Test_callHostFunction(t *testing.T) {
	t.Run("from const", func(t *testing.T) {
		const functionIndex uint32 = 0
		// Build codes.
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// Push the value onto stack.
		builder.movConstToRegister(int64(50), x86.REG_AX)
		builder.pushRegisterToStack(x86.REG_AX)
		builder.callHostFunctionFromConstIndex(functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		builder.setJITStatus(jitStatusReturned)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.hostFunctions = append(eng.hostFunctions, func() {
			eng.stack[eng.currentStackPointer-1] *= 100
		})
		mem := newMemoryInst()

		// Call into the function
		f := &compiledWasmFunction{codeSegment: code, memoryInst: mem}
		eng.exec(f)
		require.Equal(t, uint64(50)*100, eng.stack[0])
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
		builder.movConstToRegister(int64(50), tmpReg)
		builder.pushRegisterToStack(x86.REG_AX)
		// Set the function index
		builder.movConstToRegister(int64(1), tmpReg)
		builder.callHostFunctionFromRegisterIndex(tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		builder.setJITStatus(jitStatusReturned)
		builder.returnFunction()
		// Compile.
		code, err := builder.assemble()
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.hostFunctions = make([]func(), 2)
		eng.hostFunctions[1] = func() { eng.stack[eng.currentStackPointer-1] *= 200 }
		mem := newMemoryInst()

		// Call into the function
		f := &compiledWasmFunction{codeSegment: code, memoryInst: mem}
		eng.exec(f)
		require.Equal(t, uint64(50)*200, eng.stack[0])
	})

}

func Test_popFromStackToRegister(t *testing.T) {
	const targetRegister = x86.REG_AX
	// Build function.
	// Pop the value from the top twice,
	// and push back the last value to the top incremented by one.
	builder := requireNewBuilder(t)
	builder.initializeReservedRegisters()
	// Pop twice.
	builder.popFromStackToRegister(targetRegister)
	builder.popFromStackToRegister(targetRegister)
	// Increment the popped value on the register.
	prog := builder.newProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	builder.addInstruction(prog)
	// Push it back to the stack.
	builder.pushRegisterToStack(targetRegister)
	builder.returnFunction()
	// Compile.
	code, err := builder.assemble()
	require.NoError(t, err)

	// Call in.
	eng := newEngine()
	eng.currentStackPointer = 3
	eng.stack[eng.currentStackPointer-2] = 10000
	eng.stack[eng.currentStackPointer-1] = 20000
	mem := newMemoryInst()
	require.Equal(t, []uint64{0, 10000, 20000}, eng.stack[:eng.currentStackPointer])
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the sp and value.
	require.Equal(t, uint64(2), eng.currentStackPointer)
	require.Equal(t, []uint64{0, 10001}, eng.stack[:eng.currentStackPointer])
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
	require.Contains(t, builder.labelProgs, label.String())
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
	t.Run("free register", func(t *testing.T) {
		o := &wazeroir.OperationPick{Depth: 1}
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// The case when the original value is already in register.
		t.Run("on reg", func(t *testing.T) {
			// Set up the pick target original value.
			orignalReg := int16(x86.REG_AX)
			loc := &valueLocation{register: &orignalReg}
			builder.locationStack.push(loc)
			builder.locationStack.push(nil)
			builder.movConstToRegister(100, orignalReg)
			// Now insert pick code.
			err := builder.handlePick(o)
			require.NoError(t, err)
			// Increment the picked value.
			pickedLocation := builder.locationStack.peek()
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = *pickedLocation.register
			builder.addInstruction(prog)
			// To verify the behavior, we push the incremented picked value
			// to the stack.
			builder.pushRegisterToStack(*pickedLocation.register)
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
			require.Equal(t, uint64(101), eng.stack[eng.currentStackPointer-1])
		})
		// The case when the original value is in stack.
		t.Run("on stack", func(t *testing.T) {
			eng := newEngine()

			// Setup the original value.
			sp := uint64(1)
			loc := &valueLocation{stackPointer: &sp}
			builder.locationStack.push(loc)
			builder.locationStack.push(nil)
			eng.currentStackPointer = 5
			eng.currentBaseStackPointer = 1
			eng.stack[eng.currentBaseStackPointer+sp] = 100

			// Now insert pick code.
			err := builder.handlePick(o)
			require.NoError(t, err)

			// Increment the picked value.
			pickedLocation := builder.locationStack.peek()
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = *pickedLocation.register
			builder.addInstruction(prog)

			// To verify the behavior, we push the incremented picked value
			// to the stack.
			builder.pushRegisterToStack(*pickedLocation.register)
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
			require.Equal(t, uint64(100), eng.stack[eng.currentBaseStackPointer+sp]) // Original value shouldn't be affected.
			require.Equal(t, uint64(6), eng.currentStackPointer)
			require.Equal(t, uint64(101), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
		})
	})
	t.Run("steal register", func(t *testing.T) {
		t.Run("steal = pick target", func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			// Use up all the Int regs.
			for _, r := range gpIntRegisters {
				builder.locationStack.markRegisterUsed(r)
			}
			o := &wazeroir.OperationPick{Depth: 1}
			orignalReg := int16(x86.REG_AX)
			pickTarget := &valueLocation{register: &orignalReg}
			builder.locationStack.push(pickTarget)
			builder.locationStack.push(&valueLocation{})
			builder.movConstToRegister(100, orignalReg)
			builder.memoryStackPointer = 20
			// Verify the steal target will be the picked target.
			stealTarget, ok := builder.locationStack.takeStealTargetFromUsedRegister(gpTypeInt)
			require.True(t, ok)
			require.Equal(t, pickTarget, stealTarget)

			// Insert pick code.
			err := builder.handlePick(o)
			require.NoError(t, err)
			// Now the steal target's location should be on stack.
			require.False(t, stealTarget.onRegister())
			require.True(t, stealTarget.onStack())
			require.Equal(t, uint64(20), *stealTarget.stackPointer)
			require.Equal(t, uint64(21), builder.memoryStackPointer)
			// Plus the peek of value location stack should be the target reg.
			require.Equal(t, orignalReg, *builder.locationStack.peek().register)

			// To verify the behavior, we increment and push the picked value
			// to the stack.
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = orignalReg
			builder.addInstruction(prog)
			builder.pushRegisterToStack(orignalReg)
			require.Equal(t, uint64(22), builder.memoryStackPointer)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
			require.NoError(t, err)
			// Run code.
			eng := newEngine()
			mem := newMemoryInst()
			eng.currentStackPointer = 20
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Check the stack.
			require.Equal(t, uint64(22), eng.currentStackPointer)
			require.Equal(t, uint64(101), eng.stack[eng.currentStackPointer-1])
		})
		t.Run("pick target on register", func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			// Use up all the Int regs.
			for _, r := range gpIntRegisters {
				builder.locationStack.markRegisterUsed(r)
			}
			o := &wazeroir.OperationPick{Depth: 0}
			stealReg := int16(x86.REG_R9)
			stealTargetLoc := &valueLocation{register: &stealReg}
			builder.locationStack.push(stealTargetLoc)
			builder.movConstToRegister(50, stealReg)
			orignalReg := int16(x86.REG_AX)
			pickTargetLoc := &valueLocation{register: &orignalReg}
			builder.locationStack.push(pickTargetLoc)
			builder.movConstToRegister(100, orignalReg)
			builder.memoryStackPointer = 20

			// Verify the steal target will not be the picked target.
			{
				stealTarget, ok := builder.locationStack.takeStealTargetFromUsedRegister(gpTypeInt)
				require.True(t, ok)
				// require.Equal(t, stealTargetLoc, stealTarget)
				require.NotEqual(t, pickTargetLoc, stealTarget)
			}

			// Insert pick code.
			err := builder.handlePick(o)
			require.NoError(t, err)

			// Now the steal target's location should be on stack.
			require.False(t, stealTargetLoc.onRegister())
			require.True(t, stealTargetLoc.onStack())
			require.Equal(t, uint64(20), *stealTargetLoc.stackPointer)
			require.Equal(t, uint64(21), builder.memoryStackPointer)

			// To verify the behavior, we increment and push the picked value
			// to the stack.
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = stealReg
			builder.addInstruction(prog)
			builder.pushRegisterToStack(stealReg)
			require.Equal(t, uint64(22), builder.memoryStackPointer)
			builder.returnFunction()

			// Assemble.
			code, err := builder.assemble()
			require.NoError(t, err)
			// Run code.
			// Run code.
			eng := newEngine()
			mem := newMemoryInst()
			eng.currentStackPointer = 20
			jitcall(
				uintptr(unsafe.Pointer(&code[0])),
				uintptr(unsafe.Pointer(eng)),
				uintptr(unsafe.Pointer(&mem.Buffer[0])),
			)

			// Check the stack.
			require.Equal(t, uint64(22), eng.currentStackPointer)
			require.Equal(t, uint64(101), eng.stack[eng.currentStackPointer-1])
			// Steal target's value should be evicted on the stack.
			require.Equal(t, uint64(50), eng.stack[eng.currentStackPointer-2])
		})

		t.Run("pick target on stack", func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			// Use up all the Int regs.
			for _, r := range gpIntRegisters {
				builder.locationStack.markRegisterUsed(r)
			}
			eng := newEngine()

			// Setup the stack.
			pickTargetStackPointer := uint64(1)
			eng.currentStackPointer = 5
			eng.currentBaseStackPointer = 1
			eng.stack[eng.currentBaseStackPointer+pickTargetStackPointer] = 100

			// Setup value locations.
			o := &wazeroir.OperationPick{Depth: 0}
			stealReg := int16(x86.REG_R9)
			stealTargetLoc := &valueLocation{register: &stealReg}
			builder.locationStack.push(stealTargetLoc)
			builder.movConstToRegister(50, stealReg)
			pickTargetLoc := &valueLocation{stackPointer: &pickTargetStackPointer}
			builder.locationStack.push(pickTargetLoc)
			builder.memoryStackPointer = 5

			// Verify the steal target will not be the picked target.
			{
				stealTarget, ok := builder.locationStack.takeStealTargetFromUsedRegister(gpTypeInt)
				require.True(t, ok)
				// require.Equal(t, stealTargetLoc, stealTarget)
				require.NotEqual(t, pickTargetLoc, stealTarget)
			}

			// Insert pick code.
			err := builder.handlePick(o)
			require.NoError(t, err)

			// Now the steal target's location should be on stack.
			require.False(t, stealTargetLoc.onRegister())
			require.True(t, stealTargetLoc.onStack())
			require.Equal(t, uint64(5), *stealTargetLoc.stackPointer)
			require.Equal(t, uint64(6), builder.memoryStackPointer)

			// To verify the behavior, we increment and push the picked value
			// to the stack.
			prog := builder.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = stealReg
			builder.addInstruction(prog)
			builder.pushRegisterToStack(stealReg)
			require.Equal(t, uint64(7), builder.memoryStackPointer)
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
			require.Equal(t, uint64(7), eng.currentStackPointer)
			require.Equal(t, uint64(101), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
			// Steal target's value should be evicted on the stack.
			require.Equal(t, uint64(50), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-2])
		})
	})
}

func TestAmd64Builder_handleConstI64(t *testing.T) {
	t.Run("free register", func(t *testing.T) {
		o := &wazeroir.OperationConstI64{Value: 10000}
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()

		err := builder.handleConstI64(o)
		require.NoError(t, err)

		reg := *builder.locationStack.peek().register

		// To verify the behavior, we increment and push the const value
		// to the stack.
		prog := builder.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = reg
		builder.addInstruction(prog)
		builder.pushRegisterToStack(reg)
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
		require.Equal(t, o.Value+1, eng.stack[eng.currentStackPointer-1])
	})

	t.Run("steal", func(t *testing.T) {
		o := &wazeroir.OperationConstI64{Value: 10000}
		builder := requireNewBuilder(t)
		builder.initializeReservedRegisters()
		// Use up all the Int regs.
		for _, r := range gpIntRegisters {
			builder.locationStack.markRegisterUsed(r)
		}
		builder.memoryStackPointer = 10

		// Set the steal target location with the initial const value.
		stealTargetReg := int16(x86.REG_AX)
		stealTargetLoc := &valueLocation{register: &stealTargetReg}
		builder.locationStack.push(stealTargetLoc)
		builder.movConstToRegister(50, stealTargetReg)

		// Now emit the const code.
		err := builder.handleConstI64(o)
		require.NoError(t, err)
		// And at this point, the stolen value's location is on the stack.
		require.False(t, stealTargetLoc.onRegister())
		require.True(t, stealTargetLoc.onStack())
		require.Equal(t, uint64(10), *stealTargetLoc.stackPointer)
		require.Equal(t, uint64(11), builder.memoryStackPointer)

		// To verify the behavior, we increment and push the const value
		// to the stack.
		prog := builder.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = stealTargetReg
		builder.addInstruction(prog)
		builder.pushRegisterToStack(stealTargetReg)
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
		require.Equal(t, uint64(2), eng.currentStackPointer)
		require.Equal(t, o.Value+1, eng.stack[eng.currentStackPointer-1])
		require.Equal(t, uint64(50), eng.stack[eng.currentStackPointer-2])
	})
}

func TestAmd64Builder_handleAdd(t *testing.T) {
	t.Run("int64", func(t *testing.T) {
		o := &wazeroir.OperationAdd{Type: wazeroir.SignLessTypeI64}
		t.Run("x1:reg,x2:reg", func(t *testing.T) {
			builder := requireNewBuilder(t)
			builder.initializeReservedRegisters()
			x1Reg := int16(x86.REG_AX)
			x2Reg := int16(x86.REG_R10)
			builder.locationStack.markRegisterUsed(x1Reg)
			builder.locationStack.markRegisterUsed(x2Reg)
			builder.locationStack.push(&valueLocation{register: &x1Reg})
			builder.locationStack.push(&valueLocation{register: &x2Reg})
			builder.movConstToRegister(100, x1Reg)
			builder.movConstToRegister(300, x2Reg)
			builder.handleAdd(o)
			require.Contains(t, builder.locationStack.usedRegisters, x1Reg)
			require.NotContains(t, builder.locationStack.usedRegisters, x2Reg)

			// To verify the behavior, we push the value
			// to the stack.
			builder.pushRegisterToStack(x1Reg)
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
			x2Reg := int16(x86.REG_R10)
			x1StackPointer := uint64(1)
			eng.currentStackPointer = 10
			eng.stack[x1StackPointer] = 5000
			builder.locationStack.push(&valueLocation{stackPointer: &x1StackPointer})
			builder.locationStack.push(&valueLocation{register: &x2Reg})
			builder.movConstToRegister(300, x2Reg)
			builder.handleAdd(o)

			// To verify the behavior, we push the value
			// to the stack.
			builder.pushRegisterToStack(*builder.locationStack.peek().register)
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
			require.Equal(t, uint64(11), eng.currentStackPointer)
			require.Equal(t, uint64(5300), eng.stack[eng.currentStackPointer-1])
		})
		// TODO: add "x1:stack,x2:stack", "x1:stack,x2:reg" tests.
	})
	// TODO: add tests for I32,F32,F64
}
