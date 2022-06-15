package amd64

import (
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNodeImpl_AssignJumpTarget(t *testing.T) {
	n := &NodeImpl{}
	target := &NodeImpl{}
	n.AssignJumpTarget(target)
	require.Equal(t, n.JumpTarget, target)
}

func TestNodeImpl_AssignDestinationConstant(t *testing.T) {
	n := &NodeImpl{}
	n.AssignDestinationConstant(12345)
	require.Equal(t, int64(12345), n.DstConst)
}

func TestNodeImpl_AssignSourceConstant(t *testing.T) {
	n := &NodeImpl{}
	n.AssignSourceConstant(12345)
	require.Equal(t, int64(12345), n.SrcConst)
}

func TestNodeImpl_String(t *testing.T) {
	tests := []struct {
		in  *NodeImpl
		exp string
	}{
		{
			in:  &NodeImpl{Instruction: NOP},
			exp: "NOP",
		},
		{
			in:  &NodeImpl{Instruction: SETCC, Types: OperandTypesNoneToRegister, DstReg: RegAX},
			exp: "SETCC AX",
		},
		{
			in:  &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToMemory, DstReg: RegAX, DstConst: 100},
			exp: "JMP [AX + 0x64]",
		},
		{
			in:  &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToMemory, DstReg: RegAX, DstConst: 100, DstMemScale: 8, DstMemIndex: RegR11},
			exp: "JMP [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToBranch, JumpTarget: &NodeImpl{Instruction: JMP, Types: OperandTypesNoneToMemory, DstReg: RegAX, DstConst: 100}},
			exp: "JMP {JMP [AX + 0x64]}",
		},
		{
			in:  &NodeImpl{Instruction: IDIVQ, Types: OperandTypesRegisterToNone, SrcReg: RegDX},
			exp: "IDIVQ DX",
		},
		{
			in:  &NodeImpl{Instruction: ADDL, Types: OperandTypesRegisterToRegister, SrcReg: RegDX, DstReg: RegR14},
			exp: "ADDL DX, R14",
		},
		{
			in: &NodeImpl{Instruction: MOVQ, Types: OperandTypesRegisterToMemory,
				SrcReg: RegDX, DstReg: RegR14, DstConst: 100},
			exp: "MOVQ DX, [R14 + 0x64]",
		},
		{
			in: &NodeImpl{Instruction: MOVQ, Types: OperandTypesRegisterToMemory,
				SrcReg: RegDX, DstReg: RegR14, DstConst: 100, DstMemIndex: RegCX, DstMemScale: 4},
			exp: "MOVQ DX, [R14 + 0x64 + CX*0x4]",
		},
		{
			in: &NodeImpl{Instruction: CMPL, Types: OperandTypesRegisterToConst,
				SrcReg: RegDX, DstConst: 100},
			exp: "CMPL DX, 0x64",
		},
		{
			in: &NodeImpl{Instruction: MOVL, Types: OperandTypesMemoryToRegister,
				SrcReg: RegDX, SrcConst: 1, DstReg: RegAX},
			exp: "MOVL [DX + 0x1], AX",
		},
		{
			in: &NodeImpl{Instruction: MOVL, Types: OperandTypesMemoryToRegister,
				SrcReg: RegDX, SrcConst: 1, SrcMemIndex: RegR12, SrcMemScale: 2,
				DstReg: RegAX},
			exp: "MOVL [DX + 1 + R12*0x2], AX",
		},
		{
			in: &NodeImpl{Instruction: CMPQ, Types: OperandTypesMemoryToConst,
				SrcReg: RegDX, SrcConst: 1, SrcMemIndex: RegR12, SrcMemScale: 2,
				DstConst: 123},
			exp: "CMPQ [DX + 1 + R12*0x2], 0x7b",
		},
		{
			in:  &NodeImpl{Instruction: MOVQ, Types: OperandTypesConstToMemory, SrcConst: 123, DstReg: RegAX, DstConst: 100, DstMemScale: 8, DstMemIndex: RegR11},
			exp: "MOVQ 0x7b, [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &NodeImpl{Instruction: MOVQ, Types: OperandTypesConstToRegister, SrcConst: 123, DstReg: RegAX},
			exp: "MOVQ 0x7b, AX",
		},
	}

	for _, tt := range tests {
		tc := tt
		require.Equal(t, tc.exp, tc.in.String())
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := NewAssemblerImpl()

	root := &NodeImpl{}
	a.addNode(root)
	require.Equal(t, a.Root, root)
	require.Equal(t, a.Current, root)
	require.Nil(t, root.Next)

	next := &NodeImpl{}
	a.addNode(next)
	require.Equal(t, a.Root, root)
	require.Equal(t, a.Current, next)
	require.Equal(t, next, root.Next)
	require.Nil(t, next.Next)
}

func TestAssemblerImpl_newNode(t *testing.T) {
	a := NewAssemblerImpl()
	actual := a.newNode(ADDL, OperandTypesConstToMemory)
	require.Equal(t, ADDL, actual.Instruction)
	require.Equal(t, OperandTypeConst, actual.Types.src)
	require.Equal(t, OperandTypeMemory, actual.Types.dst)
	require.Equal(t, actual, a.Root)
	require.Equal(t, actual, a.Current)
}

func TestAssemblerImpl_encodeNode(t *testing.T) {
	a := NewAssemblerImpl()
	err := a.EncodeNode(&NodeImpl{
		Types: OperandTypes{OperandTypeBranch, OperandTypeRegister},
	})
	require.EqualError(t, err, "encoder undefined for [from:branch,to:register] operand type")
}

func TestAssemblerImpl_padNOP(t *testing.T) {
	tests := []struct {
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
	}

	for _, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(tc.num), func(t *testing.T) {
			a := NewAssemblerImpl()
			a.padNOP(tc.num)
			actual := a.Buf.Bytes()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileStandAlone(RET)
	actualNode := a.Current
	require.Equal(t, RET, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileConstToRegister(MOVQ, 1000, RegAX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, int64(1000), actualNode.SrcConst)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, OperandTypeConst, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToRegister(MOVQ, RegBX, RegAX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileMemoryToRegister(MOVQ, RegBX, 100, RegAX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, int64(100), actualNode.SrcConst)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToMemory(MOVQ, RegBX, RegAX, 100)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, int64(100), actualNode.DstConst)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileJump(JMP)
	actualNode := a.Current
	require.Equal(t, JMP, actualNode.Instruction)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeBranch, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileJumpToRegister(JNE, RegAX)
	actualNode := a.Current
	require.Equal(t, JNE, actualNode.Instruction)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileJumpToMemory(JNE, RegAX, 100)
	actualNode := a.Current
	require.Equal(t, JNE, actualNode.Instruction)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, int64(100), actualNode.DstConst)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileReadInstructionAddress(RegR10, RET)
	actualNode := a.Current
	require.Equal(t, LEAQ, actualNode.Instruction)
	require.Equal(t, RegR10, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
	require.Equal(t, RET, actualNode.readInstructionAddressBeforeTargetInstruction)
}

func TestAssemblerImpl_CompileRegisterToRegisterWithArg(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToRegisterWithArg(MOVQ, RegBX, RegAX, 123)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, byte(123), actualNode.Arg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryWithIndexToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileMemoryWithIndexToRegister(MOVQ, RegBX, 100, RegR10, 8, RegAX)
	actualNode := a.Current
	require.Equal(t, MOVQ, actualNode.Instruction)
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, int64(100), actualNode.SrcConst)
	require.Equal(t, RegR10, actualNode.SrcMemIndex)
	require.Equal(t, byte(8), actualNode.SrcMemScale)
	require.Equal(t, RegAX, actualNode.DstReg)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToConst(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToConst(MOVQ, RegBX, 123)
	actualNode := a.Current
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, int64(123), actualNode.DstConst)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeConst, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileRegisterToNone(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileRegisterToNone(MOVQ, RegBX)
	actualNode := a.Current
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, OperandTypeRegister, actualNode.Types.src)
	require.Equal(t, OperandTypeNone, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileNoneToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileNoneToRegister(MOVQ, RegBX)
	actualNode := a.Current
	require.Equal(t, RegBX, actualNode.DstReg)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileNoneToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileNoneToMemory(MOVQ, RegBX, 1234)
	actualNode := a.Current
	require.Equal(t, RegBX, actualNode.DstReg)
	require.Equal(t, int64(1234), actualNode.DstConst)
	require.Equal(t, OperandTypeNone, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileConstToMemory(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileConstToMemory(MOVQ, -9999, RegBX, 1234)
	actualNode := a.Current
	require.Equal(t, RegBX, actualNode.DstReg)
	require.Equal(t, int64(-9999), actualNode.SrcConst)
	require.Equal(t, int64(1234), actualNode.DstConst)
	require.Equal(t, OperandTypeConst, actualNode.Types.src)
	require.Equal(t, OperandTypeMemory, actualNode.Types.dst)
}

func TestAssemblerImpl_CompileMemoryToConst(t *testing.T) {
	a := NewAssemblerImpl()
	a.CompileMemoryToConst(MOVQ, RegBX, 1234, -9999)
	actualNode := a.Current
	require.Equal(t, RegBX, actualNode.SrcReg)
	require.Equal(t, int64(1234), actualNode.SrcConst)
	require.Equal(t, int64(-9999), actualNode.DstConst)
	require.Equal(t, OperandTypeMemory, actualNode.Types.src)
	require.Equal(t, OperandTypeConst, actualNode.Types.dst)
}

func TestAssemblerImpl_encodeNoneToNone(t *testing.T) {
	tests := []struct {
		inst   asm.Instruction
		exp    []byte
		expErr bool
	}{
		{inst: ADDL, expErr: true},
		{inst: CDQ, exp: []byte{0x99}},
		{inst: CQO, exp: []byte{0x48, 0x99}},
		{inst: NOP, exp: nil},
		{inst: RET, exp: []byte{0xc3}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(InstructionName(tc.inst), func(t *testing.T) {
			a := NewAssemblerImpl()
			err := a.encodeNoneToNone(&NodeImpl{Instruction: tc.inst, Types: OperandTypesNoneToNone})
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.exp, a.Buf.Bytes())
			}
		})
	}
}

func TestAssemblerImpl_EncodeMemoryToRegister(t *testing.T) {
	// These are not supported by golang-asm, so we test here instead of integration tests.
	tests := []struct {
		n   *NodeImpl
		exp []byte
	}{
		{
			n: &NodeImpl{
				Instruction: MOVDQU,
				Types:       OperandTypesMemoryToRegister,
				SrcReg:      RegAX,
				DstReg:      RegX3,
				SrcConst:    10,
			},
			exp: []byte{0xf3, 0xf, 0x6f, 0x58, 0xa},
		},
		{
			n: &NodeImpl{
				Instruction: MOVDQU,
				Types:       OperandTypesMemoryToRegister,
				SrcReg:      RegR13,
				DstReg:      RegX3,
				SrcConst:    10,
			},
			exp: []byte{0xf3, 0x41, 0xf, 0x6f, 0x5d, 0xa},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.n.String(), func(t *testing.T) {
			a := NewAssemblerImpl()
			err := a.EncodeMemoryToRegister(tc.n)
			require.NoError(t, err)

			actual, err := a.Assemble()
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))

		})
	}
}

func TestAssemblerImpl_EncodeRegisterToMemory(t *testing.T) {
	// These are not supported by golang-asm, so we test here instead of integration tests.
	tests := []struct {
		n   *NodeImpl
		exp []byte
	}{
		{
			n: &NodeImpl{
				Instruction: MOVDQU,
				Types:       OperandTypesRegisterToMemory,
				SrcReg:      RegX3,
				DstReg:      RegAX,
				SrcConst:    10,
			},
			exp: []byte{0xf3, 0xf, 0x7f, 0x18},
		},
		{
			n: &NodeImpl{
				Instruction: MOVDQU,
				Types:       OperandTypesRegisterToMemory,
				SrcReg:      RegX3,
				DstReg:      RegR13,
				SrcConst:    10,
			},
			exp: []byte{0xf3, 0x41, 0xf, 0x7f, 0x5d, 0x0},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.n.String(), func(t *testing.T) {
			a := NewAssemblerImpl()
			err := a.EncodeRegisterToMemory(tc.n)
			require.NoError(t, err)

			actual, err := a.Assemble()
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))

		})
	}
}

func TestAssemblerImpl_EncodeRegisterToRegister(t *testing.T) {
	// These are not supported by golang-asm, so we test here instead of integration tests.
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "MOVDQU",
			n: &NodeImpl{
				Instruction: MOVDQU,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0xf3, 0x44, 0xf, 0x6f, 0xd3},
		},
		{
			name: "MOVDQU",
			n: &NodeImpl{
				Instruction: MOVDQU,
				SrcReg:      RegX10,
				DstReg:      RegX3,
			},
			exp: []byte{0xf3, 0x41, 0xf, 0x6f, 0xda},
		},
		{
			name: "MOVDQU",
			n: &NodeImpl{
				Instruction: MOVDQU,
				SrcReg:      RegX10,
				DstReg:      RegX15,
			},
			exp: []byte{0xf3, 0x45, 0xf, 0x6f, 0xfa},
		},
		{
			name: "MOVDQA",
			n: &NodeImpl{
				Instruction: MOVDQA,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x6f, 0xd3},
		},
		{
			name: "MOVDQA",
			n: &NodeImpl{
				Instruction: MOVDQA,
				SrcReg:      RegX10,
				DstReg:      RegX3,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x6f, 0xda},
		},
		{
			name: "MOVDQA",
			n: &NodeImpl{
				Instruction: MOVDQA,
				SrcReg:      RegX10,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x45, 0xf, 0x6f, 0xfa},
		},
		{
			name: "PACKSSWB",
			n: &NodeImpl{
				Instruction: PACKSSWB,
				SrcReg:      RegX10,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x45, 0xf, 0x63, 0xfa},
		},
		{
			name: "pmovmskb r15d, xmm10",
			n: &NodeImpl{
				Instruction: PMOVMSKB,
				SrcReg:      RegX10,
				DstReg:      RegR15,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xd7, 0xfa},
		},
		{
			name: "movmskps eax, xmm10",
			n: &NodeImpl{
				Instruction: MOVMSKPS,
				SrcReg:      RegX10,
				DstReg:      RegAX,
			},
			exp: []byte{0x41, 0xf, 0x50, 0xc2},
		},
		{
			name: "movmskps r13d, xmm1",
			n: &NodeImpl{
				Instruction: MOVMSKPS,
				SrcReg:      RegX1,
				DstReg:      RegR13,
			},
			exp: []byte{0x44, 0xf, 0x50, 0xe9},
		},
		{
			name: "movmskpd eax, xmm10",
			n: &NodeImpl{
				Instruction: MOVMSKPD,
				SrcReg:      RegX10,
				DstReg:      RegAX,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x50, 0xc2},
		},
		{
			name: "movmskpd r15d, xmm1",
			n: &NodeImpl{
				Instruction: MOVMSKPD,
				SrcReg:      RegX1,
				DstReg:      RegR15,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x50, 0xf9},
		},
		{
			name: "pand xmm15, xmm1",
			n: &NodeImpl{
				Instruction: PAND,
				SrcReg:      RegX1,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xdb, 0xf9},
		},
		{
			name: "por xmm1, xmm15",
			n: &NodeImpl{
				Instruction: POR,
				SrcReg:      RegX15,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0xeb, 0xcf},
		},
		{
			name: "pandn xmm13, xmm15",
			n: &NodeImpl{
				Instruction: PANDN,
				SrcReg:      RegX15,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xdf, 0xef},
		},
		{
			name: "psrad xmm13, xmm15",
			n: &NodeImpl{
				Instruction: PSRAD,
				SrcReg:      RegX15,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xe2, 0xef},
		},
		{
			name: "psraw xmm1, xmm1",
			n: &NodeImpl{
				Instruction: PSRAW,
				SrcReg:      RegX1,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0xe1, 0xc9},
		},
		{
			name: "psrlq xmm14, xmm14",
			n: &NodeImpl{
				Instruction: PSRLQ,
				SrcReg:      RegX14,
				DstReg:      RegX14,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xd3, 0xf6},
		},
		{
			name: "psrld xmm3, xmm3",
			n: &NodeImpl{
				Instruction: PSRLD,
				SrcReg:      RegX3,
				DstReg:      RegX3,
			},
			exp: []byte{0x66, 0xf, 0xd2, 0xdb},
		},
		{
			name: "psrlw xmm15, xmm1",
			n: &NodeImpl{
				Instruction: PSRLW,
				SrcReg:      RegX1,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xd1, 0xf9},
		},
		{
			name: "psllw xmm1, xmm15",
			n: &NodeImpl{
				Instruction: PSLLW,
				SrcReg:      RegX15,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0xf1, 0xcf},
		},
		{
			name: "punpcklbw xmm1, xmm15",
			n: &NodeImpl{
				Instruction: PUNPCKLBW,
				SrcReg:      RegX15,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x60, 0xcf},
		},
		{
			name: "punpckhbw xmm11, xmm1",
			n: &NodeImpl{
				Instruction: PUNPCKHBW,
				SrcReg:      RegX1,
				DstReg:      RegX11,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x68, 0xd9},
		},
		{
			name: "pslld xmm11, xmm1",
			n: &NodeImpl{
				Instruction: PSLLD,
				SrcReg:      RegX1,
				DstReg:      RegX11,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xf2, 0xd9},
		},
		{
			name: "psllq xmm11, xmm15",
			n: &NodeImpl{
				Instruction: PSLLQ,
				SrcReg:      RegX15,
				DstReg:      RegX11,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xf3, 0xdf},
		},
		{
			name: "cmpeqps xmm11, xmm15",
			n: &NodeImpl{
				Instruction: CMPPS,
				SrcReg:      RegX15,
				DstReg:      RegX11,
				Arg:         0, // CMPPS with arg=0 == Pseudo-Op CMPEQPS.
			},
			exp: []byte{0x45, 0xf, 0xc2, 0xdf, 0x0},
		},
		{
			name: "cmpordps xmm1, xmm5",
			n: &NodeImpl{
				Instruction: CMPPS,
				SrcReg:      RegX5,
				DstReg:      RegX1,
				Arg:         7, // CMPPS with arg=7 == Pseudo-Op CMPORDPS.
			},
			exp: []byte{0xf, 0xc2, 0xcd, 0x7},
		},
		{
			name: "cmplepd xmm11, xmm15",
			n: &NodeImpl{
				Instruction: CMPPD,
				SrcReg:      RegX15,
				DstReg:      RegX11,
				Arg:         2, // CMPPD with arg=2 == Pseudo-Op CMPLEPD.
			},
			exp: []byte{0x66, 0x45, 0xf, 0xc2, 0xdf, 0x2},
		},
		{
			name: "cmpneqpd xmm1, xmm5",
			n: &NodeImpl{
				Instruction: CMPPD,
				SrcReg:      RegX5,
				DstReg:      RegX1,
				Arg:         4, // CMPPD with arg=4 == Pseudo-Op CMPNEQPD.
			},
			exp: []byte{0x66, 0xf, 0xc2, 0xcd, 0x4},
		},
		{
			name: "pcmpgtq xmm10, xmm3",
			n: &NodeImpl{
				Instruction: PCMPGTQ,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x37, 0xd3},
		},
		{
			name: "pcmpgtd xmm10, xmm3",
			n: &NodeImpl{
				Instruction: PCMPGTD,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x66, 0xd3},
		},
		{
			name: "pminsd xmm10, xmm3",
			n: &NodeImpl{
				Instruction: PMINSD,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x39, 0xd3},
		},
		{
			name: "pmaxsd xmm1, xmm12",
			n: &NodeImpl{
				Instruction: PMAXSD,
				SrcReg:      RegX12,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x38, 0x3d, 0xcc},
		},
		{
			name: "pmaxsw xmm1, xmm12",
			n: &NodeImpl{
				Instruction: PMAXSW,
				SrcReg:      RegX12,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0xee, 0xcc},
		},
		{
			name: "pminsw xmm1, xmm12",
			n: &NodeImpl{
				Instruction: PMINSW,
				SrcReg:      RegX12,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0xea, 0xcc},
		},
		{
			name: "pcmpgtb xmm1, xmm12",
			n: &NodeImpl{
				Instruction: PCMPGTB,
				SrcReg:      RegX12,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x64, 0xcc},
		},
		{
			name: "pminsb xmm1, xmm12",
			n: &NodeImpl{
				Instruction: PMINSB,
				SrcReg:      RegX12,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x38, 0x38, 0xcc},
		},
		{
			name: "pmaxsb xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMAXSB,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0x38, 0x3c, 0xca},
		},
		{
			name: "pminud xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMINUD,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0x38, 0x3b, 0xca},
		},
		{
			name: "pminuw xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMINUW,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0x38, 0x3a, 0xca},
		},
		{
			name: "pminub xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMINUB,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0xda, 0xca},
		},
		{
			name: "pmaxud xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMAXUD,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0x38, 0x3f, 0xca},
		},
		{
			name: "pmaxuw xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMAXUW,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0x38, 0x3e, 0xca},
		},
		{
			name: "pmaxub xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PMAXUB,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0xde, 0xca},
		},
		{
			name: "pcmpgtw xmm1, xmm2",
			n: &NodeImpl{
				Instruction: PCMPGTW,
				SrcReg:      RegX2,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0x65, 0xca},
		},

		{
			name: "pmullw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PMULLW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xd5, 0xe9},
		},
		{
			name: "pmulld xmm1, xmm11",
			n: &NodeImpl{
				Instruction: PMULLD,
				SrcReg:      RegX11,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x38, 0x40, 0xcb},
		},
		{
			name: "pmuludq xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PMULUDQ,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xf4, 0xe9},
		},
		{
			name: "psubsb xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PSUBSB,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xe8, 0xe9},
		},
		{
			name: "psubsw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PSUBSW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xe9, 0xe9},
		},
		{
			name: "psubusb xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PSUBUSB,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xd8, 0xe9},
		},
		{
			name: "psubusw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PSUBUSW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xd9, 0xe9},
		},
		{
			name: "paddsw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PADDSW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xed, 0xe9},
		},
		{
			name: "paddsb xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PADDSB,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xec, 0xe9},
		},
		{
			name: "paddusw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PADDUSW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xdd, 0xe9},
		},
		{
			name: "pavgb xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PAVGB,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xe0, 0xe9},
		},
		{
			name: "pavgw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PAVGW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xe3, 0xe9},
		},
		{
			name: "pabsb xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PABSB,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x1c, 0xe9},
		},
		{
			name: "pabsw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PABSW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x1d, 0xe9},
		},
		{
			name: "pabsd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PABSD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x1e, 0xe9},
		},
		{
			name: "blendvpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: BLENDVPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x15, 0xe9},
		},
		{
			name: "maxpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: MAXPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x5f, 0xe9},
		},
		{
			name: "maxps xmm13, xmm1",
			n: &NodeImpl{
				Instruction: MAXPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x44, 0xf, 0x5f, 0xe9},
		},
		{
			name: "minpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: MINPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x5d, 0xe9},
		},
		{
			name: "minps xmm13, xmm1",
			n: &NodeImpl{
				Instruction: MINPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x44, 0xf, 0x5d, 0xe9},
		},
		{
			name: "andnpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: ANDNPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x55, 0xe9},
		},
		{
			name: "andnps xmm13, xmm1",
			n: &NodeImpl{
				Instruction: ANDNPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x44, 0xf, 0x55, 0xe9},
		},
		{
			name: "mulps xmm13, xmm1",
			n: &NodeImpl{
				Instruction: MULPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x44, 0xf, 0x59, 0xe9},
		},
		{
			name: "mulpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: MULPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x59, 0xe9},
		},
		{
			name: "divps xmm13, xmm1",
			n: &NodeImpl{
				Instruction: DIVPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x44, 0xf, 0x5e, 0xe9},
		},
		{
			name: "divpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: DIVPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x5e, 0xe9},
		},
		{
			name: "sqrtps xmm13, xmm1",
			n: &NodeImpl{
				Instruction: SQRTPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x44, 0xf, 0x51, 0xe9},
		},
		{
			name: "sqrtpd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: SQRTPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x51, 0xe9},
		},
		{
			name: "roundps xmm13, xmm1, 0",
			n: &NodeImpl{
				Instruction: ROUNDPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         0,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x8, 0xe9, 0x0},
		},
		{
			name: "roundps xmm13, xmm1, 1",
			n: &NodeImpl{
				Instruction: ROUNDPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         1,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x8, 0xe9, 0x1},
		},
		{
			name: "roundps xmm13, xmm1, 3",
			n: &NodeImpl{
				Instruction: ROUNDPS,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         3,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x8, 0xe9, 0x3},
		},
		{
			name: "roundpd xmm13, xmm1, 0",
			n: &NodeImpl{
				Instruction: ROUNDPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         0,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x9, 0xe9, 0x0},
		},
		{
			name: "roundpd xmm13, xmm1, 1",
			n: &NodeImpl{
				Instruction: ROUNDPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         1,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x9, 0xe9, 0x1},
		},
		{
			name: "roundpd xmm13, xmm1, 3",
			n: &NodeImpl{
				Instruction: ROUNDPD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         3,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x9, 0xe9, 0x3},
		},
		{
			name: "palignr xmm13, xmm1, 3",
			n: &NodeImpl{
				Instruction: PALIGNR,
				SrcReg:      RegX1,
				DstReg:      RegX13,
				Arg:         3,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x3a, 0xf, 0xe9, 0x3},
		},
		{
			name: "punpcklwd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PUNPCKLWD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x61, 0xe9},
		},
		{
			name: "punpckhwd xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PUNPCKHWD,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x69, 0xe9},
		},
		{
			name: "pmulhuw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PMULHUW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xe4, 0xe9},
		},
		{
			name: "pmuldq xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PMULDQ,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0x28, 0xe9},
		},
		{
			name: "pmulhrsw xmm13, xmm1",
			n: &NodeImpl{
				Instruction: PMULHRSW,
				SrcReg:      RegX1,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x38, 0xb, 0xe9},
		},

		{
			name: "pmovsxbw xmm5, xmm10",
			n:    &NodeImpl{Instruction: PMOVSXBW, SrcReg: RegX10, DstReg: RegX5},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x20, 0xea},
		},
		{
			name: "pmovsxwd xmm5, xmm10",
			n:    &NodeImpl{Instruction: PMOVSXWD, SrcReg: RegX10, DstReg: RegX5},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x23, 0xea},
		},
		{
			name: "pmovsxdq xmm5, xmm10",
			n:    &NodeImpl{Instruction: PMOVSXDQ, SrcReg: RegX10, DstReg: RegX5},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x25, 0xea},
		},
		{
			name: "pmovzxbw xmm5, xmm10",
			n:    &NodeImpl{Instruction: PMOVZXBW, SrcReg: RegX10, DstReg: RegX5},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x30, 0xea},
		},
		{
			name: "pmovzxwd xmm5, xmm10",
			n:    &NodeImpl{Instruction: PMOVZXWD, SrcReg: RegX10, DstReg: RegX5},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x33, 0xea},
		},
		{
			name: "pmovzxdq xmm5, xmm10",
			n:    &NodeImpl{Instruction: PMOVZXDQ, SrcReg: RegX10, DstReg: RegX5},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x35, 0xea},
		},
		{
			name: "pmulhw xmm2, xmm1",
			n:    &NodeImpl{Instruction: PMULHW, SrcReg: RegX1, DstReg: RegX2},
			exp:  []byte{0x66, 0xf, 0xe5, 0xd1},
		},
		{
			name: "cmpltps xmm1, xmm14",
			n:    &NodeImpl{Instruction: CMPEQPS, SrcReg: RegX14, DstReg: RegX1, Arg: 1},
			exp:  []byte{0x41, 0xf, 0xc2, 0xce, 0x1},
		},
		{
			name: "cmpunordpd xmm1, xmm14",
			n:    &NodeImpl{Instruction: CMPEQPD, SrcReg: RegX14, DstReg: RegX1, Arg: 3},
			exp:  []byte{0x66, 0x41, 0xf, 0xc2, 0xce, 0x3},
		},
		{
			name: "cvttps2dq xmm1, xmm14",
			n:    &NodeImpl{Instruction: CVTTPS2DQ, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0xf3, 0x41, 0xf, 0x5b, 0xce},
		},
		{
			name: "cvtdq2ps xmm1, xmm14",
			n:    &NodeImpl{Instruction: CVTDQ2PS, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x41, 0xf, 0x5b, 0xce},
		},
		{
			name: "movupd xmm1, xmm14",
			n:    &NodeImpl{Instruction: MOVUPD, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0x10, 0xce},
		},
		{
			name: "shufps xmm1, xmm14, 5",
			n:    &NodeImpl{Instruction: SHUFPS, SrcReg: RegX14, DstReg: RegX1, Arg: 5},
			exp:  []byte{0x41, 0xf, 0xc6, 0xce, 0x5},
		},
		{
			name: "pmaddwd xmm1, xmm14",
			n:    &NodeImpl{Instruction: PMADDWD, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0xf5, 0xce},
		},
		{
			name: "cvtdq2pd xmm1, xmm14",
			n:    &NodeImpl{Instruction: CVTDQ2PD, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0xf3, 0x41, 0xf, 0xe6, 0xce},
		},
		{
			name: "unpcklps xmm1, xmm14",
			n:    &NodeImpl{Instruction: UNPCKLPS, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x41, 0xf, 0x14, 0xce},
		},
		{
			name: "packuswb xmm1, xmm14",
			n:    &NodeImpl{Instruction: PACKUSWB, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0x67, 0xce},
		},
		{
			name: "packssdw xmm1, xmm14",
			n:    &NodeImpl{Instruction: PACKSSDW, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0x6b, 0xce},
		},
		{
			name: "packusdw xmm1, xmm14",
			n:    &NodeImpl{Instruction: PACKUSDW, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x2b, 0xce},
		},
		{
			name: "cvtps2pd xmm1, xmm14",
			n:    &NodeImpl{Instruction: CVTPS2PD, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x41, 0xf, 0x5a, 0xce},
		},
		{
			name: "cvtpd2ps xmm1, xmm14",
			n:    &NodeImpl{Instruction: CVTPD2PS, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0x5a, 0xce},
		},
		{
			name: "pmaddubsw xmm1, xmm14",
			n:    &NodeImpl{Instruction: PMADDUBSW, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0x38, 0x4, 0xce},
		},
		{
			name: "cvttpd2dq xmm1, xmm14",
			n:    &NodeImpl{Instruction: CVTTPD2DQ, SrcReg: RegX14, DstReg: RegX1},
			exp:  []byte{0x66, 0x41, 0xf, 0xe6, 0xce},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl()
			err := a.EncodeRegisterToRegister(tc.n)
			require.NoError(t, err)

			actual, err := a.Assemble()
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}

func TestAssemblerImpl_EncodeConstToRegister(t *testing.T) {
	// These are not supported by golang-asm, so we test here instead of integration tests.
	tests := []struct {
		name string
		n    *NodeImpl
		exp  []byte
	}{
		{
			name: "psraw xmm10, 1",
			n: &NodeImpl{
				Instruction: PSRAW,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    1,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x71, 0xe2, 0x1},
		},
		{
			name: "psraw xmm10, 8",
			n: &NodeImpl{
				Instruction: PSRAW,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    8,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x71, 0xe2, 0x8},
		},

		{
			name: "psrlw xmm10, 1",
			n: &NodeImpl{
				Instruction: PSRLW,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    1,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x71, 0xd2, 0x1},
		},
		{
			name: "psrlw xmm10, 8",
			n: &NodeImpl{
				Instruction: PSRLW,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    8,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x71, 0xd2, 0x8},
		},
		{
			name: "psllw xmm10, 1",
			n: &NodeImpl{
				Instruction: PSLLW,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    1,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x71, 0xf2, 0x1},
		},
		{
			name: "psllw xmm10, 8",
			n: &NodeImpl{
				Instruction: PSLLW,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    8,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x71, 0xf2, 0x8},
		},
		{
			name: "psrad xmm10, 0x1f",
			n: &NodeImpl{
				Instruction: PSRAD,
				Types:       OperandTypesRegisterToRegister,
				SrcConst:    0x1f,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x72, 0xe2, 0x1f},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl()
			err := a.EncodeConstToRegister(tc.n)
			require.NoError(t, err)

			actual, err := a.Assemble()
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual, hex.EncodeToString(actual))
		})
	}
}
