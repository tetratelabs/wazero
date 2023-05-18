package compiler

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	arm64 "github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// TestArm64Compiler_indirectCallWithTargetOnCallingConvReg is the regression test for #526.
// In short, the offset register for call_indirect might be the same as arm64CallingConventionModuleInstanceAddressRegister
// and that must not be a failure.
func TestArm64Compiler_indirectCallWithTargetOnCallingConvReg(t *testing.T) {
	env := newCompilerEnvironment()
	table := make([]wasm.Reference, 1)
	env.addTable(&wasm.TableInstance{References: table})
	// Ensure that the module instance has the type information for targetOperation.TypeIndex,
	// and the typeID  matches the table[targetOffset]'s type ID.
	operation := operationPtr(wazeroir.NewOperationCallIndirect(0, 0))
	env.module().TypeIDs = []wasm.FunctionTypeID{0}
	env.module().Engine = &moduleEngine{functions: []function{}}

	me := env.moduleEngine()
	{ // Compiling call target.
		compiler := env.requireNewCompiler(t, &wasm.FunctionType{}, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code := asm.CodeSegment{}
		defer func() { require.NoError(t, code.Unmap()) }()

		_, err = compiler.compile(code.Next())
		require.NoError(t, err)

		executable := code.Bytes()
		makeExecutable(executable)

		f := function{
			parent:             &compiledFunction{parent: &compiledModule{executable: code}},
			codeInitialAddress: code.Addr(),
			moduleInstance:     env.moduleInstance,
		}
		me.functions = append(me.functions, f)
		table[0] = uintptr(unsafe.Pointer(&f))
	}

	compiler := env.requireNewCompiler(t, &wasm.FunctionType{}, newCompiler, &wazeroir.CompilationResult{
		Types:    []wasm.FunctionType{{}},
		HasTable: true,
	}).(*arm64Compiler)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Place the offset into the calling-convention reserved register.
	offsetLoc := compiler.pushRuntimeValueLocationOnRegister(arm64CallingConventionModuleInstanceAddressRegister,
		runtimeValueTypeI32)
	compiler.assembler.CompileConstToRegister(arm64.MOVD, 0, offsetLoc.register)

	require.NoError(t, compiler.compileCallIndirect(operation))

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	code := asm.CodeSegment{}
	defer func() { require.NoError(t, code.Unmap()) }()

	// Generate the code under test and run.
	_, err = compiler.compile(code.Next())
	require.NoError(t, err)
	env.exec(code.Bytes())
}

func TestArm64Compiler_readInstructionAddress(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, &wasm.FunctionType{}, newArm64Compiler, nil).(*arm64Compiler)

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Set the acquisition target instruction to the one after RET,
	// and read the absolute address into destinationRegister.
	const addressReg = arm64ReservedRegisterForTemporary
	compiler.assembler.CompileReadInstructionAddress(addressReg, arm64.RET)

	// Branch to the instruction after RET below via the absolute
	// address stored in destinationRegister.
	compiler.assembler.CompileJumpToRegister(arm64.B, addressReg)

	// If we fail to branch, we reach here and exit with unreachable status,
	// so the assertion would fail.
	compiler.compileExitFromNativeCode(nativeCallStatusCodeUnreachable)

	// This could be the read instruction target as this is the
	// right after RET. Therefore, the branch instruction above
	// must target here.
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	code := asm.CodeSegment{}
	defer func() { require.NoError(t, code.Unmap()) }()

	_, err = compiler.compile(code.Next())
	require.NoError(t, err)
	env.exec(code.Bytes())

	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
}

func TestArm64Compiler_label(t *testing.T) {
	c := &arm64Compiler{}
	c.label(wazeroir.NewLabel(wazeroir.LabelKindContinuation, 100))
	require.Equal(t, 100, c.frameIDMax)
	require.Equal(t, 101, len(c.labels[wazeroir.LabelKindContinuation]))

	// frameIDMax is for all LabelKind, so this shouldn't change frameIDMax.
	c.label(wazeroir.NewLabel(wazeroir.LabelKindHeader, 2))
	require.Equal(t, 100, c.frameIDMax)
	require.Equal(t, 3, len(c.labels[wazeroir.LabelKindHeader]))
}

func TestArm64Compiler_Init(t *testing.T) {
	c := &arm64Compiler{
		locationStackForEntrypoint: newRuntimeValueLocationStack(),
		assembler:                  arm64.NewAssembler(0),
	}
	const stackCap = 12345
	c.locationStackForEntrypoint.stack = make([]runtimeValueLocation, stackCap)
	c.locationStackForEntrypoint.sp = 5555

	c.Init(&wasm.FunctionType{}, nil, false)

	// locationStack is the pointer to locationStackForEntrypoint after init.
	require.Equal(t, c.locationStack, &c.locationStackForEntrypoint)
	// And the underlying stack must be reused (the capacity preserved).
	require.Equal(t, stackCap, cap(c.locationStack.stack))
	require.Equal(t, stackCap, cap(c.locationStackForEntrypoint.stack))
}

func TestArm64Compiler_resetLabels(t *testing.T) {
	c := newArm64Compiler().(*arm64Compiler)
	nop := c.compileNOP()

	const (
		frameIDMax = 50
		capacity   = 12345
	)
	c.frameIDMax = frameIDMax
	for i := range c.labels {
		ifs := make([]arm64LabelInfo, frameIDMax*2)
		c.labels[i] = ifs
		for j := 0; j <= frameIDMax; j++ {
			ifs[j].stackInitialized = true
			ifs[j].initialInstruction = nop
			ifs[j].initialStack = newRuntimeValueLocationStack()
			ifs[j].initialStack.sp = 5555 // should be cleared via runtimeLocationStack.Reset().
			ifs[j].initialStack.stack = make([]runtimeValueLocation, 0, capacity)
		}
	}
	c.resetLabels()
	for i := range c.labels {
		for j := 0; j < len(c.labels[i]); j++ {
			l := &c.labels[i][j]
			require.False(t, l.stackInitialized)
			require.Nil(t, l.initialInstruction)
			require.Equal(t, 0, len(l.initialStack.stack))
			if j > frameIDMax {
				require.Equal(t, 0, cap(l.initialStack.stack))
			} else {
				require.Equal(t, capacity, cap(l.initialStack.stack))
			}
			require.Equal(t, uint64(0), l.initialStack.sp)
		}
	}
}

func TestArm64Compiler_getSavedTemporaryLocationStack(t *testing.T) {
	t.Run("len(brTableTmp)<len(current)", func(t *testing.T) {
		st := newRuntimeValueLocationStack()
		c := &arm64Compiler{locationStack: &st}

		c.locationStack.sp = 3
		c.locationStack.stack = []runtimeValueLocation{{stackPointer: 150}, {stackPointer: 200}, {stackPointer: 300}}

		actual := c.getSavedTemporaryLocationStack()
		require.Equal(t, uint64(3), actual.sp)
		require.Equal(t, 3, len(actual.stack))
		require.Equal(t, c.locationStack.stack[:3], actual.stack)
	})
	t.Run("len(brTableTmp)==len(current)", func(t *testing.T) {
		st := newRuntimeValueLocationStack()
		c := &arm64Compiler{locationStack: &st, brTableTmp: make([]runtimeValueLocation, 3)}
		initSlicePtr := &c.brTableTmp

		c.locationStack.sp = 3
		c.locationStack.stack = []runtimeValueLocation{{stackPointer: 150}, {stackPointer: 200}, {stackPointer: 300}}

		actual := c.getSavedTemporaryLocationStack()
		require.Equal(t, uint64(3), actual.sp)
		require.Equal(t, 3, len(actual.stack))
		require.Equal(t, c.locationStack.stack[:3], actual.stack)
		// The underlying temporary slice shouldn't be changed.
		require.Equal(t, initSlicePtr, &c.brTableTmp)
	})

	t.Run("len(brTableTmp)>len(current)", func(t *testing.T) {
		const temporarySliceSize = 100
		st := newRuntimeValueLocationStack()
		c := &arm64Compiler{locationStack: &st, brTableTmp: make([]runtimeValueLocation, temporarySliceSize)}

		c.locationStack.sp = 3
		c.locationStack.stack = []runtimeValueLocation{
			{stackPointer: 150},
			{stackPointer: 200},
			{stackPointer: 300},
			{},
			{},
			{},
			{},
			{stackPointer: 1231455}, // Entries here shouldn't be copied as they are avobe sp.
		}

		actual := c.getSavedTemporaryLocationStack()
		require.Equal(t, uint64(3), actual.sp)
		require.Equal(t, temporarySliceSize, len(actual.stack))
		require.Equal(t, c.locationStack.stack[:3], actual.stack[:3])
		for i := int(actual.sp); i < len(actual.stack); i++ {
			// Above the stack pointer, the values must not be copied.
			require.Zero(t, actual.stack[i].stackPointer)
		}
	})
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *arm64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setRuntimeValueLocationStack for the amd64 architecture.
func (c *arm64Compiler) setRuntimeValueLocationStack(s *runtimeValueLocationStack) {
	c.locationStack = s
}
