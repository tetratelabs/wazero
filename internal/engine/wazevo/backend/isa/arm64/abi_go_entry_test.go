package arm64

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAbiImpl_constructGoEntryPreamble(t *testing.T) {
	const i32, f32, i64, f64, v128 = ssa.TypeI32, ssa.TypeF32, ssa.TypeI64, ssa.TypeF64, ssa.TypeV128

	for _, tc := range []struct {
		name string
		sig  *ssa.Signature
		exp  string
	}{
		{
			name: "empty",
			sig:  &ssa.Signature{},
			exp: `
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
	mov sp, x26
	bl #0x18
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
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
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
	mov sp, x26
	ldr s0, [x19]
	ldr s1, [x19, #0x8]
	ldr s2, [x19, #0x10]
	ldr s3, [x19, #0x18]
	ldr d4, [x19, #0x20]
	bl #0x18
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
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
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
	mov sp, x26
	ldr w2, [x19]
	ldr w3, [x19, #0x8]
	ldr w4, [x19, #0x10]
	ldr x5, [x19, #0x18]
	ldr w6, [x19, #0x20]
	bl #0x18
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
	ret
`,
		},
		{
			name: "int/float reg params interleaved",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i64, i64, // first and second will be skipped.
					i32, f64, i32, f32, i64, i32, i64, f64, i32, f32, v128, f32,
				},
			},
			exp: `
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
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
	ldr q4, [x19, #0x50]
	ldr s5, [x19, #0x60]
	bl #0x18
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
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
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
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
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
	ret
`,
		},
		{
			name: "many results",
			sig: &ssa.Signature{
				Results: []ssa.Type{
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
					f32, f64, f64, f32, f64, v128, v128,
				},
			},
			exp: `
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
	sub x26, x26, #0x70
	mov sp, x26
	bl #0xb0
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
	str s6, [x19], #0x8
	str d7, [x19], #0x8
	ldr d15, [sp, #0x30]
	str d15, [x19], #0x8
	ldr s15, [sp, #0x38]
	str s15, [x19], #0x8
	ldr d15, [sp, #0x40]
	str d15, [x19], #0x8
	ldr q15, [sp, #0x50]
	str q15, [x19], #0x10
	ldr q15, [sp, #0x60]
	str q15, [x19], #0x10
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
	ret
`,
		},
		{
			name: "many params",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i32, i32, v128, v128, v128, i64, i32, i32, i64, i32, i64,
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
					f32, f64, f64, f32, f64, v128, v128, v128, v128, v128,
				},
			},
			exp: `
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
	sub x26, x26, #0xa0
	mov sp, x26
	ldr q0, [x19]
	ldr q1, [x19, #0x10]
	ldr q2, [x19, #0x20]
	ldr x2, [x19, #0x30]
	ldr w3, [x19, #0x38]
	ldr w4, [x19, #0x40]
	ldr x5, [x19, #0x48]
	ldr w6, [x19, #0x50]
	ldr x7, [x19, #0x58]
	ldr s3, [x19, #0x60]
	ldr d4, [x19, #0x68]
	ldr w27, [x19, #0x70]
	str w27, [sp]
	ldr s5, [x19, #0x78]
	ldr x27, [x19, #0x80]
	str x27, [sp, #0x8]
	ldr w27, [x19, #0x88]
	str w27, [sp, #0x10]
	ldr w27, [x19, #0x90]
	str w27, [sp, #0x18]
	ldr x27, [x19, #0x98]
	str x27, [sp, #0x20]
	ldr w27, [x19, #0xa0]
	str w27, [sp, #0x28]
	ldr x27, [x19, #0xa8]
	str x27, [sp, #0x30]
	ldr s6, [x19, #0xb0]
	ldr d7, [x19, #0xb8]
	ldr d15, [x19, #0xc0]
	str d15, [sp, #0x38]
	ldr s15, [x19, #0xc8]
	str s15, [sp, #0x40]
	ldr d15, [x19, #0xd0]
	str d15, [sp, #0x48]
	ldr q15, [x19, #0xd8]
	str q15, [sp, #0x50]
	ldr q15, [x19, #0xe8]
	str q15, [sp, #0x60]
	ldr q15, [x19, #0xf8]
	str q15, [sp, #0x70]
	movz x27, #0x108, lsl 0
	ldr q15, [x19, x27]
	str q15, [sp, #0x80]
	movz x27, #0x118, lsl 0
	ldr q15, [x19, x27]
	str q15, [sp, #0x90]
	bl #0x18
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
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
