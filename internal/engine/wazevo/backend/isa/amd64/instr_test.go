package amd64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_lowerAluRmiROp_Uses_AssignUse(t *testing.T) {
	vr0 := regalloc.VReg(0).SetRegType(regalloc.RegTypeInt)
	vr1 := regalloc.VReg(1).SetRegType(regalloc.RegTypeInt)
	tests := []struct {
		name     string
		instr    func(*instruction)
		expected string
	}{
		{
			name:     "reg_reg",
			instr:    func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(vr0), vr1, false) },
			expected: "add %eax, %ecx",
		},
		{
			name: "mem_reg",
			instr: func(i *instruction) {
				_, _, m := newSetupWithMockContext()
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandMem(m.newAmodeImmReg(123, vr0)), vr1, false)
			},
			expected: "add 123(%rax), %ecx",
		},
		{
			name:     "imm_reg",
			instr:    func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(123), vr1, false) },
			expected: "add $123, %eax",
		},
	}
	rs := []regalloc.RealReg{rax, rcx}
	for _, tt := range tests {
		tc := tt
		t.Run(tt.name, func(t *testing.T) {
			regs := &[]regalloc.VReg{}
			instr := &instruction{}
			tc.instr(instr)
			for i, use := range instr.Uses(regs) {
				instr.AssignUse(i, use.SetRealReg(rs[i]))
			}
			require.Equal(t, tc.expected, instr.String())
		})
	}
}
