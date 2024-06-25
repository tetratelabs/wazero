package amd64

import (
	"context"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_calleeSavedVRegs(t *testing.T) {
	var exp []regalloc.VReg
	regInfo.CalleeSavedRegisters.Range(func(r regalloc.RealReg) {
		exp = append(exp, regInfo.RealRegToVReg[r])
	})
	require.Equal(t, exp, calleeSavedVRegs)
}

func TestMachine_CompileGoFunctionTrampoline(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		exitCode             wazevoapi.ExitCode
		sig                  *ssa.Signature
		needModuleContextPtr bool
		exp                  string
	}{
		{
			name:     "go call",
			exitCode: wazevoapi.ExitCodeCallGoFunctionWithIndex(100, false),
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeF64},
				Results: []ssa.Type{ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64},
			},
			needModuleContextPtr: true,
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	sub $40, %rsp
	cmpq 40(%rax), %rsp
	jnbe L1
	add $40, %rsp
	pushq %r15
	movabsq $40, %r15
	mov.q %r15, 64(%rax)
	popq %r15
	callq *80(%rax)
	jmp L2
L1:
	add $40, %rsp
L2:
	mov.q %rdx, 96(%rax)
	mov.q %r12, 112(%rax)
	mov.q %r13, 128(%rax)
	mov.q %r14, 144(%rax)
	mov.q %r15, 160(%rax)
	movdqu %xmm8, 176(%rax)
	movdqu %xmm9, 192(%rax)
	movdqu %xmm10, 208(%rax)
	movdqu %xmm11, 224(%rax)
	movdqu %xmm12, 240(%rax)
	movdqu %xmm13, 256(%rax)
	movdqu %xmm14, 272(%rax)
	movdqu %xmm15, 288(%rax)
	mov.q %rbx, 1120(%rax)
	sub $32, %rsp
	movsd %xmm0, (%rsp)
	pushq $32
	movl $25606, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L3, %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L3:
	add $8, %rsp
	movq 8(%rsp), %rbx
	movss 16(%rsp), %xmm0
	movsd 24(%rsp), %xmm1
	movq 96(%rax), %rdx
	movq 112(%rax), %r12
	movq 128(%rax), %r13
	movq 144(%rax), %r14
	movq 160(%rax), %r15
	movdqu 176(%rax), %xmm8
	movdqu 192(%rax), %xmm9
	movdqu 208(%rax), %xmm10
	movdqu 224(%rax), %xmm11
	movdqu 240(%rax), %xmm12
	movdqu 256(%rax), %xmm13
	movdqu 272(%rax), %xmm14
	movdqu 288(%rax), %xmm15
	movq (%rsp), %rax
	movq %rbp, %rsp
	popq %rbp
	ret
`,
		},
		{
			name:     "go call",
			exitCode: wazevoapi.ExitCodeCallGoFunctionWithIndex(100, false),
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeF64, ssa.TypeF64, ssa.TypeI32, ssa.TypeI32},
				Results: []ssa.Type{},
			},
			needModuleContextPtr: true,
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	sub $40, %rsp
	cmpq 40(%rax), %rsp
	jnbe L1
	add $40, %rsp
	pushq %r15
	movabsq $40, %r15
	mov.q %r15, 64(%rax)
	popq %r15
	callq *80(%rax)
	jmp L2
L1:
	add $40, %rsp
L2:
	mov.q %rdx, 96(%rax)
	mov.q %r12, 112(%rax)
	mov.q %r13, 128(%rax)
	mov.q %r14, 144(%rax)
	mov.q %r15, 160(%rax)
	movdqu %xmm8, 176(%rax)
	movdqu %xmm9, 192(%rax)
	movdqu %xmm10, 208(%rax)
	movdqu %xmm11, 224(%rax)
	movdqu %xmm12, 240(%rax)
	movdqu %xmm13, 256(%rax)
	movdqu %xmm14, 272(%rax)
	movdqu %xmm15, 288(%rax)
	mov.q %rbx, 1120(%rax)
	sub $32, %rsp
	movsd %xmm0, (%rsp)
	movsd %xmm1, 8(%rsp)
	mov.l %rcx, 16(%rsp)
	mov.l %rdi, 24(%rsp)
	pushq $32
	movl $25606, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L3, %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L3:
	add $8, %rsp
	movq 96(%rax), %rdx
	movq 112(%rax), %r12
	movq 128(%rax), %r13
	movq 144(%rax), %r14
	movq 160(%rax), %r15
	movdqu 176(%rax), %xmm8
	movdqu 192(%rax), %xmm9
	movdqu 208(%rax), %xmm10
	movdqu 224(%rax), %xmm11
	movdqu 240(%rax), %xmm12
	movdqu 256(%rax), %xmm13
	movdqu 272(%rax), %xmm14
	movdqu 288(%rax), %xmm15
	movq %rbp, %rsp
	popq %rbp
	ret
`,
		},
		{
			name:     "grow memory",
			exitCode: wazevoapi.ExitCodeGrowMemory,
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI32, ssa.TypeI32},
				Results: []ssa.Type{ssa.TypeI32},
			},
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	sub $24, %rsp
	cmpq 40(%rax), %rsp
	jnbe L1
	add $24, %rsp
	pushq %r15
	movabsq $24, %r15
	mov.q %r15, 64(%rax)
	popq %r15
	callq *80(%rax)
	jmp L2
L1:
	add $24, %rsp
L2:
	mov.q %rdx, 96(%rax)
	mov.q %r12, 112(%rax)
	mov.q %r13, 128(%rax)
	mov.q %r14, 144(%rax)
	mov.q %r15, 160(%rax)
	movdqu %xmm8, 176(%rax)
	movdqu %xmm9, 192(%rax)
	movdqu %xmm10, 208(%rax)
	movdqu %xmm11, 224(%rax)
	movdqu %xmm12, 240(%rax)
	movdqu %xmm13, 256(%rax)
	movdqu %xmm14, 272(%rax)
	movdqu %xmm15, 288(%rax)
	sub $16, %rsp
	mov.l %rbx, (%rsp)
	pushq $8
	movl $2, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L3, %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L3:
	add $8, %rsp
	movq 96(%rax), %rdx
	movq 112(%rax), %r12
	movq 128(%rax), %r13
	movq 144(%rax), %r14
	movq 160(%rax), %r15
	movdqu 176(%rax), %xmm8
	movdqu 192(%rax), %xmm9
	movdqu 208(%rax), %xmm10
	movdqu 224(%rax), %xmm11
	movdqu 240(%rax), %xmm12
	movdqu 256(%rax), %xmm13
	movdqu 272(%rax), %xmm14
	movdqu 288(%rax), %xmm15
	movq (%rsp), %rax
	movq %rbp, %rsp
	popq %rbp
	ret
`,
		},
		{
			name:     "many",
			exitCode: wazevoapi.ExitCodeCallGoFunctionWithIndex(100, false),
			sig: &ssa.Signature{
				Params: []ssa.Type{
					ssa.TypeI64, ssa.TypeI64, ssa.TypeF64,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
				},
				Results: []ssa.Type{
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
					ssa.TypeF64, ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32,
				},
			},
			exp: `
	pushq %rbp
	movq %rsp, %rbp
	sub $248, %rsp
	cmpq 40(%rax), %rsp
	jnbe L1
	add $248, %rsp
	pushq %r15
	movabsq $248, %r15
	mov.q %r15, 64(%rax)
	popq %r15
	callq *80(%rax)
	jmp L2
L1:
	add $248, %rsp
L2:
	mov.q %rdx, 96(%rax)
	mov.q %r12, 112(%rax)
	mov.q %r13, 128(%rax)
	mov.q %r14, 144(%rax)
	mov.q %r15, 160(%rax)
	movdqu %xmm8, 176(%rax)
	movdqu %xmm9, 192(%rax)
	movdqu %xmm10, 208(%rax)
	movdqu %xmm11, 224(%rax)
	movdqu %xmm12, 240(%rax)
	movdqu %xmm13, 256(%rax)
	movdqu %xmm14, 272(%rax)
	movdqu %xmm15, 288(%rax)
	mov.q %rbx, 1120(%rax)
	sub $240, %rsp
	movsd %xmm0, (%rsp)
	movsd %xmm1, 8(%rsp)
	movdqu %xmm2, 16(%rsp)
	mov.l %rcx, 32(%rsp)
	mov.q %rdi, 40(%rsp)
	movss %xmm3, 48(%rsp)
	movsd %xmm4, 56(%rsp)
	movdqu %xmm5, 64(%rsp)
	mov.l %rsi, 80(%rsp)
	mov.q %r8, 88(%rsp)
	movss %xmm6, 96(%rsp)
	movsd %xmm7, 104(%rsp)
	movdqu 16(%rbp), %xmm15
	movdqu %xmm15, 112(%rsp)
	mov.l %r9, 128(%rsp)
	mov.q %r10, 136(%rsp)
	movss 32(%rbp), %xmm15
	movss %xmm15, 144(%rsp)
	pushq $240
	movl $25606, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L3, %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L3:
	add $8, %rsp
	movsd (%rsp), %xmm0
	movdqu 8(%rsp), %xmm1
	movq 32(%rsp), %rbx
	movss 40(%rsp), %xmm2
	movsd 48(%rsp), %xmm3
	movdqu 56(%rsp), %xmm4
	movzx.lq 72(%rsp), %rcx
	movq 80(%rsp), %rdi
	movss 88(%rsp), %xmm5
	movsd 96(%rsp), %xmm6
	movdqu 104(%rsp), %xmm7
	movzx.lq 120(%rsp), %rsi
	movq 128(%rsp), %r8
	movss 136(%rsp), %xmm15
	movss %xmm15, 40(%rbp)
	movsd 144(%rsp), %xmm15
	movsd %xmm15, 48(%rbp)
	movdqu 152(%rsp), %xmm15
	movdqu %xmm15, 56(%rbp)
	movzx.lq 168(%rsp), %r9
	movq 176(%rsp), %r10
	movss 184(%rsp), %xmm15
	movss %xmm15, 72(%rbp)
	movsd 192(%rsp), %xmm15
	movsd %xmm15, 80(%rbp)
	movdqu 200(%rsp), %xmm15
	movdqu %xmm15, 88(%rbp)
	movzx.lq 216(%rsp), %r11
	movq 224(%rsp), %r15
	mov.q %r15, 104(%rbp)
	movss 232(%rsp), %xmm15
	movss %xmm15, 112(%rbp)
	movq 96(%rax), %rdx
	movq 112(%rax), %r12
	movq 128(%rax), %r13
	movq 144(%rax), %r14
	movq 160(%rax), %r15
	movdqu 176(%rax), %xmm8
	movdqu 192(%rax), %xmm9
	movdqu 208(%rax), %xmm10
	movdqu 224(%rax), %xmm11
	movdqu 240(%rax), %xmm12
	movdqu 256(%rax), %xmm13
	movdqu 272(%rax), %xmm14
	movdqu 288(%rax), %xmm15
	movq 24(%rsp), %rax
	movq %rbp, %rsp
	popq %rbp
	ret
`,
			needModuleContextPtr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.nextLabel = 1
			m.CompileGoFunctionTrampoline(tc.exitCode, tc.sig, tc.needModuleContextPtr)
			require.Equal(t, tc.exp, m.Format())
			err := m.Encode(context.Background())
			require.NoError(t, err)
		})
	}
}

func Test_stackGrowSaveVRegs(t *testing.T) {
	var exp []regalloc.VReg
	for _, rs := range regInfo.AllocatableRegisters {
		for _, r := range rs {
			if r != rsp && r != rbp && r != rax {
				exp = append(exp, regInfo.RealRegToVReg[r])
			}
		}
	}
	// Copy stackGrowSaveVRegs to avoid modifying the original.
	var actual []regalloc.VReg
	actual = append(actual, stackGrowSaveVRegs...)
	sort.Slice(exp, func(i, j int) bool { return exp[i].ID() < exp[j].ID() })
	sort.Slice(actual, func(i, j int) bool { return actual[i].ID() < actual[j].ID() })
	require.Equal(t, exp, actual)
}

func TestMachine_CompileStackGrowCallSequence(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	m.nextLabel = 1

	_ = m.CompileStackGrowCallSequence()

	require.Equal(t, `
	pushq %rbp
	movq %rsp, %rbp
	mov.q %rdx, 96(%rax)
	mov.q %r12, 112(%rax)
	mov.q %r13, 128(%rax)
	mov.q %r14, 144(%rax)
	mov.q %r15, 160(%rax)
	mov.q %rcx, 176(%rax)
	mov.q %rbx, 192(%rax)
	mov.q %rsi, 208(%rax)
	mov.q %rdi, 224(%rax)
	mov.q %r8, 240(%rax)
	mov.q %r9, 256(%rax)
	mov.q %r10, 272(%rax)
	mov.q %r11, 288(%rax)
	movdqu %xmm8, 304(%rax)
	movdqu %xmm9, 320(%rax)
	movdqu %xmm10, 336(%rax)
	movdqu %xmm11, 352(%rax)
	movdqu %xmm12, 368(%rax)
	movdqu %xmm13, 384(%rax)
	movdqu %xmm14, 400(%rax)
	movdqu %xmm15, 416(%rax)
	movdqu %xmm0, 432(%rax)
	movdqu %xmm1, 448(%rax)
	movdqu %xmm2, 464(%rax)
	movdqu %xmm3, 480(%rax)
	movdqu %xmm4, 496(%rax)
	movdqu %xmm5, 512(%rax)
	movdqu %xmm6, 528(%rax)
	movdqu %xmm7, 544(%rax)
	movl $1, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L1, %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L1:
	movq 96(%rax), %rdx
	movq 112(%rax), %r12
	movq 128(%rax), %r13
	movq 144(%rax), %r14
	movq 160(%rax), %r15
	movq 176(%rax), %rcx
	movq 192(%rax), %rbx
	movq 208(%rax), %rsi
	movq 224(%rax), %rdi
	movq 240(%rax), %r8
	movq 256(%rax), %r9
	movq 272(%rax), %r10
	movq 288(%rax), %r11
	movdqu 304(%rax), %xmm8
	movdqu 320(%rax), %xmm9
	movdqu 336(%rax), %xmm10
	movdqu 352(%rax), %xmm11
	movdqu 368(%rax), %xmm12
	movdqu 384(%rax), %xmm13
	movdqu 400(%rax), %xmm14
	movdqu 416(%rax), %xmm15
	movdqu 432(%rax), %xmm0
	movdqu 448(%rax), %xmm1
	movdqu 464(%rax), %xmm2
	movdqu 480(%rax), %xmm3
	movdqu 496(%rax), %xmm4
	movdqu 512(%rax), %xmm5
	movdqu 528(%rax), %xmm6
	movdqu 544(%rax), %xmm7
	movq %rbp, %rsp
	popq %rbp
	ret
`, m.Format())
}

func TestMachine_insertStackBoundsCheck(t *testing.T) {
	for _, tc := range []struct {
		exp               string
		requiredStackSize int64
	}{
		{
			requiredStackSize: 0x1000,
			exp: `
	sub $4096, %rsp
	cmpq 40(%rax), %rsp
	jnbe L1
	add $4096, %rsp
	pushq %r15
	movabsq $4096, %r15
	mov.q %r15, 64(%rax)
	popq %r15
	callq *80(%rax)
	jmp L2
L1:
	add $4096, %rsp
L2:
`,
		},
		{
			requiredStackSize: 0x10,
			exp: `
	sub $16, %rsp
	cmpq 40(%rax), %rsp
	jnbe L1
	add $16, %rsp
	pushq %r15
	movabsq $16, %r15
	mov.q %r15, 64(%rax)
	popq %r15
	callq *80(%rax)
	jmp L2
L1:
	add $16, %rsp
L2:
`,
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.nextLabel = 1
			m.rootInstr = m.allocateNop()
			m.insertStackBoundsCheck(tc.requiredStackSize, m.rootInstr)
			err := m.Encode(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
