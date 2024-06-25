package arm64

import (
	"context"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_calleeSavedRegistersSorted(t *testing.T) {
	var exp []regalloc.VReg
	regInfo.CalleeSavedRegisters.Range(func(r regalloc.RealReg) {
		exp = append(exp, regInfo.RealRegToVReg[r])
	})
	sort.Slice(exp, func(i, j int) bool {
		return exp[i].RealReg() < exp[j].RealReg()
	})

	require.Equal(t, exp, calleeSavedRegistersSorted)
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
	movz x27, #0xa0, lsl 0
	sub sp, sp, x27
	stp x30, x27, [sp, #-0x10]!
	sub x27, sp, #0x130
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	movz x27, #0x130, lsl 0
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
	add x17, sp, #0x10
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
	ldr q11, [x17], #0x10
	str q11, [x15], #0x10
	str x6, [x15], #0x8
	str x7, [x15], #0x8
	ldr q11, [x17], #0x10
	str q11, [x15], #0x10
	ldr s11, [x17], #0x8
	str d11, [x15], #0x8
	ldr q11, [x17], #0x10
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
	add sp, sp, #0x130
	ldr x30, [sp], #0x10
	add x17, sp, #0x38
	add sp, sp, #0xa0
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
	str s11, [x17], #0x8
	ldr q11, [x15], #0x10
	str q11, [x17], #0x10
	ldr w6, [x15], #0x8
	ldr x7, [x15], #0x8
	ldr q11, [x15], #0x10
	str q11, [x17], #0x10
	ldr s11, [x15], #0x8
	str s11, [x17], #0x8
	ldr q11, [x15], #0x10
	str q11, [x17], #0x10
	ldr w11, [x15], #0x8
	str w11, [x17], #0x8
	ldr x11, [x15], #0x8
	str x11, [x17], #0x8
	ldr q11, [x15], #0x10
	str q11, [x17], #0x10
	ldr s11, [x15], #0x8
	str s11, [x17], #0x8
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
	add x15, sp, #0x10
	add sp, sp, #0x30
	ldr x30, [sp], #0x10
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

			err := m.Encode(context.Background())
			require.NoError(t, err)
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
			name:         "i32",
			arg:          &backend.ABIArg{Type: ssa.TypeI32},
			expResultReg: x11VReg,
			exp: `
	ldr w11, [x17], #0x8
`,
		},
		{
			name:         "i64",
			arg:          &backend.ABIArg{Type: ssa.TypeI64},
			expResultReg: x11VReg,
			exp: `
	ldr x11, [x17], #0x8
`,
		},
		{
			name:         "f32",
			arg:          &backend.ABIArg{Type: ssa.TypeF32},
			expResultReg: v11VReg,
			exp: `
	ldr s11, [x17], #0x8
`,
		},
		{
			name:         "f64",
			arg:          &backend.ABIArg{Type: ssa.TypeF64},
			expResultReg: v11VReg,
			exp: `
	ldr d11, [x17], #0x8
`,
		},
		{
			name:         "v128",
			arg:          &backend.ABIArg{Type: ssa.TypeV128},
			expResultReg: v11VReg,
			exp: `
	ldr q11, [x17], #0x10
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

				require.Equal(t, tc.exp, m.Format())
				err := m.Encode(context.Background())
				require.NoError(t, err)
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
	str w11, [sp], #0x8
`,
		},
		{
			name:      "i64",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeI64},
			resultReg: x11VReg,
			exp: `
	str x11, [sp], #0x8
`,
		},
		{
			name:      "f32",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeF32},
			resultReg: v11VReg,
			exp: `
	str s11, [sp], #0x8
`,
		},
		{
			name:      "f64",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeF64},
			resultReg: v11VReg,
			exp: `
	str d11, [sp], #0x8
`,
		},
		{
			name:      "v128",
			result:    &backend.ABIArg{Offset: 16 * 3, Type: ssa.TypeV128},
			resultReg: v11VReg,
			exp: `
	str q11, [sp], #0x10
`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Run(tc.name, func(t *testing.T) {
				_, _, m := newSetupWithMockContext()
				m.currentABI = &backend.FunctionABI{ArgStackSize: 8}

				nop := m.allocateNop()
				m.goFunctionCallStoreStackResult(nop, spVReg, tc.result, tc.resultReg)

				m.rootInstr = nop

				require.Equal(t, tc.exp, m.Format())
				err := m.Encode(context.Background())
				require.NoError(t, err)
			})
		})
	}
}
