package amd64

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/asm"
)

func TestNodeImpl_AssignJumpTarget(t *testing.T) {
	n := &nodeImpl{}
	target := &nodeImpl{}
	n.AssignJumpTarget(target)
	require.Equal(t, n.jumpTarget, target)
}

func TestNodeImpl_AssignDestinationConstant(t *testing.T) {
	n := &nodeImpl{}
	n.AssignDestinationConstant(12345)
	require.Equal(t, int64(12345), n.dstConst)
}

func TestNodeImpl_AssignSourceConstant(t *testing.T) {
	n := &nodeImpl{}
	n.AssignSourceConstant(12345)
	require.Equal(t, int64(12345), n.srcConst)
}

func TestNodeImpl_String(t *testing.T) {
	for _, tc := range []struct {
		in  *nodeImpl
		exp string
	}{
		{
			in:  &nodeImpl{instruction: NOP},
			exp: "NOP",
		},
		{
			in:  &nodeImpl{instruction: SETCC, types: operandTypesNoneToRegister, dstReg: REG_AX},
			exp: "SETCC AX",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: REG_AX, dstConst: 100},
			exp: "JMP [AX + 0x64]",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: REG_AX, dstConst: 100, dstMemScale: 8, dstMemIndex: REG_R11},
			exp: "JMP [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToBranch, jumpTarget: &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: REG_AX, dstConst: 100}},
			exp: "JMP {JMP [AX + 0x64]}",
		},
		{
			in:  &nodeImpl{instruction: IDIVQ, types: operandTypesRegisterToNone, srcReg: REG_DX},
			exp: "IDIVQ DX",
		},
		{
			in:  &nodeImpl{instruction: ADDL, types: operandTypesRegisterToRegister, srcReg: REG_DX, dstReg: REG_R14},
			exp: "ADDL DX, R14",
		},
		{
			in: &nodeImpl{instruction: MOVQ, types: operandTypesRegisterToMemory,
				srcReg: REG_DX, dstReg: REG_R14, dstConst: 100},
			exp: "MOVQ DX, [R14 + 0x64]",
		},
		{
			in: &nodeImpl{instruction: MOVQ, types: operandTypesRegisterToMemory,
				srcReg: REG_DX, dstReg: REG_R14, dstConst: 100, dstMemIndex: REG_CX, dstMemScale: 4},
			exp: "MOVQ DX, [R14 + 0x64 + CX*0x4]",
		},
		{
			in: &nodeImpl{instruction: CMPL, types: operandTypesRegisterToConst,
				srcReg: REG_DX, dstConst: 100},
			exp: "CMPL DX, 0x64",
		},
		{
			in: &nodeImpl{instruction: MOVL, types: operandTypesMemoryToRegister,
				srcReg: REG_DX, srcConst: 1, dstReg: REG_AX},
			exp: "MOVL [DX + 0x1], AX",
		},
		{
			in: &nodeImpl{instruction: MOVL, types: operandTypesMemoryToRegister,
				srcReg: REG_DX, srcConst: 1, srcMemIndex: REG_R12, srcMemScale: 2,
				dstReg: REG_AX},
			exp: "MOVL [DX + 1 + R12*0x2], AX",
		},
		{
			in: &nodeImpl{instruction: CMPQ, types: operandTypesMemoryToConst,
				srcReg: REG_DX, srcConst: 1, srcMemIndex: REG_R12, srcMemScale: 2,
				dstConst: 123},
			exp: "CMPQ [DX + 1 + R12*0x2], 0x7b",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToMemory, srcConst: 123, dstReg: REG_AX, dstConst: 100, dstMemScale: 8, dstMemIndex: REG_R11},
			exp: "MOVQ 0x7b, [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToRegister, srcConst: 123, dstReg: REG_AX},
			exp: "MOVQ 0x7b, AX",
		},
	} {
		require.Equal(t, tc.exp, tc.in.String())
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := newAssemblerImpl()

	root := &nodeImpl{}
	a.addNode(root)
	require.Equal(t, a.root, root)
	require.Equal(t, a.current, root)
	require.Nil(t, root.next)

	next := &nodeImpl{}
	a.addNode(next)
	require.Equal(t, a.root, root)
	require.Equal(t, a.current, next)
	require.Equal(t, next, root.next)
	require.Nil(t, next.next)
}

func TestAssemblerImpl_newNode(t *testing.T) {
	a := newAssemblerImpl()
	actual := a.newNode(ADDL, operandTypesConstToMemory)
	require.Equal(t, ADDL, actual.instruction)
	require.Equal(t, operandTypeConst, actual.types.src)
	require.Equal(t, operandTypeMemory, actual.types.dst)
	require.Equal(t, actual, a.root)
	require.Equal(t, actual, a.current)
}

func TestAssemblerImpl_encodeNode(t *testing.T) {
	a := newAssemblerImpl()
	err := a.encodeNode(&nodeImpl{
		types: operandTypes{operandTypeBranch, operandTypeRegister},
	})
	require.EqualError(t, err, "encoder undefined for [from:branch,to:register] operand type")
}

func TestAssemblerImpl_Assemble(t *testing.T) {
	t.Run("callback", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			a := newAssemblerImpl()
			callbacked := false
			a.AddOnGenerateCallBack(func(b []byte) error { callbacked = true; return nil })
			_, err := a.Assemble()
			require.NoError(t, err)
			require.True(t, callbacked)
		})
		t.Run("error", func(t *testing.T) {
			a := newAssemblerImpl()
			a.AddOnGenerateCallBack(func(b []byte) error { return errors.New("some error") })
			_, err := a.Assemble()
			require.EqualError(t, err, "some error")
		})
	})
	t.Run("no reassemble", func(t *testing.T) {
		// TODO: remove golang-asm dependency in tests.
		goasm, err := newGolangAsmAssembler()
		require.NoError(t, err)

		a := newAssemblerImpl()
		for _, assembler := range []Assembler{a, goasm} {
			jmp := assembler.CompileJump(JCC)
			const dummyInstruction = CDQ
			assembler.CompileStandAlone(dummyInstruction)
			assembler.CompileStandAlone(dummyInstruction)
			assembler.CompileStandAlone(dummyInstruction)
			jmp.AssignJumpTarget(assembler.CompileStandAlone(dummyInstruction))
		}

		actual, err := a.Assemble()
		require.NoError(t, err)

		expected, err := goasm.Assemble()
		require.NoError(t, err)

		require.Equal(t, expected, actual)
	})
	t.Run("re-assemble", func(t *testing.T) {
		// TODO: remove golang-asm dependency in tests.
		goasm, err := newGolangAsmAssembler()
		require.NoError(t, err)

		a := newAssemblerImpl()
		for _, assembler := range []Assembler{a, goasm} {
			jmp := assembler.CompileJump(JCC)
			const dummyInstruction = CDQ
			// Ensure that at least 128 bytes between JCC and the target which results in
			// reassemble as we have to convert the forward jump as long variant.
			for i := 0; i < 128; i++ {
				assembler.CompileStandAlone(dummyInstruction)
			}
			jmp.AssignJumpTarget(assembler.CompileStandAlone(dummyInstruction))
		}

		a.initializeNodesForEncoding()

		// For the first encoding, we must be forced to reassemble.
		err = a.encode()
		require.NoError(t, err)
		require.True(t, a.forceReAssemble)
	})
}

func TestAssemblerImpl_Assemble_NOPPadding(t *testing.T) {
	t.Run("non relative jumps", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			setupFn func(assembler Assembler)
		}{
			{
				name: "RET",
				setupFn: func(assembler Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileStandAlone(RET)
					}
				},
			},
			{
				name: "JMP to register",
				setupFn: func(assembler Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileJumpToRegister(JMP, REG_AX)
					}
				},
			},
			{
				name: "JMP to memory",
				setupFn: func(assembler Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileJumpToMemory(JMP, REG_AX, 10)
					}
				},
			},
			{
				name: "JMP to memory large offset",
				setupFn: func(assembler Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileJumpToMemory(JMP, REG_AX, math.MaxInt32)
					}
				},
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				// TODO: remove golang-asm dependency in tests.
				goasm, err := newGolangAsmAssembler()
				require.NoError(t, err)

				a := newAssemblerImpl()
				for _, assembler := range []Assembler{a, goasm} {
					tc.setupFn(assembler)
				}

				actual, err := a.Assemble()
				require.NoError(t, err)

				expected, err := goasm.Assemble()
				require.NoError(t, err)

				require.Equal(t, expected, actual)
			})
		}
	})
	t.Run("relative jumps", func(t *testing.T) {
		for _, backward := range []bool{false} {
			backward := backward
			t.Run(fmt.Sprintf("backward=%v", backward), func(t *testing.T) {
				for _, instruction := range []asm.Instruction{JMP, JCC, JCS, JEQ, JGE, JGT, JHI, JLE, JLS, JLT, JMI, JNE, JPC, JPS} {
					instruction := instruction
					t.Run(instructionName(instruction), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)

						a := newAssemblerImpl()

						for _, assembler := range []Assembler{a, goasm} {
							head := assembler.CompileStandAlone(RET)
							var jmps []asm.Node
							for i := 0; i < 128; i++ { // Large enough so that this includes long jump.
								jmps = append(jmps, assembler.CompileJump(instruction))
							}
							tail := assembler.CompileStandAlone(RET)

							for _, jmp := range jmps {
								if backward {
									jmp.AssignJumpTarget(head)
								} else {
									jmp.AssignJumpTarget(tail)
								}
							}
						}

						actual, err := a.Assemble()
						require.NoError(t, err)

						expected, err := goasm.Assemble()
						require.NoError(t, err)

						require.Equal(t, expected, actual)
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_Assemble_NOPPadding_fusedJumps(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setupFn func(assembler Assembler)
	}{
		{
			name: "CMPL(register to const)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToConst(CMPL, REG_AX, math.MaxInt16)
			},
		},
		{
			name: "CMPL(memory to const)",
			setupFn: func(assembler Assembler) {
				assembler.CompileMemoryToConst(CMPL, REG_AX, 1, 10)
			},
		},
		{
			name: "CMPL(register to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(CMPL, REG_R14, REG_R10)
			},
		},
		{
			name: "CMPQ(register to const)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToConst(CMPQ, REG_AX, math.MaxInt16)
			},
		},
		{
			name: "CMPQ(register to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(CMPQ, REG_R14, REG_R10)
			},
		},
		{
			name: "TESTL",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(TESTL, REG_AX, REG_AX)
			},
		},
		{
			name: "TESTQ",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(TESTQ, REG_AX, REG_AX)
			},
		},
		{
			name: "ADDL (register to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(ADDL, REG_R10, REG_AX)
			},
		},
		{
			name: "ADDL(memory to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileMemoryToRegister(ADDL, REG_R10, 1234, REG_AX)
			},
		},
		{
			name: "ADDQ (register to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(ADDQ, REG_R10, REG_AX)
			},
		},
		{
			name: "ADDQ(memory to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileMemoryToRegister(ADDQ, REG_R10, 1234, REG_AX)
			},
		},
		{
			name: "ADDQ(const to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileConstToRegister(ADDQ, 1234, REG_R10)
			},
		},
		{
			name: "SUBL",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(SUBL, REG_R10, REG_AX)
			},
		},
		{
			name: "SUBQ (register to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(SUBQ, REG_R10, REG_AX)
			},
		},
		{
			name: "SUBQ (memory to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileMemoryToRegister(SUBQ, REG_R10, math.MaxInt16, REG_AX)
			},
		},
		{
			name: "ANDL",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(ANDL, REG_R10, REG_AX)
			},
		},
		{
			name: "ANDQ (register to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileRegisterToRegister(ANDQ, REG_R10, REG_AX)
			},
		},
		{
			name: "ANDQ (const to register)",
			setupFn: func(assembler Assembler) {
				assembler.CompileConstToRegister(ANDQ, -123, REG_R10)
			},
		},
		{
			name: "INCQ",
			setupFn: func(assembler Assembler) {
				assembler.CompileNoneToMemory(INCQ, REG_R10, 123)
			},
		},
		{
			name: "DECQ",
			setupFn: func(assembler Assembler) {
				assembler.CompileNoneToMemory(DECQ, REG_R10, 0)
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, jmpInst := range []asm.Instruction{JCC, JCS, JEQ, JGE, JGT, JHI, JLE, JLS, JLT, JMI, JNE, JPC, JPS} {
				t.Run(instructionName(jmpInst), func(t *testing.T) {

					// TODO: remove golang-asm dependency in tests.
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					a := newAssemblerImpl()

					for _, assembler := range []Assembler{a, goasm} {
						var jmps []asm.Node
						for i := 0; i < 12; i++ { // Large enough so that this includes long jump.
							tc.setupFn(assembler)
							jmps = append(jmps, assembler.CompileJump(jmpInst))
						}

						target := assembler.CompileStandAlone(RET)
						for _, jmp := range jmps {
							jmp.AssignJumpTarget(target)
						}
					}

					actual, err := a.Assemble()
					require.NoError(t, err)

					expected, err := goasm.Assemble()
					require.NoError(t, err)

					require.Equal(t, expected, actual)
				})
			}
		})
	}
}

func TestAssemblerImpl_padNOP(t *testing.T) {
	for _, tc := range []struct {
		num      int
		expected []byte
	}{
		{num: 1, expected: nopOpcodes[0][:1]},
		{num: 2, expected: nopOpcodes[1][:2]},
		{num: 3, expected: nopOpcodes[2][:3]},
		{num: 4, expected: nopOpcodes[3][:4]},
		{num: 5, expected: nopOpcodes[4][:5]},
		{num: 6, expected: nopOpcodes[5][:6]},
		{num: 7, expected: nopOpcodes[6][:7]},
		{num: 8, expected: nopOpcodes[7][:8]},
		{num: 9, expected: nopOpcodes[8][:9]},
		{num: 10, expected: append(nopOpcodes[8][:9], nopOpcodes[0][:1]...)},
		{num: 11, expected: append(nopOpcodes[8][:9], nopOpcodes[1][:2]...)},
		{num: 12, expected: append(nopOpcodes[8][:9], nopOpcodes[2][:3]...)},
		{num: 13, expected: append(nopOpcodes[8][:9], nopOpcodes[3][:4]...)},
		{num: 14, expected: append(nopOpcodes[8][:9], nopOpcodes[4][:5]...)},
		{num: 15, expected: append(nopOpcodes[8][:9], nopOpcodes[5][:6]...)},
		{num: 16, expected: append(nopOpcodes[8][:9], nopOpcodes[6][:7]...)},
		{num: 17, expected: append(nopOpcodes[8][:9], nopOpcodes[7][:8]...)},
		{num: 18, expected: append(nopOpcodes[8][:9], nopOpcodes[8][:9]...)},
	} {
		tc := tc
		t.Run(strconv.Itoa(tc.num), func(t *testing.T) {
			a := newAssemblerImpl()
			a.padNOP(tc.num)
			actual := a.buf.Bytes()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileStandAlone(RET)
	actualNode := a.current
	require.Equal(t, RET, actualNode.instruction)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeNone, actualNode.types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileConstToRegister(MOVQ, 1000, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, int64(1000), actualNode.srcConst)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeConst, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileRegisterToRegister(MOVQ, REG_BX, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileMemoryToRegister(MOVQ, REG_BX, 100, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(100), actualNode.srcConst)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileRegisterToMemory(MOVQ, REG_BX, REG_AX, 100)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, int64(100), actualNode.dstConst)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileJump(JMP)
	actualNode := a.current
	require.Equal(t, JMP, actualNode.instruction)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeBranch, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileJumpToRegister(JNE, REG_AX)
	actualNode := a.current
	require.Equal(t, JNE, actualNode.instruction)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileJumpToMemory(JNE, REG_AX, 100)
	actualNode := a.current
	require.Equal(t, JNE, actualNode.instruction)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, int64(100), actualNode.dstConst)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileReadInstructionAddress(REG_R10, RET)
	actualNode := a.current
	require.Equal(t, LEAQ, actualNode.instruction)
	require.Equal(t, REG_R10, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
	require.Equal(t, RET, actualNode.readInstructionAddressBeforeTargetInstruction)
}

func TestAssemblerImpl_CompileRegisterToRegisterWithMode(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileRegisterToRegisterWithMode(MOVQ, REG_BX, REG_AX, 123)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, byte(123), actualNode.mode)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryWithIndexToRegister(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileMemoryWithIndexToRegister(MOVQ, REG_BX, 100, REG_R10, 8, REG_AX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(100), actualNode.srcConst)
	require.Equal(t, REG_R10, actualNode.srcMemIndex)
	require.Equal(t, byte(8), actualNode.srcMemScale)
	require.Equal(t, REG_AX, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToConst(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileRegisterToConst(MOVQ, REG_BX, 123)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(123), actualNode.dstConst)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeConst, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToNone(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileRegisterToNone(MOVQ, REG_BX)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeNone, actualNode.types.dst)
}

func TestAssemblerImpl_CompileNoneToRegister(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileNoneToRegister(MOVQ, REG_BX)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.dstReg)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileNoneToMemory(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileNoneToMemory(MOVQ, REG_BX, 1234)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.dstReg)
	require.Equal(t, int64(1234), actualNode.dstConst)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileConstToMemory(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileConstToMemory(MOVQ, -9999, REG_BX, 1234)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.dstReg)
	require.Equal(t, int64(-9999), actualNode.srcConst)
	require.Equal(t, int64(1234), actualNode.dstConst)
	require.Equal(t, operandTypeConst, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryToConst(t *testing.T) {
	a := newAssemblerImpl()
	a.CompileMemoryToConst(MOVQ, REG_BX, 1234, -9999)
	actualNode := a.current
	require.Equal(t, REG_BX, actualNode.srcReg)
	require.Equal(t, int64(1234), actualNode.srcConst)
	require.Equal(t, int64(-9999), actualNode.dstConst)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeConst, actualNode.types.dst)
}

func TestAssemblerImpl_encodeNoneToNone(t *testing.T) {
	for _, tc := range []struct {
		inst   asm.Instruction
		exp    []byte
		expErr bool
	}{
		{inst: ADDL, expErr: true},
		{inst: CDQ, exp: []byte{0x99}},
		{inst: CQO, exp: []byte{0x48, 0x99}},
		{inst: NOP, exp: nil},
		{inst: RET, exp: []byte{0xc3}},
	} {
		tc := tc
		t.Run(instructionName(tc.inst), func(t *testing.T) {
			a := newAssemblerImpl()
			err := a.encodeNoneToNone(&nodeImpl{instruction: tc.inst, types: operandTypesNoneToNone})
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.exp, a.buf.Bytes())
			}
		})
	}
}

var intRegisters = []asm.Register{
	REG_AX, REG_CX, REG_DX, REG_BX, REG_SP, REG_BP, REG_SI, REG_DI,
	REG_R8, REG_R9, REG_R10, REG_R11, REG_R12, REG_R13, REG_R14, REG_R15,
}

var floatRegisters = []asm.Register{
	REG_X0, REG_X1, REG_X2, REG_X3, REG_X4, REG_X5, REG_X6, REG_X7,
	REG_X8, REG_X9, REG_X10, REG_X11, REG_X12, REG_X13, REG_X14, REG_X15,
}

func TestAssemblerImpl_encodeNoneToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		a := newAssemblerImpl()
		err := a.encodeNoneToRegister(&nodeImpl{instruction: ADDL,
			types: operandTypesNoneToRegister, dstReg: REG_AX})
		require.Error(t, err)

		t.Run("error", func(t *testing.T) {
			for _, tc := range []struct {
				n      *nodeImpl
				expErr string
			}{
				{
					n:      &nodeImpl{instruction: ADDL, types: operandTypesNoneToRegister, dstReg: REG_AX},
					expErr: "ADDL is unsupported for from:none,to:register type",
				},
				{
					n:      &nodeImpl{instruction: JMP, types: operandTypesNoneToRegister},
					expErr: "invalid register [nil]",
				},
			} {
				t.Run(tc.expErr, func(t *testing.T) {
					tc := tc
					a := newAssemblerImpl()
					err := a.encodeNoneToRegister(tc.n)
					require.EqualError(t, err, tc.expErr)
				})
			}
		})
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{
			JMP, SETCC, SETCS, SETEQ, SETGE, SETGT, SETHI, SETLE, SETLS, SETLT, SETNE, SETPC, SETPS,
		} {
			inst := inst
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileNoneToRegister(inst, reg)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := newAssemblerImpl()
						err = a.encodeNoneToRegister(&nodeImpl{instruction: inst,
							types: operandTypesNoneToRegister, dstReg: reg})
						require.NoError(t, err)
						require.Equal(t, bs, a.buf.Bytes())
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_encodeNoneToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesNoneToMemory, dstReg: REG_AX},
				expErr: "ADDL is unsupported for from:none,to:memory type",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeNoneToMemory(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{DECQ, INCQ, JMP} {
			inst := inst
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						for _, offset := range []int64{0, 1, -1, 1243, math.MaxInt32, math.MinInt16} {
							offset := offset
							t.Run(fmt.Sprintf("0x%x", uint32(offset)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileNoneToMemory(inst, reg, int64(offset))
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeNoneToMemory(&nodeImpl{instruction: inst,
									types: operandTypesNoneToMemory, dstReg: reg, dstConst: int64(offset)})
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_resolveForwardRelativeJumps(t *testing.T) {
	t.Run("long jump", func(t *testing.T) {
		t.Run("error", func(t *testing.T) {
			originOffset, targetOffset := uint64(0), uint64(math.MaxInt64)
			origin := &nodeImpl{instruction: JMP, offsetInBinary: originOffset}
			target := &nodeImpl{offsetInBinary: targetOffset, jumpOrigins: map[*nodeImpl]struct{}{origin: {}}}
			a := newAssemblerImpl()
			err := a.resolveForwardRelativeJumps(target)
			require.EqualError(t, err, "too large jump offset 9223372036854775802 for encoding JMP")
		})
		t.Run("ok", func(t *testing.T) {
			originOffset := uint64(0)
			for _, tc := range []struct {
				instruction                asm.Instruction
				targetOffset               uint64
				expectedOffsetFromEIP      int32
				writtenOffsetIndexInBinary int
			}{
				{
					instruction: JMP, targetOffset: 1234,
					writtenOffsetIndexInBinary: 1,        // JMP has one opcode byte for long jump.
					expectedOffsetFromEIP:      1234 - 5, // the instruction length of long relative JMP.
				},
				{
					instruction: JCC, targetOffset: 1234,
					writtenOffsetIndexInBinary: 2,        // Conditional jumps has two opcode for long jump.
					expectedOffsetFromEIP:      1234 - 6, // the instruction length of long relative JCC
				},
			} {
				origin := &nodeImpl{instruction: tc.instruction, offsetInBinary: originOffset}
				target := &nodeImpl{offsetInBinary: tc.targetOffset, jumpOrigins: map[*nodeImpl]struct{}{origin: {}}}
				a := newAssemblerImpl()

				// Grow the capacity of buffer so that we could put the offset.
				a.buf.Write([]byte{0, 0, 0, 0, 0, 0}) // Relative long jumps are at most 6 bytes.

				err := a.resolveForwardRelativeJumps(target)
				require.NoError(t, err)

				actual := binary.LittleEndian.Uint32(a.buf.Bytes()[tc.writtenOffsetIndexInBinary:])
				require.Equal(t, tc.expectedOffsetFromEIP, int32(actual))
			}
		})
	})
	t.Run("short jump", func(t *testing.T) {
		t.Run("reassemble", func(t *testing.T) {
			originOffset := uint64(0)
			for _, tc := range []struct {
				instruction  asm.Instruction
				targetOffset uint64
			}{
				{
					instruction:  JMP,
					targetOffset: 10000,
				},
				{
					instruction: JMP,
					// Relative jump offset = 130 - len(JMP instruction bytes) = 130 - 2 = 128 > math.MaxInt8.
					targetOffset: 130,
				},
				{
					instruction:  JCC,
					targetOffset: 10000,
				},
				{
					instruction: JCC,
					// Relative jump offset = 130 - len(JCC instruction bytes) = 130 -2 = 128 > math.MaxInt8.
					targetOffset: 130,
				},
			} {
				origin := &nodeImpl{instruction: tc.instruction, offsetInBinary: originOffset, flag: nodeFlagShortForwardJump}
				target := &nodeImpl{offsetInBinary: tc.targetOffset, jumpOrigins: map[*nodeImpl]struct{}{origin: {}}}
				origin.jumpTarget = target

				a := newAssemblerImpl()
				err := a.resolveForwardRelativeJumps(target)
				require.NoError(t, err)

				require.True(t, a.forceReAssemble)
				require.True(t, origin.flag&nodeFlagShortForwardJump == 0)
			}
		})
		t.Run("ok", func(t *testing.T) {
			originOffset := uint64(0)
			for _, tc := range []struct {
				instruction           asm.Instruction
				targetOffset          uint64
				expectedOffsetFromEIP byte
			}{
				{
					instruction: JMP, targetOffset: 129,
					expectedOffsetFromEIP: 129 - 2, // short jumps are of 2 bytes.
				},
				{
					instruction: JCC, targetOffset: 129,
					expectedOffsetFromEIP: 129 - 2, // short jumps are of 2 bytes.
				},
			} {
				origin := &nodeImpl{instruction: tc.instruction, offsetInBinary: originOffset, flag: nodeFlagShortForwardJump}
				target := &nodeImpl{offsetInBinary: tc.targetOffset, jumpOrigins: map[*nodeImpl]struct{}{origin: {}}}
				origin.jumpTarget = target

				a := newAssemblerImpl()

				// Grow the capacity of buffer so that we could put the offset.
				a.buf.Write([]byte{0, 0}) // Relative short jumps are of 2 bytes.

				err := a.resolveForwardRelativeJumps(target)
				require.NoError(t, err)

				actual := a.buf.Bytes()[1] // For short jumps, the opcode has one opcode so the offset is writte at 2nd byte.
				require.Equal(t, tc.expectedOffsetFromEIP, actual)
			}
		})
	})
}

func TestAssemblerImpl_encodeNoneToBranch(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{types: operandTypesNoneToBranch, instruction: JMP},
				expErr: "jump traget must not be nil for relative JMP",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeRelativeJump(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("backward jump", func(t *testing.T) {
		t.Run("too large offset", func(t *testing.T) {
			a := newAssemblerImpl()
			targetOffsetInBinary := uint64(0)
			offsetInBinary := uint64(math.MaxInt32)
			node := &nodeImpl{instruction: JMP,
				jumpTarget: &nodeImpl{
					offsetInBinary: targetOffsetInBinary, jumpOrigins: map[*nodeImpl]struct{}{},
				},
				flag:           nodeFlagBackwardJump,
				offsetInBinary: offsetInBinary}
			err := a.encodeRelativeJump(node)
			require.Error(t, err)
		})

		for _, isShortJump := range []bool{true, false} {
			isShortJump := isShortJump
			t.Run(fmt.Sprintf("is_short_jump=%v", isShortJump), func(t *testing.T) {
				for _, inst := range []asm.Instruction{
					JCC, JCS, JEQ, JGE, JGT, JHI, JLE, JLS, JLT, JMI,
					JMP, JNE, JPC, JPS,
				} {
					inst := inst
					t.Run(instructionName(inst), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)

						a := newAssemblerImpl()
						for _, assembler := range []Assembler{goasm, a} {
							const dummyInstruction = CDQ
							target := assembler.CompileStandAlone(dummyInstruction)
							if !isShortJump {
								for i := 0; i < 128; i++ {
									assembler.CompileStandAlone(dummyInstruction)
								}
							}
							jmp := assembler.CompileJump(inst)
							jmp.AssignJumpTarget(target)
						}

						actual, err := a.Assemble()
						require.NoError(t, err)
						expected, err := goasm.Assemble()
						require.NoError(t, err)

						require.Equal(t, expected, actual)
					})
				}
			})
		}
	})
	t.Run("forward jump", func(t *testing.T) {
		for _, isShortJump := range []bool{true, false} {
			isShortJump := isShortJump
			t.Run(fmt.Sprintf("is_short_jump=%v", isShortJump), func(t *testing.T) {
				for _, inst := range []asm.Instruction{
					JCC, JCS, JEQ, JGE, JGT, JHI, JLE, JLS, JLT, JMI,
					JMP, JNE, JPC, JPS,
				} {
					inst := inst
					t.Run(instructionName(inst), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)

						a := newAssemblerImpl()
						for _, assembler := range []Assembler{goasm, a} {
							const dummyInstruction = CDQ
							jmp := assembler.CompileJump(inst)

							if !isShortJump {
								for i := 0; i < 128; i++ {
									assembler.CompileStandAlone(dummyInstruction)
								}
							}
							target := assembler.CompileStandAlone(dummyInstruction)
							jmp.AssignJumpTarget(target)
						}

						actual, err := a.Assemble()
						require.NoError(t, err)
						expected, err := goasm.Assemble()
						require.NoError(t, err)

						require.Equal(t, expected, actual)
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_encodeRegisterToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesRegisterToNone, srcReg: REG_AX},
				expErr: "ADDL is unsupported for from:register,to:none type",
			},
			{
				n:      &nodeImpl{instruction: DIVQ, types: operandTypesRegisterToNone},
				expErr: "invalid register [nil]",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeRegisterToNone(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{DIVL, DIVQ, IDIVL, IDIVQ, MULL, MULQ} {

			inst := inst
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileRegisterToNone(inst, reg)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := newAssemblerImpl()
						err = a.encodeRegisterToNone(&nodeImpl{instruction: inst,
							types: operandTypesNoneToRegister, srcReg: reg})
						require.NoError(t, err)
						require.Equal(t, bs, a.buf.Bytes())
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_encodeRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: JMP, types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_AX},
				expErr: "JMP is unsupported for from:register,to:register type",
			},
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesRegisterToRegister, dstReg: REG_AX},
				expErr: "invalid register [nil]",
			},
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesRegisterToRegister, srcReg: REG_AX},
				expErr: "invalid register [nil]",
			}, {
				n:      &nodeImpl{instruction: MOVL, types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_X1},
				expErr: "MOVL for float to float is undefined",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeRegisterToRegister(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	intRegisters := []asm.Register{REG_AX, REG_R8}
	floatRegisters := []asm.Register{REG_X0, REG_X8}
	allRegisters := append(intRegisters, floatRegisters...)
	for _, tc := range []struct {
		instruction      asm.Instruction
		srcRegs, dstRegs []asm.Register
		mode             byte
	}{
		{instruction: ADDL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: ADDQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: ADDSD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ADDSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ANDL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: ANDPD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ANDPS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ANDQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: BSRL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: BSRQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: CMOVQCS, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: CMPL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: CMPQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: COMISD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: COMISS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: CVTSD2SS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: CVTSL2SD, srcRegs: intRegisters, dstRegs: floatRegisters},
		{instruction: CVTSL2SS, srcRegs: intRegisters, dstRegs: floatRegisters},
		{instruction: CVTSQ2SD, srcRegs: intRegisters, dstRegs: floatRegisters},
		{instruction: CVTSQ2SS, srcRegs: intRegisters, dstRegs: floatRegisters},
		{instruction: CVTSS2SD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: CVTTSD2SL, srcRegs: floatRegisters, dstRegs: intRegisters},
		{instruction: CVTTSD2SQ, srcRegs: floatRegisters, dstRegs: intRegisters},
		{instruction: CVTTSS2SL, srcRegs: floatRegisters, dstRegs: intRegisters},
		{instruction: CVTTSS2SQ, srcRegs: floatRegisters, dstRegs: intRegisters},
		{instruction: DIVSD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: DIVSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: LZCNTL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: LZCNTQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MAXSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: MINSD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: MAXSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: MINSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: MOVBLSX, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MOVBLZX, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MOVBQSX, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MOVLQSX, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MOVL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MOVL, srcRegs: intRegisters, dstRegs: floatRegisters},
		{instruction: MOVL, srcRegs: floatRegisters, dstRegs: intRegisters},
		{instruction: MOVQ, srcRegs: allRegisters, dstRegs: allRegisters},
		{instruction: MOVWLSX, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MOVWQSX, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: MULSD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: MULSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ORL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: ORPD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ORPS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: ORQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: POPCNTL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: POPCNTQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: ROLL, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: ROLQ, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: RORL, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: RORQ, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: ROUNDSD, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x00},
		{instruction: ROUNDSS, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x00},
		{instruction: ROUNDSD, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x01},
		{instruction: ROUNDSS, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x01},
		{instruction: ROUNDSD, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x02},
		{instruction: ROUNDSS, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x02},
		{instruction: ROUNDSD, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x03},
		{instruction: ROUNDSS, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x03},
		{instruction: ROUNDSD, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x04},
		{instruction: ROUNDSS, srcRegs: floatRegisters, dstRegs: floatRegisters, mode: 0x04},
		{instruction: SARL, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: SARQ, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: SHLL, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: SHLQ, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: SHRL, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: SHRQ, srcRegs: []asm.Register{REG_CX}, dstRegs: intRegisters},
		{instruction: SQRTSD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: SQRTSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: SUBL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: SUBQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: SUBSD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: SUBSS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: TESTL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: TESTQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: TZCNTL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: TZCNTQ, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: UCOMISD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: UCOMISS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: XORL, srcRegs: intRegisters, dstRegs: intRegisters},
		{instruction: XORPD, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: XORPS, srcRegs: floatRegisters, dstRegs: floatRegisters},
		{instruction: XORQ, srcRegs: intRegisters, dstRegs: intRegisters},
	} {
		tc := tc
		t.Run(instructionName(tc.instruction), func(t *testing.T) {
			t.Run("error", func(t *testing.T) {
				srcFloat, dstFloat := isFloatRegister(tc.srcRegs[0]), isFloatRegister(tc.dstRegs[0])
				isMOV := tc.instruction == MOVL || tc.instruction == MOVQ
				a := newAssemblerImpl()
				if _, isShiftOp := registerToRegisterShiftOpcode[tc.instruction]; isShiftOp {
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_CX, dstReg: REG_X0}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_X0}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_CX}))
				} else if srcFloat && dstFloat && !isMOV { // Float to Float
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_AX}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_AX}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_X0}))
				} else if srcFloat && !dstFloat && !isMOV { // Float to Int
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_X1}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_X1}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_AX}))
				} else if !srcFloat && dstFloat && !isMOV { // Int to Float
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_AX}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_AX}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_X0}))
				} else if !isMOV { // Int to Int
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_X0}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_X0, dstReg: REG_AX}))
					require.Error(t, a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
						types: operandTypesRegisterToRegister, srcReg: REG_AX, dstReg: REG_X0}))
				}
			})
			for _, srcReg := range tc.srcRegs {
				srcReg := srcReg
				t.Run(fmt.Sprintf("src=%s", registerName(srcReg)), func(t *testing.T) {
					srcReg := srcReg
					for _, dstReg := range tc.dstRegs {
						dstReg := dstReg
						t.Run(fmt.Sprintf("dst=%s", registerName(dstReg)), func(t *testing.T) {
							// TODO: remove golang-asm dependency in tests.
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)
							if tc.instruction == ROUNDSD || tc.instruction == ROUNDSS {
								goasm.CompileRegisterToRegisterWithMode(tc.instruction, srcReg, dstReg, tc.mode)
							} else {
								goasm.CompileRegisterToRegister(tc.instruction, srcReg, dstReg)
							}
							bs, err := goasm.Assemble()
							require.NoError(t, err)

							a := newAssemblerImpl()
							err = a.encodeRegisterToRegister(&nodeImpl{instruction: tc.instruction,
								types: operandTypesRegisterToRegister, srcReg: srcReg, dstReg: dstReg,
								mode: tc.mode,
							})
							require.NoError(t, err)
							// fmt.Printf("modRM: want: 0b%b, got: 0b%b\n", bs[1], a.buf.Bytes()[1])
							require.Equal(t, bs, a.buf.Bytes())
						})
					}
				})
			}
		})
	}
}

func TestAssemblerImpl_encodeRegisterToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n: &nodeImpl{instruction: JMP,
					types:  operandTypesRegisterToMemory,
					srcReg: REG_AX, dstReg: REG_AX},
				expErr: "JMP is unsupported for from:register,to:memory type",
			},
			{
				n: &nodeImpl{instruction: SHLQ,
					types:  operandTypesRegisterToMemory,
					srcReg: REG_AX, dstReg: REG_AX},
				expErr: "shifting instruction SHLQ require CX register as src but got AX",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {

				a := newAssemblerImpl()
				require.EqualError(t, a.encodeRegisterToMemory(tc.n), tc.expErr)
			})
		}
	})

	t.Run("non shift", func(t *testing.T) {
		scales := []byte{1, 4}
		for _, instruction := range []asm.Instruction{CMPL, CMPQ, MOVB, MOVL, MOVQ, MOVW} {
			regs := []asm.Register{REG_AX, REG_R8}
			srcRegs := regs
			if instruction == MOVL || instruction == MOVQ {
				srcRegs = append(srcRegs, REG_X0, REG_X10)
			}
			for _, srcReg := range srcRegs {
				srcReg := srcReg
				for _, dstReg := range regs {
					dstReg := dstReg
					for _, offset := range []int64{
						0, 1, math.MaxInt32,
					} {
						offset := offset
						n := &nodeImpl{instruction: instruction,
							types:  operandTypesRegisterToMemory,
							srcReg: srcReg, dstReg: dstReg, dstConst: offset}

						// Without index.
						t.Run(n.String(), func(t *testing.T) {
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)
							goasm.CompileRegisterToMemory(instruction, srcReg, dstReg, offset)
							bs, err := goasm.Assemble()
							require.NoError(t, err)

							a := newAssemblerImpl()
							err = a.encodeRegisterToMemory(n)
							require.NoError(t, err)
							require.Equal(t, bs, a.buf.Bytes())
						})

						// With index.
						for _, indexReg := range regs {
							n.dstMemIndex = indexReg
							for _, scale := range scales {
								n.dstMemScale = scale
								t.Run(n.String(), func(t *testing.T) {
									goasm, err := newGolangAsmAssembler()
									require.NoError(t, err)
									goasm.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstReg, offset, indexReg, int16(scale))
									bs, err := goasm.Assemble()
									require.NoError(t, err)

									a := newAssemblerImpl()
									err = a.encodeRegisterToMemory(n)
									require.NoError(t, err)
									require.Equal(t, bs, a.buf.Bytes())
								})
							}
						}
					}
				}
			}
		}
	})
	t.Run("shift", func(t *testing.T) {
		regs := []asm.Register{REG_AX, REG_R8}
		scales := []byte{1, 4}
		for _, instruction := range []asm.Instruction{SARL, SARQ, SHLL, SHLQ, SHRL, SHRQ} {
			for _, dstReg := range regs {
				dstReg := dstReg
				for _, offset := range []int64{
					0, 1, math.MaxInt32,
				} {
					offset := offset
					n := &nodeImpl{instruction: instruction,
						types:  operandTypesRegisterToMemory,
						srcReg: REG_CX, dstReg: dstReg, dstConst: offset}

					// Without index.
					t.Run(n.String(), func(t *testing.T) {
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileRegisterToMemory(instruction, REG_CX, dstReg, offset)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := newAssemblerImpl()
						err = a.encodeRegisterToMemory(n)
						require.NoError(t, err)
						require.Equal(t, bs, a.buf.Bytes())
					})

					// With index.
					for _, indexReg := range regs {
						n.dstMemIndex = indexReg
						for _, scale := range scales {
							n.dstMemScale = scale
							t.Run(n.String(), func(t *testing.T) {
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileRegisterToMemoryWithIndex(instruction, REG_CX, dstReg, offset, indexReg, int16(scale))
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeRegisterToMemory(n)
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())
							})
						}
					}
				}
			}
		}
	})
}

func TestAssemblerImpl_encodeRegisterToConst(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesRegisterToConst, srcReg: REG_AX},
				expErr: "ADDL is unsupported for from:register,to:const type",
			},
			{
				n:      &nodeImpl{instruction: DIVQ, types: operandTypesRegisterToConst},
				expErr: "invalid register [nil]",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeRegisterToNone(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{CMPL, CMPQ} {
			inst := inst
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						for _, c := range []int64{0, 1, -1, 1243, -1234, math.MaxInt32, math.MinInt32, math.MaxInt16, math.MinInt16} {
							c := c
							t.Run(fmt.Sprintf("0x%x", uint64(c)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileRegisterToConst(inst, reg, c)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeRegisterToConst(&nodeImpl{instruction: inst,
									types: operandTypesRegisterToConst, srcReg: reg, dstConst: c})
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_encodeReadInstructionAddress(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		const targetBeforeInstruction = RET
		for _, dstReg := range []asm.Register{REG_AX, REG_R8} {
			dstReg := dstReg
			t.Run(registerName(dstReg), func(t *testing.T) {
				goasm, err := newGolangAsmAssembler()
				require.NoError(t, err)

				a := newAssemblerImpl()

				// Setup target.
				for _, assembler := range []Assembler{a, goasm} {
					assembler.CompileReadInstructionAddress(dstReg, targetBeforeInstruction)
					assembler.CompileStandAlone(CDQ) // Dummy.
					assembler.CompileStandAlone(targetBeforeInstruction)
					assembler.CompileStandAlone(CDQ) // Target.
				}

				actual, err := a.Assemble()
				require.NoError(t, err)
				expected, err := goasm.Assemble()
				require.NoError(t, err)

				require.Equal(t, expected, actual)
			})
		}
	})
	t.Run("not found", func(t *testing.T) {
		a := newAssemblerImpl()
		a.CompileReadInstructionAddress(REG_R10, NOP)
		a.CompileStandAlone(CDQ)
		_, err := a.Assemble()
		require.EqualError(t, err, "BUG: target instruction not found for read instruction address")
	})
	t.Run("offset too large", func(t *testing.T) {
		a := newAssemblerImpl()
		a.CompileReadInstructionAddress(REG_R10, RET)
		a.CompileStandAlone(RET)
		a.CompileStandAlone(CDQ)

		for n := a.root; n != nil; n = n.next {
			n.offsetInBinary = (uint64(a.buf.Len()))

			err := a.encodeNode(n)
			require.NoError(t, err)
		}

		require.Len(t, a.OnGenerateCallbacks, 1)
		cb := a.OnGenerateCallbacks[0]

		targetNode := a.current
		targetNode.offsetInBinary = (uint64(math.MaxInt64))

		err := cb(nil)
		require.EqualError(t, err, "BUG: too large offset for LEAQ instruction")
	})
}

func TestAssemblerImpl_encodeMemoryToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		n := &nodeImpl{instruction: JMP,
			types:  operandTypesMemoryToRegister,
			srcReg: REG_AX, dstReg: REG_AX}
		a := newAssemblerImpl()
		require.EqualError(t, a.encodeMemoryToRegister(n), "JMP is unsupported for from:memory,to:register type")
	})
	intRegs := []asm.Register{REG_AX, REG_R8}
	floatRegs := []asm.Register{REG_X0, REG_X8}
	scales := []byte{1, 4}
	for _, tc := range []struct {
		instruction asm.Instruction
		isFloatInst bool
	}{
		{instruction: ADDL},
		{instruction: ADDQ},
		{instruction: CMPL},
		{instruction: CMPQ},
		{instruction: LEAQ},
		{instruction: MOVBLSX},
		{instruction: MOVBLZX},
		{instruction: MOVBQSX},
		{instruction: MOVBQZX},
		{instruction: MOVLQSX},
		{instruction: MOVLQZX},
		{instruction: MOVL},
		{instruction: MOVQ},
		{instruction: MOVWLSX},
		{instruction: MOVWLZX},
		{instruction: MOVWQSX},
		{instruction: MOVWQZX},
		{instruction: SUBQ},
		{instruction: SUBSD, isFloatInst: true},
		{instruction: SUBSS, isFloatInst: true},
		{instruction: UCOMISD, isFloatInst: true},
		{instruction: UCOMISS, isFloatInst: true},
	} {
		tc := tc
		for _, srcReg := range intRegs {
			srcReg := srcReg
			dstRegs := intRegs
			if tc.instruction == MOVL || tc.instruction == MOVQ {
				dstRegs = append(dstRegs, floatRegs...)
			} else if tc.isFloatInst {
				dstRegs = floatRegs
			}

			for _, dstReg := range dstRegs {
				dstReg := dstReg
				for _, offset := range []int64{
					0, 1, math.MaxInt32,
				} {
					offset := offset
					n := &nodeImpl{instruction: tc.instruction,
						types:  operandTypesMemoryToRegister,
						srcReg: srcReg, srcConst: offset, dstReg: dstReg}

					// Without index.
					t.Run(n.String(), func(t *testing.T) {
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileMemoryToRegister(tc.instruction, srcReg, offset, dstReg)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := newAssemblerImpl()
						err = a.encodeMemoryToRegister(n)
						require.NoError(t, err)
						require.Equal(t, bs, a.buf.Bytes())
					})

					// With index.
					for _, indexReg := range intRegs {
						n.srcMemIndex = indexReg
						for _, scale := range scales {
							n.srcMemScale = scale
							t.Run(n.String(), func(t *testing.T) {
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileMemoryWithIndexToRegister(tc.instruction, srcReg, offset, indexReg, int16(scale), dstReg)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeMemoryToRegister(n)
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())
							})
						}
					}
				}
			}
		}
	}
}

func TestAssemblerImpl_encodeConstToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: RET, types: operandTypesConstToRegister, dstReg: REG_AX},
				expErr: "RET is unsupported for from:const,to:register type",
			},
			{
				n:      &nodeImpl{instruction: PSLLL, types: operandTypesConstToRegister},
				expErr: "invalid register [nil]",
			},
			{
				n:      &nodeImpl{instruction: PSLLL, types: operandTypesConstToRegister, dstReg: REG_AX},
				expErr: "PSLLL needs float register but got AX",
			},
			{
				n:      &nodeImpl{instruction: ADDQ, types: operandTypesConstToRegister, dstReg: REG_X0},
				expErr: "ADDQ needs int register but got X0",
			},
			{
				n:      &nodeImpl{instruction: PSLLL, types: operandTypesConstToRegister, dstReg: REG_X0, srcConst: 2199023255552},
				expErr: "constant must fit in 32-bit integer for PSLLL, but got 2199023255552",
			},
			{
				n:      &nodeImpl{instruction: SHLQ, types: operandTypesConstToRegister, dstReg: REG_R10, srcConst: 32768},
				expErr: "constant must fit in positive 8-bit integer for SHLQ, but got 32768",
			},
			{
				n:      &nodeImpl{instruction: PSRLQ, types: operandTypesConstToRegister, dstReg: REG_X0, srcConst: 32768},
				expErr: "constant must fit in signed 8-bit integer for PSRLQ, but got 32768",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeConstToRegister(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("int instructions", func(t *testing.T) {
		for _, inst := range []asm.Instruction{ADDQ, ANDQ, MOVL, MOVQ, SHLQ, SHRQ, XORL, XORQ} {
			inst := inst
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						for _, c := range []int64{
							0, 1, -1, 11, -11, 1243, -1234, math.MaxUint8, math.MaxInt32, math.MinInt32, math.MaxInt16,
							math.MaxUint32,
							math.MinInt16, math.MaxInt64, math.MinInt64} {
							if inst != MOVQ && !fitIn32bit(c) {
								continue
							} else if (inst == SHLQ || inst == SHRQ) && (c < 0 || c > math.MaxUint8) {
								continue
							}
							c := c
							t.Run(fmt.Sprintf("0x%x", uint64(c)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileConstToRegister(inst, c, reg)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeConstToRegister(&nodeImpl{instruction: inst,
									types: operandTypesConstToRegister, srcConst: c, dstReg: reg})
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
	t.Run("float instructions", func(t *testing.T) {
		for _, inst := range []asm.Instruction{PSLLL, PSLLQ, PSRLL, PSRLQ} {
			inst := inst
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range floatRegisters {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						for _, c := range []int64{0, 1, -1, math.MaxInt8, math.MinInt8} {
							c := c
							t.Run(fmt.Sprintf("0x%x", uint64(c)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileConstToRegister(inst, c, reg)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeConstToRegister(&nodeImpl{instruction: inst,
									types: operandTypesConstToRegister, srcConst: c, dstReg: reg})
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_encodeMemoryToConst(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesMemoryToConst, dstReg: REG_AX},
				expErr: "ADDL is unsupported for from:memory,to:const type",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeMemoryToConst(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	const inst = CMPL
	t.Run("ok", func(t *testing.T) {
		for _, reg := range []asm.Register{REG_AX, REG_R8} {
			reg := reg
			t.Run(registerName(reg), func(t *testing.T) {
				for _, offset := range []int64{0, 1, -1, 1243, -1234, math.MaxInt32, math.MinInt32, math.MaxInt16, math.MinInt16} {
					offset := offset
					t.Run(fmt.Sprintf("offset=0x%x", uint32(offset)), func(t *testing.T) {
						for _, c := range []int64{0, 1, -1, 100, -100, math.MaxInt8, math.MinInt8, math.MaxInt16,
							math.MinInt16, 1 << 20, -(1 << 20), math.MaxInt32, math.MinInt32} {
							c := c
							t.Run(fmt.Sprintf("const=0x%x", uint64(c)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileMemoryToConst(inst, reg, int64(offset), c)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := newAssemblerImpl()
								err = a.encodeMemoryToConst(&nodeImpl{instruction: inst,
									types: operandTypesMemoryToConst, srcReg: reg, srcConst: int64(offset), dstConst: c})
								require.NoError(t, err)
								require.Equal(t, bs, a.buf.Bytes())

							})
						}
					})
				}
			})
		}
	})

}

func TestAssemblerImpl_encodeConstToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesConstToMemory, dstReg: REG_AX},
				expErr: "ADDL is unsupported for from:const,to:memory type",
			},
			{
				n: &nodeImpl{instruction: MOVB, types: operandTypesConstToMemory,
					srcConst: math.MaxInt16,
					dstReg:   REG_AX, dstConst: 0xff_ff},
				expErr: "too large load target const 32767 for MOVB",
			},
			{
				n: &nodeImpl{instruction: MOVL, types: operandTypesConstToMemory,
					srcConst: math.MaxInt64,
					dstReg:   REG_AX, dstConst: 0xff_ff},
				expErr: "too large load target const 9223372036854775807 for MOVL",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := newAssemblerImpl()
				err := a.encodeConstToMemory(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	constsMOVB := []int64{0, 1, -1, 100, -100, math.MaxInt8, math.MinInt8}
	constsMOVL := append(constsMOVB, []int64{math.MaxInt16, math.MinInt16, 1 << 20, -(1 << 20), math.MaxInt32, math.MinInt32}...)
	constsMOVQ := constsMOVL
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{MOVB, MOVL, MOVQ} {
			inst := inst
			var consts []int64
			switch inst {
			case MOVB:
				consts = constsMOVB
			case MOVL:
				consts = constsMOVL
			case MOVQ:
				consts = constsMOVQ
			}
			t.Run(instructionName(inst), func(t *testing.T) {
				for _, reg := range []asm.Register{REG_AX, REG_R8} {
					reg := reg
					t.Run(registerName(reg), func(t *testing.T) {
						for _, offset := range []int64{0, 1, -1, 1243, -1234, math.MaxInt32, math.MinInt32, math.MaxInt16, math.MinInt16} {
							offset := offset
							t.Run(fmt.Sprintf("offset=0x%x", uint32(offset)), func(t *testing.T) {
								for _, c := range consts {
									c := c
									t.Run(fmt.Sprintf("const=0x%x", uint64(c)), func(t *testing.T) {
										// TODO: remove golang-asm dependency in tests.
										goasm, err := newGolangAsmAssembler()
										require.NoError(t, err)
										goasm.CompileConstToMemory(inst, c, reg, int64(offset))
										bs, err := goasm.Assemble()
										require.NoError(t, err)

										a := newAssemblerImpl()
										err = a.encodeConstToMemory(&nodeImpl{instruction: inst,
											types: operandTypesConstToMemory, srcConst: c, dstReg: reg, dstConst: int64(offset)})
										require.NoError(t, err)
										require.Equal(t, bs, a.buf.Bytes())

									})
								}
							})
						}
					})
				}
			})
		}
	})
}

func TestNodeImpl_getMemoryLocation(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, tc := range []struct {
			n      *nodeImpl
			expErr string
		}{
			{
				n:      &nodeImpl{instruction: ADDL, types: operandTypesMemoryToRegister, srcConst: math.MaxInt64, srcReg: REG_AX, dstReg: REG_R10},
				expErr: "offset does not fit in 32-bit integer",
			},
			{
				n: &nodeImpl{instruction: ADDL, types: operandTypesMemoryToRegister,
					srcConst: 10, srcReg: asm.NilRegister, srcMemIndex: REG_R12, srcMemScale: 1, dstReg: REG_R10},
				expErr: "addressing without base register but with index is not implemented",
			},
			{
				n: &nodeImpl{instruction: ADDL, types: operandTypesMemoryToRegister,
					srcConst: 10, srcReg: REG_AX, srcMemIndex: REG_SP, srcMemScale: 1, dstReg: REG_R10},
				expErr: "SP cannot be used for SIB index",
			},
			{
				n: &nodeImpl{instruction: ADDL, types: operandTypesMemoryToRegister,
					srcConst: 10, srcReg: REG_AX, srcMemIndex: REG_R9, srcMemScale: 3, dstReg: REG_R10},
				expErr: "scale in SIB must be one of 1, 2, 4, 8 but got 3",
			},
		} {
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				_, _, _, _, err := tc.n.getMemoryLocation()
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("without base", func(t *testing.T) {
		for _, offset := range []int64{
			0, 1, -1, math.MaxInt32, math.MinInt32,
		} {
			offset := offset
			t.Run(fmt.Sprintf("%d", offset), func(t *testing.T) {
				goasm, err := newGolangAsmAssembler()
				require.NoError(t, err)
				goasm.CompileRegisterToMemory(CMPL, REG_AX, asm.NilRegister, offset)
				require.NoError(t, err)

				expectedBytes, err := goasm.Assemble()
				require.NoError(t, err)

				if expectedBytes[0]&rexPrefixDefault == rexPrefixDefault {
					expectedBytes = append(expectedBytes[0:1], expectedBytes[2:]...)
				} else {
					expectedBytes = expectedBytes[1:]
				}

				n := &nodeImpl{types: operandTypesMemoryToRegister,
					srcReg: asm.NilRegister, srcConst: offset}

				rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
				require.NoError(t, err)

				a := newAssemblerImpl()
				if rexPrefix != rexPrefixNone {
					a.buf.WriteByte(rexPrefix)
				}

				a.buf.WriteByte(modRM)
				if sbi != nil {
					a.buf.WriteByte(*sbi)
				}
				if displacementWidth != 0 {
					a.writeConst(n.srcConst, displacementWidth)
				}
				actual := a.buf.Bytes()
				require.Equal(t, expectedBytes, actual)
			})
		}
	})
	t.Run("with base", func(t *testing.T) {
		for _, srcReg := range intRegisters {
			srcReg := srcReg
			for _, offset := range []int64{
				0, 1, -1, math.MaxInt32, math.MinInt32,
			} {
				offset := offset
				n := &nodeImpl{types: operandTypesMemoryToRegister,
					srcReg: srcReg, srcConst: offset}

				for _, indexReg := range append(intRegisters,
					// Without index.
					asm.NilRegister) {
					if indexReg == REG_SP {
						continue
					}
					n.srcMemIndex = indexReg
					for _, scale := range []byte{1, 2, 4, 8} {
						n.srcMemScale = scale
						t.Run(n.String(), func(t *testing.T) {
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)
							goasm.CompileMemoryWithIndexToRegister(ADDL, srcReg, offset, indexReg, int16(scale), REG_AX)
							expectedBytes, err := goasm.Assemble()
							require.NoError(t, err)

							if expectedBytes[0]&rexPrefixDefault == rexPrefixDefault {
								expectedBytes = append(expectedBytes[0:1], expectedBytes[2:]...)
							} else {
								expectedBytes = expectedBytes[1:]
							}

							rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
							require.NoError(t, err)

							a := newAssemblerImpl()
							if rexPrefix != rexPrefixNone {
								a.buf.WriteByte(rexPrefix)
							}

							a.buf.WriteByte(modRM)
							if sbi != nil {
								a.buf.WriteByte(*sbi)
							}
							if displacementWidth != 0 {
								a.writeConst(n.srcConst, displacementWidth)
							}
							actual := a.buf.Bytes()
							require.Equal(t, expectedBytes, actual)
						})
					}
				}
			}
		}
	})
}

func TestNodeImpl_getRegisterToRegisterModRM(t *testing.T) {
	t.Run("int to int", func(t *testing.T) {
		for _, srcOnModRMReg := range []bool{true, false} {
			srcOnModRMReg := srcOnModRMReg
			t.Run(fmt.Sprintf("srcOnModRMReg=%v", srcOnModRMReg), func(t *testing.T) {
				for _, srcReg := range intRegisters {
					srcReg := srcReg
					for _, dstReg := range intRegisters {
						dstReg := dstReg
						t.Run(fmt.Sprintf("src=%s,dst=%s", registerName(srcReg), registerName(dstReg)), func(t *testing.T) {
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)

							// Temporarily pass MOVL to golang-asm to produce the expected bytes.
							if srcOnModRMReg {
								goasm.CompileRegisterToRegister(MOVL, srcReg, dstReg)
							} else {
								goasm.CompileRegisterToRegister(MOVL, dstReg, srcReg)
							}

							expectedBytes, err := goasm.Assemble()
							require.NoError(t, err)

							n := nodeImpl{srcReg: srcReg, dstReg: dstReg}
							rexPrefix, modRM, err := n.getRegisterToRegisterModRM(srcOnModRMReg)
							require.NoError(t, err)

							// Skip the opcode for MOVL to make this test opcode-independent.
							if rexPrefix != rexPrefixNone {
								require.Len(t, expectedBytes, 3)
								require.Equal(t, expectedBytes[0], rexPrefix)
								require.Equal(t, expectedBytes[2], modRM)
							} else {
								require.Len(t, expectedBytes, 2)
								require.Equal(t, expectedBytes[1], modRM)
							}
						})
					}
				}
			})
		}
	})
	t.Run("float to float", func(t *testing.T) {
		for _, srcReg := range floatRegisters {
			srcReg := srcReg
			for _, dstReg := range floatRegisters {
				dstReg := dstReg
				t.Run(fmt.Sprintf("src=%s,dst=%s", registerName(srcReg), registerName(dstReg)), func(t *testing.T) {
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					// Temporarily pass SUBSS to golang-asm to produce the expected bytes.
					goasm.CompileRegisterToRegister(SUBSS, srcReg, dstReg)
					expectedBytes, err := goasm.Assemble()
					require.NoError(t, err)

					n := nodeImpl{srcReg: srcReg, dstReg: dstReg}
					rexPrefix, modRM, err := n.getRegisterToRegisterModRM(false)
					require.NoError(t, err)

					if rexPrefix != rexPrefixNone {
						require.Len(t, expectedBytes, 5)
						require.Equal(t, expectedBytes[1], rexPrefix)
						require.Equal(t, expectedBytes[4], modRM)
					} else {
						require.Len(t, expectedBytes, 4)
						require.Equal(t, expectedBytes[3], modRM)
					}
				})
			}
		}
	})
	t.Run("float to int", func(t *testing.T) {
		for _, srcReg := range floatRegisters {
			srcReg := srcReg
			for _, dstReg := range intRegisters {
				dstReg := dstReg
				t.Run(fmt.Sprintf("src=%s,dst=%s", registerName(srcReg), registerName(dstReg)), func(t *testing.T) {
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					// Temporarily pass CVTTSS2SL to golang-asm to produce the expected bytes.
					goasm.CompileRegisterToRegister(CVTTSS2SL, srcReg, dstReg)
					expectedBytes, err := goasm.Assemble()
					require.NoError(t, err)

					n := nodeImpl{srcReg: srcReg, dstReg: dstReg}
					rexPrefix, modRM, err := n.getRegisterToRegisterModRM(false)
					require.NoError(t, err)

					if rexPrefix != rexPrefixNone {
						require.Len(t, expectedBytes, 5)
						require.Equal(t, expectedBytes[1], rexPrefix)
						require.Equal(t, expectedBytes[4], modRM)
					} else {
						require.Len(t, expectedBytes, 4)
						require.Equal(t, expectedBytes[3], modRM)
					}
				})
			}
		}
	})
	t.Run("int to float", func(t *testing.T) {
		for _, srcReg := range intRegisters {
			srcReg := srcReg
			for _, dstReg := range floatRegisters {
				dstReg := dstReg
				t.Run(fmt.Sprintf("src=%s,dst=%s", registerName(srcReg), registerName(dstReg)), func(t *testing.T) {
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					// Temporarily pass CVTSL2SS to golang-asm to produce the expected bytes.
					goasm.CompileRegisterToRegister(CVTSL2SS, srcReg, dstReg)
					expectedBytes, err := goasm.Assemble()
					require.NoError(t, err)

					n := nodeImpl{srcReg: srcReg, dstReg: dstReg}
					rexPrefix, modRM, err := n.getRegisterToRegisterModRM(false)
					require.NoError(t, err)

					if rexPrefix != rexPrefixNone {
						require.Len(t, expectedBytes, 5)
						require.Equal(t, expectedBytes[1], rexPrefix)
						require.Equal(t, expectedBytes[4], modRM)
					} else {
						require.Len(t, expectedBytes, 4)
						require.Equal(t, expectedBytes[3], modRM)
					}
				})
			}
		}
	})
}
