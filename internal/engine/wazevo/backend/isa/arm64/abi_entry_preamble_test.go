package arm64

import (
	"context"
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
	ldr x15, [sp, #-0x70]
	str x15, [x19], #0x8
	ldr w15, [sp, #-0x68]
	str w15, [x19], #0x8
	ldr w15, [sp, #-0x60]
	str w15, [x19], #0x8
	ldr x15, [sp, #-0x58]
	str x15, [x19], #0x8
	ldr w15, [sp, #-0x50]
	str w15, [x19], #0x8
	ldr x15, [sp, #-0x48]
	str x15, [x19], #0x8
	str s6, [x19], #0x8
	str d7, [x19], #0x8
	ldr d15, [sp, #-0x40]
	str d15, [x19], #0x8
	ldr s15, [sp, #-0x38]
	str s15, [x19], #0x8
	ldr d15, [sp, #-0x30]
	str d15, [x19], #0x8
	ldr q15, [sp, #-0x28]
	str q15, [x19], #0x10
	ldr q15, [sp, #-0x18]
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
	ldr w15, [x19], #0x8
	str w15, [sp, #-0xa0]
	ldr s5, [x19], #0x8
	ldr x15, [x19], #0x8
	str x15, [sp, #-0x98]
	ldr w15, [x19], #0x8
	str w15, [sp, #-0x90]
	ldr w15, [x19], #0x8
	str w15, [sp, #-0x88]
	ldr x15, [x19], #0x8
	str x15, [sp, #-0x80]
	ldr w15, [x19], #0x8
	str w15, [sp, #-0x78]
	ldr x15, [x19], #0x8
	str x15, [sp, #-0x70]
	ldr s6, [x19], #0x8
	ldr d7, [x19], #0x8
	ldr d15, [x19], #0x8
	str d15, [sp, #-0x68]
	ldr s15, [x19], #0x8
	str s15, [sp, #-0x60]
	ldr d15, [x19], #0x8
	str d15, [sp, #-0x58]
	ldr q15, [x19], #0x10
	str q15, [sp, #-0x50]
	ldr q15, [x19], #0x10
	str q15, [sp, #-0x40]
	ldr q15, [x19], #0x10
	str q15, [sp, #-0x30]
	ldr q15, [x19], #0x10
	str q15, [sp, #-0x20]
	ldr q15, [x19], #0x10
	str q15, [sp, #-0x10]
	bl x24
	ldr x29, [x20, #0x10]
	ldr x27, [x20, #0x18]
	mov sp, x27
	ldr x30, [x20, #0x20]
	ret
`,
		},
		{
			name: "many params and results",
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
	ldr w15, [x25], #0x8
	str w15, [sp, #-0xe0]
	ldr s5, [x25], #0x8
	ldr x15, [x25], #0x8
	str x15, [sp, #-0xd8]
	ldr w15, [x25], #0x8
	str w15, [sp, #-0xd0]
	ldr w15, [x25], #0x8
	str w15, [sp, #-0xc8]
	ldr x15, [x25], #0x8
	str x15, [sp, #-0xc0]
	ldr w15, [x25], #0x8
	str w15, [sp, #-0xb8]
	ldr x15, [x25], #0x8
	str x15, [sp, #-0xb0]
	ldr s6, [x25], #0x8
	ldr d7, [x25], #0x8
	ldr d15, [x25], #0x8
	str d15, [sp, #-0xa8]
	ldr s15, [x25], #0x8
	str s15, [sp, #-0xa0]
	ldr d15, [x25], #0x8
	str d15, [sp, #-0x98]
	ldr q15, [x25], #0x10
	str q15, [sp, #-0x90]
	ldr q15, [x25], #0x10
	str q15, [sp, #-0x80]
	ldr q15, [x25], #0x10
	str q15, [sp, #-0x70]
	ldr q15, [x25], #0x10
	str q15, [sp, #-0x60]
	ldr q15, [x25], #0x10
	str q15, [sp, #-0x50]
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
	ldr w15, [sp, #-0x40]
	str w15, [x19], #0x8
	str q3, [x19], #0x10
	str q4, [x19], #0x10
	str q5, [x19], #0x10
	ldr x15, [sp, #-0x38]
	str x15, [x19], #0x8
	ldr w15, [sp, #-0x30]
	str w15, [x19], #0x8
	ldr w15, [sp, #-0x28]
	str w15, [x19], #0x8
	ldr x15, [sp, #-0x20]
	str x15, [x19], #0x8
	ldr w15, [sp, #-0x18]
	str w15, [x19], #0x8
	ldr x15, [sp, #-0x10]
	str x15, [x19], #0x8
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
			m.rootInstr = m.constructEntryPreamble(tc.sig)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_goEntryPreamblePassArg(t *testing.T) {
	paramSlicePtr := x16VReg
	for _, tc := range []struct {
		arg                      backend.ABIArg
		argSlotBeginOffsetFromSP int64
		exp                      string
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
	ldr w15, [x16], #0x8
	str w15, [sp]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Offset: 8, Kind: backend.ABIArgKindStack},
			exp: `
	ldr w15, [x16], #0x8
	str w15, [sp, #0x8]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 0, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x15, [x16], #0x8
	str x15, [sp]
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 128, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x15, [x16], #0x8
	str x15, [sp, #0x80]
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
		{
			arg:                      backend.ABIArg{Type: ssa.TypeV128, Offset: 1024, Kind: backend.ABIArgKindStack},
			argSlotBeginOffsetFromSP: -1648,
			exp: `
	ldr q15, [x16], #0x10
	movn x27, #0x26f, lsl 0
	str q15, [sp, x27]
`,
		},
	} {
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			cur := m.allocateNop()
			m.rootInstr = cur
			m.goEntryPreamblePassArg(cur, paramSlicePtr, &tc.arg, tc.argSlotBeginOffsetFromSP)
			require.Equal(t, tc.exp, m.Format())
			err := m.Encode(context.Background())
			require.NoError(t, err)
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
	ldr w15, [sp]
	str w15, [x16], #0x8
`,
		},
		{
			arg:      backend.ABIArg{Type: ssa.TypeI32, Offset: 0, Kind: backend.ABIArgKindStack},
			retStart: 1024,
			exp: `
	ldr w15, [sp, #0x400]
	str w15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI32, Offset: 8, Kind: backend.ABIArgKindStack},
			exp: `
	ldr w15, [sp, #0x8]
	str w15, [x16], #0x8
`,
		},
		{
			arg:      backend.ABIArg{Type: ssa.TypeI32, Offset: 8, Kind: backend.ABIArgKindStack},
			retStart: 1024,
			exp: `
	ldr w15, [sp, #0x408]
	str w15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 0, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x15, [sp]
	str x15, [x16], #0x8
`,
		},
		{
			arg: backend.ABIArg{Type: ssa.TypeI64, Offset: 128, Kind: backend.ABIArgKindStack},
			exp: `
	ldr x15, [sp, #0x80]
	str x15, [x16], #0x8
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
			retStart: -1024,
			exp: `
	movz x27, #0x408, lsl 0
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
			require.Equal(t, tc.exp, m.Format())
			err := m.Encode(context.Background())
			require.NoError(t, err)
		})
	}
}
