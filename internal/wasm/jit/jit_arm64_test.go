package jit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm/arm64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

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

func TestArm64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("target instruction not found", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil).(*arm64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.assembler.CompileReadInstructionAddress(arm64ReservedRegisterForTemporary, arm64.B)

		compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

		// If generate the code without JMP after compileReadInstructionAddress,
		// the call back added must return error.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
		require.Contains(t, err.Error(), "target instruction not found")
	})
	t.Run("too large offset", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil).(*arm64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after RET.
		compiler.assembler.CompileReadInstructionAddress(arm64ReservedRegisterForTemporary, arm64.RET)

		// Add many instruction between the target and compileReadInstructionAddress.
		for i := 0; i < 100; i++ {
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 10})
			require.NoError(t, err)
		}

		compiler.assembler.CompileJumpToRegister(arm64.RET, arm64ReservedRegisterForTemporary)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		// If generate the code with too many instruction between ADR and
		// the target, compile must fail.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
		require.Contains(t, err.Error(), "too large offset")
	})
	t.Run("ok", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil).(*arm64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		const addressReg = arm64ReservedRegisterForTemporary
		compiler.assembler.CompileReadInstructionAddress(addressReg, arm64.RET)

		// Branch to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		compiler.assembler.CompileJumpToMemory(arm64.B, addressReg, 0)

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
	})
}
