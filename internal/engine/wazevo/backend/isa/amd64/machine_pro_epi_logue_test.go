package amd64

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_setupPrologue(t *testing.T) {
	for _, tc := range []struct {
		spillSlotSize int64
		clobberedRegs []regalloc.VReg
		exp           string
		abi           backend.FunctionABI
	}{
		{
			spillSlotSize: 0,
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	ud2
`,
		},
		{
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	sub $16, %rsp
	movdqu %xmm15, (%rsp)
	sub $16, %rsp
	movdqu %xmm1, (%rsp)
	sub $16, %rsp
	movdqu %xmm0, (%rsp)
	pushq %rcx
	pushq %rax
	ud2
`,
			spillSlotSize: 0,
			clobberedRegs: []regalloc.VReg{raxVReg, rcxVReg, xmm0VReg, xmm1VReg, xmm15VReg},
		},
		{
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	sub $16, %rsp
	movdqu %xmm15, (%rsp)
	sub $16, %rsp
	movdqu %xmm1, (%rsp)
	sub $16, %rsp
	movdqu %xmm0, (%rsp)
	pushq %rcx
	pushq %rax
	sub $48, %rsp
	ud2
`,
			spillSlotSize: 48,
			clobberedRegs: []regalloc.VReg{raxVReg, rcxVReg, xmm0VReg, xmm1VReg, xmm15VReg},
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.DisableStackCheck()
			m.spillSlotSize = tc.spillSlotSize
			m.clobberedRegs = tc.clobberedRegs
			m.currentABI = &tc.abi

			root := m.allocateNop()
			m.rootInstr = root
			udf := m.allocateInstr()
			udf.asUD2()
			root.next = udf
			udf.prev = root

			m.setupPrologue()
			require.Equal(t, root, m.rootInstr)
			err := m.Encode(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_postRegAlloc(t *testing.T) {
	for _, tc := range []struct {
		exp           string
		abi           backend.FunctionABI
		clobberedRegs []regalloc.VReg
		spillSlotSize int64
	}{
		{
			exp: `
	movq %rbp, %rsp
	popq %rbp
	ret
`,
			spillSlotSize: 0,
			clobberedRegs: nil,
		},
		{
			exp: `
	popq %rax
	popq %rcx
	movdqu (%rsp), %xmm0
	add $16, %rsp
	movdqu (%rsp), %xmm1
	add $16, %rsp
	movdqu (%rsp), %xmm15
	add $16, %rsp
	movq %rbp, %rsp
	popq %rbp
	ret
`,
			spillSlotSize: 0,
			clobberedRegs: []regalloc.VReg{raxVReg, rcxVReg, xmm0VReg, xmm1VReg, xmm15VReg},
		},
		{
			exp: `
	add $160, %rsp
	popq %rax
	popq %rcx
	movdqu (%rsp), %xmm0
	add $16, %rsp
	movdqu (%rsp), %xmm1
	add $16, %rsp
	movdqu (%rsp), %xmm15
	add $16, %rsp
	movq %rbp, %rsp
	popq %rbp
	ret
`,
			spillSlotSize: 160,
			clobberedRegs: []regalloc.VReg{raxVReg, rcxVReg, xmm0VReg, xmm1VReg, xmm15VReg},
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.spillSlotSize = tc.spillSlotSize
			m.clobberedRegs = tc.clobberedRegs
			m.currentABI = &tc.abi

			root := m.allocateNop()
			m.rootInstr = root
			ret := m.allocateInstr()
			ret.asRet()
			root.next = ret
			ret.prev = root
			m.postRegAlloc()

			require.Equal(t, root, m.rootInstr)
			err := m.Encode(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
