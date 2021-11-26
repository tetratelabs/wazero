package jit

import (
	"fmt"
	"io"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/wasm"
	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

func newMemoryInst() *wasm.MemoryInstance {
	return &wasm.MemoryInstance{Buffer: make([]byte, 1024)}
}

// Test engine.exec method on the resursive function calls.
func TestRecursiveFunctionCalls(t *testing.T) {
	const tmpReg = x86.REG_AX
	// Build a function that decrements top of stack,
	// and recursively call itself until the top value becomes zero,
	// and if the value becomes zero, add 5 onto the top and return.
	builder, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	initializeReservedRegisters(builder)
	// Pop the value from the stack
	popFromStackToRegister(builder, tmpReg)
	// Decrement tha value.
	prog := builder.NewProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = tmpReg
	builder.AddInstruction(prog)
	// Check if the value equals zero
	prog = builder.NewProg()
	prog.As = x86.ACMPQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_CONST
	prog.To.Offset = 0
	builder.AddInstruction(prog)
	// If zero, jump to ::End
	jmp := builder.NewProg()
	jmp.As = x86.AJEQ
	jmp.To.Type = obj.TYPE_BRANCH
	builder.AddInstruction(jmp)
	// If not zero, we call push back the value to the stack
	// and call itself recursively.
	pushRegisterToStack(builder, tmpReg)
	callFunctionFromConstIndex(builder, 0)
	// ::End
	// If zero, we return from this function after pushing 5.
	prog = movConstToRegister(builder, 5, tmpReg)
	jmp.To.SetTarget(prog) // the above mov instruction is the jump target of the JEQ.
	pushRegisterToStack(builder, tmpReg)
	setJITStatus(builder, jitStatusReturned)
	returnFunction(builder)

	// Compile.
	code, err := mmapCodeSegment(builder.Assemble())
	require.NoError(t, err)
	// Setup engine.
	mem := newMemoryInst()
	eng := newEngine()
	eng.stack[0] = 10 // We call recursively 10 times.
	eng.sp++
	compiledFunc := &compiledFunction{codeSegment: code, memoryInst: mem}
	eng.compiledWasmFunctions = []*compiledFunction{compiledFunc}
	// Call into the function
	eng.exec(compiledFunc)

	// We must return 10 times, so 5 is pushed onto the stack 10 times.
	require.Equal(t, []uint64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}, eng.stack[:eng.sp])
	// And the callstack should be empty.
	require.Nil(t, eng.callFrameStack)

	// Check the stability with busy Go runtime.
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
			eng.sp++
			compiledFunc := &compiledFunction{codeSegment: code, memoryInst: mem}
			eng.compiledWasmFunctions = []*compiledFunction{compiledFunc}
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
			builder, err := asm.NewBuilder("amd64", 128)
			require.NoError(t, err)
			initializeReservedRegisters(builder)
			// Push consts to pushTargetRegister.
			movConstToRegister(builder, int64(targetValue), pushTargetRegister)
			// Push pushTargetRegister into the engine.stack[engine.sp].
			pushRegisterToStack(builder, pushTargetRegister)
			// Finally increment the stack pointer and write it back to the eng.sp
			returnFunction(builder)

			// Compile.
			code, err := mmapCodeSegment(builder.Assemble())
			require.NoError(t, err)

			eng := newEngine()
			mem := newMemoryInst()

			f := &compiledFunction{codeSegment: code, memoryInst: mem}

			// Call into the function
			eng.exec(f)

			// Because we pushed the value, eng.sp must be incremented by 1
			if eng.sp != 1 {
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
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		setJITStatus(builder, s)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
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
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		setFunctionCallIndexFromConst(builder, index)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
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
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		movConstToRegister(builder, int64(index), reg)
		setFunctionCallIndexFromRegister(builder, reg)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
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
	builder, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	initializeReservedRegisters(builder)
	setContinuationOffsetAtNextInstructionAndReturn(builder)
	exp := uintptr(len(builder.Assemble()))
	// On the continuation, we have to setup the registers again.
	initializeReservedRegisters(builder)
	// The continuation after function calls.
	movConstToRegister(builder, int64(50), x86.REG_AX)
	pushRegisterToStack(builder, x86.REG_AX)
	setJITStatus(builder, jitStatusCallFunction)
	returnFunction(builder)
	// Compile.
	code, err := mmapCodeSegment(builder.Assemble())
	require.NoError(t, err)

	// Run codes
	eng := newEngine()
	eng.sp++
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
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		callFunctionFromConstIndex(builder, functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		movConstToRegister(builder, int64(50), x86.REG_AX)
		pushRegisterToStack(builder, x86.REG_AX)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.sp++
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
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		movConstToRegister(builder, int64(functionIndex), reg)
		callFunctionFromRegisterIndex(builder, reg)
		// On the continuation after function call,
		// We push the value onto stack
		movConstToRegister(builder, int64(50), x86.REG_AX)
		pushRegisterToStack(builder, x86.REG_AX)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.sp++
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
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		// Push the value onto stack.
		movConstToRegister(builder, int64(50), x86.REG_AX)
		pushRegisterToStack(builder, x86.REG_AX)
		callHostFunctionFromConstIndex(builder, functionIndex)
		// On the continuation after function call,
		// We push the value onto stack
		setJITStatus(builder, jitStatusReturned)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.hostFunctions = append(eng.hostFunctions, func() {
			eng.stack[eng.sp-1] *= 100
		})
		mem := newMemoryInst()

		// Call into the function
		f := &compiledFunction{codeSegment: code, memoryInst: mem}
		eng.exec(f)
		require.Equal(t, uint64(50)*100, eng.stack[0])
	})

	t.Run("from register", func(t *testing.T) {
		const (
			functionIndex uint32 = 1
			tmpReg               = x86.REG_AX
		)
		// Build codes.
		builder, err := asm.NewBuilder("amd64", 128)
		require.NoError(t, err)
		initializeReservedRegisters(builder)
		// Push the value onto stack.
		movConstToRegister(builder, int64(50), tmpReg)
		pushRegisterToStack(builder, x86.REG_AX)
		// Set the function index
		movConstToRegister(builder, int64(1), tmpReg)
		callHostFunctionFromRegisterIndex(builder, tmpReg)
		// On the continuation after function call,
		// We push the value onto stack
		setJITStatus(builder, jitStatusReturned)
		returnFunction(builder)
		// Compile.
		code, err := mmapCodeSegment(builder.Assemble())
		require.NoError(t, err)

		// Setup.
		eng := newEngine()
		eng.hostFunctions = make([]func(), 2)
		eng.hostFunctions[1] = func() { eng.stack[eng.sp-1] *= 200 }
		mem := newMemoryInst()

		// Call into the function
		f := &compiledFunction{codeSegment: code, memoryInst: mem}
		eng.exec(f)
		require.Equal(t, uint64(50)*200, eng.stack[0])
	})

}

func Test_popFromStackToRegister(t *testing.T) {
	const targetRegister = x86.REG_AX
	// Build function.
	// Pop the value from the top twice,
	// and push back the last value to the top incremented by one.
	builder, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	initializeReservedRegisters(builder)
	// Pop twice.
	popFromStackToRegister(builder, targetRegister)
	popFromStackToRegister(builder, targetRegister)
	// Increment the popped value on the register.
	prog := builder.NewProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	builder.AddInstruction(prog)
	// Push it back to the stack.
	pushRegisterToStack(builder, targetRegister)
	returnFunction(builder)
	// Compile.
	code, err := mmapCodeSegment(builder.Assemble())
	require.NoError(t, err)

	// Call in.
	eng := newEngine()
	eng.sp = 3
	eng.stack[eng.sp-2] = 10000
	eng.stack[eng.sp-1] = 20000
	mem := newMemoryInst()
	require.Equal(t, []uint64{0, 10000, 20000}, eng.stack[:eng.sp])
	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(eng)),
		uintptr(unsafe.Pointer(&mem.Buffer[0])),
	)
	// Check the sp and value.
	require.Equal(t, uint64(2), eng.sp)
	require.Equal(t, []uint64{0, 10001}, eng.stack[:eng.sp])
}
