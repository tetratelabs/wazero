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
	movq %rbx, %rsp
	callq *%rdi
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
				Results: []ssa.Type{ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64},
			},
			exp: `
	movq %rax, %rdx
	mov.q %rbp, 16(%rax)
	mov.q %rsp, 24(%rax)
	movq %rbx, %rsp
	callq *%rdi
	mov.l %rax, (%r12)
	mov.q %rcx, 8(%r12)
	movss %xmm0, 16(%r12)
	movsd %xmm1, 24(%r12)
	movq 16(%rdx), %rbp
	movq 24(%rdx), %rsp
	ret
`,
		},
		// TODO: add Wasm param, result cases.
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.ectx.RootInstr = m.compileEntryPreamble(tc.sig)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
