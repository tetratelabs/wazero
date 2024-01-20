package amd64

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_SetupPrologue(t *testing.T) {
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
			clobberedRegs: []regalloc.VReg{
				raxVReg,
				rcxVReg,
				xmm0VReg,
				xmm1VReg,
				xmm15VReg,
			},
		},
		// TODO: add more test cases.
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.DisableStackCheck()
			m.spillSlotSize = tc.spillSlotSize
			m.clobberedRegs = tc.clobberedRegs
			m.currentABI = &tc.abi

			root := m.allocateNop()
			m.ectx.RootInstr = root
			udf := m.allocateInstr()
			udf.asUD2()
			root.next = udf
			udf.prev = root

			m.SetupPrologue()
			require.Equal(t, root, m.ectx.RootInstr)
			m.Encode(context.Background())
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_SetupEpilogue(t *testing.T) {
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
			clobberedRegs: []regalloc.VReg{
				raxVReg,
				rcxVReg,
				xmm0VReg,
				xmm1VReg,
				xmm15VReg,
			},
		},
		// TODO: add more test cases.
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.spillSlotSize = tc.spillSlotSize
			m.clobberedRegs = tc.clobberedRegs
			m.currentABI = &tc.abi

			root := m.allocateNop()
			m.ectx.RootInstr = root
			ret := m.allocateInstr()
			ret.asRet(nil)
			root.next = ret
			ret.prev = root
			m.SetupEpilogue()

			require.Equal(t, root, m.ectx.RootInstr)
			m.Encode(context.Background())
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
