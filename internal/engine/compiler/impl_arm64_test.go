package compiler

import (
	"testing"
	"unsafe"

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
	operation := &wazeroir.OperationCallIndirect{TypeIndex: 0}
	env.module().TypeIDs = []wasm.FunctionTypeID{0}
	env.module().Engine = &moduleEngine{functions: []*function{}}

	me := env.moduleEngine()
	{ // Compiling call target.
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		c, _, err := compiler.compile()
		require.NoError(t, err)

		f := &function{
			parent:                &code{codeSegment: c},
			codeInitialAddress:    uintptr(unsafe.Pointer(&c[0])),
			moduleInstanceAddress: uintptr(unsafe.Pointer(env.moduleInstance)),
			source:                &wasm.FunctionInstance{TypeID: 0},
		}
		me.functions = append(me.functions, f)
		table[0] = uintptr(unsafe.Pointer(f))
	}

	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Types:     []*wasm.FunctionType{{}},
		HasTable:  true,
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

	// Generate the code under test and run.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)
}

func TestArm64Compiler_readInstructionAddress(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newArm64Compiler, nil).(*arm64Compiler)

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

	code, _, err := compiler.compile()
	require.NoError(t, err)

	env.exec(code)

	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
}

// compile implements compilerImpl.getOnStackPointerCeilDeterminedCallBack for the amd64 architecture.
func (c *arm64Compiler) getOnStackPointerCeilDeterminedCallBack() func(uint64) {
	return c.onStackPointerCeilDeterminedCallBack
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *arm64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setRuntimeValueLocationStack for the amd64 architecture.
func (c *arm64Compiler) setRuntimeValueLocationStack(s *runtimeValueLocationStack) {
	c.locationStack = s
}
