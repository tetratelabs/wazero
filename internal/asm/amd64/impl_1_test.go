package amd64

import (
	"errors"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
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

func TestAssemblerImpl_Assemble(t *testing.T) {
	t.Run("callback", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			a := NewAssembler()
			callbacked := false
			a.AddOnGenerateCallBack(func(b []byte) error { callbacked = true; return nil })
			_, err := a.Assemble()
			require.NoError(t, err)
			require.True(t, callbacked)
		})
		t.Run("error", func(t *testing.T) {
			a := NewAssembler()
			a.AddOnGenerateCallBack(func(b []byte) error { return errors.New("some error") })
			_, err := a.Assemble()
			require.EqualError(t, err, "some error")
		})
	})
	t.Run("no reassemble", func(t *testing.T) {
		a := NewAssembler()

		jmp := a.CompileJump(JCC)
		const dummyInstruction = CDQ
		a.CompileStandAlone(dummyInstruction)
		a.CompileStandAlone(dummyInstruction)
		a.CompileStandAlone(dummyInstruction)
		jmp.AssignJumpTarget(a.CompileStandAlone(dummyInstruction))

		actual, err := a.Assemble()
		require.NoError(t, err)

		require.Equal(t, []byte{0x73, 0x3, 0x99, 0x99, 0x99, 0x99}, actual)
	})
	t.Run("re-assemble", func(t *testing.T) {
		a := NewAssembler()
		jmp := a.CompileJump(JCC)
		const dummyInstruction = CDQ
		// Ensure that at least 128 bytes between JCC and the target which results in
		// reassemble as we have to convert the forward jump as long variant.
		for i := 0; i < 128; i++ {
			a.CompileStandAlone(dummyInstruction)
		}
		jmp.AssignJumpTarget(a.CompileStandAlone(dummyInstruction))

		a.InitializeNodesForEncoding()

		// For the first encoding, we must be forced to reassemble.
		err := a.Encode()
		require.NoError(t, err)
		require.True(t, a.forceReAssemble)
	})
}

func TestNodeImpl_String(t *testing.T) {
	tests := []struct {
		in  *nodeImpl
		exp string
	}{
		{
			in:  &nodeImpl{instruction: NOP},
			exp: "NOP",
		},
		{
			in:  &nodeImpl{instruction: SETCC, types: operandTypesNoneToRegister, dstReg: RegAX},
			exp: "SETCC AX",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: RegAX, dstConst: 100},
			exp: "JMP [AX + 0x64]",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: RegAX, dstConst: 100, dstMemScale: 8, dstMemIndex: RegR11},
			exp: "JMP [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &nodeImpl{instruction: JMP, types: operandTypesNoneToBranch, jumpTarget: &nodeImpl{instruction: JMP, types: operandTypesNoneToMemory, dstReg: RegAX, dstConst: 100}},
			exp: "JMP {JMP [AX + 0x64]}",
		},
		{
			in:  &nodeImpl{instruction: IDIVQ, types: operandTypesRegisterToNone, srcReg: RegDX},
			exp: "IDIVQ DX",
		},
		{
			in:  &nodeImpl{instruction: ADDL, types: operandTypesRegisterToRegister, srcReg: RegDX, dstReg: RegR14},
			exp: "ADDL DX, R14",
		},
		{
			in: &nodeImpl{instruction: MOVQ, types: operandTypesRegisterToMemory,
				srcReg: RegDX, dstReg: RegR14, dstConst: 100},
			exp: "MOVQ DX, [R14 + 0x64]",
		},
		{
			in: &nodeImpl{instruction: MOVQ, types: operandTypesRegisterToMemory,
				srcReg: RegDX, dstReg: RegR14, dstConst: 100, dstMemIndex: RegCX, dstMemScale: 4},
			exp: "MOVQ DX, [R14 + 0x64 + CX*0x4]",
		},
		{
			in: &nodeImpl{instruction: CMPL, types: operandTypesRegisterToConst,
				srcReg: RegDX, dstConst: 100},
			exp: "CMPL DX, 0x64",
		},
		{
			in: &nodeImpl{instruction: MOVL, types: operandTypesMemoryToRegister,
				srcReg: RegDX, srcConst: 1, dstReg: RegAX},
			exp: "MOVL [DX + 0x1], AX",
		},
		{
			in: &nodeImpl{instruction: MOVL, types: operandTypesMemoryToRegister,
				srcReg: RegDX, srcConst: 1, srcMemIndex: RegR12, srcMemScale: 2,
				dstReg: RegAX},
			exp: "MOVL [DX + 0x1 + R12*0x2], AX",
		},
		{
			in: &nodeImpl{instruction: CMPQ, types: operandTypesMemoryToConst,
				srcReg: RegDX, srcConst: 1, srcMemIndex: RegR12, srcMemScale: 2,
				dstConst: 123},
			exp: "CMPQ [DX + 0x1 + R12*0x2], 0x7b",
		},
		{
			in:  &nodeImpl{instruction: CMPQ, types: operandTypesMemoryToConst, srcReg: RegDX, srcConst: 1, dstConst: 123},
			exp: "CMPQ [DX + 0x1], 0x7b",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToMemory, srcConst: 123, dstReg: RegAX, dstConst: 100, dstMemScale: 8, dstMemIndex: RegR11},
			exp: "MOVQ 0x7b, [AX + 0x64 + R11*0x8]",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToMemory, srcConst: 123, dstReg: RegAX, dstConst: 100},
			exp: "MOVQ 0x7b, [AX + 0x64]",
		},
		{
			in:  &nodeImpl{instruction: MOVQ, types: operandTypesConstToRegister, srcConst: 123, dstReg: RegAX},
			exp: "MOVQ 0x7b, AX",
		},
		{
			in:  &nodeImpl{instruction: LEAQ, types: operandTypesStaticConstToRegister, staticConst: &asm.StaticConst{Raw: []byte{0xff, 0x01, 0x2, 0xff}}, dstReg: RegAX},
			exp: "LEAQ $0xff0102ff, AX",
		},
		{
			in:  &nodeImpl{instruction: CMPQ, types: operandTypesRegisterToStaticConst, staticConst: &asm.StaticConst{Raw: []byte{0xff, 0x01, 0x2, 0xff}}, srcReg: RegAX},
			exp: "CMPQ AX, $0xff0102ff",
		},
	}

	for _, tt := range tests {
		tc := tt
		require.Equal(t, tc.exp, tc.in.String())
	}
}

func TestAssemblerImpl_addNode(t *testing.T) {
	a := NewAssembler()

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
	a := NewAssembler()
	actual := a.newNode(ADDL, operandTypesConstToMemory)
	require.Equal(t, ADDL, actual.instruction)
	require.Equal(t, operandTypeConst, actual.types.src)
	require.Equal(t, operandTypeMemory, actual.types.dst)
	require.Equal(t, actual, a.root)
	require.Equal(t, actual, a.current)
}

func TestAssemblerImpl_encodeNode(t *testing.T) {
	a := NewAssembler()
	err := a.EncodeNode(&nodeImpl{
		instruction: ADDPD,
		types:       operandTypesRegisterToMemory,
	})
	require.EqualError(t, err, "ADDPD is unsupported for from:register,to:memory type: ADDPD nil, [nil + 0x0]")
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
		{num: 10, expected: nopOpcodes[9][:10]},
		{num: 11, expected: nopOpcodes[10][:11]},
		{num: 12, expected: append(nopOpcodes[10][:11], nopOpcodes[0][:1]...)},
		{num: 13, expected: append(nopOpcodes[10][:11], nopOpcodes[1][:2]...)},
		{num: 14, expected: append(nopOpcodes[10][:11], nopOpcodes[2][:3]...)},
		{num: 15, expected: append(nopOpcodes[10][:11], nopOpcodes[3][:4]...)},
		{num: 16, expected: append(nopOpcodes[10][:11], nopOpcodes[4][:5]...)},
		{num: 17, expected: append(nopOpcodes[10][:11], nopOpcodes[5][:6]...)},
		{num: 18, expected: append(nopOpcodes[10][:11], nopOpcodes[6][:7]...)},
		{num: 19, expected: append(nopOpcodes[10][:11], nopOpcodes[7][:8]...)},
		{num: 20, expected: append(nopOpcodes[10][:11], nopOpcodes[8][:9]...)},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(tc.num), func(t *testing.T) {
			a := NewAssembler()
			a.padNOP(tc.num)
			actual := a.buf.Bytes()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestAssemblerImpl_CompileStandAlone(t *testing.T) {
	a := NewAssembler()
	a.CompileStandAlone(RET)
	actualNode := a.current
	require.Equal(t, RET, actualNode.instruction)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeNone, actualNode.types.dst)
}

func TestAssemblerImpl_CompileConstToRegister(t *testing.T) {
	a := NewAssembler()
	a.CompileConstToRegister(MOVQ, 1000, RegAX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, int64(1000), actualNode.srcConst)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, operandTypeConst, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToRegister(t *testing.T) {
	a := NewAssembler()
	a.CompileRegisterToRegister(MOVQ, RegBX, RegAX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryToRegister(t *testing.T) {
	a := NewAssembler()
	a.CompileMemoryToRegister(MOVQ, RegBX, 100, RegAX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, int64(100), actualNode.srcConst)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToMemory(t *testing.T) {
	a := NewAssembler()
	a.CompileRegisterToMemory(MOVQ, RegBX, RegAX, 100)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, int64(100), actualNode.dstConst)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJump(t *testing.T) {
	a := NewAssembler()
	a.CompileJump(JMP)
	actualNode := a.current
	require.Equal(t, JMP, actualNode.instruction)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeBranch, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJumpToRegister(t *testing.T) {
	a := NewAssembler()
	a.CompileJumpToRegister(JNE, RegAX)
	actualNode := a.current
	require.Equal(t, JNE, actualNode.instruction)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileJumpToMemory(t *testing.T) {
	a := NewAssembler()
	a.CompileJumpToMemory(JNE, RegAX, 100)
	actualNode := a.current
	require.Equal(t, JNE, actualNode.instruction)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, int64(100), actualNode.dstConst)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileReadInstructionAddress(t *testing.T) {
	a := NewAssembler()
	a.CompileReadInstructionAddress(RegR10, RET)
	actualNode := a.current
	require.Equal(t, LEAQ, actualNode.instruction)
	require.Equal(t, RegR10, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
	require.Equal(t, RET, actualNode.readInstructionAddressBeforeTargetInstruction)
}

func TestAssemblerImpl_CompileRegisterToRegisterWithArg(t *testing.T) {
	a := NewAssembler()
	a.CompileRegisterToRegisterWithArg(MOVQ, RegBX, RegAX, 123)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, byte(123), actualNode.arg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryWithIndexToRegister(t *testing.T) {
	a := NewAssembler()
	a.CompileMemoryWithIndexToRegister(MOVQ, RegBX, 100, RegR10, 8, RegAX)
	actualNode := a.current
	require.Equal(t, MOVQ, actualNode.instruction)
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, int64(100), actualNode.srcConst)
	require.Equal(t, RegR10, actualNode.srcMemIndex)
	require.Equal(t, byte(8), actualNode.srcMemScale)
	require.Equal(t, RegAX, actualNode.dstReg)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToConst(t *testing.T) {
	a := NewAssembler()
	a.CompileRegisterToConst(MOVQ, RegBX, 123)
	actualNode := a.current
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, int64(123), actualNode.dstConst)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeConst, actualNode.types.dst)
}

func TestAssemblerImpl_CompileRegisterToNone(t *testing.T) {
	a := NewAssembler()
	a.CompileRegisterToNone(MOVQ, RegBX)
	actualNode := a.current
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, operandTypeRegister, actualNode.types.src)
	require.Equal(t, operandTypeNone, actualNode.types.dst)
}

func TestAssemblerImpl_CompileNoneToRegister(t *testing.T) {
	a := NewAssembler()
	a.CompileNoneToRegister(MOVQ, RegBX)
	actualNode := a.current
	require.Equal(t, RegBX, actualNode.dstReg)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeRegister, actualNode.types.dst)
}

func TestAssemblerImpl_CompileNoneToMemory(t *testing.T) {
	a := NewAssembler()
	a.CompileNoneToMemory(MOVQ, RegBX, 1234)
	actualNode := a.current
	require.Equal(t, RegBX, actualNode.dstReg)
	require.Equal(t, int64(1234), actualNode.dstConst)
	require.Equal(t, operandTypeNone, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileConstToMemory(t *testing.T) {
	a := NewAssembler()
	a.CompileConstToMemory(MOVQ, -9999, RegBX, 1234)
	actualNode := a.current
	require.Equal(t, RegBX, actualNode.dstReg)
	require.Equal(t, int64(-9999), actualNode.srcConst)
	require.Equal(t, int64(1234), actualNode.dstConst)
	require.Equal(t, operandTypeConst, actualNode.types.src)
	require.Equal(t, operandTypeMemory, actualNode.types.dst)
}

func TestAssemblerImpl_CompileMemoryToConst(t *testing.T) {
	a := NewAssembler()
	a.CompileMemoryToConst(MOVQ, RegBX, 1234, -9999)
	actualNode := a.current
	require.Equal(t, RegBX, actualNode.srcReg)
	require.Equal(t, int64(1234), actualNode.srcConst)
	require.Equal(t, int64(-9999), actualNode.dstConst)
	require.Equal(t, operandTypeMemory, actualNode.types.src)
	require.Equal(t, operandTypeConst, actualNode.types.dst)
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
		{inst: REPMOVSQ, exp: []byte{0xf3, RexPrefixW, 0xa5}},
		{inst: STD, exp: []byte{0xfd}},
		{inst: CLD, exp: []byte{0xfc}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(InstructionName(tc.inst), func(t *testing.T) {
			a := NewAssembler()
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

func TestAssemblerImpl_EncodeMemoryToRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *nodeImpl
		exp  []byte
	}{
		{name: "MOVDQU", n: &nodeImpl{instruction: MOVDQU, srcReg: RegAX, dstReg: RegX3, srcConst: 10}, exp: []byte{0xf3, 0xf, 0x6f, 0x58, 0xa}},
		{name: "MOVDQU/2", n: &nodeImpl{instruction: MOVDQU, srcReg: RegR13, dstReg: RegX3, srcConst: 10}, exp: []byte{0xf3, 0x41, 0xf, 0x6f, 0x5d, 0xa}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0x20, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0x20, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0x20, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0x20, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x0, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x0, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x28, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x28, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x28, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x28, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x30, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x30, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x30, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x30, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0x20, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0x20, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0x20, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0x20, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x0, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x0, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x28, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x28, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x28, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x28, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x30, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x30, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x30, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x30, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0x60, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0x60, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0x60, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0x60, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x40, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x40, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x40, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x40, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x68, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x68, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x68, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x68, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x70, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x70, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x70, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x70, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0x60, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0x60, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0x60, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0x60, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x40, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x40, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x40, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x40, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x68, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x68, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x68, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x68, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x70, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x70, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x70, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x70, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0xa0, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0xa0, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0xa0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0xa0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x80, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0x80, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x80, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0x80, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xa8, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xa8, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xa8, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xa8, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xb0, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xb0, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xb0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xb0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0xa0, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0xa0, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0xa0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0xa0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x80, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0x80, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x80, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0x80, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xa8, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xa8, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xa8, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xa8, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xb0, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xb0, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xb0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xb0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0xe0, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x4, 0xe0, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0xe0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x44, 0xe0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x20, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xc0, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xc0, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xc0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xc0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xe8, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xe8, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xe8, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xe8, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xf0, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x4, 0xf0, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xf0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x44, 0xf0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x20, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0xe0, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x2c, 0xe0, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0xe0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0x6c, 0xe0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x20, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xc0, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xc0, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xc0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xc0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xe8, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xe8, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xe8, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xe8, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xf0, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x2c, 0xf0, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xf0, 0x1, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0x6c, 0xf0, 0x1, 0x1}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRB/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRB, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x20, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0x20, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0x20, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0x20, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0x20, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x0, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x0, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x28, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x28, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x28, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x28, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x30, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x30, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x30, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x30, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0x20, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0x20, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0x20, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0x20, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x0, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x0, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x28, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x28, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x28, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x28, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x30, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x30, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x30, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x30, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0x60, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0x60, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0x60, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0x60, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x40, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x40, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x40, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x40, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x68, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x68, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x68, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x68, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x70, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x70, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x70, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x70, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0x60, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0x60, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0x60, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0x60, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x40, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x40, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x40, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x40, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x68, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x68, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x68, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x68, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x70, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x70, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x70, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x70, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0xa0, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0xa0, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0xa0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0xa0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x80, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0x80, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x80, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0x80, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xa8, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xa8, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xa8, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xa8, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xb0, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xb0, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xb0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xb0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0xa0, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0xa0, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0xa0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0xa0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x80, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0x80, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x80, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0x80, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xa8, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xa8, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xa8, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xa8, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xb0, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xb0, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xb0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xb0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0xe0, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x4, 0xe0, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0xe0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x44, 0xe0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0xc4, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xc0, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xc0, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xc0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xc0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xe8, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xe8, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xe8, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xe8, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xf0, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x4, 0xf0, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xf0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x44, 0xf0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0xc4, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0xe0, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x2c, 0xe0, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0xe0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0x6c, 0xe0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0xc4, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xc0, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xc0, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xc0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xc0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xe8, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xe8, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xe8, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xe8, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xf0, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x2c, 0xf0, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xf0, 0x1, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0x6c, 0xf0, 0x1, 0x1}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRW/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRW, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0xc4, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0x20, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0x20, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0x20, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0x20, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x0, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x0, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x28, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x28, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x28, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x28, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x30, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x30, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x30, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x30, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0x20, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0x20, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0x20, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0x20, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x0, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x0, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x28, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x28, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x28, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x28, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x30, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x30, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x30, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x30, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0x60, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0x60, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0x60, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0x60, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x40, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x40, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x40, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x40, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x68, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x68, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x68, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x68, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x70, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x70, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x70, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x70, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0x60, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0x60, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0x60, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0x60, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x40, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x40, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x40, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x40, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x68, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x68, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x68, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x68, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x70, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x70, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x70, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x70, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0xa0, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0xa0, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0xa0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0xa0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x80, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0x80, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x80, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0x80, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xa8, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xa8, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xa8, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xa8, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xb0, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xb0, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xb0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xb0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0xa0, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0xa0, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0xa0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0xa0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x80, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0x80, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x80, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0x80, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xa8, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xa8, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xa8, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xa8, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xb0, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xb0, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xb0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xb0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0xe0, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x4, 0xe0, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0xe0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x44, 0xe0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x42, 0xf, 0x3a, 0x22, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xc0, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xc0, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xc0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xc0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xe8, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xe8, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xe8, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xe8, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xf0, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x4, 0xf0, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xf0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x44, 0xf0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0xf, 0x3a, 0x22, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0xe0, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x2c, 0xe0, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0xe0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0x6c, 0xe0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x46, 0xf, 0x3a, 0x22, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xc0, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xc0, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xc0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xc0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xe8, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xe8, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xe8, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xe8, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xf0, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x2c, 0xf0, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xf0, 0x1, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0x6c, 0xf0, 0x1, 0x1}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRD/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRD, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x44, 0xf, 0x3a, 0x22, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0x20, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0x20, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0x20, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0x20, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x0, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x0, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x28, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x28, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x28, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x28, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x30, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x30, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x30, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x30, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0x20, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0x20, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0x20, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0x20, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0x20, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x0, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x0, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x28, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x28, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x28, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x28, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x28, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x30, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x30, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x30, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x30, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=1/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 1, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x30, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0x60, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0x60, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0x60, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0x60, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x40, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x40, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x40, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x40, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x68, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x68, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x68, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x68, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x70, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x70, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x70, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x70, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0x60, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0x60, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0x60, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0x60, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0x60, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x40, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x40, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x40, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x40, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x40, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x68, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x68, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x68, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x68, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x68, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x70, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x70, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x70, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x70, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=2/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 2, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x70, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0xa0, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0xa0, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0xa0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0xa0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x80, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0x80, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x80, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0x80, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xa8, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xa8, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xa8, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xa8, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xb0, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xb0, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xb0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xb0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0xa0, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0xa0, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0xa0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0xa0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0xa0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x80, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0x80, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x80, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0x80, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xa8, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xa8, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xa8, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xa8, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xa8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xb0, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xb0, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xb0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xb0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=4/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 4, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xb0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0xe0, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x4, 0xe0, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0xe0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x44, 0xe0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x4a, 0xf, 0x3a, 0x22, 0x84, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xc0, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xc0, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xc0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xc0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xe8, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xe8, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xe8, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xe8, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xf0, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=0/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x4, 0xf0, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xf0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=1/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x44, 0xf0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 0}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=2147483647/dst=X0/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX0, arg: 1}, exp: []byte{0x66, 0x48, 0xf, 0x3a, 0x22, 0x84, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0xe0, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x2c, 0xe0, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0xe0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0x6c, 0xe0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=R12/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegR12, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4e, 0xf, 0x3a, 0x22, 0xac, 0xe0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xc0, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xc0, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xc0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xc0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=AX/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegAX, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xc0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xe8, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xe8, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xe8, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xe8, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=BP/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegBP, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xe8, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xf0, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=0/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 0, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x2c, 0xf0, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xf0, 0x1, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=1/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 1, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0x6c, 0xf0, 0x1, 0x1}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=0", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 0}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x0}},
		{name: "PINSRQ/src=AX/index=SI/scale=8/offset=2147483647/dst=X13/arg=1", n: &nodeImpl{instruction: PINSRQ, srcReg: RegAX, srcMemIndex: RegSI, srcMemScale: 8, srcConst: 2147483647, dstReg: RegX13, arg: 1}, exp: []byte{0x66, 0x4c, 0xf, 0x3a, 0x22, 0xac, 0xf0, 0xff, 0xff, 0xff, 0x7f, 0x1}},
		{name: "ADDL/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x3, 0x8}},
		{name: "ADDL/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x3, 0xc, 0xb0}},
		{name: "ADDL/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x3, 0x48, 0x1}},
		{name: "ADDL/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x3, 0x4c, 0xb0, 0x1}},
		{name: "ADDL/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x3, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDL/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x3, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDL/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0x3, 0xa}},
		{name: "ADDL/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x3, 0xc, 0xb2}},
		{name: "ADDL/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0x3, 0x4a, 0x1}},
		{name: "ADDL/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x3, 0x4c, 0xb2, 0x1}},
		{name: "ADDL/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0x3, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDL/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDL, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x3, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDQ/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0x3, 0x8}},
		{name: "ADDQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x3, 0xc, 0xb0}},
		{name: "ADDQ/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0x3, 0x48, 0x1}},
		{name: "ADDQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x3, 0x4c, 0xb0, 0x1}},
		{name: "ADDQ/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0x3, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x3, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDQ/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0x3, 0xa}},
		{name: "ADDQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x3, 0xc, 0xb2}},
		{name: "ADDQ/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0x3, 0x4a, 0x1}},
		{name: "ADDQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x3, 0x4c, 0xb2, 0x1}},
		{name: "ADDQ/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0x3, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "ADDQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: ADDQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x3, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPL/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x39, 0x8}},
		{name: "CMPL/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x39, 0xc, 0xb0}},
		{name: "CMPL/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x39, 0x48, 0x1}},
		{name: "CMPL/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x39, 0x4c, 0xb0, 0x1}},
		{name: "CMPL/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x39, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPL/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x39, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPL/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0x39, 0xa}},
		{name: "CMPL/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x39, 0xc, 0xb2}},
		{name: "CMPL/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0x39, 0x4a, 0x1}},
		{name: "CMPL/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x39, 0x4c, 0xb2, 0x1}},
		{name: "CMPL/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0x39, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPL/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPL, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x39, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPQ/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0x39, 0x8}},
		{name: "CMPQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x39, 0xc, 0xb0}},
		{name: "CMPQ/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0x39, 0x48, 0x1}},
		{name: "CMPQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x39, 0x4c, 0xb0, 0x1}},
		{name: "CMPQ/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0x39, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x39, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPQ/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0x39, 0xa}},
		{name: "CMPQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x39, 0xc, 0xb2}},
		{name: "CMPQ/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0x39, 0x4a, 0x1}},
		{name: "CMPQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x39, 0x4c, 0xb2, 0x1}},
		{name: "CMPQ/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0x39, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "CMPQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: CMPQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x39, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "LEAQ/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0x8d, 0x8}},
		{name: "LEAQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x8d, 0xc, 0xb0}},
		{name: "LEAQ/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0x8d, 0x48, 0x1}},
		{name: "LEAQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x8d, 0x4c, 0xb0, 0x1}},
		{name: "LEAQ/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0x8d, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "LEAQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x8d, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "LEAQ/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0x8d, 0xa}},
		{name: "LEAQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x8d, 0xc, 0xb2}},
		{name: "LEAQ/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0x8d, 0x4a, 0x1}},
		{name: "LEAQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x8d, 0x4c, 0xb2, 0x1}},
		{name: "LEAQ/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0x8d, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "LEAQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: LEAQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x8d, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLSX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0xf, 0xbe, 0x8}},
		{name: "MOVBLSX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xbe, 0xc, 0xb0}},
		{name: "MOVBLSX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0xf, 0xbe, 0x48, 0x1}},
		{name: "MOVBLSX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xbe, 0x4c, 0xb0, 0x1}},
		{name: "MOVBLSX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0xf, 0xbe, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLSX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xbe, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLSX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xbe, 0xa}},
		{name: "MOVBLSX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xbe, 0xc, 0xb2}},
		{name: "MOVBLSX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xbe, 0x4a, 0x1}},
		{name: "MOVBLSX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xbe, 0x4c, 0xb2, 0x1}},
		{name: "MOVBLSX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xbe, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLSX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLSX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xbe, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLZX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0xf, 0xb6, 0x8}},
		{name: "MOVBLZX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xb6, 0xc, 0xb0}},
		{name: "MOVBLZX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0xf, 0xb6, 0x48, 0x1}},
		{name: "MOVBLZX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xb6, 0x4c, 0xb0, 0x1}},
		{name: "MOVBLZX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0xf, 0xb6, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLZX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xb6, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLZX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xb6, 0xa}},
		{name: "MOVBLZX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xb6, 0xc, 0xb2}},
		{name: "MOVBLZX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xb6, 0x4a, 0x1}},
		{name: "MOVBLZX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xb6, 0x4c, 0xb2, 0x1}},
		{name: "MOVBLZX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xb6, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBLZX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBLZX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xb6, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQSX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xbe, 0x8}},
		{name: "MOVBQSX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xbe, 0xc, 0xb0}},
		{name: "MOVBQSX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xbe, 0x48, 0x1}},
		{name: "MOVBQSX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xbe, 0x4c, 0xb0, 0x1}},
		{name: "MOVBQSX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xbe, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQSX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xbe, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQSX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xbe, 0xa}},
		{name: "MOVBQSX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xbe, 0xc, 0xb2}},
		{name: "MOVBQSX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xbe, 0x4a, 0x1}},
		{name: "MOVBQSX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xbe, 0x4c, 0xb2, 0x1}},
		{name: "MOVBQSX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xbe, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQSX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQSX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xbe, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQZX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xb6, 0x8}},
		{name: "MOVBQZX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xb6, 0xc, 0xb0}},
		{name: "MOVBQZX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xb6, 0x48, 0x1}},
		{name: "MOVBQZX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xb6, 0x4c, 0xb0, 0x1}},
		{name: "MOVBQZX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xb6, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQZX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xb6, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQZX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xb6, 0xa}},
		{name: "MOVBQZX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xb6, 0xc, 0xb2}},
		{name: "MOVBQZX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xb6, 0x4a, 0x1}},
		{name: "MOVBQZX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xb6, 0x4c, 0xb2, 0x1}},
		{name: "MOVBQZX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xb6, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVBQZX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVBQZX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xb6, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQSX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0x63, 0x8}},
		{name: "MOVLQSX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x63, 0xc, 0xb0}},
		{name: "MOVLQSX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0x63, 0x48, 0x1}},
		{name: "MOVLQSX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x63, 0x4c, 0xb0, 0x1}},
		{name: "MOVLQSX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0x63, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQSX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x63, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQSX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0x63, 0xa}},
		{name: "MOVLQSX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x63, 0xc, 0xb2}},
		{name: "MOVLQSX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0x63, 0x4a, 0x1}},
		{name: "MOVLQSX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x63, 0x4c, 0xb2, 0x1}},
		{name: "MOVLQSX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0x63, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQSX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQSX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x63, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQZX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x8b, 0x8}},
		{name: "MOVLQZX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x8b, 0xc, 0xb0}},
		{name: "MOVLQZX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x8b, 0x48, 0x1}},
		{name: "MOVLQZX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x8b, 0x4c, 0xb0, 0x1}},
		{name: "MOVLQZX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x8b, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQZX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x8b, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQZX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0x8b, 0xa}},
		{name: "MOVLQZX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x8b, 0xc, 0xb2}},
		{name: "MOVLQZX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0x8b, 0x4a, 0x1}},
		{name: "MOVLQZX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x8b, 0x4c, 0xb2, 0x1}},
		{name: "MOVLQZX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0x8b, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVLQZX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVLQZX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x8b, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x8b, 0x8}},
		{name: "MOVL/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x8b, 0xc, 0xb0}},
		{name: "MOVL/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x8b, 0x48, 0x1}},
		{name: "MOVL/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x8b, 0x4c, 0xb0, 0x1}},
		{name: "MOVL/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x8b, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0x8b, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x6e, 0x18}},
		{name: "MOVL/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x6e, 0x1c, 0xb0}},
		{name: "MOVL/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x6e, 0x58, 0x1}},
		{name: "MOVL/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x6e, 0x5c, 0xb0, 0x1}},
		{name: "MOVL/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x6e, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x6e, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0x8b, 0xa}},
		{name: "MOVL/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x8b, 0xc, 0xb2}},
		{name: "MOVL/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0x8b, 0x4a, 0x1}},
		{name: "MOVL/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x8b, 0x4c, 0xb2, 0x1}},
		{name: "MOVL/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0x8b, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0x8b, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x6e, 0x1a}},
		{name: "MOVL/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x6e, 0x1c, 0xb2}},
		{name: "MOVL/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x6e, 0x5a, 0x1}},
		{name: "MOVL/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x6e, 0x5c, 0xb2, 0x1}},
		{name: "MOVL/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x6e, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVL/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVL, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x6e, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0x8b, 0x8}},
		{name: "MOVQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x8b, 0xc, 0xb0}},
		{name: "MOVQ/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0x8b, 0x48, 0x1}},
		{name: "MOVQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x8b, 0x4c, 0xb0, 0x1}},
		{name: "MOVQ/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0x8b, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x8b, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf3, 0xf, 0x7e, 0x18}},
		{name: "MOVQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x42, 0xf, 0x7e, 0x1c, 0xb0}},
		{name: "MOVQ/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf3, 0xf, 0x7e, 0x58, 0x1}},
		{name: "MOVQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x42, 0xf, 0x7e, 0x5c, 0xb0, 0x1}},
		{name: "MOVQ/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf3, 0xf, 0x7e, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x42, 0xf, 0x7e, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0x8b, 0xa}},
		{name: "MOVQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x8b, 0xc, 0xb2}},
		{name: "MOVQ/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0x8b, 0x4a, 0x1}},
		{name: "MOVQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x8b, 0x4c, 0xb2, 0x1}},
		{name: "MOVQ/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0x8b, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x8b, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf3, 0x41, 0xf, 0x7e, 0x1a}},
		{name: "MOVQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x43, 0xf, 0x7e, 0x1c, 0xb2}},
		{name: "MOVQ/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf3, 0x41, 0xf, 0x7e, 0x5a, 0x1}},
		{name: "MOVQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x43, 0xf, 0x7e, 0x5c, 0xb2, 0x1}},
		{name: "MOVQ/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf3, 0x41, 0xf, 0x7e, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: MOVQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x43, 0xf, 0x7e, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLSX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0xf, 0xbf, 0x8}},
		{name: "MOVWLSX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xbf, 0xc, 0xb0}},
		{name: "MOVWLSX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0xf, 0xbf, 0x48, 0x1}},
		{name: "MOVWLSX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xbf, 0x4c, 0xb0, 0x1}},
		{name: "MOVWLSX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0xf, 0xbf, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLSX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xbf, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLSX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xbf, 0xa}},
		{name: "MOVWLSX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xbf, 0xc, 0xb2}},
		{name: "MOVWLSX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xbf, 0x4a, 0x1}},
		{name: "MOVWLSX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xbf, 0x4c, 0xb2, 0x1}},
		{name: "MOVWLSX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xbf, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLSX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLSX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xbf, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLZX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0xf, 0xb7, 0x8}},
		{name: "MOVWLZX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xb7, 0xc, 0xb0}},
		{name: "MOVWLZX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0xf, 0xb7, 0x48, 0x1}},
		{name: "MOVWLZX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xb7, 0x4c, 0xb0, 0x1}},
		{name: "MOVWLZX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0xf, 0xb7, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLZX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x42, 0xf, 0xb7, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLZX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xb7, 0xa}},
		{name: "MOVWLZX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xb7, 0xc, 0xb2}},
		{name: "MOVWLZX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xb7, 0x4a, 0x1}},
		{name: "MOVWLZX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xb7, 0x4c, 0xb2, 0x1}},
		{name: "MOVWLZX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x41, 0xf, 0xb7, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWLZX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWLZX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x43, 0xf, 0xb7, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQSX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xbf, 0x8}},
		{name: "MOVWQSX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xbf, 0xc, 0xb0}},
		{name: "MOVWQSX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xbf, 0x48, 0x1}},
		{name: "MOVWQSX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xbf, 0x4c, 0xb0, 0x1}},
		{name: "MOVWQSX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xbf, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQSX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xbf, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQSX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xbf, 0xa}},
		{name: "MOVWQSX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xbf, 0xc, 0xb2}},
		{name: "MOVWQSX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xbf, 0x4a, 0x1}},
		{name: "MOVWQSX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xbf, 0x4c, 0xb2, 0x1}},
		{name: "MOVWQSX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xbf, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQSX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQSX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xbf, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQZX/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xb7, 0x8}},
		{name: "MOVWQZX/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xb7, 0xc, 0xb0}},
		{name: "MOVWQZX/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xb7, 0x48, 0x1}},
		{name: "MOVWQZX/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xb7, 0x4c, 0xb0, 0x1}},
		{name: "MOVWQZX/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0xf, 0xb7, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQZX/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0xf, 0xb7, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQZX/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xb7, 0xa}},
		{name: "MOVWQZX/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xb7, 0xc, 0xb2}},
		{name: "MOVWQZX/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xb7, 0x4a, 0x1}},
		{name: "MOVWQZX/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xb7, 0x4c, 0xb2, 0x1}},
		{name: "MOVWQZX/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0xf, 0xb7, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "MOVWQZX/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: MOVWQZX, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0xf, 0xb7, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBQ/baseReg=AX/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x48, 0x2b, 0x8}},
		{name: "SUBQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x2b, 0xc, 0xb0}},
		{name: "SUBQ/baseReg=AX/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x48, 0x2b, 0x48, 0x1}},
		{name: "SUBQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x2b, 0x4c, 0xb0, 0x1}},
		{name: "SUBQ/baseReg=AX/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x48, 0x2b, 0x88, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4a, 0x2b, 0x8c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBQ/baseReg=R10/offset=0x0/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegCX}, exp: []byte{0x49, 0x2b, 0xa}},
		{name: "SUBQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x2b, 0xc, 0xb2}},
		{name: "SUBQ/baseReg=R10/offset=0x1/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegCX}, exp: []byte{0x49, 0x2b, 0x4a, 0x1}},
		{name: "SUBQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x2b, 0x4c, 0xb2, 0x1}},
		{name: "SUBQ/baseReg=R10/offset=0x7fffffff/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegCX}, exp: []byte{0x49, 0x2b, 0x8a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=CX", n: &nodeImpl{instruction: SUBQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegCX}, exp: []byte{0x4b, 0x2b, 0x8c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSD/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf2, 0xf, 0x5c, 0x18}},
		{name: "SUBSD/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf2, 0x42, 0xf, 0x5c, 0x1c, 0xb0}},
		{name: "SUBSD/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf2, 0xf, 0x5c, 0x58, 0x1}},
		{name: "SUBSD/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf2, 0x42, 0xf, 0x5c, 0x5c, 0xb0, 0x1}},
		{name: "SUBSD/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf2, 0xf, 0x5c, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSD/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf2, 0x42, 0xf, 0x5c, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSD/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf2, 0x41, 0xf, 0x5c, 0x1a}},
		{name: "SUBSD/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf2, 0x43, 0xf, 0x5c, 0x1c, 0xb2}},
		{name: "SUBSD/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf2, 0x41, 0xf, 0x5c, 0x5a, 0x1}},
		{name: "SUBSD/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf2, 0x43, 0xf, 0x5c, 0x5c, 0xb2, 0x1}},
		{name: "SUBSD/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf2, 0x41, 0xf, 0x5c, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSD/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSD, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf2, 0x43, 0xf, 0x5c, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSS/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf3, 0xf, 0x5c, 0x18}},
		{name: "SUBSS/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x42, 0xf, 0x5c, 0x1c, 0xb0}},
		{name: "SUBSS/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf3, 0xf, 0x5c, 0x58, 0x1}},
		{name: "SUBSS/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x42, 0xf, 0x5c, 0x5c, 0xb0, 0x1}},
		{name: "SUBSS/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf3, 0xf, 0x5c, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSS/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x42, 0xf, 0x5c, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSS/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf3, 0x41, 0xf, 0x5c, 0x1a}},
		{name: "SUBSS/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x43, 0xf, 0x5c, 0x1c, 0xb2}},
		{name: "SUBSS/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf3, 0x41, 0xf, 0x5c, 0x5a, 0x1}},
		{name: "SUBSS/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x43, 0xf, 0x5c, 0x5c, 0xb2, 0x1}},
		{name: "SUBSS/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf3, 0x41, 0xf, 0x5c, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "SUBSS/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: SUBSS, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0xf3, 0x43, 0xf, 0x5c, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISD/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x2e, 0x18}},
		{name: "UCOMISD/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x2e, 0x1c, 0xb0}},
		{name: "UCOMISD/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x2e, 0x58, 0x1}},
		{name: "UCOMISD/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x2e, 0x5c, 0xb0, 0x1}},
		{name: "UCOMISD/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x2e, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISD/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x2e, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISD/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x2e, 0x1a}},
		{name: "UCOMISD/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x2e, 0x1c, 0xb2}},
		{name: "UCOMISD/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x2e, 0x5a, 0x1}},
		{name: "UCOMISD/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x2e, 0x5c, 0xb2, 0x1}},
		{name: "UCOMISD/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x2e, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISD/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISD, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x2e, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISS/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0xf, 0x2e, 0x18}},
		{name: "UCOMISS/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x42, 0xf, 0x2e, 0x1c, 0xb0}},
		{name: "UCOMISS/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0xf, 0x2e, 0x58, 0x1}},
		{name: "UCOMISS/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x42, 0xf, 0x2e, 0x5c, 0xb0, 0x1}},
		{name: "UCOMISS/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0xf, 0x2e, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISS/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x42, 0xf, 0x2e, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISS/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x41, 0xf, 0x2e, 0x1a}},
		{name: "UCOMISS/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x43, 0xf, 0x2e, 0x1c, 0xb2}},
		{name: "UCOMISS/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x41, 0xf, 0x2e, 0x5a, 0x1}},
		{name: "UCOMISS/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x43, 0xf, 0x2e, 0x5c, 0xb2, 0x1}},
		{name: "UCOMISS/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x41, 0xf, 0x2e, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "UCOMISS/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: UCOMISS, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x43, 0xf, 0x2e, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXBW/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x20, 0x18}},
		{name: "PMOVSXBW/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x20, 0x1c, 0xb0}},
		{name: "PMOVSXBW/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x20, 0x58, 0x1}},
		{name: "PMOVSXBW/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x20, 0x5c, 0xb0, 0x1}},
		{name: "PMOVSXBW/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x20, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXBW/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x20, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXBW/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x20, 0x1a}},
		{name: "PMOVSXBW/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x20, 0x1c, 0xb2}},
		{name: "PMOVSXBW/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x20, 0x5a, 0x1}},
		{name: "PMOVSXBW/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x20, 0x5c, 0xb2, 0x1}},
		{name: "PMOVSXBW/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x20, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXBW/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXBW, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x20, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXWD/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x23, 0x18}},
		{name: "PMOVSXWD/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x23, 0x1c, 0xb0}},
		{name: "PMOVSXWD/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x23, 0x58, 0x1}},
		{name: "PMOVSXWD/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x23, 0x5c, 0xb0, 0x1}},
		{name: "PMOVSXWD/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x23, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXWD/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x23, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXWD/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x23, 0x1a}},
		{name: "PMOVSXWD/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x23, 0x1c, 0xb2}},
		{name: "PMOVSXWD/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x23, 0x5a, 0x1}},
		{name: "PMOVSXWD/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x23, 0x5c, 0xb2, 0x1}},
		{name: "PMOVSXWD/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x23, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXWD/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXWD, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x23, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXDQ/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x25, 0x18}},
		{name: "PMOVSXDQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x25, 0x1c, 0xb0}},
		{name: "PMOVSXDQ/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x25, 0x58, 0x1}},
		{name: "PMOVSXDQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x25, 0x5c, 0xb0, 0x1}},
		{name: "PMOVSXDQ/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x25, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXDQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x25, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXDQ/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x25, 0x1a}},
		{name: "PMOVSXDQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x25, 0x1c, 0xb2}},
		{name: "PMOVSXDQ/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x25, 0x5a, 0x1}},
		{name: "PMOVSXDQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x25, 0x5c, 0xb2, 0x1}},
		{name: "PMOVSXDQ/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x25, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVSXDQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVSXDQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x25, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXBW/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x30, 0x18}},
		{name: "PMOVZXBW/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x30, 0x1c, 0xb0}},
		{name: "PMOVZXBW/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x30, 0x58, 0x1}},
		{name: "PMOVZXBW/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x30, 0x5c, 0xb0, 0x1}},
		{name: "PMOVZXBW/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x30, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXBW/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x30, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXBW/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x30, 0x1a}},
		{name: "PMOVZXBW/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x30, 0x1c, 0xb2}},
		{name: "PMOVZXBW/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x30, 0x5a, 0x1}},
		{name: "PMOVZXBW/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x30, 0x5c, 0xb2, 0x1}},
		{name: "PMOVZXBW/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x30, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXBW/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXBW, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x30, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXWD/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x33, 0x18}},
		{name: "PMOVZXWD/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x33, 0x1c, 0xb0}},
		{name: "PMOVZXWD/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x33, 0x58, 0x1}},
		{name: "PMOVZXWD/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x33, 0x5c, 0xb0, 0x1}},
		{name: "PMOVZXWD/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x33, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXWD/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x33, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXWD/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x33, 0x1a}},
		{name: "PMOVZXWD/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x33, 0x1c, 0xb2}},
		{name: "PMOVZXWD/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x33, 0x5a, 0x1}},
		{name: "PMOVZXWD/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x33, 0x5c, 0xb2, 0x1}},
		{name: "PMOVZXWD/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x33, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXWD/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXWD, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x33, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXDQ/baseReg=AX/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegAX, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x35, 0x18}},
		{name: "PMOVZXDQ/baseReg=AX/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegAX, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x35, 0x1c, 0xb0}},
		{name: "PMOVZXDQ/baseReg=AX/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegAX, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x35, 0x58, 0x1}},
		{name: "PMOVZXDQ/baseReg=AX/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegAX, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x35, 0x5c, 0xb0, 0x1}},
		{name: "PMOVZXDQ/baseReg=AX/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegAX, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0xf, 0x38, 0x35, 0x98, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXDQ/baseReg=AX/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegAX, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x42, 0xf, 0x38, 0x35, 0x9c, 0xb0, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXDQ/baseReg=R10/offset=0x0/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegR10, srcConst: 0x0, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x35, 0x1a}},
		{name: "PMOVZXDQ/baseReg=R10/offset=0x0/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegR10, srcConst: 0x0, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x35, 0x1c, 0xb2}},
		{name: "PMOVZXDQ/baseReg=R10/offset=0x1/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegR10, srcConst: 0x1, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x35, 0x5a, 0x1}},
		{name: "PMOVZXDQ/baseReg=R10/offset=0x1/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegR10, srcConst: 0x1, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x35, 0x5c, 0xb2, 0x1}},
		{name: "PMOVZXDQ/baseReg=R10/offset=0x7fffffff/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegR10, srcConst: 0x7fffffff, dstReg: RegX3}, exp: []byte{0x66, 0x41, 0xf, 0x38, 0x35, 0x9a, 0xff, 0xff, 0xff, 0x7f}},
		{name: "PMOVZXDQ/baseReg=R10/offset=0x7fffffff/index=R14/scale=4/dstReg=X3", n: &nodeImpl{instruction: PMOVZXDQ, srcReg: RegR10, srcConst: 0x7fffffff, srcMemIndex: RegR14, srcMemScale: 4, dstReg: RegX3}, exp: []byte{0x66, 0x43, 0xf, 0x38, 0x35, 0x9c, 0xb2, 0xff, 0xff, 0xff, 0x7f}},
	}

	for _, tc := range tests {
		tc.n.types = operandTypesMemoryToRegister
		a := NewAssembler()
		err := a.encodeMemoryToRegister(tc.n)
		require.NoError(t, err, tc.name)

		actual, err := a.Assemble()
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.exp, actual, tc.name)
	}
}

func TestAssemblerImpl_EncodeConstToRegister(t *testing.T) {
	tests := []struct {
		name string
		n    *nodeImpl
		exp  []byte
	}{
		{
			name: "psraw xmm10, 1",
			n:    &nodeImpl{instruction: PSRAW, types: operandTypesRegisterToRegister, srcConst: 1, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x71, 0xe2, 0x1},
		},
		{
			name: "psraw xmm10, 8",
			n:    &nodeImpl{instruction: PSRAW, types: operandTypesRegisterToRegister, srcConst: 8, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x71, 0xe2, 0x8},
		},
		{
			name: "psrlw xmm10, 1",
			n:    &nodeImpl{instruction: PSRLW, types: operandTypesRegisterToRegister, srcConst: 1, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x71, 0xd2, 0x1},
		},
		{
			name: "psrlw xmm10, 8",
			n:    &nodeImpl{instruction: PSRLW, types: operandTypesRegisterToRegister, srcConst: 8, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x71, 0xd2, 0x8},
		},
		{
			name: "psllw xmm10, 1",
			n:    &nodeImpl{instruction: PSLLW, types: operandTypesRegisterToRegister, srcConst: 1, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x71, 0xf2, 0x1},
		},
		{
			name: "psllw xmm10, 8",
			n:    &nodeImpl{instruction: PSLLW, types: operandTypesRegisterToRegister, srcConst: 8, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x71, 0xf2, 0x8},
		},
		{
			name: "psrad xmm10, 0x1f",
			n:    &nodeImpl{instruction: PSRAD, types: operandTypesRegisterToRegister, srcConst: 0x1f, dstReg: RegX10},
			exp:  []byte{0x66, 0x41, 0xf, 0x72, 0xe2, 0x1f},
		},
	}

	for _, tt := range tests {
		tc := tt
		a := NewAssembler()
		err := a.encodeConstToRegister(tc.n)
		require.NoError(t, err, tc.name)

		actual, err := a.Assemble()
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.exp, actual, tc.name)
	}
}
