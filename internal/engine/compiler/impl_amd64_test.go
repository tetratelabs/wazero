package compiler

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// TestAmd64Compiler_indirectCallWithTargetOnCallingConvReg is the regression test for #526.
// In short, the offset register for call_indirect might be the same as amd64CallingConventionModuleInstanceAddressRegister
// and that must not be a failure.
func TestAmd64Compiler_indirectCallWithTargetOnCallingConvReg(t *testing.T) {
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
	}).(*amd64Compiler)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Place the offset into the calling-convention reserved register.
	offsetLoc := compiler.pushRuntimeValueLocationOnRegister(amd64CallingConventionModuleInstanceAddressRegister,
		runtimeValueTypeI32)
	compiler.assembler.CompileConstToRegister(amd64.MOVQ, 0, offsetLoc.register)

	require.NoError(t, compiler.compileCallIndirect(operation))

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate the code under test and run.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)
}

func TestAmd64Compiler_compile_Mul_Div_Rem(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindMul,
		wazeroir.OperationKindDiv,
		wazeroir.OperationKindRem,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				tests := []struct {
					name         string
					x1Reg, x2Reg asm.Register
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: amd64.RegAX,
						x2Reg: amd64.RegR10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: amd64.RegAX,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: amd64.RegR9,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: asm.NilRegister,
						x2Reg: asm.NilRegister,
					},
				}

				for _, tt := range tests {
					tc := tt
					t.Run(tc.name, func(t *testing.T) {
						env := newCompilerEnvironment()

						const x1Value uint32 = 1 << 11
						const x2Value uint32 = 51
						const dxValue uint64 = 111111

						compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
						// Here, we put it just before two operands as ["any value used by DX", x1, x2]
						// but in reality, it can exist in any position of stack.
						compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(dxValue), amd64.RegDX)
						prevOnDX := compiler.pushRuntimeValueLocationOnRegister(amd64.RegDX, runtimeValueTypeI32)

						// Setup values.
						if tc.x1Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x1Value), tc.x1Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x1Reg, runtimeValueTypeI32)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							env.stack()[loc.stackPointer] = uint64(x1Value)
						}
						if tc.x2Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x2Value), tc.x2Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x2Reg, runtimeValueTypeI32)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							env.stack()[loc.stackPointer] = uint64(x2Value)
						}

						switch kind {
						case wazeroir.OperationKindDiv:
							err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeUint32})
						case wazeroir.OperationKindMul:
							err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeI32})
						case wazeroir.OperationKindRem:
							err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint32})
						}
						require.NoError(t, err)

						require.Equal(t, registerTypeGeneralPurpose, compiler.runtimeValueLocationStack().peek().getRegisterType())
						require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
						require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
						// At this point, the previous value on the DX register is saved to the stack.
						require.True(t, prevOnDX.onStack())

						// We add the value previously on the DX with the multiplication result
						// in order to ensure that not saving existing DX value would cause
						// the failure in a subsequent instruction.
						err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
						require.NoError(t, err)

						require.NoError(t, compiler.compileReturnFunction())

						// Generate the code under test.
						code, _, err := compiler.compile()
						require.NoError(t, err)
						// Run code.
						env.exec(code)

						// Verify the stack is in the form of ["any value previously used by DX" + the result of operation]
						require.Equal(t, uint64(1), env.stackPointer())
						switch kind {
						case wazeroir.OperationKindDiv:
							require.Equal(t, x1Value/x2Value+uint32(dxValue), env.stackTopAsUint32())
						case wazeroir.OperationKindMul:
							require.Equal(t, x1Value*x2Value+uint32(dxValue), env.stackTopAsUint32())
						case wazeroir.OperationKindRem:
							require.Equal(t, x1Value%x2Value+uint32(dxValue), env.stackTopAsUint32())
						}
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				tests := []struct {
					name         string
					x1Reg, x2Reg asm.Register
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: amd64.RegAX,
						x2Reg: amd64.RegR10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: amd64.RegAX,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: amd64.RegR9,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: asm.NilRegister,
						x2Reg: asm.NilRegister,
					},
				}

				for _, tt := range tests {
					tc := tt
					t.Run(tc.name, func(t *testing.T) {
						const x1Value uint64 = 1 << 35
						const x2Value uint64 = 51
						const dxValue uint64 = 111111

						env := newCompilerEnvironment()
						compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
						// Here, we put it just before two operands as ["any value used by DX", x1, x2]
						// but in reality, it can exist in any position of stack.
						compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(dxValue), amd64.RegDX)
						prevOnDX := compiler.pushRuntimeValueLocationOnRegister(amd64.RegDX, runtimeValueTypeI64)

						// Setup values.
						if tc.x1Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x1Value), tc.x1Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x1Reg, runtimeValueTypeI64)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							env.stack()[loc.stackPointer] = uint64(x1Value)
						}
						if tc.x2Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x2Value), tc.x2Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x2Reg, runtimeValueTypeI64)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							env.stack()[loc.stackPointer] = uint64(x2Value)
						}

						switch kind {
						case wazeroir.OperationKindDiv:
							err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeInt64})
						case wazeroir.OperationKindMul:
							err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeI64})
						case wazeroir.OperationKindRem:
							err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint64})
						}
						require.NoError(t, err)

						require.Equal(t, registerTypeGeneralPurpose, compiler.runtimeValueLocationStack().peek().getRegisterType())
						require.Equal(t, uint64(2), compiler.runtimeValueLocationStack().sp)
						require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
						// At this point, the previous value on the DX register is saved to the stack.
						require.True(t, prevOnDX.onStack())

						// We add the value previously on the DX with the multiplication result
						// in order to ensure that not saving existing DX value would cause
						// the failure in a subsequent instruction.
						err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
						require.NoError(t, err)

						require.NoError(t, compiler.compileReturnFunction())

						// Generate the code under test.
						code, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Verify the stack is in the form of ["any value previously used by DX" + the result of operation]
						switch kind {
						case wazeroir.OperationKindDiv:
							require.Equal(t, uint64(1), env.stackPointer())
							require.Equal(t, uint64(x1Value/x2Value)+dxValue, env.stackTopAsUint64())
						case wazeroir.OperationKindMul:
							require.Equal(t, uint64(1), env.stackPointer())
							require.Equal(t, uint64(x1Value*x2Value)+dxValue, env.stackTopAsUint64())
						case wazeroir.OperationKindRem:
							require.Equal(t, uint64(1), env.stackPointer())
							require.Equal(t, x1Value%x2Value+dxValue, env.stackTopAsUint64())
						}
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.assembler.CompileReadInstructionAddress(amd64.RegAX, amd64.JMP)

		// If generate the code without JMP after readInstructionAddress,
		// the call back added must return error.
		_, _, err = compiler.compile()
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		const destinationRegister = amd64.RegAX
		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		compiler.assembler.CompileReadInstructionAddress(destinationRegister, amd64.RET)

		// Jump to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		compiler.assembler.CompileJumpToRegister(amd64.JMP, destinationRegister)

		compiler.assembler.CompileStandAlone(amd64.RET)

		// This could be the read instruction target as this is the
		// right after RET. Therefore, the jmp instruction above
		// must target here.
		const expectedReturnValue uint32 = 10000
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
		require.NoError(t, err)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		// Generate the code under test.
		code, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, expectedReturnValue, env.stackTopAsUint32())
	})
}

// compile implements compilerImpl.runtimeValueLocationStack for the amd64 architecture.
func (c *amd64Compiler) runtimeValueLocationStack() *runtimeValueLocationStack {
	return c.locationStack
}

// compile implements compilerImpl.getOnStackPointerCeilDeterminedCallBack for the amd64 architecture.
func (c *amd64Compiler) getOnStackPointerCeilDeterminedCallBack() func(uint64) {
	return c.onStackPointerCeilDeterminedCallBack
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *amd64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setRuntimeValueLocationStack for the amd64 architecture.
func (c *amd64Compiler) setRuntimeValueLocationStack(s *runtimeValueLocationStack) {
	c.locationStack = s
}
