package amd64_debug

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAssemblerImpl_Assemble(t *testing.T) {
	t.Run("callback", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			a := amd64.NewAssemblerImpl()
			callbacked := false
			a.AddOnGenerateCallBack(func(b []byte) error { callbacked = true; return nil })
			_, err := a.Assemble()
			require.NoError(t, err)
			require.True(t, callbacked)
		})
		t.Run("error", func(t *testing.T) {
			a := amd64.NewAssemblerImpl()
			a.AddOnGenerateCallBack(func(b []byte) error { return errors.New("some error") })
			_, err := a.Assemble()
			require.EqualError(t, err, "some error")
		})
	})
	t.Run("no reassemble", func(t *testing.T) {
		// TODO: remove golang-asm dependency in tests.
		goasm, err := newGolangAsmAssembler()
		require.NoError(t, err)

		a := amd64.NewAssemblerImpl()
		for _, assembler := range []amd64.Assembler{a, goasm} {
			jmp := assembler.CompileJump(amd64.JCC)
			const dummyInstruction = amd64.CDQ
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

		a := amd64.NewAssemblerImpl()
		for _, assembler := range []amd64.Assembler{a, goasm} {
			jmp := assembler.CompileJump(amd64.JCC)
			const dummyInstruction = amd64.CDQ
			// Ensure that at least 128 bytes between amd64.JCC and the target which results in
			// reassemble as we have to convert the forward jump as long variant.
			for i := 0; i < 128; i++ {
				assembler.CompileStandAlone(dummyInstruction)
			}
			jmp.AssignJumpTarget(assembler.CompileStandAlone(dummyInstruction))
		}

		a.InitializeNodesForEncoding()

		// For the first encoding, we must be forced to reassemble.
		err = a.Encode()
		require.NoError(t, err)
		require.True(t, a.ForceReAssemble)
	})
}

func TestAssemblerImpl_Assemble_NOPPadding(t *testing.T) {
	t.Run("non relative jumps", func(t *testing.T) {
		tests := []struct {
			name    string
			setupFn func(assembler amd64.Assembler)
		}{
			{
				name: "RET",
				setupFn: func(assembler amd64.Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileStandAlone(amd64.RET)
					}
				},
			},
			{
				name: "JMP to register",
				setupFn: func(assembler amd64.Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileJumpToRegister(amd64.JMP, amd64.RegAX)
					}
				},
			},
			{
				name: "JMP to memory",
				setupFn: func(assembler amd64.Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileJumpToMemory(amd64.JMP, amd64.RegAX, 10)
					}
				},
			},
			{
				name: "JMP to memory large offset",
				setupFn: func(assembler amd64.Assembler) {
					for i := 0; i < 128; i++ {
						assembler.CompileJumpToMemory(amd64.JMP, amd64.RegAX, math.MaxInt32)
					}
				},
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.name, func(t *testing.T) {
				// TODO: remove golang-asm dependency in tests.
				goasm, err := newGolangAsmAssembler()
				require.NoError(t, err)

				a := amd64.NewAssemblerImpl()
				for _, assembler := range []amd64.Assembler{a, goasm} {
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
				for _, instruction := range []asm.Instruction{amd64.JMP, amd64.JCC, amd64.JCS, amd64.JEQ, amd64.JGE, amd64.JGT, amd64.JHI, amd64.JLE, amd64.JLS, amd64.JLT, amd64.JMI, amd64.JNE, amd64.JPC, amd64.JPS} {
					instruction := instruction
					t.Run(amd64.InstructionName(instruction), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()

						for _, assembler := range []amd64.Assembler{a, goasm} {
							head := assembler.CompileStandAlone(amd64.RET)
							var jmps []asm.Node
							for i := 0; i < 128; i++ { // Large enough so that this includes long jump.
								jmps = append(jmps, assembler.CompileJump(instruction))
							}
							tail := assembler.CompileStandAlone(amd64.RET)

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
	tests := []struct {
		name    string
		setupFn func(assembler amd64.Assembler)
	}{
		{
			name: "CMPL(register to const)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToConst(amd64.CMPL, amd64.RegAX, math.MaxInt16)
			},
		},
		{
			name: "CMPL(memory to const)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileMemoryToConst(amd64.CMPL, amd64.RegAX, 1, 10)
			},
		},
		{
			name: "CMPL(register to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.CMPL, amd64.RegR14, amd64.RegR10)
			},
		},
		{
			name: "CMPQ(register to const)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToConst(amd64.CMPQ, amd64.RegAX, math.MaxInt16)
			},
		},
		{
			name: "CMPQ(register to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.CMPQ, amd64.RegR14, amd64.RegR10)
			},
		},
		{
			name: "TESTL",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.TESTL, amd64.RegAX, amd64.RegAX)
			},
		},
		{
			name: "TESTQ",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.TESTQ, amd64.RegAX, amd64.RegAX)
			},
		},
		{
			name: "ADDL (register to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.ADDL, amd64.RegR10, amd64.RegAX)
			},
		},
		{
			name: "ADDL(memory to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileMemoryToRegister(amd64.ADDL, amd64.RegR10, 1234, amd64.RegAX)
			},
		},
		{
			name: "ADDQ (register to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.ADDQ, amd64.RegR10, amd64.RegAX)
			},
		},
		{
			name: "ADDQ(memory to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileMemoryToRegister(amd64.ADDQ, amd64.RegR10, 1234, amd64.RegAX)
			},
		},
		{
			name: "ADDQ(const to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileConstToRegister(amd64.ADDQ, 1234, amd64.RegR10)
			},
		},
		{
			name: "SUBL",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.SUBL, amd64.RegR10, amd64.RegAX)
			},
		},
		{
			name: "SUBQ (register to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.SUBQ, amd64.RegR10, amd64.RegAX)
			},
		},
		{
			name: "SUBQ (memory to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileMemoryToRegister(amd64.SUBQ, amd64.RegR10, math.MaxInt16, amd64.RegAX)
			},
		},
		{
			name: "ANDL",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.ANDL, amd64.RegR10, amd64.RegAX)
			},
		},
		{
			name: "ANDQ (register to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileRegisterToRegister(amd64.ANDQ, amd64.RegR10, amd64.RegAX)
			},
		},
		{
			name: "ANDQ (const to register)",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileConstToRegister(amd64.ANDQ, -123, amd64.RegR10)
			},
		},
		{
			name: "INCQ",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileNoneToMemory(amd64.INCQ, amd64.RegR10, 123)
			},
		},
		{
			name: "DECQ",
			setupFn: func(assembler amd64.Assembler) {
				assembler.CompileNoneToMemory(amd64.DECQ, amd64.RegR10, 0)
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			for _, jmpInst := range []asm.Instruction{amd64.JCC, amd64.JCS, amd64.JEQ, amd64.JGE, amd64.JGT, amd64.JHI, amd64.JLE, amd64.JLS, amd64.JLT, amd64.JMI, amd64.JNE, amd64.JPC, amd64.JPS} {
				t.Run(amd64.InstructionName(jmpInst), func(t *testing.T) {

					// TODO: remove golang-asm dependency in tests.
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					a := amd64.NewAssemblerImpl()

					for _, assembler := range []amd64.Assembler{a, goasm} {
						var jmps []asm.Node
						for i := 0; i < 12; i++ { // Large enough so that this includes long jump.
							tc.setupFn(assembler)
							jmps = append(jmps, assembler.CompileJump(jmpInst))
						}

						target := assembler.CompileStandAlone(amd64.RET)
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

var intRegisters = []asm.Register{
	amd64.RegAX, amd64.RegCX, amd64.RegDX, amd64.RegBX, amd64.RegSP, amd64.RegBP, amd64.RegSI, amd64.RegDI,
	amd64.RegR8, amd64.RegR9, amd64.RegR10, amd64.RegR11, amd64.RegR12, amd64.RegR13, amd64.RegR14, amd64.RegR15,
}

var floatRegisters = []asm.Register{
	amd64.RegX0, amd64.RegX1, amd64.RegX2, amd64.RegX3, amd64.RegX4, amd64.RegX5, amd64.RegX6, amd64.RegX7,
	amd64.RegX8, amd64.RegX9, amd64.RegX10, amd64.RegX11, amd64.RegX12, amd64.RegX13, amd64.RegX14, amd64.RegX15,
}

func TestAssemblerImpl_EncodeNoneToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		a := amd64.NewAssemblerImpl()
		err := a.EncodeNoneToRegister(&amd64.NodeImpl{Instruction: amd64.ADDL,
			Types: amd64.OperandTypesNoneToRegister, DstReg: amd64.RegAX})
		require.Error(t, err)

		t.Run("error", func(t *testing.T) {
			tests := []struct {
				n      *amd64.NodeImpl
				expErr string
			}{
				{
					n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesNoneToRegister, DstReg: amd64.RegAX},
					expErr: "ADDL is unsupported for from:none,to:register type",
				},
				{
					n:      &amd64.NodeImpl{Instruction: amd64.JMP, Types: amd64.OperandTypesNoneToRegister},
					expErr: "invalid register [nil]",
				},
			}

			for _, tt := range tests {
				tc := tt
				t.Run(tc.expErr, func(t *testing.T) {
					tc := tc
					a := amd64.NewAssemblerImpl()
					err := a.EncodeNoneToRegister(tc.n)
					require.EqualError(t, err, tc.expErr)
				})
			}
		})
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{
			amd64.JMP, amd64.SETCC, amd64.SETCS, amd64.SETEQ, amd64.SETGE, amd64.SETGT, amd64.SETHI, amd64.SETLE,
			amd64.SETLS, amd64.SETLT, amd64.SETNE, amd64.SETPC, amd64.SETPS, amd64.NEGQ, amd64.INCQ, amd64.DECQ,
		} {
			inst := inst
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileNoneToRegister(inst, reg)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()
						err = a.EncodeNoneToRegister(&amd64.NodeImpl{Instruction: inst,
							Types: amd64.OperandTypesNoneToRegister, DstReg: reg})
						require.NoError(t, err)
						require.Equal(t, bs, a.Buf.Bytes())
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_EncodeNoneToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesNoneToMemory, DstReg: amd64.RegAX},
				expErr: "ADDL is unsupported for from:none,to:memory type",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeNoneToMemory(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{amd64.DECQ, amd64.INCQ, amd64.JMP} {
			inst := inst
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
						for _, offset := range []int64{0, 1, -1, 1243, math.MaxInt32, math.MinInt16} {
							offset := offset
							t.Run(fmt.Sprintf("0x%x", uint32(offset)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileNoneToMemory(inst, reg, int64(offset))
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								err = a.EncodeNoneToMemory(&amd64.NodeImpl{Instruction: inst,
									Types: amd64.OperandTypesNoneToMemory, DstReg: reg, DstConst: int64(offset)})
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_ResolveForwardRelativeJumps(t *testing.T) {
	t.Run("long jump", func(t *testing.T) {
		t.Run("error", func(t *testing.T) {
			originOffset, targetOffset := uint64(0), uint64(math.MaxInt64)
			origin := &amd64.NodeImpl{Instruction: amd64.JMP, OffsetInBinaryField: originOffset}
			target := &amd64.NodeImpl{OffsetInBinaryField: targetOffset, JumpOrigins: map[*amd64.NodeImpl]struct{}{origin: {}}}
			a := amd64.NewAssemblerImpl()
			err := a.ResolveForwardRelativeJumps(target)
			require.EqualError(t, err, "too large jump offset 9223372036854775802 for encoding JMP")
		})
		t.Run("ok", func(t *testing.T) {
			originOffset := uint64(0)
			tests := []struct {
				instruction                asm.Instruction
				targetOffset               uint64
				expectedOffsetFromEIP      int32
				writtenOffsetIndexInBinary int
			}{
				{
					instruction: amd64.JMP, targetOffset: 1234,
					writtenOffsetIndexInBinary: 1,        // amd64.JMP has one opcode byte for long jump.
					expectedOffsetFromEIP:      1234 - 5, // the instruction length of long relative jmp.
				},
				{
					instruction: amd64.JCC, targetOffset: 1234,
					writtenOffsetIndexInBinary: 2,        // Conditional jumps has two opcode for long jump.
					expectedOffsetFromEIP:      1234 - 6, // the instruction length of long relative amd64.JCC
				},
			}

			for _, tt := range tests {
				tc := tt
				origin := &amd64.NodeImpl{Instruction: tc.instruction, OffsetInBinaryField: originOffset}
				target := &amd64.NodeImpl{OffsetInBinaryField: tc.targetOffset, JumpOrigins: map[*amd64.NodeImpl]struct{}{origin: {}}}
				a := amd64.NewAssemblerImpl()

				// Grow the capacity of buffer so that we could put the offset.
				a.Buf.Write([]byte{0, 0, 0, 0, 0, 0}) // Relative long jumps are at most 6 bytes.

				err := a.ResolveForwardRelativeJumps(target)
				require.NoError(t, err)

				actual := binary.LittleEndian.Uint32(a.Buf.Bytes()[tc.writtenOffsetIndexInBinary:])
				require.Equal(t, tc.expectedOffsetFromEIP, int32(actual))
			}
		})
	})
	t.Run("short jump", func(t *testing.T) {
		t.Run("reassemble", func(t *testing.T) {
			originOffset := uint64(0)
			tests := []struct {
				instruction  asm.Instruction
				targetOffset uint64
			}{
				{
					instruction:  amd64.JMP,
					targetOffset: 10000,
				},
				{
					instruction: amd64.JMP,
					// Relative jump offset = 130 - len(amd64.JMP instruction bytes) = 130 - 2 = 128 > math.MaxInt8.
					targetOffset: 130,
				},
				{
					instruction:  amd64.JCC,
					targetOffset: 10000,
				},
				{
					instruction: amd64.JCC,
					// Relative jump offset = 130 - len(amd64.JCC instruction bytes) = 130 -2 = 128 > math.MaxInt8.
					targetOffset: 130,
				},
			}

			for _, tt := range tests {
				tc := tt
				origin := &amd64.NodeImpl{Instruction: tc.instruction, OffsetInBinaryField: originOffset, Flag: amd64.NodeFlagShortForwardJump}
				target := &amd64.NodeImpl{OffsetInBinaryField: tc.targetOffset, JumpOrigins: map[*amd64.NodeImpl]struct{}{origin: {}}}
				origin.JumpTarget = target

				a := amd64.NewAssemblerImpl()
				err := a.ResolveForwardRelativeJumps(target)
				require.NoError(t, err)

				require.True(t, a.ForceReAssemble)
				require.True(t, origin.Flag&amd64.NodeFlagShortForwardJump == 0)
			}
		})
		t.Run("ok", func(t *testing.T) {
			originOffset := uint64(0)
			tests := []struct {
				instruction           asm.Instruction
				targetOffset          uint64
				expectedOffsetFromEIP byte
			}{
				{
					instruction: amd64.JMP, targetOffset: 129,
					expectedOffsetFromEIP: 129 - 2, // short jumps are of 2 bytes.
				},
				{
					instruction: amd64.JCC, targetOffset: 129,
					expectedOffsetFromEIP: 129 - 2, // short jumps are of 2 bytes.
				},
			}

			for _, tt := range tests {
				tc := tt
				origin := &amd64.NodeImpl{Instruction: tc.instruction, OffsetInBinaryField: originOffset, Flag: amd64.NodeFlagShortForwardJump}
				target := &amd64.NodeImpl{OffsetInBinaryField: tc.targetOffset, JumpOrigins: map[*amd64.NodeImpl]struct{}{origin: {}}}
				origin.JumpTarget = target

				a := amd64.NewAssemblerImpl()

				// Grow the capacity of buffer so that we could put the offset.
				a.Buf.Write([]byte{0, 0}) // Relative short jumps are of 2 bytes.

				err := a.ResolveForwardRelativeJumps(target)
				require.NoError(t, err)

				actual := a.Buf.Bytes()[1] // For short jumps, the opcode has one opcode so the offset is writte at 2nd byte.
				require.Equal(t, tc.expectedOffsetFromEIP, actual)
			}
		})
	})
}

func TestAssemblerImpl_encodeNoneToBranch(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Types: amd64.OperandTypesNoneToBranch, Instruction: amd64.JMP},
				expErr: "jump target must not be nil for relative JMP",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeRelativeJump(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("backward jump", func(t *testing.T) {
		t.Run("too large offset", func(t *testing.T) {
			a := amd64.NewAssemblerImpl()
			targetOffsetInBinaryField := uint64(0)
			OffsetInBinaryField := uint64(math.MaxInt32)
			node := &amd64.NodeImpl{Instruction: amd64.JMP,
				JumpTarget: &amd64.NodeImpl{
					OffsetInBinaryField: targetOffsetInBinaryField, JumpOrigins: map[*amd64.NodeImpl]struct{}{},
				},
				Flag:                amd64.NodeFlagBackwardJump,
				OffsetInBinaryField: OffsetInBinaryField}
			err := a.EncodeRelativeJump(node)
			require.Error(t, err)
		})

		for _, isShortJump := range []bool{true, false} {
			isShortJump := isShortJump
			t.Run(fmt.Sprintf("is_short_jump=%v", isShortJump), func(t *testing.T) {
				for _, inst := range []asm.Instruction{
					amd64.JCC, amd64.JCS, amd64.JEQ, amd64.JGE, amd64.JGT, amd64.JHI, amd64.JLE, amd64.JLS, amd64.JLT, amd64.JMI,
					amd64.JMP, amd64.JNE, amd64.JPC, amd64.JPS, amd64.JPL,
				} {
					inst := inst
					t.Run(amd64.InstructionName(inst), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()
						for _, assembler := range []amd64.Assembler{goasm, a} {
							const dummyInstruction = amd64.CDQ
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
					amd64.JCC, amd64.JCS, amd64.JEQ, amd64.JGE, amd64.JGT, amd64.JHI, amd64.JLE, amd64.JLS, amd64.JLT,
					amd64.JMI, amd64.JMP, amd64.JNE, amd64.JPC, amd64.JPS, amd64.JPL,
				} {
					inst := inst
					t.Run(amd64.InstructionName(inst), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()
						for _, assembler := range []amd64.Assembler{goasm, a} {
							const dummyInstruction = amd64.CDQ
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

func TestAssemblerImpl_EncodeRegisterToNone(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesRegisterToNone, SrcReg: amd64.RegAX},
				expErr: "ADDL is unsupported for from:register,to:none type",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.DIVQ, Types: amd64.OperandTypesRegisterToNone},
				expErr: "invalid register [nil]",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeRegisterToNone(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{amd64.DIVL, amd64.DIVQ, amd64.IDIVL, amd64.IDIVQ, amd64.MULL, amd64.MULQ} {

			inst := inst
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
						// TODO: remove golang-asm dependency in tests.
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileRegisterToNone(inst, reg)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()
						err = a.EncodeRegisterToNone(&amd64.NodeImpl{Instruction: inst,
							Types: amd64.OperandTypesRegisterToNone, SrcReg: reg})
						require.NoError(t, err)
						require.Equal(t, bs, a.Buf.Bytes())
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_EncodeRegisterToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.JMP, Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegAX},
				expErr: "JMP is unsupported for from:register,to:register type",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesRegisterToRegister, DstReg: amd64.RegAX},
				expErr: "invalid register [nil]",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX},
				expErr: "invalid register [nil]",
			}, {
				n:      &amd64.NodeImpl{Instruction: amd64.MOVL, Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegX1},
				expErr: "MOVL for float to float is undefined",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeRegisterToRegister(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	intRegisters := []asm.Register{amd64.RegAX, amd64.RegR8}
	floatRegisters := []asm.Register{amd64.RegX0, amd64.RegX8}
	allRegisters := append(intRegisters, floatRegisters...)
	tests := []struct {
		instruction      asm.Instruction
		srcRegs, DstRegs []asm.Register
		arg              byte
	}{
		{instruction: amd64.PADDB, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PADDW, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PADDL, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PADDQ, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ADDPS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ADDPD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PSUBB, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PSUBW, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PSUBL, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PSUBQ, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.SUBPS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.SUBPD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PINSRQ, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 1},
		{instruction: amd64.PINSRQ, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 0},
		{instruction: amd64.PINSRD, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 1},
		{instruction: amd64.PINSRD, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 0},
		{instruction: amd64.PINSRW, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 1},
		{instruction: amd64.PINSRW, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 0},
		{instruction: amd64.PINSRB, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 1},
		{instruction: amd64.PINSRB, srcRegs: intRegisters, DstRegs: floatRegisters, arg: 0},
		{instruction: amd64.ADDL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.ADDQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.ADDSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ADDSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ANDL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.ANDPD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ANDPS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ANDQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.BSRL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.BSRQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.CMOVQCS, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.CMPL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.CMPQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.COMISD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.COMISS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTSD2SS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTSL2SD, srcRegs: intRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTSL2SS, srcRegs: intRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTSQ2SD, srcRegs: intRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTSQ2SS, srcRegs: intRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTSS2SD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.CVTTSD2SL, srcRegs: floatRegisters, DstRegs: intRegisters},
		{instruction: amd64.CVTTSD2SQ, srcRegs: floatRegisters, DstRegs: intRegisters},
		{instruction: amd64.CVTTSS2SL, srcRegs: floatRegisters, DstRegs: intRegisters},
		{instruction: amd64.CVTTSS2SQ, srcRegs: floatRegisters, DstRegs: intRegisters},
		{instruction: amd64.DIVSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.DIVSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.LZCNTL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.LZCNTQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MAXSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MINSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MAXSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MINSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MOVBLSX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVWLZX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVBLZX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVBQSX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVLQSX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVL, srcRegs: intRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MOVL, srcRegs: floatRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVQ, srcRegs: allRegisters, DstRegs: allRegisters},
		{instruction: amd64.MOVWLSX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MOVWQSX, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.MULSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MULSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ORL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.ORPD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ORPS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.ORQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.POPCNTL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.POPCNTQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.ROLL, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.ROLQ, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.RORL, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.RORQ, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.ROUNDSD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x00},
		{instruction: amd64.ROUNDSS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x00},
		{instruction: amd64.ROUNDSD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x01},
		{instruction: amd64.ROUNDSS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x01},
		{instruction: amd64.ROUNDSD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x02},
		{instruction: amd64.ROUNDSS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x02},
		{instruction: amd64.ROUNDSD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x03},
		{instruction: amd64.ROUNDSS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x03},
		{instruction: amd64.ROUNDSD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x04},
		{instruction: amd64.ROUNDSS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0x04},
		{instruction: amd64.SARL, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.SARQ, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.SHLL, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.SHLQ, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.SHRL, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.SHRQ, srcRegs: []asm.Register{amd64.RegCX}, DstRegs: intRegisters},
		{instruction: amd64.SQRTSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.SQRTSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.SUBL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.SUBQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.SUBSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.SUBSS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.TESTL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.TESTQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.TZCNTL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.TZCNTQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.UCOMISD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.UCOMISS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.XORL, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.XORPD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.XORPS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.XORQ, srcRegs: intRegisters, DstRegs: intRegisters},
		{instruction: amd64.PXOR, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PSHUFB, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PSHUFD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0},
		{instruction: amd64.PSHUFD, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 1},
		{instruction: amd64.PEXTRB, srcRegs: floatRegisters, DstRegs: intRegisters, arg: 0},
		{instruction: amd64.PEXTRW, srcRegs: floatRegisters, DstRegs: intRegisters, arg: 1},
		{instruction: amd64.PEXTRD, srcRegs: floatRegisters, DstRegs: intRegisters, arg: 1},
		{instruction: amd64.PEXTRQ, srcRegs: floatRegisters, DstRegs: intRegisters, arg: 1},
		{instruction: amd64.MOVLHPS, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.INSERTPS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 0},
		{instruction: amd64.INSERTPS, srcRegs: floatRegisters, DstRegs: floatRegisters, arg: 1},
		{instruction: amd64.PTEST, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PCMPEQB, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PCMPEQW, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PCMPEQD, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PCMPEQQ, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.PADDUSB, srcRegs: floatRegisters, DstRegs: floatRegisters},
		{instruction: amd64.MOVSD, srcRegs: floatRegisters, DstRegs: floatRegisters},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(amd64.InstructionName(tc.instruction), func(t *testing.T) {
			t.Run("error", func(t *testing.T) {
				srcFloat, dstFloat := amd64.IsVectorRegister(tc.srcRegs[0]), amd64.IsVectorRegister(tc.DstRegs[0])
				isMOV := tc.instruction == amd64.MOVL || tc.instruction == amd64.MOVQ
				a := amd64.NewAssemblerImpl()
				if _, isShiftOp := amd64.RegisterToRegisterShiftOpcode[tc.instruction]; isShiftOp {
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegCX, DstReg: amd64.RegX0}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegX0}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegCX}))
				} else if srcFloat && dstFloat && !isMOV { // Float to Float
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegAX}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegAX}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegX0}))
				} else if srcFloat && !dstFloat && !isMOV { // Float to Int
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegX1}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegX1}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegAX}))
				} else if !srcFloat && dstFloat && !isMOV { // Int to Float
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegAX}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegAX}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegX0}))
				} else if !isMOV { // Int to Int
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegX0}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegX0, DstReg: amd64.RegAX}))
					require.Error(t, a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
						Types: amd64.OperandTypesRegisterToRegister, SrcReg: amd64.RegAX, DstReg: amd64.RegX0}))
				}
			})
			for _, srcReg := range tc.srcRegs {
				srcReg := srcReg
				t.Run(fmt.Sprintf("src=%s", amd64.RegisterName(srcReg)), func(t *testing.T) {
					srcReg := srcReg
					for _, DstReg := range tc.DstRegs {
						DstReg := DstReg
						t.Run(fmt.Sprintf("dst=%s", amd64.RegisterName(DstReg)), func(t *testing.T) {
							// TODO: remove golang-asm dependency in tests.
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)

							switch tc.instruction {
							case amd64.ROUNDSD, amd64.ROUNDSS, amd64.PINSRQ, amd64.PINSRD, amd64.PINSRW,
								amd64.PINSRB, amd64.PSHUFD, amd64.PEXTRB, amd64.PEXTRW, amd64.PEXTRD, amd64.PEXTRQ,
								amd64.INSERTPS:
								goasm.CompileRegisterToRegisterWithArg(tc.instruction, srcReg, DstReg, tc.arg)
							default:
								goasm.CompileRegisterToRegister(tc.instruction, srcReg, DstReg)
							}

							bs, err := goasm.Assemble()
							require.NoError(t, err)

							a := amd64.NewAssemblerImpl()
							err = a.EncodeRegisterToRegister(&amd64.NodeImpl{Instruction: tc.instruction,
								Types: amd64.OperandTypesRegisterToRegister, SrcReg: srcReg, DstReg: DstReg,
								Arg: tc.arg,
							})
							require.NoError(t, err)
							require.Equal(t, bs, a.Buf.Bytes())
						})
					}
				})
			}
		})
	}
}

func TestAssemblerImpl_CompileMemoryWithIndexAndArgToRegister(t *testing.T) {
	tests := []struct {
		name        string
		instruction asm.Instruction
	}{
		{name: "PINSRB", instruction: amd64.PINSRB},
		{name: "PINSRW", instruction: amd64.PINSRW},
		{name: "PINSRD", instruction: amd64.PINSRD},
		{name: "PINSRQ", instruction: amd64.PINSRQ},
	}

	dstRegs := []asm.Register{amd64.RegX0, amd64.RegX13}
	indexRegs := []asm.Register{amd64.RegR12, amd64.RegAX, amd64.RegBP, amd64.RegSI}
	offsets := []int64{0, 1, math.MaxInt32}
	args := []byte{0, 1}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {

			for _, dstReg := range dstRegs {
				for _, indexReg := range indexRegs {
					for _, offset := range offsets {
						for _, arg := range args {
							dstReg, indexReg, offset, arg := dstReg, indexReg, offset, arg
							t.Run(fmt.Sprintf("dst=%s,index=%s,offset=%d,arg=%d",
								amd64.RegisterName(dstReg), amd64.RegisterName(indexReg), offset, arg), func(t *testing.T) {

								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								for _, assembler := range []amd64.Assembler{goasm, a} {
									assembler.CompileMemoryWithIndexAndArgToRegister(tc.instruction,
										amd64.RegAX, offset, amd64.RegCX, 1, dstReg, arg)
								}

								actual, err := a.Assemble()
								require.NoError(t, err)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								require.Equal(t, expected, actual)
							})
						}

					}
				}
			}

		})
	}
}

func TestAssemblerImpl_CompileRegisterToMemoryWithIndexAndArg(t *testing.T) {
	tests := []struct {
		name        string
		instruction asm.Instruction
	}{
		{name: "PEXTRB", instruction: amd64.PEXTRB},
		{name: "PEXTRW", instruction: amd64.PEXTRW},
		{name: "PEXTRD", instruction: amd64.PEXTRD},
		{name: "PEXTRQ", instruction: amd64.PEXTRQ},
	}

	srcRegs := []asm.Register{amd64.RegX0, amd64.RegX13}
	indexRegs := []asm.Register{amd64.RegR12, amd64.RegAX, amd64.RegBP, amd64.RegSI}
	offsets := []int64{0, 1, math.MaxInt32}
	args := []byte{0, 1}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {

			for _, srcReg := range srcRegs {
				for _, indexReg := range indexRegs {
					for _, offset := range offsets {
						for _, arg := range args {
							srcReg, indexReg, offset, arg := srcReg, indexReg, offset, arg
							t.Run(fmt.Sprintf("src=%s,index=%s,offset=%d,arg=%d",
								amd64.RegisterName(srcReg), amd64.RegisterName(indexReg), offset, arg), func(t *testing.T) {

								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								for _, assembler := range []amd64.Assembler{goasm, a} {
									assembler.CompileRegisterToMemoryWithIndexAndArg(tc.instruction, srcReg,
										amd64.RegCX, offset, indexReg, 1, arg)
								}

								actual, err := a.Assemble()
								require.NoError(t, err)
								expected, err := goasm.Assemble()
								require.NoError(t, err)

								require.Equal(t, expected, actual)
							})
						}

					}
				}
			}

		})
	}
}

func TestAssemblerImpl_EncodeRegisterToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n: &amd64.NodeImpl{Instruction: amd64.JMP,
					Types:  amd64.OperandTypesRegisterToMemory,
					SrcReg: amd64.RegAX, DstReg: amd64.RegAX},
				expErr: "JMP is unsupported for from:register,to:memory type",
			},
			{
				n: &amd64.NodeImpl{Instruction: amd64.SHLQ,
					Types:  amd64.OperandTypesRegisterToMemory,
					SrcReg: amd64.RegAX, DstReg: amd64.RegAX},
				expErr: "shifting instruction SHLQ require CX register as src but got AX",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {

				a := amd64.NewAssemblerImpl()
				require.EqualError(t, a.EncodeRegisterToMemory(tc.n), tc.expErr)
			})
		}
	})

	t.Run("non shift", func(t *testing.T) {
		scales := []byte{1, 4}
		for _, instruction := range []asm.Instruction{
			amd64.CMPL, amd64.CMPQ, amd64.MOVB, amd64.MOVL, amd64.MOVQ, amd64.MOVW,
		} {
			regs := []asm.Register{amd64.RegR12, amd64.RegAX, amd64.RegBP, amd64.RegSI}
			srcRegs := regs
			if instruction == amd64.MOVL || instruction == amd64.MOVQ {
				srcRegs = append(srcRegs, amd64.RegX0, amd64.RegX10)
			}

			for _, srcReg := range srcRegs {
				srcReg := srcReg
				for _, DstReg := range regs {
					DstReg := DstReg
					for _, offset := range []int64{
						0, 1, math.MaxInt32,
					} {
						offset := offset
						n := &amd64.NodeImpl{Instruction: instruction,
							Types:  amd64.OperandTypesRegisterToMemory,
							SrcReg: srcReg, DstReg: DstReg, DstConst: offset}

						// Without index.
						t.Run(n.String(), func(t *testing.T) {
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)
							goasm.CompileRegisterToMemory(instruction, srcReg, DstReg, offset)
							bs, err := goasm.Assemble()
							require.NoError(t, err)

							a := amd64.NewAssemblerImpl()
							err = a.EncodeRegisterToMemory(n)
							require.NoError(t, err)
							require.Equal(t, bs, a.Buf.Bytes())
						})

						// With index.
						for _, indexReg := range regs {
							n.DstMemIndex = indexReg
							for _, scale := range scales {
								n.DstMemScale = scale
								t.Run(n.String(), func(t *testing.T) {
									goasm, err := newGolangAsmAssembler()
									require.NoError(t, err)
									goasm.CompileRegisterToMemoryWithIndex(instruction, srcReg, DstReg, offset, indexReg, int16(scale))
									bs, err := goasm.Assemble()
									require.NoError(t, err)

									a := amd64.NewAssemblerImpl()
									err = a.EncodeRegisterToMemory(n)
									require.NoError(t, err)
									require.Equal(t, bs, a.Buf.Bytes())
								})
							}
						}
					}
				}
			}
		}
	})
	t.Run("shift", func(t *testing.T) {
		regs := []asm.Register{amd64.RegAX, amd64.RegR8}
		scales := []byte{1, 4}
		for _, instruction := range []asm.Instruction{
			amd64.SARL, amd64.SARQ, amd64.SHLL, amd64.SHLQ, amd64.SHRL, amd64.SHRQ,
			amd64.ROLL, amd64.ROLQ, amd64.RORL, amd64.RORQ,
		} {
			for _, DstReg := range regs {
				DstReg := DstReg
				for _, offset := range []int64{
					0, 1, math.MaxInt32,
				} {
					offset := offset
					n := &amd64.NodeImpl{Instruction: instruction,
						Types:  amd64.OperandTypesRegisterToMemory,
						SrcReg: amd64.RegCX, DstReg: DstReg, DstConst: offset}

					// Without index.
					t.Run(n.String(), func(t *testing.T) {
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileRegisterToMemory(instruction, amd64.RegCX, DstReg, offset)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()
						err = a.EncodeRegisterToMemory(n)
						require.NoError(t, err)
						require.Equal(t, bs, a.Buf.Bytes())
					})

					// With index.
					for _, indexReg := range regs {
						n.DstMemIndex = indexReg
						for _, scale := range scales {
							n.DstMemScale = scale
							t.Run(n.String(), func(t *testing.T) {
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileRegisterToMemoryWithIndex(instruction, amd64.RegCX, DstReg, offset, indexReg, int16(scale))
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								err = a.EncodeRegisterToMemory(n)
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())
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
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesRegisterToConst, SrcReg: amd64.RegAX},
				expErr: "ADDL is unsupported for from:register,to:const type",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.DIVQ, Types: amd64.OperandTypesRegisterToConst},
				expErr: "invalid register [nil]",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeRegisterToNone(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{amd64.CMPL, amd64.CMPQ} {
			inst := inst
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
						for _, c := range []int64{0, 1, -1, 1243, -1234, math.MaxInt32, math.MinInt32, math.MaxInt16, math.MinInt16} {
							c := c
							t.Run(fmt.Sprintf("0x%x", uint64(c)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileRegisterToConst(inst, reg, c)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								err = a.EncodeRegisterToConst(&amd64.NodeImpl{Instruction: inst,
									Types: amd64.OperandTypesRegisterToConst, SrcReg: reg, DstConst: c})
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())
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
		const targetBeforeInstruction = amd64.RET
		for _, DstReg := range []asm.Register{amd64.RegAX, amd64.RegR8} {
			DstReg := DstReg
			t.Run(amd64.RegisterName(DstReg), func(t *testing.T) {
				goasm, err := newGolangAsmAssembler()
				require.NoError(t, err)

				a := amd64.NewAssemblerImpl()

				// Setup target.
				for _, assembler := range []amd64.Assembler{a, goasm} {
					assembler.CompileReadInstructionAddress(DstReg, targetBeforeInstruction)
					assembler.CompileStandAlone(amd64.CDQ) // Dummy.
					assembler.CompileStandAlone(targetBeforeInstruction)
					assembler.CompileStandAlone(amd64.CDQ) // Target.
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
		a := amd64.NewAssemblerImpl()
		a.CompileReadInstructionAddress(amd64.RegR10, amd64.NOP)
		a.CompileStandAlone(amd64.CDQ)
		_, err := a.Assemble()
		require.EqualError(t, err, "BUG: target instruction not found for read instruction address")
	})
	t.Run("offset too large", func(t *testing.T) {
		a := amd64.NewAssemblerImpl()
		a.CompileReadInstructionAddress(amd64.RegR10, amd64.RET)
		a.CompileStandAlone(amd64.RET)
		a.CompileStandAlone(amd64.CDQ)

		for n := a.Root; n != nil; n = n.Next {
			n.OffsetInBinaryField = uint64(a.Buf.Len())

			err := a.EncodeNode(n)
			require.NoError(t, err)
		}

		require.Equal(t, 1, len(a.OnGenerateCallbacks))
		cb := a.OnGenerateCallbacks[0]

		targetNode := a.Current
		targetNode.OffsetInBinaryField = (uint64(math.MaxInt64))

		err := cb(nil)
		require.EqualError(t, err, "BUG: too large offset for LEAQ instruction")
	})
}

func TestAssemblerImpl_EncodeMemoryToRegister(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		n := &amd64.NodeImpl{Instruction: amd64.JMP,
			Types:  amd64.OperandTypesMemoryToRegister,
			SrcReg: amd64.RegAX, DstReg: amd64.RegAX}
		a := amd64.NewAssemblerImpl()
		require.EqualError(t, a.EncodeMemoryToRegister(n), "JMP is unsupported for from:memory,to:register type")
	})
	intRegs := []asm.Register{amd64.RegAX, amd64.RegBP, amd64.RegDI, amd64.RegR10}
	floatRegs := []asm.Register{amd64.RegX0, amd64.RegX8}
	scales := []byte{1, 4}
	tests := []struct {
		instruction asm.Instruction
		isFloatInst bool
	}{
		{instruction: amd64.ADDL},
		{instruction: amd64.ADDQ},
		{instruction: amd64.CMPL},
		{instruction: amd64.CMPQ},
		{instruction: amd64.LEAQ},
		{instruction: amd64.MOVBLSX},
		{instruction: amd64.MOVBLZX},
		{instruction: amd64.MOVBQSX},
		{instruction: amd64.MOVBQZX},
		{instruction: amd64.MOVLQSX},
		{instruction: amd64.MOVLQZX},
		{instruction: amd64.MOVL},
		{instruction: amd64.MOVQ},
		{instruction: amd64.MOVWLSX},
		{instruction: amd64.MOVWLZX},
		{instruction: amd64.MOVWQSX},
		{instruction: amd64.MOVWQZX},
		{instruction: amd64.SUBQ},
		{instruction: amd64.SUBSD, isFloatInst: true},
		{instruction: amd64.SUBSS, isFloatInst: true},
		{instruction: amd64.UCOMISD, isFloatInst: true},
		{instruction: amd64.UCOMISS, isFloatInst: true},
		{instruction: amd64.PMOVSXBW, isFloatInst: true},
		{instruction: amd64.PMOVSXWD, isFloatInst: true},
		{instruction: amd64.PMOVSXDQ, isFloatInst: true},
		{instruction: amd64.PMOVZXBW, isFloatInst: true},
		{instruction: amd64.PMOVZXWD, isFloatInst: true},
		{instruction: amd64.PMOVZXDQ, isFloatInst: true},
	}

	for _, tt := range tests {
		tc := tt
		for _, srcReg := range intRegs {
			srcReg := srcReg
			DstRegs := intRegs
			if tc.instruction == amd64.MOVL || tc.instruction == amd64.MOVQ {
				DstRegs = append(DstRegs, floatRegs...)
			} else if tc.isFloatInst {
				DstRegs = floatRegs
			}

			for _, DstReg := range DstRegs {
				DstReg := DstReg
				for _, offset := range []int64{
					0, 1, math.MaxInt32,
				} {
					offset := offset
					n := &amd64.NodeImpl{Instruction: tc.instruction,
						Types:  amd64.OperandTypesMemoryToRegister,
						SrcReg: srcReg, SrcConst: offset, DstReg: DstReg}

					// Without index.
					t.Run(n.String(), func(t *testing.T) {
						goasm, err := newGolangAsmAssembler()
						require.NoError(t, err)
						goasm.CompileMemoryToRegister(tc.instruction, srcReg, offset, DstReg)
						bs, err := goasm.Assemble()
						require.NoError(t, err)

						a := amd64.NewAssemblerImpl()
						err = a.EncodeMemoryToRegister(n)
						require.NoError(t, err)
						require.Equal(t, bs, a.Buf.Bytes())
					})

					// With index.
					for _, indexReg := range intRegs {
						n.SrcMemIndex = indexReg
						for _, scale := range scales {
							n.SrcMemScale = scale
							t.Run(n.String(), func(t *testing.T) {
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileMemoryWithIndexToRegister(tc.instruction, srcReg, offset, indexReg, int16(scale), DstReg)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								err = a.EncodeMemoryToRegister(n)
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())
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
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.RET, Types: amd64.OperandTypesConstToRegister, DstReg: amd64.RegAX},
				expErr: "RET is unsupported for from:const,to:register type",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.PSLLL, Types: amd64.OperandTypesConstToRegister},
				expErr: "invalid register [nil]",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.PSLLL, Types: amd64.OperandTypesConstToRegister, DstReg: amd64.RegAX},
				expErr: "PSLLL needs float register but got AX",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDQ, Types: amd64.OperandTypesConstToRegister, DstReg: amd64.RegX0},
				expErr: "ADDQ needs int register but got X0",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.PSLLL, Types: amd64.OperandTypesConstToRegister, DstReg: amd64.RegX0, SrcConst: 2199023255552},
				expErr: "constant must fit in 32-bit integer for PSLLL, but got 2199023255552",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.SHLQ, Types: amd64.OperandTypesConstToRegister, DstReg: amd64.RegR10, SrcConst: 32768},
				expErr: "constant must fit in positive 8-bit integer for SHLQ, but got 32768",
			},
			{
				n:      &amd64.NodeImpl{Instruction: amd64.PSRLQ, Types: amd64.OperandTypesConstToRegister, DstReg: amd64.RegX0, SrcConst: 32768},
				expErr: "constant must fit in signed 8-bit integer for PSRLQ, but got 32768",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeConstToRegister(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})
	t.Run("int instructions", func(t *testing.T) {
		for _, inst := range []asm.Instruction{amd64.ADDQ, amd64.ANDQ, amd64.MOVL, amd64.MOVQ, amd64.SHLQ, amd64.SHRQ, amd64.XORL, amd64.XORQ} {
			inst := inst
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range intRegisters {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
						for _, c := range []int64{
							0, 1, -1, 11, -11, 1243, -1234, math.MaxUint8, math.MaxInt32, math.MinInt32, math.MaxInt16,
							math.MaxUint32,
							math.MinInt16, math.MaxInt64, math.MinInt64} {
							if inst != amd64.MOVQ && !amd64.FitIn32bit(c) {
								continue
							} else if (inst == amd64.SHLQ || inst == amd64.SHRQ) && (c < 0 || c > math.MaxUint8) {
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

								a := amd64.NewAssemblerImpl()
								err = a.EncodeConstToRegister(&amd64.NodeImpl{Instruction: inst,
									Types: amd64.OperandTypesConstToRegister, SrcConst: c, DstReg: reg})
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
	t.Run("float instructions", func(t *testing.T) {
		for _, inst := range []asm.Instruction{amd64.PSLLL, amd64.PSLLQ, amd64.PSRLL, amd64.PSRLQ} {
			inst := inst
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range floatRegisters {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
						for _, c := range []int64{0, 1, -1, math.MaxInt8, math.MinInt8} {
							c := c
							t.Run(fmt.Sprintf("0x%x", uint64(c)), func(t *testing.T) {
								// TODO: remove golang-asm dependency in tests.
								goasm, err := newGolangAsmAssembler()
								require.NoError(t, err)
								goasm.CompileConstToRegister(inst, c, reg)
								bs, err := goasm.Assemble()
								require.NoError(t, err)

								a := amd64.NewAssemblerImpl()
								err = a.EncodeConstToRegister(&amd64.NodeImpl{Instruction: inst,
									Types: amd64.OperandTypesConstToRegister, SrcConst: c, DstReg: reg})
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())
							})
						}
					})
				}
			})
		}
	})
}

func TestAssemblerImpl_EncodeMemoryToConst(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesMemoryToConst, DstReg: amd64.RegAX},
				expErr: "ADDL is unsupported for from:memory,to:const type",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeMemoryToConst(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	const inst = amd64.CMPL
	t.Run("ok", func(t *testing.T) {
		for _, reg := range []asm.Register{amd64.RegAX, amd64.RegR8} {
			reg := reg
			t.Run(amd64.RegisterName(reg), func(t *testing.T) {
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

								a := amd64.NewAssemblerImpl()
								err = a.EncodeMemoryToConst(&amd64.NodeImpl{Instruction: inst,
									Types: amd64.OperandTypesMemoryToConst, SrcReg: reg, SrcConst: int64(offset), DstConst: c})
								require.NoError(t, err)
								require.Equal(t, bs, a.Buf.Bytes())

							})
						}
					})
				}
			})
		}
	})

}

func TestAssemblerImpl_EncodeConstToMemory(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesConstToMemory, DstReg: amd64.RegAX},
				expErr: "ADDL is unsupported for from:const,to:memory type",
			},
			{
				n: &amd64.NodeImpl{Instruction: amd64.MOVB, Types: amd64.OperandTypesConstToMemory,
					SrcConst: math.MaxInt16,
					DstReg:   amd64.RegAX, DstConst: 0xff_ff},
				expErr: "too large load target const 32767 for MOVB",
			},
			{
				n: &amd64.NodeImpl{Instruction: amd64.MOVL, Types: amd64.OperandTypesConstToMemory,
					SrcConst: math.MaxInt64,
					DstReg:   amd64.RegAX, DstConst: 0xff_ff},
				expErr: "too large load target const 9223372036854775807 for MOVL",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				a := amd64.NewAssemblerImpl()
				err := a.EncodeConstToMemory(tc.n)
				require.EqualError(t, err, tc.expErr)
			})
		}
	})

	constsMOVB := []int64{0, 1, -1, 100, -100, math.MaxInt8, math.MinInt8}
	constsMOVL := append(constsMOVB, []int64{math.MaxInt16, math.MinInt16, 1 << 20, -(1 << 20), math.MaxInt32, math.MinInt32}...)
	constsMOVQ := constsMOVL
	t.Run("ok", func(t *testing.T) {
		for _, inst := range []asm.Instruction{amd64.MOVB, amd64.MOVL, amd64.MOVQ} {
			inst := inst
			var consts []int64
			switch inst {
			case amd64.MOVB:
				consts = constsMOVB
			case amd64.MOVL:
				consts = constsMOVL
			case amd64.MOVQ:
				consts = constsMOVQ
			}
			t.Run(amd64.InstructionName(inst), func(t *testing.T) {
				for _, reg := range []asm.Register{amd64.RegAX, amd64.RegR8} {
					reg := reg
					t.Run(amd64.RegisterName(reg), func(t *testing.T) {
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

										a := amd64.NewAssemblerImpl()
										err = a.EncodeConstToMemory(&amd64.NodeImpl{Instruction: inst,
											Types: amd64.OperandTypesConstToMemory, SrcConst: c, DstReg: reg, DstConst: int64(offset)})
										require.NoError(t, err)
										require.Equal(t, bs, a.Buf.Bytes())

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

func TestNodeImpl_GetMemoryLocation(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tests := []struct {
			n      *amd64.NodeImpl
			expErr string
		}{
			{
				n:      &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesMemoryToRegister, SrcConst: math.MaxInt64, SrcReg: amd64.RegAX, DstReg: amd64.RegR10},
				expErr: "offset does not fit in 32-bit integer",
			},
			{
				n: &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesMemoryToRegister,
					SrcConst: 10, SrcReg: asm.NilRegister, SrcMemIndex: amd64.RegR12, SrcMemScale: 1, DstReg: amd64.RegR10},
				expErr: "addressing without base register but with index is not implemented",
			},
			{
				n: &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesMemoryToRegister,
					SrcConst: 10, SrcReg: amd64.RegAX, SrcMemIndex: amd64.RegSP, SrcMemScale: 1, DstReg: amd64.RegR10},
				expErr: "SP cannot be used for SIB index",
			},
			{
				n: &amd64.NodeImpl{Instruction: amd64.ADDL, Types: amd64.OperandTypesMemoryToRegister,
					SrcConst: 10, SrcReg: amd64.RegAX, SrcMemIndex: amd64.RegR9, SrcMemScale: 3, DstReg: amd64.RegR10},
				expErr: "scale in SIB must be one of 1, 2, 4, 8 but got 3",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expErr, func(t *testing.T) {
				tc := tc
				_, _, _, _, err := tc.n.GetMemoryLocation()
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
				goasm.CompileRegisterToMemory(amd64.CMPL, amd64.RegAX, asm.NilRegister, offset)
				require.NoError(t, err)

				expectedBytes, err := goasm.Assemble()
				require.NoError(t, err)

				if expectedBytes[0]&amd64.RexPrefixDefault == amd64.RexPrefixDefault {
					expectedBytes = append(expectedBytes[0:1], expectedBytes[2:]...)
				} else {
					expectedBytes = expectedBytes[1:]
				}

				n := &amd64.NodeImpl{Types: amd64.OperandTypesMemoryToRegister,
					SrcReg: asm.NilRegister, SrcConst: offset}

				rexPrefix, modRM, sbi, displacementWidth, err := n.GetMemoryLocation()
				require.NoError(t, err)

				a := amd64.NewAssemblerImpl()
				if rexPrefix != amd64.RexPrefixNone {
					a.Buf.WriteByte(rexPrefix)
				}

				a.Buf.WriteByte(modRM)
				if sbi != nil {
					a.Buf.WriteByte(*sbi)
				}
				if displacementWidth != 0 {
					a.WriteConst(n.SrcConst, displacementWidth)
				}
				actual := a.Buf.Bytes()
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
				n := &amd64.NodeImpl{Types: amd64.OperandTypesMemoryToRegister,
					SrcReg: srcReg, SrcConst: offset}

				for _, indexReg := range append(intRegisters,
					// Without index.
					asm.NilRegister) {
					if indexReg == amd64.RegSP {
						continue
					}
					n.SrcMemIndex = indexReg
					for _, scale := range []byte{1, 2, 4, 8} {
						n.SrcMemScale = scale
						t.Run(n.String(), func(t *testing.T) {
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)
							goasm.CompileMemoryWithIndexToRegister(amd64.ADDL, srcReg, offset, indexReg, int16(scale), amd64.RegAX)
							expectedBytes, err := goasm.Assemble()
							require.NoError(t, err)

							if expectedBytes[0]&amd64.RexPrefixDefault == amd64.RexPrefixDefault {
								expectedBytes = append(expectedBytes[0:1], expectedBytes[2:]...)
							} else {
								expectedBytes = expectedBytes[1:]
							}

							rexPrefix, modRM, sbi, displacementWidth, err := n.GetMemoryLocation()
							require.NoError(t, err)

							a := amd64.NewAssemblerImpl()
							if rexPrefix != amd64.RexPrefixNone {
								a.Buf.WriteByte(rexPrefix)
							}

							a.Buf.WriteByte(modRM)
							if sbi != nil {
								a.Buf.WriteByte(*sbi)
							}
							if displacementWidth != 0 {
								a.WriteConst(n.SrcConst, displacementWidth)
							}
							actual := a.Buf.Bytes()
							require.Equal(t, expectedBytes, actual)
						})
					}
				}
			}
		}
	})
}

func TestNodeImpl_GetRegisterToRegisterModRM(t *testing.T) {
	t.Run("int to int", func(t *testing.T) {
		for _, srcOnModRMReg := range []bool{true, false} {
			srcOnModRMReg := srcOnModRMReg
			t.Run(fmt.Sprintf("srcOnModRMReg=%v", srcOnModRMReg), func(t *testing.T) {
				for _, srcReg := range intRegisters {
					srcReg := srcReg
					for _, DstReg := range intRegisters {
						DstReg := DstReg
						t.Run(fmt.Sprintf("src=%s,dst=%s", amd64.RegisterName(srcReg), amd64.RegisterName(DstReg)), func(t *testing.T) {
							goasm, err := newGolangAsmAssembler()
							require.NoError(t, err)

							// Temporarily pass amd64.MOVL to golang-asm to produce the expected bytes.
							if srcOnModRMReg {
								goasm.CompileRegisterToRegister(amd64.MOVL, srcReg, DstReg)
							} else {
								goasm.CompileRegisterToRegister(amd64.MOVL, DstReg, srcReg)
							}

							expectedBytes, err := goasm.Assemble()
							require.NoError(t, err)

							n := amd64.NodeImpl{SrcReg: srcReg, DstReg: DstReg}
							rexPrefix, modRM, err := n.GetRegisterToRegisterModRM(srcOnModRMReg)
							require.NoError(t, err)

							// Skip the opcode for MOVL to make this test opcode-independent.
							if rexPrefix != amd64.RexPrefixNone {
								require.Equal(t, 3, len(expectedBytes))
								require.Equal(t, expectedBytes[0], rexPrefix)
								require.Equal(t, expectedBytes[2], modRM)
							} else {
								require.Equal(t, 2, len(expectedBytes))
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
			for _, DstReg := range floatRegisters {
				DstReg := DstReg
				t.Run(fmt.Sprintf("src=%s,dst=%s", amd64.RegisterName(srcReg), amd64.RegisterName(DstReg)), func(t *testing.T) {
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					// Temporarily pass amd64.SUBSS to golang-asm to produce the expected bytes.
					goasm.CompileRegisterToRegister(amd64.SUBSS, srcReg, DstReg)
					expectedBytes, err := goasm.Assemble()
					require.NoError(t, err)

					n := amd64.NodeImpl{SrcReg: srcReg, DstReg: DstReg}
					rexPrefix, modRM, err := n.GetRegisterToRegisterModRM(false)
					require.NoError(t, err)

					if rexPrefix != amd64.RexPrefixNone {
						require.Equal(t, 5, len(expectedBytes))
						require.Equal(t, expectedBytes[1], rexPrefix)
						require.Equal(t, expectedBytes[4], modRM)
					} else {
						require.Equal(t, 4, len(expectedBytes))
						require.Equal(t, expectedBytes[3], modRM)
					}
				})
			}
		}
	})
	t.Run("float to int", func(t *testing.T) {
		for _, srcReg := range floatRegisters {
			srcReg := srcReg
			for _, DstReg := range intRegisters {
				DstReg := DstReg
				t.Run(fmt.Sprintf("src=%s,dst=%s", amd64.RegisterName(srcReg), amd64.RegisterName(DstReg)), func(t *testing.T) {
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					// Temporarily pass amd64.CVTTSS2SL to golang-asm to produce the expected bytes.
					goasm.CompileRegisterToRegister(amd64.CVTTSS2SL, srcReg, DstReg)
					expectedBytes, err := goasm.Assemble()
					require.NoError(t, err)

					n := amd64.NodeImpl{SrcReg: srcReg, DstReg: DstReg}
					rexPrefix, modRM, err := n.GetRegisterToRegisterModRM(false)
					require.NoError(t, err)

					if rexPrefix != amd64.RexPrefixNone {
						require.Equal(t, 5, len(expectedBytes))
						require.Equal(t, expectedBytes[1], rexPrefix)
						require.Equal(t, expectedBytes[4], modRM)
					} else {
						require.Equal(t, 4, len(expectedBytes))
						require.Equal(t, expectedBytes[3], modRM)
					}
				})
			}
		}
	})
	t.Run("int to float", func(t *testing.T) {
		for _, srcReg := range intRegisters {
			srcReg := srcReg
			for _, DstReg := range floatRegisters {
				DstReg := DstReg
				t.Run(fmt.Sprintf("src=%s,dst=%s", amd64.RegisterName(srcReg), amd64.RegisterName(DstReg)), func(t *testing.T) {
					goasm, err := newGolangAsmAssembler()
					require.NoError(t, err)

					// Temporarily pass amd64.CVTSL2SS to golang-asm to produce the expected bytes.
					goasm.CompileRegisterToRegister(amd64.CVTSL2SS, srcReg, DstReg)
					expectedBytes, err := goasm.Assemble()
					require.NoError(t, err)

					n := amd64.NodeImpl{SrcReg: srcReg, DstReg: DstReg}
					rexPrefix, modRM, err := n.GetRegisterToRegisterModRM(false)
					require.NoError(t, err)

					if rexPrefix != amd64.RexPrefixNone {
						require.Equal(t, 5, len(expectedBytes))
						require.Equal(t, expectedBytes[1], rexPrefix)
						require.Equal(t, expectedBytes[4], modRM)
					} else {
						require.Equal(t, 4, len(expectedBytes))
						require.Equal(t, expectedBytes[3], modRM)
					}
				})
			}
		}
	})
}
