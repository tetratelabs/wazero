package arm64

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAbiImpl_constructGoEntryPreamble(t *testing.T) {
	const i32, f32, i64, f64 = ssa.TypeI32, ssa.TypeF32, ssa.TypeI64, ssa.TypeF64

	for _, tc := range []struct {
		name string
		sig  *ssa.Signature
		exp  string
	}{
		{
			name: "empty",
			sig:  &ssa.Signature{},
			exp: `
	mov x18, x0
	str x29, [x18, #0x10]
	mov x27, sp
	str x27, [x18, #0x18]
	str x30, [x18, #0x20]
	mov sp, x26
	bl #0x18
	ldr x29, [x18, #0x10]
	ldr x27, [x18, #0x18]
	mov sp, x27
	ldr x30, [x18, #0x20]
	ret
`,
		},
		{
			name: "float reg params",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i64, i64, // first and second will be skipped.
					f32, f32, f32, f32, f64,
				},
			},
			exp: `
	mov x18, x0
	str x29, [x18, #0x10]
	mov x27, sp
	str x27, [x18, #0x18]
	str x30, [x18, #0x20]
	mov sp, x26
	ldr s0, [x19]
	ldr s1, [x19, #0x8]
	ldr s2, [x19, #0x10]
	ldr s3, [x19, #0x18]
	ldr d4, [x19, #0x20]
	bl #0x18
	ldr x29, [x18, #0x10]
	ldr x27, [x18, #0x18]
	mov sp, x27
	ldr x30, [x18, #0x20]
	ret
`,
		},
		{
			name: "int reg params",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i64, i64, // first and second will be skipped.
					i32, i32, i32, i64, i32,
				},
			},
			exp: `
	mov x18, x0
	str x29, [x18, #0x10]
	mov x27, sp
	str x27, [x18, #0x18]
	str x30, [x18, #0x20]
	mov sp, x26
	ldr w2, [x19]
	ldr w3, [x19, #0x8]
	ldr w4, [x19, #0x10]
	ldr x5, [x19, #0x18]
	ldr w6, [x19, #0x20]
	bl #0x18
	ldr x29, [x18, #0x10]
	ldr x27, [x18, #0x18]
	mov sp, x27
	ldr x30, [x18, #0x20]
	ret
`,
		},
		{
			name: "int/float reg params interleaved",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i64, i64, // first and second will be skipped.
					i32, f64, i32, f32, i64, i32, i64, f64, i32, f32,
				},
			},
			exp: `
	mov x18, x0
	str x29, [x18, #0x10]
	mov x27, sp
	str x27, [x18, #0x18]
	str x30, [x18, #0x20]
	mov sp, x26
	ldr w2, [x19]
	ldr d0, [x19, #0x8]
	ldr w3, [x19, #0x10]
	ldr s1, [x19, #0x18]
	ldr x4, [x19, #0x20]
	ldr w5, [x19, #0x28]
	ldr x6, [x19, #0x30]
	ldr d2, [x19, #0x38]
	ldr w7, [x19, #0x40]
	ldr s3, [x19, #0x48]
	bl #0x18
	ldr x29, [x18, #0x10]
	ldr x27, [x18, #0x18]
	mov sp, x27
	ldr x30, [x18, #0x20]
	ret
`,
		},
		{
			name: "int/float reg params/results interleaved",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i64, i64, // first and second will be skipped.
					i32, f64, i32, f32, i64,
				},
				Results: []ssa.Type{f32, f64, i32, f32, i64, i32, f64},
			},
			exp: `
	mov x18, x0
	str x29, [x18, #0x10]
	mov x27, sp
	str x27, [x18, #0x18]
	str x30, [x18, #0x20]
	mov sp, x26
	ldr w2, [x19]
	ldr d0, [x19, #0x8]
	ldr w3, [x19, #0x10]
	ldr s1, [x19, #0x18]
	ldr x4, [x19, #0x20]
	bl #0x34
	str s0, [x19], #0x8
	str d1, [x19], #0x8
	str w0, [x19], #0x8
	str s2, [x19], #0x8
	str x1, [x19], #0x8
	str w2, [x19], #0x8
	str d3, [x19], #0x8
	ldr x29, [x18, #0x10]
	ldr x27, [x18, #0x18]
	mov sp, x27
	ldr x30, [x18, #0x20]
	ret
`,
		},
		{
			name: "many results",
			sig: &ssa.Signature{
				Results: []ssa.Type{
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
				},
			},
			exp: `
	mov x18, x0
	str x29, [x18, #0x10]
	mov x27, sp
	str x27, [x18, #0x18]
	str x30, [x18, #0x20]
	sub x26, x26, #0x30
	mov sp, x26
	bl #0x80
	str s0, [x19], #0x8
	str d1, [x19], #0x8
	str w0, [x19], #0x8
	str s2, [x19], #0x8
	str x1, [x19], #0x8
	str w2, [x19], #0x8
	str w3, [x19], #0x8
	str x4, [x19], #0x8
	str w5, [x19], #0x8
	str x6, [x19], #0x8
	str s3, [x19], #0x8
	str d4, [x19], #0x8
	str w7, [x19], #0x8
	str s5, [x19], #0x8
	ldr x27, [sp]
	str x27, [x19], #0x8
	ldr w27, [sp, #0x8]
	str w27, [x19], #0x8
	ldr w27, [sp, #0x10]
	str w27, [x19], #0x8
	ldr x27, [sp, #0x18]
	str x27, [x19], #0x8
	ldr w27, [sp, #0x20]
	str w27, [x19], #0x8
	ldr x27, [sp, #0x28]
	str x27, [x19], #0x8
	ldr x29, [x18, #0x10]
	ldr x27, [x18, #0x18]
	mov sp, x27
	ldr x30, [x18, #0x20]
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			abi := m.getOrCreateABIImpl(tc.sig)
			m.rootInstr = abi.constructGoEntryPreamble()
			fmt.Println(m.Format())
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
