package arm64

import (
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_calleeSavedRegistersPlusLinkRegSorted(t *testing.T) {
	var exp []regalloc.VReg
	for r := range regInfo.CalleeSavedRegisters {
		exp = append(exp, regInfo.RealRegToVReg[r])
	}
	exp = append(exp, regInfo.RealRegToVReg[lr])
	sort.Slice(exp, func(i, j int) bool {
		return exp[i].RealReg() < exp[j].RealReg()
	})

	require.Equal(t, exp, calleeSavedRegistersPlusLinkRegSorted)
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
			name:                 "listener",
			exitCode:             wazevoapi.ExitCodeCallListenerBefore,
			needModuleContextPtr: true,
			sig: &ssa.Signature{
				Params: []ssa.Type{
					ssa.TypeI64, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32, ssa.TypeV128,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32, ssa.TypeV128,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32, ssa.TypeV128,
					ssa.TypeI64, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32, ssa.TypeV128,
				},
				Results: []ssa.Type{
					ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32,
					ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32,
					ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32,
					ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32,
					ssa.TypeV128, ssa.TypeI32, ssa.TypeI64, ssa.TypeV128, ssa.TypeF32,
				},
			},
			exp: `
	sub x27, sp, #0x140
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	movz x27, #0x140, lsl 0
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	mov x17, sp
	orr x27, xzr, #0xc0
	stp x30, x27, [sp, #-0x10]!
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
	str x1, [x0, #0x460]
	sub sp, sp, #0x120
	mov x15, sp
	str q0, [x15], #0x10
	str d1, [x15], #0x8
	str q2, [x15], #0x10
	str x2, [x15], #0x8
	str x3, [x15], #0x8
	str q3, [x15], #0x10
	str d4, [x15], #0x8
	str q5, [x15], #0x10
	str x4, [x15], #0x8
	str x5, [x15], #0x8
	str q6, [x15], #0x10
	str d7, [x15], #0x8
	add x11, x17, #0x0
	ldr q11, [x11, #0x10]
	str q11, [x15], #0x10
	str x6, [x15], #0x8
	str x7, [x15], #0x8
	add x11, x17, #0x10
	ldr q11, [x11, #0x10]
	str q11, [x15], #0x10
	add x11, x17, #0x20
	ldr s11, [x11, #0x8]
	str d11, [x15], #0x8
	add x11, x17, #0x30
	ldr q11, [x11, #0x10]
	str q11, [x15], #0x10
	movz x27, #0x120, lsl 0
	movz x16, #0x23, lsl 0
	stp x27, x16, [sp, #-0x10]!
	orr w17, wzr, #0xe
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
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add x15, sp, #0x10
	add sp, sp, #0x140
	ldr q0, [x15], #0x10
	ldr w0, [x15], #0x8
	ldr x1, [x15], #0x8
	ldr q1, [x15], #0x10
	ldr s2, [x15], #0x8
	ldr q3, [x15], #0x10
	ldr w2, [x15], #0x8
	ldr x3, [x15], #0x8
	ldr q4, [x15], #0x10
	ldr s5, [x15], #0x8
	ldr q6, [x15], #0x10
	ldr w4, [x15], #0x8
	ldr x5, [x15], #0x8
	ldr q7, [x15], #0x10
	ldr s11, [x15], #0x8
	add x27, sp, #0x40
	str s11, [x27]
	ldr q11, [x15], #0x10
	add x27, sp, #0x50
	str q11, [x27]
	ldr w6, [x15], #0x8
	ldr x7, [x15], #0x8
	ldr q11, [x15], #0x10
	add x27, sp, #0x60
	str q11, [x27]
	ldr s11, [x15], #0x8
	add x27, sp, #0x70
	str s11, [x27]
	ldr q11, [x15], #0x10
	add x27, sp, #0x80
	str q11, [x27]
	ldr w11, [x15], #0x8
	add x27, sp, #0x90
	str w11, [x27]
	ldr x11, [x15], #0x8
	add x27, sp, #0x98
	str x11, [x27]
	ldr q11, [x15], #0x10
	add x27, sp, #0xa0
	str q11, [x27]
	ldr s11, [x15], #0x8
	add x27, sp, #0xb0
	str s11, [x27]
	ret
`,
		},
		{
			name:     "go call",
			exitCode: wazevoapi.ExitCodeCallGoFunctionWithIndex(100, false),
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeF64},
				Results: []ssa.Type{ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64},
			},
			needModuleContextPtr: true,
			exp: `
	sub x27, sp, #0x40
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	orr x27, xzr, #0x40
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	stp x30, xzr, [sp, #-0x10]!
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
	str x1, [x0, #0x460]
	sub sp, sp, #0x20
	mov x15, sp
	str d0, [x15], #0x8
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
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add x15, sp, #0x10
	add sp, sp, #0x40
	ldr w0, [x15], #0x8
	ldr x1, [x15], #0x8
	ldr s0, [x15], #0x8
	ldr d1, [x15], #0x8
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
	sub x27, sp, #0x40
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	orr x27, xzr, #0x40
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	stp x30, xzr, [sp, #-0x10]!
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
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
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add sp, sp, #0x40
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
	sub x27, sp, #0x30
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	orr x27, xzr, #0x30
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	stp x30, xzr, [sp, #-0x10]!
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
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
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add x15, sp, #0x10
	add sp, sp, #0x30
	ldr w0, [x15], #0x8
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.CompileGoFunctionTrampoline(tc.exitCode, tc.sig, tc.needModuleContextPtr)

			fmt.Println(m.Format())
			require.Equal(t, tc.exp, m.Format())

			m.Encode()
			fmt.Println(hex.EncodeToString(m.compiler.Buf()))
		})
	}
}

func Test_goFunctionCallRequiredStackSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sig      *ssa.Signature
		argBegin int
		exp      int64
	}{
		{
			name: "no param",
			sig:  &ssa.Signature{},
			exp:  0,
		},
		{
			name: "only param",
			sig:  &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128}},
			exp:  32,
		},
		{
			name: "only result",
			sig:  &ssa.Signature{Results: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}},
			exp:  32,
		},
		{
			name: "param < result",
			sig:  &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128}, Results: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}},
			exp:  32,
		},
		{
			name: "param > result",
			sig:  &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}, Results: []ssa.Type{ssa.TypeI64, ssa.TypeV128}},
			exp:  32,
		},
		{
			name:     "param < result / argBegin=2",
			argBegin: 2,
			sig:      &ssa.Signature{Params: []ssa.Type{ssa.TypeI64, ssa.TypeV128, ssa.TypeI32}, Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64}},
			exp:      16,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			requiredSize, _ := goFunctionCallRequiredStackSize(tc.sig, tc.argBegin)
			require.Equal(t, tc.exp, requiredSize)
		})
	}
}

func Test_goFunctionCallLoadStackArg(t *testing.T) {
	originalArg0Reg := x17VReg
	intVReg := x11VReg
	floatVReg := v11VReg
	for _, tc := range []struct {
		name         string
		arg          *backend.ABIArg
		expResultReg regalloc.VReg
		exp          string
	}{
		{
			name:         "i32 / small offset",
			arg:          &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeI32},
			expResultReg: x11VReg,
			exp: `
	add x11, x17, #0x30
	ldr w11, [x11, #0x8]
`,
		},
		{
			name:         "i64 / small offset",
			arg:          &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeI64},
			expResultReg: x11VReg,
			exp: `
	add x11, x17, #0x30
	ldr x11, [x11, #0x8]
`,
		},
		{
			name:         "i32 / large offset",
			arg:          &backend.ABIArg{Offset: 16 * 300, Type: ssa.TypeI32},
			expResultReg: x11VReg,
			exp: `
	movz x27, #0x12c0, lsl 0
	add x11, x17, x27
	ldr w11, [x11, #0x8]
`,
		},
		{
			name:         "i64 / large offset",
			arg:          &backend.ABIArg{Offset: 16 * 300, Type: ssa.TypeI64},
			expResultReg: x11VReg,
			exp: `
	movz x27, #0x12c0, lsl 0
	add x11, x17, x27
	ldr x11, [x11, #0x8]
`,
		},
		{
			name:         "f32 / small offset",
			arg:          &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeF32},
			expResultReg: v11VReg,
			exp: `
	add x11, x17, #0x30
	ldr s11, [x11, #0x8]
`,
		},
		{
			name:         "f64 / small offset",
			arg:          &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeF64},
			expResultReg: v11VReg,
			exp: `
	add x11, x17, #0x30
	ldr d11, [x11, #0x8]
`,
		},
		{
			name:         "f32 / large offset",
			arg:          &backend.ABIArg{Offset: 16 * 300, Type: ssa.TypeF32},
			expResultReg: v11VReg,
			exp: `
	movz x27, #0x12c0, lsl 0
	add x11, x17, x27
	ldr s11, [x11, #0x8]
`,
		},
		{
			name:         "f64 / large offset",
			arg:          &backend.ABIArg{Offset: 16 * 300, Type: ssa.TypeF64},
			expResultReg: v11VReg,
			exp: `
	movz x27, #0x12c0, lsl 0
	add x11, x17, x27
	ldr d11, [x11, #0x8]
`,
		},
		{
			name:         "v128 / small offset",
			arg:          &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeV128},
			expResultReg: v11VReg,
			exp: `
	add x11, x17, #0x30
	ldr q11, [x11, #0x10]
`,
		},
		{
			name:         "v128 / large offset",
			arg:          &backend.ABIArg{Offset: 16 * 300, Type: ssa.TypeV128},
			expResultReg: v11VReg,
			exp: `
	movz x27, #0x12c0, lsl 0
	add x11, x17, x27
	ldr q11, [x11, #0x10]
`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Run(tc.name, func(t *testing.T) {
				_, _, m := newSetupWithMockContext()

				nop := m.allocateNop()
				_, result := m.goFunctionCallLoadStackArg(nop, originalArg0Reg, tc.arg, intVReg, floatVReg)
				require.Equal(t, tc.expResultReg, result)

				m.rootInstr = nop

				fmt.Println(m.Format())
				require.Equal(t, tc.exp, m.Format())

				m.Encode()
				fmt.Println(hex.EncodeToString(m.compiler.Buf()))
			})
		})
	}
}

func Test_goFunctionCallStoreStackResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    *backend.ABIArg
		resultReg regalloc.VReg
		exp       string
	}{
		{
			name:      "i32",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeI32},
			resultReg: x11VReg,
			exp: `
	add x27, sp, #0x38
	str w11, [x27]
`,
		},
		{
			name:      "i32 / large offset",
			result:    &backend.ABIArg{Offset: 16 * 400, Type: ssa.TypeI32},
			resultReg: x11VReg,
			exp: `
	movz x27, #0x1908, lsl 0
	add x27, sp, x27
	str w11, [x27]
`,
		},
		{
			name:      "i64",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeI64},
			resultReg: x11VReg,
			exp: `
	add x27, sp, #0x38
	str x11, [x27]
`,
		},
		{
			name:      "i64 / large offset",
			result:    &backend.ABIArg{Offset: 16 * 400, Type: ssa.TypeI64},
			resultReg: x11VReg,
			exp: `
	movz x27, #0x1908, lsl 0
	add x27, sp, x27
	str x11, [x27]
`,
		},
		{
			name:      "f32",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeF32},
			resultReg: v11VReg,
			exp: `
	add x27, sp, #0x38
	str s11, [x27]
`,
		},
		{
			name:      "f32 / large offset",
			result:    &backend.ABIArg{Offset: 16 * 400, Type: ssa.TypeF32},
			resultReg: v11VReg,
			exp: `
	movz x27, #0x1908, lsl 0
	add x27, sp, x27
	str s11, [x27]
`,
		},
		{
			name:      "f64",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeF64},
			resultReg: v11VReg,
			exp: `
	add x27, sp, #0x38
	str d11, [x27]
`,
		},
		{
			name:      "f64 / large offset",
			result:    &backend.ABIArg{Offset: 16 * 400, Type: ssa.TypeF64},
			resultReg: v11VReg,
			exp: `
	movz x27, #0x1908, lsl 0
	add x27, sp, x27
	str d11, [x27]
`,
		},
		{
			name:      "v128",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeV128},
			resultReg: v11VReg,
			exp: `
	add x27, sp, #0x38
	str q11, [x27]
`,
		},
		{
			name:      "v128 / large offset",
			result:    &backend.ABIArg{Offset: 16 * 400, Type: ssa.TypeV128},
			resultReg: v11VReg,
			exp: `
	movz x27, #0x1908, lsl 0
	add x27, sp, x27
	str q11, [x27]
`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Run(tc.name, func(t *testing.T) {
				_, _, m := newSetupWithMockContext()
				m.currentABI = &abiImpl{argStackSize: 8}

				nop := m.allocateNop()
				m.goFunctionCallStoreStackResult(nop, spVReg, tc.result, tc.resultReg)

				m.rootInstr = nop

				fmt.Println(m.Format())
				require.Equal(t, tc.exp, m.Format())

				m.Encode()
				fmt.Println(hex.EncodeToString(m.compiler.Buf()))
			})
		})
	}
}
