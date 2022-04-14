package jit

import (
	"testing"

	arm64 "github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestArm64Compiler_readInstructionAddress(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t, newArm64Compiler, nil).(*arm64Compiler)

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Set the acquisition target instruction to the one after RET,
	// and read the absolute address into destinationRegister.
	const addressReg = arm64ReservedRegisterForTemporary
	compiler.assembler.CompileReadInstructionAddress(addressReg, arm64.RET)

	// Branch to the instruction after RET below via the absolute
	// address stored in destinationRegister.
	compiler.assembler.CompileJumpToMemory(arm64.B, addressReg)

	// If we fail to branch, we reach here and exit with unreachable status,
	// so the assertion would fail.
	compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)

	// This could be the read instruction target as this is the
	// right after RET. Therefore, the branch instruction above
	// must target here.
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	env.exec(code)

	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

// compile implements compilerImpl.valueLocationStack for the amd64 architecture.
func (c *arm64Compiler) valueLocationStack() *valueLocationStack {
	return c.locationStack
}

// compile implements compilerImpl.getOnStackPointerCeilDeterminedCallBack for the amd64 architecture.
func (c *arm64Compiler) getOnStackPointerCeilDeterminedCallBack() func(uint64) {
	return c.onStackPointerCeilDeterminedCallBack
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *arm64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setValueLocationStack for the amd64 architecture.
func (c *arm64Compiler) setValueLocationStack(s *valueLocationStack) {
	c.locationStack = s
}

func (a *arm64Compiler) compileNOP() {
	a.assembler.CompileStandAlone(arm64.NOP)
}
