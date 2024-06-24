package amd64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachineCompileEntryPreamble(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  *ssa.Signature
		exp  string
	}{
		{
			name: "basic",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params: []ssa.Type{ssa.TypeI64, ssa.TypeI64},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	xor %rbp, %rbp
	callq *%r14
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		{
			name: "only regs args",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params: []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	movzx.lq (%r12), %rcx
	movq 8(%r12), %rdi
	movss 16(%r12), %xmm0
	movsd 24(%r12), %xmm1
	movdqu 32(%r12), %xmm2
	movq 48(%r12), %rsi
	xor %rbp, %rbp
	callq *%r14
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		{
			name: "only regs rets",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64},
				Results: []ssa.Type{ssa.TypeI32, ssa.TypeV128, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	xor %rbp, %rbp
	callq *%r14
	mov.l %rax, (%r12)
	movdqu %xmm0, 8(%r12)
	mov.q %rbx, 24(%r12)
	movss %xmm1, 32(%r12)
	movsd %xmm2, 40(%r12)
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		{
			name: "only regs args/rets",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64},
				Results: []ssa.Type{ssa.TypeI32, ssa.TypeV128, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	movzx.lq (%r12), %rcx
	movq 8(%r12), %rdi
	movss 16(%r12), %xmm0
	movsd 24(%r12), %xmm1
	movdqu 32(%r12), %xmm2
	movq 48(%r12), %rsi
	xor %rbp, %rbp
	callq *%r14
	mov.l %rax, (%r12)
	movdqu %xmm0, 8(%r12)
	mov.q %rbx, 24(%r12)
	movss %xmm1, 32(%r12)
	movsd %xmm2, 40(%r12)
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		{
			name: "many args",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params: []ssa.Type{
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
				},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	sub $64, %rsp
	movzx.lq (%r12), %rcx
	movq 8(%r12), %rdi
	movss 16(%r12), %xmm0
	movsd 24(%r12), %xmm1
	movdqu 32(%r12), %xmm2
	movq 48(%r12), %rsi
	movq 56(%r12), %r8
	movq 64(%r12), %r9
	movzx.lq 72(%r12), %r10
	movq 80(%r12), %r11
	movss 88(%r12), %xmm3
	movsd 96(%r12), %xmm4
	movdqu 104(%r12), %xmm5
	movq 120(%r12), %r15
	mov.q %r15, (%rsp)
	movq 128(%r12), %r15
	mov.q %r15, 8(%rsp)
	movq 136(%r12), %r15
	mov.q %r15, 16(%rsp)
	movzx.lq 144(%r12), %r15
	mov.l %r15, 24(%rsp)
	movq 152(%r12), %r15
	mov.q %r15, 32(%rsp)
	movss 160(%r12), %xmm6
	movsd 168(%r12), %xmm7
	movdqu 176(%r12), %xmm15
	movdqu %xmm15, 40(%rsp)
	movq 192(%r12), %r15
	mov.q %r15, 56(%rsp)
	xor %rbp, %rbp
	callq *%r14
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		{
			name: "many results",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params: []ssa.Type{ssa.TypeI64, ssa.TypeI64},
				Results: []ssa.Type{
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
				},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	sub $64, %rsp
	xor %rbp, %rbp
	callq *%r14
	mov.q %rax, (%r12)
	mov.q %rbx, 8(%r12)
	mov.l %rcx, 16(%r12)
	mov.q %rdi, 24(%r12)
	movss %xmm0, 32(%r12)
	movsd %xmm1, 40(%r12)
	movdqu %xmm2, 48(%r12)
	mov.q %rsi, 64(%r12)
	mov.q %r8, 72(%r12)
	mov.q %r9, 80(%r12)
	mov.l %r10, 88(%r12)
	mov.q %r11, 96(%r12)
	movss %xmm3, 104(%r12)
	movsd %xmm4, 112(%r12)
	movdqu %xmm5, 120(%r12)
	movq (%rsp), %r15
	mov.q %r15, 136(%r12)
	movq 8(%rsp), %r15
	mov.q %r15, 144(%r12)
	movq 16(%rsp), %r15
	mov.q %r15, 152(%r12)
	movzx.lq 24(%rsp), %r15
	mov.l %r15, 160(%r12)
	movq 32(%rsp), %r15
	mov.q %r15, 168(%r12)
	movss %xmm6, 176(%r12)
	movsd %xmm7, 184(%r12)
	movdqu 40(%rsp), %xmm15
	movdqu %xmm15, 192(%r12)
	movq 56(%rsp), %r15
	mov.q %r15, 208(%r12)
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		{
			name: "many args results",
			sig: &ssa.Signature{
				// execContext and moduleContext are passed in %rax and %rcx.
				Params: []ssa.Type{
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
				},
				Results: []ssa.Type{
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64, ssa.TypeV128, ssa.TypeI64,
				},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %r13, %rsp
	sub $128, %rsp
	movzx.lq (%r12), %rcx
	movq 8(%r12), %rdi
	movss 16(%r12), %xmm0
	movsd 24(%r12), %xmm1
	movdqu 32(%r12), %xmm2
	movq 48(%r12), %rsi
	movq 56(%r12), %r8
	movq 64(%r12), %r9
	movzx.lq 72(%r12), %r10
	movq 80(%r12), %r11
	movss 88(%r12), %xmm3
	movsd 96(%r12), %xmm4
	movdqu 104(%r12), %xmm5
	movq 120(%r12), %r15
	mov.q %r15, (%rsp)
	movq 128(%r12), %r15
	mov.q %r15, 8(%rsp)
	movq 136(%r12), %r15
	mov.q %r15, 16(%rsp)
	movzx.lq 144(%r12), %r15
	mov.l %r15, 24(%rsp)
	movq 152(%r12), %r15
	mov.q %r15, 32(%rsp)
	movss 160(%r12), %xmm6
	movsd 168(%r12), %xmm7
	movdqu 176(%r12), %xmm15
	movdqu %xmm15, 40(%rsp)
	movq 192(%r12), %r15
	mov.q %r15, 56(%rsp)
	xor %rbp, %rbp
	callq *%r14
	mov.q %rax, (%r12)
	mov.q %rbx, 8(%r12)
	mov.l %rcx, 16(%r12)
	mov.q %rdi, 24(%r12)
	movss %xmm0, 32(%r12)
	movsd %xmm1, 40(%r12)
	movdqu %xmm2, 48(%r12)
	mov.q %rsi, 64(%r12)
	mov.q %r8, 72(%r12)
	mov.q %r9, 80(%r12)
	mov.l %r10, 88(%r12)
	mov.q %r11, 96(%r12)
	movss %xmm3, 104(%r12)
	movsd %xmm4, 112(%r12)
	movdqu %xmm5, 120(%r12)
	movq 64(%rsp), %r15
	mov.q %r15, 136(%r12)
	movq 72(%rsp), %r15
	mov.q %r15, 144(%r12)
	movq 80(%rsp), %r15
	mov.q %r15, 152(%r12)
	movzx.lq 88(%rsp), %r15
	mov.l %r15, 160(%r12)
	movq 96(%rsp), %r15
	mov.q %r15, 168(%r12)
	movss %xmm6, 176(%r12)
	movsd %xmm7, 184(%r12)
	movdqu 104(%rsp), %xmm15
	movdqu %xmm15, 192(%r12)
	movq 120(%rsp), %r15
	mov.q %r15, 208(%r12)
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.rootInstr = m.compileEntryPreamble(tc.sig)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
