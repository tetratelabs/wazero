package amd64

import (
	"context"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"testing"
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
	lea L1(%rip), %r1?
	mov.q %r1?, 48(%rax)
	exit_sequence %rax
L1:
	movq 96(%rdx), %rdx
	movq 112(%r12), %r12
	movq 128(%r13), %r13
	movq 144(%r14), %r14
	movq 160(%r15), %r15
	movdqu 176(%xmm8), %xmm8
	movdqu 192(%xmm9), %xmm9
	movdqu 208(%xmm10), %xmm10
	movdqu 224(%xmm11), %xmm11
	movdqu 240(%xmm12), %xmm12
	movdqu 256(%xmm13), %xmm13
	movdqu 272(%xmm14), %xmm14
	movdqu 288(%xmm15), %xmm15
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
	stp x30, xzr, [sp, #-0x10]!
	sub x27, sp, #0x30
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	orr x27, xzr, #0x30
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str q18, [x0, #0xf0]
	str q19, [x0, #0x100]
	str q20, [x0, #0x110]
	str q21, [x0, #0x120]
	str q22, [x0, #0x130]
	str q23, [x0, #0x140]
	str q24, [x0, #0x150]
	str q25, [x0, #0x160]
	str q26, [x0, #0x170]
	str q27, [x0, #0x180]
	str q28, [x0, #0x190]
	str q29, [x0, #0x1a0]
	str q30, [x0, #0x1b0]
	str q31, [x0, #0x1c0]
	str x1, [x0, #0x460]
	sub sp, sp, #0x20
	mov x15, sp
	str d0, [x15], #0x8
	str d1, [x15], #0x8
	str x2, [x15], #0x8
	str x3, [x15], #0x8
	orr x27, xzr, #0x20
	orr x16, xzr, #0x4
	stp x27, x16, [sp, #-0x10]!
	movz w17, #0x6406, lsl 0
	str w17, [x0]
	mov x27, sp
	str x27, [x0, #0x38]
	adr x27, #0x20
	str x27, [x0, #0x30]
	exit_sequence x0
	ldr x19, [x0, #0x60]
	ldr x20, [x0, #0x70]
	ldr x21, [x0, #0x80]
	ldr x22, [x0, #0x90]
	ldr x23, [x0, #0xa0]
	ldr x24, [x0, #0xb0]
	ldr x25, [x0, #0xc0]
	ldr x26, [x0, #0xd0]
	ldr x28, [x0, #0xe0]
	ldr q18, [x0, #0xf0]
	ldr q19, [x0, #0x100]
	ldr q20, [x0, #0x110]
	ldr q21, [x0, #0x120]
	ldr q22, [x0, #0x130]
	ldr q23, [x0, #0x140]
	ldr q24, [x0, #0x150]
	ldr q25, [x0, #0x160]
	ldr q26, [x0, #0x170]
	ldr q27, [x0, #0x180]
	ldr q28, [x0, #0x190]
	ldr q29, [x0, #0x1a0]
	ldr q30, [x0, #0x1b0]
	ldr q31, [x0, #0x1c0]
	add sp, sp, #0x30
	ldr x30, [sp], #0x10
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
	stp x30, xzr, [sp, #-0x10]!
	sub x27, sp, #0x20
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	orr x27, xzr, #0x20
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str q18, [x0, #0xf0]
	str q19, [x0, #0x100]
	str q20, [x0, #0x110]
	str q21, [x0, #0x120]
	str q22, [x0, #0x130]
	str q23, [x0, #0x140]
	str q24, [x0, #0x150]
	str q25, [x0, #0x160]
	str q26, [x0, #0x170]
	str q27, [x0, #0x180]
	str q28, [x0, #0x190]
	str q29, [x0, #0x1a0]
	str q30, [x0, #0x1b0]
	str q31, [x0, #0x1c0]
	sub sp, sp, #0x10
	mov x15, sp
	str x1, [x15], #0x8
	orr x27, xzr, #0x10
	orr x16, xzr, #0x1
	stp x27, x16, [sp, #-0x10]!
	orr w17, wzr, #0x2
	str w17, [x0]
	mov x27, sp
	str x27, [x0, #0x38]
	adr x27, #0x20
	str x27, [x0, #0x30]
	exit_sequence x0
	ldr x19, [x0, #0x60]
	ldr x20, [x0, #0x70]
	ldr x21, [x0, #0x80]
	ldr x22, [x0, #0x90]
	ldr x23, [x0, #0xa0]
	ldr x24, [x0, #0xb0]
	ldr x25, [x0, #0xc0]
	ldr x26, [x0, #0xd0]
	ldr x28, [x0, #0xe0]
	ldr q18, [x0, #0xf0]
	ldr q19, [x0, #0x100]
	ldr q20, [x0, #0x110]
	ldr q21, [x0, #0x120]
	ldr q22, [x0, #0x130]
	ldr q23, [x0, #0x140]
	ldr q24, [x0, #0x150]
	ldr q25, [x0, #0x160]
	ldr q26, [x0, #0x170]
	ldr q27, [x0, #0x180]
	ldr q28, [x0, #0x190]
	ldr q29, [x0, #0x1a0]
	ldr q30, [x0, #0x1b0]
	ldr q31, [x0, #0x1c0]
	add x15, sp, #0x10
	add sp, sp, #0x20
	ldr x30, [sp], #0x10
	ldr w0, [x15], #0x8
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
