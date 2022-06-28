package compiler

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// TestCompiler_conditional_value_saving ensure that saving conditional register works correctly even if there's
// no free registers available.
func TestCompiler_conditional_value_saving(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, nil)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Generate constants to occupy all the unreserved GP registers.
	for i := 0; i < len(unreservedGeneralPurposeRegisters); i++ {
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 100})
		require.NoError(t, err)
	}

	// Ensures that no free registers are available.
	_, ok := compiler.runtimeValueLocationStack().takeFreeRegister(registerTypeGeneralPurpose)
	require.False(t, ok)

	// Generate conditional flag via floating point comparisons
	err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: 1.0})
	require.NoError(t, err)
	err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: 2.0})
	require.NoError(t, err)
	err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat32})
	require.NoError(t, err)

	// Ensures that we have conditional value at top of stack.
	l := compiler.runtimeValueLocationStack().peek()
	require.True(t, l.onConditionalRegister())

	// On function return, the conditional value must be saved to a general purpose reg, and then written to stack.
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, uint32(1), env.stackTopAsUint32()) // expect 1 as the result of 1.0 < 2.0.
}
