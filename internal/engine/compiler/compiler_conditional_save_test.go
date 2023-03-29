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

	// Place the f32 local.
	err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: 1.0})
	require.NoError(t, err)

	// Generate constants to occupy all the unreserved GP registers.
	for i := 0; i < len(unreservedGeneralPurposeRegisters); i++ {
		err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 100})
		require.NoError(t, err)
	}

	// Pick the f32 floating point local (1.0) twice.
	// Note that the f32 (function local variable in general) is placed above the call frame.
	err = compiler.compilePick(wazeroir.OperationPick{Depth: int(compiler.runtimeValueLocationStack().sp - 1 - callFrameDataSizeInUint64)})

	require.NoError(t, err)
	err = compiler.compilePick(wazeroir.OperationPick{Depth: int(compiler.runtimeValueLocationStack().sp - 1 - callFrameDataSizeInUint64)})

	require.NoError(t, err)
	// Generate conditional flag via floating point comparisons.
	err = compiler.compileLe(wazeroir.NewOperationLe(wazeroir.SignedTypeFloat32))
	require.NoError(t, err)

	// Ensures that we have conditional value at top of stack.
	l := compiler.runtimeValueLocationStack().peek()
	require.True(t, l.onConditionalRegister())

	// Ensures that no free registers are available.
	_, ok := compiler.runtimeValueLocationStack().takeFreeRegister(registerTypeGeneralPurpose)
	require.False(t, ok)

	// We should be able to use the conditional value (an i32 value in Wasm) as an operand for, say, i32.add.
	err = compiler.compileAdd(wazeroir.NewOperationAdd(wazeroir.UnsignedTypeI32))
	require.NoError(t, err)

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// expect 101 = 100(== the integer const) + 1 (== flag value == the result of (1.0 <= 1.0))
	require.Equal(t, uint32(101), env.stackTopAsUint32())
}
