package amd64

import (
	"context"
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
	mov.q %rcx, 1120(%rax)
	sub $32, %rsp
	movsd %xmm0, (%rsp)
	pushq $32
	movl $25606, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L1(%rip), %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L1:
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
	add $8, %rsp
	movzx.lq (%rsp), %rax
	movq 8(%rsp), %rcx
	movss 16(%rsp), %xmm0
	movsd 24(%rsp), %xmm1
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
	mov.q %rcx, 1120(%rax)
	sub $32, %rsp
	movsd %xmm0, (%rsp)
	movsd %xmm1, 8(%rsp)
	mov.l %rbx, 16(%rsp)
	mov.l %rsi, 24(%rsp)
	pushq $32
	movl $25606, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L1(%rip), %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L1:
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
	add $8, %rsp
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
	mov.l %rcx, (%rsp)
	pushq $8
	movl $2, %r12d
	mov.l %r12, (%rax)
	mov.q %rsp, 56(%rax)
	mov.q %rbp, 1152(%rax)
	lea L1(%rip), %r12
	mov.q %r12, 48(%rax)
	exit_sequence %rax
L1:
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
	add $8, %rsp
	movzx.lq (%rsp), %rax
	movq %rbp, %rsp
	popq %rbp
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.CompileGoFunctionTrampoline(tc.exitCode, tc.sig, tc.needModuleContextPtr)

			require.Equal(t, tc.exp, m.Format())

			m.Encode(context.Background())
		})
	}
}
