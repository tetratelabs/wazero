package compiler

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// TestAmd64Compiler_V128Shuffle_ConstTable_MiddleOfFunction ensures that flushing constant table in the middle of
// function works well by intentionally setting amd64.AssemblerImpl MaxDisplacementForConstantPool = 0.
func TestAmd64Compiler_V128Shuffle_ConstTable_MiddleOfFunction(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler,
		&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	lanes := [16]byte{1, 1, 1, 1, 0, 0, 0, 0, 10, 10, 10, 10, 0, 0, 0, 0}
	v := [16]byte{0: 0xa, 1: 0xb, 10: 0xc}
	w := [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	exp := [16]byte{
		0xb, 0xb, 0xb, 0xb,
		0xa, 0xa, 0xa, 0xa,
		0xc, 0xc, 0xc, 0xc,
		0xa, 0xa, 0xa, 0xa,
	}

	err = compiler.compileV128Const(wazeroir.OperationV128Const{
		Lo: binary.LittleEndian.Uint64(v[:8]),
		Hi: binary.LittleEndian.Uint64(v[8:]),
	})
	require.NoError(t, err)

	err = compiler.compileV128Const(wazeroir.OperationV128Const{
		Lo: binary.LittleEndian.Uint64(w[:8]),
		Hi: binary.LittleEndian.Uint64(w[8:]),
	})
	require.NoError(t, err)

	err = compiler.compileV128Shuffle(wazeroir.OperationV128Shuffle{Lanes: lanes})
	require.NoError(t, err)

	assembler := compiler.(*amd64Compiler).assembler.(*amd64.AssemblerImpl)
	assembler.MaxDisplacementForConstantPool = 0 // Ensures that constant table for shuffle will be flushed immediately.

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	lo, hi := env.stackTopAsV128()
	var actual [16]byte
	binary.LittleEndian.PutUint64(actual[:8], lo)
	binary.LittleEndian.PutUint64(actual[8:], hi)
	require.Equal(t, exp, actual)
}

func TestAmd64Compiler_compileV128ShrI64x2SignedImpl(t *testing.T) {
	x := [16]byte{
		0, 0, 0, 0x80, 0, 0, 0, 0x80,
		0, 0, 0, 0x80, 0, 0, 0, 0x80,
	}
	exp := [16]byte{
		0, 0, 0, 0x40, 0, 0, 0, 0x80 | 0x80>>1,
		0, 0, 0, 0x40, 0, 0, 0, 0x80 | 0x80>>1,
	}
	shiftAmount := uint32(1)

	tests := []struct {
		name               string
		shiftAmountSetupFn func(t *testing.T, c *amd64Compiler)
		verifyFn           func(t *testing.T, env *compilerEnv)
	}{
		{
			name: "RegR10/CX not in use",
			shiftAmountSetupFn: func(t *testing.T, c *amd64Compiler) {
				// Move the shift amount to R10.
				loc := c.locationStack.peek()
				oldReg, newReg := loc.register, amd64.RegR10
				c.assembler.CompileRegisterToRegister(amd64.MOVQ, oldReg, newReg)
				loc.setRegister(newReg)
				c.locationStack.markRegisterUnused(oldReg)
				c.locationStack.markRegisterUsed(newReg)
			},
			verifyFn: func(t *testing.T, env *compilerEnv) {},
		},
		{
			name: "RegR10/CX not in use and CX is next free register",
			shiftAmountSetupFn: func(t *testing.T, c *amd64Compiler) {
				// Move the shift amount to R10.
				loc := c.locationStack.peek()
				oldReg, newReg := loc.register, amd64.RegR10
				c.assembler.CompileRegisterToRegister(amd64.MOVQ, oldReg, newReg)
				loc.setRegister(newReg)
				c.locationStack.markRegisterUnused(oldReg)
				c.locationStack.markRegisterUsed(newReg)

				// Ensures that the next free becomes CX.
				newUnreservedRegs := make([]asm.Register, len(c.locationStack.unreservedVectorRegisters))
				copy(newUnreservedRegs, c.locationStack.unreservedGeneralPurposeRegisters)
				for i, r := range newUnreservedRegs {
					// If CX register is found, we swap it with the first register in the list.
					// This forces runtimeLocationStack to take CX as a first free register.
					if r == amd64.RegCX {
						newUnreservedRegs[0], newUnreservedRegs[i] = newUnreservedRegs[i], newUnreservedRegs[0]
					}
				}
				c.locationStack.unreservedGeneralPurposeRegisters = newUnreservedRegs
			},
			verifyFn: func(t *testing.T, env *compilerEnv) {},
		},
		{
			name: "RegR10/CX in use",
			shiftAmountSetupFn: func(t *testing.T, c *amd64Compiler) {
				// Pop the shift amount and vector values temporarily.
				shiftAmountLocation := c.locationStack.pop()
				vecReg := c.locationStack.popV128().register

				// Move the shift amount to R10.
				oldReg, newReg := shiftAmountLocation.register, amd64.RegR10
				c.assembler.CompileRegisterToRegister(amd64.MOVQ, oldReg, newReg)
				c.locationStack.markRegisterUnused(oldReg)
				c.locationStack.markRegisterUsed(newReg)

				// Create the previous usage of CX register.
				c.pushRuntimeValueLocationOnRegister(amd64.RegCX, runtimeValueTypeI32)
				c.assembler.CompileConstToRegister(amd64.MOVQ, 100, amd64.RegCX)

				// push the operands back to the location registers.
				c.pushVectorRuntimeValueLocationOnRegister(vecReg)
				c.pushRuntimeValueLocationOnRegister(newReg, runtimeValueTypeI32)
			},
			verifyFn: func(t *testing.T, env *compilerEnv) {
				// at the bottom of stack, the previous value on the CX register must be saved.
				actual := env.stack()[callFrameDataSizeInUint64]
				require.Equal(t, uint64(100), actual)
			},
		},
		{
			name: "Stack/CX not in use",
			shiftAmountSetupFn: func(t *testing.T, c *amd64Compiler) {
				// Release the shift amount value to the stack.
				loc := c.locationStack.peek()
				c.compileReleaseRegisterToStack(loc)
			},
			verifyFn: func(t *testing.T, env *compilerEnv) {},
		},
		{
			name: "Stack/CX in use",
			shiftAmountSetupFn: func(t *testing.T, c *amd64Compiler) {
				// Pop the shift amount and vector values temporarily.
				shiftAmountReg := c.locationStack.pop().register
				require.NotEqual(t, amd64.RegCX, shiftAmountReg)
				vecReg := c.locationStack.popV128().register

				// Create the previous usage of CX register.
				c.pushRuntimeValueLocationOnRegister(amd64.RegCX, runtimeValueTypeI32)
				c.assembler.CompileConstToRegister(amd64.MOVQ, 100, amd64.RegCX)

				// push the operands back to the location registers.
				c.pushVectorRuntimeValueLocationOnRegister(vecReg)
				// Release the shift amount value to the stack.
				loc := c.pushRuntimeValueLocationOnRegister(shiftAmountReg, runtimeValueTypeI32)
				c.compileReleaseRegisterToStack(loc)
			},
			verifyFn: func(t *testing.T, env *compilerEnv) {
				// at the bottom of stack, the previous value on the CX register must be saved.
				actual := env.stack()[callFrameDataSizeInUint64]
				require.Equal(t, uint64(100), actual)
			},
		},
		{
			name: "CondReg/CX not in use",
			shiftAmountSetupFn: func(t *testing.T, c *amd64Compiler) {
				// Ignore the pushed const.
				loc := c.locationStack.pop()
				c.locationStack.markRegisterUnused(loc.register)

				// Instead, push the conditional flag value which is supposed be interpreted as 1 (=shiftAmount).
				err := c.compileConstI32(wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = c.compileConstI32(wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = c.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeI32))
				require.NoError(t, err)
			},
			verifyFn: func(t *testing.T, env *compilerEnv) {},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(x[:8]),
				Hi: binary.LittleEndian.Uint64(x[8:]),
			})
			require.NoError(t, err)

			err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: shiftAmount})
			require.NoError(t, err)

			amdCompiler := compiler.(*amd64Compiler)
			tc.shiftAmountSetupFn(t, amdCompiler)

			err = amdCompiler.compileV128ShrI64x2SignedImpl()
			require.NoError(t, err)

			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, exp, actual)

			tc.verifyFn(t, env)
		})
	}
}

// TestAmd64Compiler_compileV128Neg_NaNOnTemporary ensures compileV128Neg for floating point variants works well
// even if the temporary register used by the instruction holds NaN values previously.
func TestAmd64Compiler_compileV128Neg_NaNOnTemporary(t *testing.T) {
	tests := []struct {
		name   string
		shape  wazeroir.Shape
		v, exp [16]byte
	}{
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(51234.12341, -123, float32(math.Inf(1)), 0.1),
			exp:   f32x4(-51234.12341, 123, float32(math.Inf(-1)), -0.1),
		},
		{
			name:  "f32x4",
			shape: wazeroir.ShapeF32x4,
			v:     f32x4(51234.12341, 0, float32(math.Inf(1)), 0.1),
			exp:   f32x4(-51234.12341, float32(math.Copysign(0, -1)), float32(math.Inf(-1)), -0.1),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(1.123, math.Inf(-1)),
			exp:   f64x2(-1.123, math.Inf(1)),
		},
		{
			name:  "f64x2",
			shape: wazeroir.ShapeF64x2,
			v:     f64x2(0, math.Inf(-1)),
			exp:   f64x2(math.Copysign(0, -1), math.Inf(1)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileV128Const(wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(tc.v[:8]),
				Hi: binary.LittleEndian.Uint64(tc.v[8:]),
			})
			require.NoError(t, err)

			// Ensures that the previous state of temporary register used by Neg holds
			// NaN values.
			err = compiler.compileV128Const(wazeroir.OperationV128Const{
				Lo: math.Float64bits(math.NaN()),
				Hi: math.Float64bits(math.NaN()),
			})
			require.NoError(t, err)

			// Mark that the temp register is available for Neg instruction below.
			loc := compiler.runtimeValueLocationStack().popV128()
			compiler.runtimeValueLocationStack().markRegisterUnused(loc.register)

			// Now compiling Neg where it uses temporary register holding NaN values at this point.
			err = compiler.compileV128Neg(wazeroir.OperationV128Neg{Shape: tc.shape})
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, tc.exp, actual)
		})
	}
}
