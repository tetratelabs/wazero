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
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0xf3, 0x44, 0xf, 0x6f, 0xd3},
		},
		{
			name: "MOVDQU",
			n: &NodeImpl{
				Instruction: MOVDQU,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegX3,
			},
			exp: []byte{0xf3, 0x41, 0xf, 0x6f, 0xda},
		},
		{
			name: "MOVDQU",
			n: &NodeImpl{
				Instruction: MOVDQU,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegX15,
			},
			exp: []byte{0xf3, 0x45, 0xf, 0x6f, 0xfa},
		},
		{
			name: "MOVDQA",
			n: &NodeImpl{
				Instruction: MOVDQA,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX3,
				DstReg:      RegX10,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x6f, 0xd3},
		},
		{
			name: "MOVDQA",
			n: &NodeImpl{
				Instruction: MOVDQA,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegX3,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x6f, 0xda},
		},
		{
			name: "MOVDQA",
			n: &NodeImpl{
				Instruction: MOVDQA,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x45, 0xf, 0x6f, 0xfa},
		},
		{
			name: "PACKSSWB",
			n: &NodeImpl{
				Instruction: PACKSSWB,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x45, 0xf, 0x63, 0xfa},
		},
		{
			name: "pmovmskb r15d, xmm10",
			n: &NodeImpl{
				Instruction: PMOVMSKB,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegR15,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xd7, 0xfa},
		},
		{
			name: "movmskps eax, xmm10",
			n: &NodeImpl{
				Instruction: MOVMSKPS,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegAX,
			},
			exp: []byte{0x41, 0xf, 0x50, 0xc2},
		},
		{
			name: "movmskps r13d, xmm1",
			n: &NodeImpl{
				Instruction: MOVMSKPS,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegR13,
			},
			exp: []byte{0x44, 0xf, 0x50, 0xe9},
		},
		{
			name: "movmskpd eax, xmm10",
			n: &NodeImpl{
				Instruction: MOVMSKPD,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX10,
				DstReg:      RegAX,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x50, 0xc2},
		},
		{
			name: "movmskpd r15d, xmm1",
			n: &NodeImpl{
				Instruction: MOVMSKPD,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegR15,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x50, 0xf9},
		},
		{
			name: "pand xmm15, xmm1",
			n: &NodeImpl{
				Instruction: PAND,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xdb, 0xf9},
		},
		{
			name: "por xmm1, xmm15",
			n: &NodeImpl{
				Instruction: POR,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX15,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0xeb, 0xcf},
		},
		{
			name: "pandn xmm13, xmm15",
			n: &NodeImpl{
				Instruction: PANDN,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX15,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xdf, 0xef},
		},
		{
			name: "psrad xmm13, xmm15",
			n: &NodeImpl{
				Instruction: PSRAD,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX15,
				DstReg:      RegX13,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xe2, 0xef},
		},
		{
			name: "psraw xmm1, xmm1",
			n: &NodeImpl{
				Instruction: PSRAW,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0xf, 0xe1, 0xc9},
		},
		{
			name: "psrlq xmm14, xmm14",
			n: &NodeImpl{
				Instruction: PSRLQ,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX14,
				DstReg:      RegX14,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xd3, 0xf6},
		},
		{
			name: "psrld xmm3, xmm3",
			n: &NodeImpl{
				Instruction: PSRLD,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX3,
				DstReg:      RegX3,
			},
			exp: []byte{0x66, 0xf, 0xd2, 0xdb},
		},
		{
			name: "psrlw xmm15, xmm1",
			n: &NodeImpl{
				Instruction: PSRLW,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegX15,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xd1, 0xf9},
		},
		{
			name: "psllw xmm1, xmm15",
			n: &NodeImpl{
				Instruction: PSLLW,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX15,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0xf1, 0xcf},
		},
		{
			name: "punpcklbw xmm1, xmm15",
			n: &NodeImpl{
				Instruction: PUNPCKLBW,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX15,
				DstReg:      RegX1,
			},
			exp: []byte{0x66, 0x41, 0xf, 0x60, 0xcf},
		},
		{
			name: "punpckhbw xmm11, xmm1",
			n: &NodeImpl{
				Instruction: PUNPCKHBW,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegX11,
			},
			exp: []byte{0x66, 0x44, 0xf, 0x68, 0xd9},
		},
		{
			name: "pslld xmm11, xmm1",
			n: &NodeImpl{
				Instruction: PSLLD,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX1,
				DstReg:      RegX11,
			},
			exp: []byte{0x66, 0x44, 0xf, 0xf2, 0xd9},
		},
		{
			name: "psllq xmm11, xmm15",
			n: &NodeImpl{
				Instruction: PSLLQ,
				Types:       OperandTypesRegisterToRegister,
				SrcReg:      RegX15,
				DstReg:      RegX11,
			},
			exp: []byte{0x66, 0x45, 0xf, 0xf3, 0xdf},
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
