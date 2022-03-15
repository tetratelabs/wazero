package jit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestAmd64Compiler_compile_Mul_Div_Rem(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindMul,
		wazeroir.OperationKindDiv,
		wazeroir.OperationKindRem,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for _, tc := range []struct {
					name string
					// Interpret -1 as stack.
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: -1,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: -1,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: -1,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: -1,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: -1,
						x2Reg: -1,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						env := newJITEnvironment()

						const x1Value uint32 = 1 << 11
						const x2Value uint32 = 51
						const dxValue uint64 = 111111

						compiler := env.requireNewCompiler(t, nil).(*amd64Compiler)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
						// Here, we put it just before two operands as ["any value used by DX", x1, x2]
						// but in reality, it can exist in any position of stack.
						compiler.compileConstToRegisterInstruction(x86.AMOVQ, int64(dxValue), x86.REG_DX)
						prevOnDX := compiler.valueLocationStack().pushValueLocationOnRegister(x86.REG_DX)

						// Setup values.
						if tc.x1Reg != nilRegister {
							compiler.compileConstToRegisterInstruction(x86.AMOVQ, int64(x1Value), tc.x1Reg)
							compiler.valueLocationStack().pushValueLocationOnRegister(tc.x1Reg)
						} else {
							loc := compiler.valueLocationStack().pushValueLocationOnStack()
							env.stack()[loc.stackPointer] = uint64(x1Value)
						}
						if tc.x2Reg != nilRegister {
							compiler.compileConstToRegisterInstruction(x86.AMOVQ, int64(x2Value), tc.x2Reg)
							compiler.valueLocationStack().pushValueLocationOnRegister(tc.x2Reg)
						} else {
							loc := compiler.valueLocationStack().pushValueLocationOnStack()
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

						require.Equal(t, int16(x86.REG_AX), compiler.valueLocationStack().peek().register)
						require.Equal(t, generalPurposeRegisterTypeInt, compiler.valueLocationStack().peek().regType)
						require.Equal(t, uint64(2), compiler.valueLocationStack().sp)
						require.Len(t, compiler.valueLocationStack().usedRegisters, 1)
						// At this point, the previous value on the DX register is saved to the stack.
						require.True(t, prevOnDX.onStack())

						// We add the value previously on the DX with the multiplication result
						// in order to ensure that not saving existing DX value would cause
						// the failure in a subsequent instruction.
						err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
						require.NoError(t, err)

						require.NoError(t, compiler.compileReturnFunction())

						// Generate the code under test.
						code, _, _, err := compiler.compile()
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
							require.Equal(t, uint64(x1Value%x2Value)+dxValue, env.stackTopAsUint64())
						}
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for _, tc := range []struct {
					name         string
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: nilRegister,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: nilRegister,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: nilRegister,
						x2Reg: nilRegister,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const x1Value uint64 = 1 << 35
						const x2Value uint64 = 51
						const dxValue uint64 = 111111

						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t, nil).(*amd64Compiler)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
						// Here, we put it just before two operands as ["any value used by DX", x1, x2]
						// but in reality, it can exist in any position of stack.
						compiler.compileConstToRegisterInstruction(x86.AMOVQ, int64(dxValue), x86.REG_DX)
						prevOnDX := compiler.valueLocationStack().pushValueLocationOnRegister(x86.REG_DX)

						// Setup values.
						if tc.x1Reg != nilRegister {
							compiler.compileConstToRegisterInstruction(x86.AMOVQ, int64(x1Value), tc.x1Reg)
							compiler.valueLocationStack().pushValueLocationOnRegister(tc.x1Reg)
						} else {
							loc := compiler.valueLocationStack().pushValueLocationOnStack()
							env.stack()[loc.stackPointer] = uint64(x1Value)
						}
						if tc.x2Reg != nilRegister {
							compiler.compileConstToRegisterInstruction(x86.AMOVQ, int64(x2Value), tc.x2Reg)
							compiler.valueLocationStack().pushValueLocationOnRegister(tc.x2Reg)
						} else {
							loc := compiler.valueLocationStack().pushValueLocationOnStack()
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

						require.Equal(t, int16(x86.REG_AX), compiler.valueLocationStack().peek().register)
						require.Equal(t, generalPurposeRegisterTypeInt, compiler.valueLocationStack().peek().regType)
						require.Equal(t, uint64(2), compiler.valueLocationStack().sp)
						require.Len(t, compiler.valueLocationStack().usedRegisters, 1)
						// At this point, the previous value on the DX register is saved to the stack.
						require.True(t, prevOnDX.onStack())

						// We add the value previously on the DX with the multiplication result
						// in order to ensure that not saving existing DX value would cause
						// the failure in a subsequent instruction.
						err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
						require.NoError(t, err)

						require.NoError(t, compiler.compileReturnFunction())

						// Generate the code under test.
						code, _, _, err := compiler.compile()
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
							require.Equal(t, uint64(x1Value%x2Value)+dxValue, env.stackTopAsUint64())
						}
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil).(*amd64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.compileReadInstructionAddress(x86.REG_AX, obj.AJMP)

		// If generate the code without JMP after readInstructionAddress,
		// the call back added must return error.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil).(*amd64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		const destinationRegister = x86.REG_AX
		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		compiler.compileReadInstructionAddress(destinationRegister, obj.ARET)

		// Jump to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		jmpToAfterRet := compiler.newProg()
		jmpToAfterRet.As = obj.AJMP
		jmpToAfterRet.To.Type = obj.TYPE_REG
		jmpToAfterRet.To.Reg = destinationRegister
		compiler.addInstruction(jmpToAfterRet)

		ret := compiler.newProg()
		ret.As = obj.ARET
		compiler.addInstruction(ret)

		// This could be the read instruction target as this is the
		// right after RET. Therefore, the jmp instruction above
		// must target here.
		const expectedReturnValue uint32 = 10000
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
		require.NoError(t, err)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, expectedReturnValue, env.stackTopAsUint32())
	})
}
