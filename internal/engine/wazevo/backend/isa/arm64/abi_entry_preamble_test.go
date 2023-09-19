package arm64

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAbiImpl_constructEntryPreamble(t *testing.T) {
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
	bl x24
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
	ldr s0, [x19], #0x8
	ldr s1, [x19], #0x8
	ldr s2, [x19], #0x8
	ldr s3, [x19], #0x8
	ldr d4, [x19], #0x8
	bl x24
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
	ldr w2, [x19], #0x8
	ldr w3, [x19], #0x8
	ldr w4, [x19], #0x8
	ldr x5, [x19], #0x8
	ldr w6, [x19], #0x8
	bl x24
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
	ldr w2, [x19], #0x8
	ldr d0, [x19], #0x8
	ldr w3, [x19], #0x8
	ldr s1, [x19], #0x8
	ldr x4, [x19], #0x8
	ldr w5, [x19], #0x8
	ldr x6, [x19], #0x8
	ldr d2, [x19], #0x8
	ldr w7, [x19], #0x8
	ldr s3, [x19], #0x8
	ldr q4, [x19], #0x10
	ldr s5, [x19], #0x8
	bl x24
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
	mov x25, x19
	ldr w2, [x25], #0x8
	ldr d0, [x25], #0x8
	ldr w3, [x25], #0x8
	ldr s1, [x25], #0x8
	ldr x4, [x25], #0x8
	bl x24
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
	bl x24
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
	ldr q0, [x19], #0x10
	ldr q1, [x19], #0x10
	ldr q2, [x19], #0x10
	ldr x2, [x19], #0x8
	ldr w3, [x19], #0x8
	ldr w4, [x19], #0x8
	ldr x5, [x19], #0x8
	ldr w6, [x19], #0x8
	ldr x7, [x19], #0x8
	ldr s3, [x19], #0x8
	ldr d4, [x19], #0x8
	ldr w27, [x19], #0x8
	str w27, [sp]
	ldr s5, [x19], #0x8
	ldr x27, [x19], #0x8
	str x27, [sp, #0x8]
	ldr w27, [x19], #0x8
	str w27, [sp, #0x10]
	ldr w27, [x19], #0x8
	str w27, [sp, #0x18]
	ldr x27, [x19], #0x8
	str x27, [sp, #0x20]
	ldr w27, [x19], #0x8
	str w27, [sp, #0x28]
	ldr x27, [x19], #0x8
	str x27, [sp, #0x30]
	ldr s6, [x19], #0x8
	ldr d7, [x19], #0x8
	ldr d15, [x19], #0x8
	str d15, [sp, #0x38]
	ldr s15, [x19], #0x8
	str s15, [sp, #0x40]
	ldr d15, [x19], #0x8
	str d15, [sp, #0x48]
	ldr q15, [x19], #0x10
	str q15, [sp, #0x50]
	ldr q15, [x19], #0x10
	str q15, [sp, #0x60]
	ldr q15, [x19], #0x10
	str q15, [sp, #0x70]
	ldr q15, [x19], #0x10
	str q15, [sp, #0x80]
	ldr q15, [x19], #0x10
	str q15, [sp, #0x90]
	bl x24
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
	ret
`,
		},
		{
			name: "many params/results",
			sig: &ssa.Signature{
				Params: []ssa.Type{
					i32, i32, v128, v128, v128, i64, i32, i32, i64, i32, i64,
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
					f32, f64, f64, f32, f64, v128, v128, v128, v128, v128,
				},
				Results: []ssa.Type{
					f32, f64, i32, f32, i64, i32, i32, i64, i32, i64,
					i32, i32, v128, v128, v128, i64, i32, i32, i64, i32, i64,
				},
			},
			exp: `
	mov x20, x0
	str x29, [x20, #0x10]
	mov x27, sp
	str x27, [x20, #0x18]
	str x30, [x20, #0x20]
	sub x26, x26, #0xe0
	mov sp, x26
	mov x25, x19
	ldr q0, [x25], #0x10
	ldr q1, [x25], #0x10
	ldr q2, [x25], #0x10
	ldr x2, [x25], #0x8
	ldr w3, [x25], #0x8
	ldr w4, [x25], #0x8
	ldr x5, [x25], #0x8
	ldr w6, [x25], #0x8
	ldr x7, [x25], #0x8
	ldr s3, [x25], #0x8
	ldr d4, [x25], #0x8
	ldr w27, [x25], #0x8
	str w27, [sp]
	ldr s5, [x25], #0x8
	ldr x27, [x25], #0x8
	str x27, [sp, #0x8]
	ldr w27, [x25], #0x8
	str w27, [sp, #0x10]
	ldr w27, [x25], #0x8
	str w27, [sp, #0x18]
	ldr x27, [x25], #0x8
	str x27, [sp, #0x20]
	ldr w27, [x25], #0x8
	str w27, [sp, #0x28]
	ldr x27, [x25], #0x8
	str x27, [sp, #0x30]
	ldr s6, [x25], #0x8
	ldr d7, [x25], #0x8
	ldr d15, [x25], #0x8
	str d15, [sp, #0x38]
	ldr s15, [x25], #0x8
	str s15, [sp, #0x40]
	ldr d15, [x25], #0x8
	str d15, [sp, #0x48]
	ldr q15, [x25], #0x10
	str q15, [sp, #0x50]
	ldr q15, [x25], #0x10
	str q15, [sp, #0x60]
	ldr q15, [x25], #0x10
	str q15, [sp, #0x70]
	ldr q15, [x25], #0x10
	str q15, [sp, #0x80]
	ldr q15, [x25], #0x10
	str q15, [sp, #0x90]
	bl x24
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
	str w7, [x19], #0x8
	ldr w27, [sp]
	str w27, [x19], #0x8
	str q3, [x19], #0x10
	str q4, [x19], #0x10
	str q5, [x19], #0x10
	ldr x27, [sp, #0x8]
	str x27, [x19], #0x8
	ldr w27, [sp, #0x10]
	str w27, [x19], #0x8
	ldr w27, [sp, #0x18]
	str w27, [x19], #0x8
	ldr x27, [sp, #0x20]
	str x27, [x19], #0x8
	ldr w27, [sp, #0x28]
	str w27, [x19], #0x8
	ldr x27, [sp, #0x30]
	str x27, [x19], #0x8
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
			m.rootInstr = abi.constructEntryPreamble()
			fmt.Println(m.Format())
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_goEntryPreamblePassArg(t *testing.T) {
	paramSlicePtr := x16VReg
	for _, tc := range []struct {
		arg backend.ABIArg
		exp string
	}{
		// Reg kinds.
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Reg: x0VReg, Kind: backend.ABIArgKindReg},
			exp: `
	ldr w0, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Reg: x0VReg, Kind: backend.ABIArgKindReg},
			exp: `
	ldr x0, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF32, Reg: v11VReg, Kind: backend.ABIArgKindReg},
			exp: `
	ldr s11, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF64, Reg: v12VReg, Kind: backend.ABIArgKindReg},
			exp: `
	ldr d12, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Reg: v12VReg, Kind: backend.ABIArgKindReg},
			exp: `
	ldr q12, [x16], #0x10
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Reg: v12VReg, Kind: backend.ABIArgKindReg},
			exp: `
	ldr q12, [x16], #0x10
`,
		},
		// Stack kinds.
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Offset: 0, Kind: backend.ABIArgKindStack},
			exp: `
	ldr w27, [x16], #0x8
	str w27, [sp]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Offset: 8, Kind: backend.ABIArgKindStack},
			exp: `
	ldr w27, [x16], #0x8
	str w27, [sp, #0x8]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 0, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x27, [x16], #0x8
	str x27, [sp]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 128, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x27, [x16], #0x8
	str x27, [sp, #0x80]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF32, Offset: 64, Kind: backend.ABIArgKindStack},
			exp: `
	ldr s15, [x16], #0x8
	str s15, [sp, #0x40]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF32, Offset: 2056, Kind: backend.ABIArgKindStack},
			exp: `
	ldr s15, [x16], #0x8
	str s15, [sp, #0x808]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF64, Offset: 64, Kind: backend.ABIArgKindStack},
			exp: `
	ldr d15, [x16], #0x8
	str d15, [sp, #0x40]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF64, Offset: 2056, Kind: backend.ABIArgKindStack},
			exp: `
	ldr d15, [x16], #0x8
	str d15, [sp, #0x808]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Offset: 64, Kind: backend.ABIArgKindStack},
			exp: `
	ldr q15, [x16], #0x10
	str q15, [sp, #0x40]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Offset: 2056, Kind: backend.ABIArgKindStack},
			exp: `
	ldr q15, [x16], #0x10
	movz x27, #0x808, lsl 0
	str q15, [sp, x27]
`,
		},
	} {
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			cur := m.allocateNop()
			m.rootInstr = cur
			m.goEntryPreamblePassArg(cur, paramSlicePtr, &tc.arg)
			fmt.Println(m.Format())
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_goEntryPreamblePassResult(t *testing.T) {
	paramSlicePtr := x16VReg
	for _, tc := range []struct {
		arg      backend.ABIArg
		retStart int64
		exp      string
	}{
		// Reg kinds.
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Reg: x0VReg, Kind: backend.ABIArgKindReg},
			exp: `
	str w0, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Reg: x0VReg, Kind: backend.ABIArgKindReg},
			exp: `
	str x0, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF32, Reg: v11VReg, Kind: backend.ABIArgKindReg},
			exp: `
	str s11, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF64, Reg: v12VReg, Kind: backend.ABIArgKindReg},
			exp: `
	str d12, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Reg: v12VReg, Kind: backend.ABIArgKindReg},
			exp: `
	str q12, [x16], #0x10
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Reg: v12VReg, Kind: backend.ABIArgKindReg},
			exp: `
	str q12, [x16], #0x10
`,
		},
		// Stack kinds.
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Offset: 0, Kind: backend.ABIArgKindStack},
			exp: `
	ldr w27, [sp]
	str w27, [x16], #0x8
`,
		},
		{
			arg:      backend.ABIArg{Type: ssa.TypeI32, Offset: 0, Kind: backend.ABIArgKindStack},
			retStart: 1024,
			exp: `
	ldr w27, [sp, #0x400]
	str w27, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Offset: 8, Kind: backend.ABIArgKindStack},
			exp: `
	ldr w27, [sp, #0x8]
	str w27, [x16], #0x8
`,
		},
		{
			arg:      backend.ABIArg{Type: ssa.TypeI32, Offset: 8, Kind: backend.ABIArgKindStack},
			retStart: 1024,
			exp: `
	ldr w27, [sp, #0x408]
	str w27, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 0, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x27, [sp]
	str x27, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 128, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x27, [sp, #0x80]
	str x27, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF32, Offset: 64, Kind: backend.ABIArgKindStack},
			exp: `
	ldr s15, [sp, #0x40]
	str s15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF32, Offset: 2056, Kind: backend.ABIArgKindStack},
			exp: `
	ldr s15, [sp, #0x808]
	str s15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF64, Offset: 64, Kind: backend.ABIArgKindStack},
			exp: `
	ldr d15, [sp, #0x40]
	str d15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeF64, Offset: 2056, Kind: backend.ABIArgKindStack},
			exp: `
	ldr d15, [sp, #0x808]
	str d15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Offset: 64, Kind: backend.ABIArgKindStack},
			exp: `
	ldr q15, [sp, #0x40]
	str q15, [x16], #0x10
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeV128, Offset: 2056, Kind: backend.ABIArgKindStack},
			exp: `
	movz x27, #0x808, lsl 0
	ldr q15, [sp, x27]
	str q15, [x16], #0x10
`,
		},
		{
			arg:      backend.ABIArg{Type: ssa.TypeV128, Offset: 2056, Kind: backend.ABIArgKindStack},
			retStart: 1024,
			exp: `
	movz x27, #0xc08, lsl 0
	ldr q15, [sp, x27]
	str q15, [x16], #0x10
`,
		},
	} {
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			cur := m.allocateNop()
			m.rootInstr = cur
			m.goEntryPreamblePassResult(cur, paramSlicePtr, &tc.arg, tc.retStart)
			fmt.Println(m.Format())
			require.Equal(t, tc.exp, m.Format())
		})
	}
}
