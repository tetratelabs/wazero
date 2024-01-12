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
		// TODO: add Wasm param, result cases.
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.ectx.RootInstr = m.compileEntryPreamble(tc.sig)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
