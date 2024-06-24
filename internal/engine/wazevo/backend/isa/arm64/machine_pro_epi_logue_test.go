package arm64

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_setupPrologue(t *testing.T) {
	for _, tc := range []struct {
		spillSlotSize int64
		clobberedRegs []regalloc.VReg
		exp           string
		abi           backend.FunctionABI
	}{
		{
			spillSlotSize: 0,
			exp: `
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	udf
`,
		},
		{
			spillSlotSize: 0,
			abi:           backend.FunctionABI{ArgStackSize: 16, RetStackSize: 16},
			exp: `
	orr x27, xzr, #0x20
	sub sp, sp, x27
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	udf
`,
		},
		{
			spillSlotSize: 16,
			exp: `
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x10
	orr x27, xzr, #0x10
	str x27, [sp, #-0x10]!
	udf
`,
		},
		{
			spillSlotSize: 0,
			clobberedRegs: []regalloc.VReg{v18VReg, v19VReg, x18VReg, x25VReg},
			exp: `
	stp x30, xzr, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	str x18, [sp, #-0x10]!
	str x25, [sp, #-0x10]!
	orr x27, xzr, #0x40
	str x27, [sp, #-0x10]!
	udf
`,
		},
		{
			spillSlotSize: 320,
			clobberedRegs: []regalloc.VReg{v18VReg, v19VReg, x18VReg, x25VReg},
			exp: `
	stp x30, xzr, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	str x18, [sp, #-0x10]!
	str x25, [sp, #-0x10]!
	sub sp, sp, #0x140
	orr x27, xzr, #0x180
	str x27, [sp, #-0x10]!
	udf
`,
		},
		{
			spillSlotSize: 320,
			abi:           backend.FunctionABI{ArgStackSize: 320, RetStackSize: 160},
			clobberedRegs: []regalloc.VReg{v18VReg, v19VReg, x18VReg, x25VReg},
			exp: `
	orr x27, xzr, #0x1e0
	sub sp, sp, x27
	stp x30, x27, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	str x18, [sp, #-0x10]!
	str x25, [sp, #-0x10]!
	sub sp, sp, #0x140
	orr x27, xzr, #0x180
	str x27, [sp, #-0x10]!
	udf
`,
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.DisableStackCheck()
			m.spillSlotSize = tc.spillSlotSize
			m.clobberedRegs = tc.clobberedRegs
			m.currentABI = &tc.abi

			root := m.allocateNop()
			m.rootInstr = root
			udf := m.allocateInstr()
			udf.asUDF()
			root.next = udf
			udf.prev = root

			m.setupPrologue()
			require.Equal(t, root, m.rootInstr)
			err := m.Encode(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_postRegAlloc(t *testing.T) {
	for _, tc := range []struct {
		exp           string
		abi           backend.FunctionABI
		clobberedRegs []regalloc.VReg
		spillSlotSize int64
	}{
		{
			exp: `
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
			spillSlotSize: 0,
			clobberedRegs: nil,
		},
		{
			exp: `
	add sp, sp, #0x10
	add sp, sp, #0x50
	ldr x30, [sp], #0x10
	ret
`,
			spillSlotSize: 16 * 5,
			clobberedRegs: nil,
		},
		{
			exp: `
	add sp, sp, #0x10
	add sp, sp, #0x50
	ldr x30, [sp], #0x10
	add sp, sp, #0x20
	ret
`,
			abi:           backend.FunctionABI{ArgStackSize: 16, RetStackSize: 16},
			spillSlotSize: 16 * 5,
			clobberedRegs: nil,
		},
		{
			exp: `
	add sp, sp, #0x10
	ldr q27, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
			clobberedRegs: []regalloc.VReg{v18VReg, v27VReg},
		},
		{
			exp: `
	add sp, sp, #0x10
	ldr x25, [sp], #0x10
	ldr x18, [sp], #0x10
	ldr q27, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
			clobberedRegs: []regalloc.VReg{v18VReg, v27VReg, x18VReg, x25VReg},
		},
		{
			exp: `
	add sp, sp, #0x10
	add sp, sp, #0xa0
	ldr x25, [sp], #0x10
	ldr x18, [sp], #0x10
	ldr q27, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
			spillSlotSize: 16 * 10,
			clobberedRegs: []regalloc.VReg{v18VReg, v27VReg, x18VReg, x25VReg},
		},
		{
			exp: `
	add sp, sp, #0x10
	add sp, sp, #0xa0
	ldr x25, [sp], #0x10
	ldr x18, [sp], #0x10
	ldr q27, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x30, [sp], #0x10
	add sp, sp, #0x150
	ret
`,
			spillSlotSize: 16 * 10,
			abi:           backend.FunctionABI{ArgStackSize: 16, RetStackSize: 320},
			clobberedRegs: []regalloc.VReg{v18VReg, v27VReg, x18VReg, x25VReg},
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.spillSlotSize = tc.spillSlotSize
			m.clobberedRegs = tc.clobberedRegs
			m.currentABI = &tc.abi

			root := m.allocateNop()
			m.rootInstr = root
			ret := m.allocateInstr()
			ret.asRet()
			root.next = ret
			ret.prev = root
			m.postRegAlloc()

			require.Equal(t, root, m.rootInstr)
			err := m.Encode(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_insertStackBoundsCheck(t *testing.T) {
	for _, tc := range []struct {
		exp               string
		requiredStackSize int64
	}{
		{
			requiredStackSize: 0xfff_0,
			exp: `
	movz x27, #0xfff0, lsl 0
	sub x27, sp, x27
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	movz x27, #0xfff0, lsl 0
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
`,
		},
		{
			requiredStackSize: 0x10,
			exp: `
	sub x27, sp, #0x10
	ldr x11, [x0, #0x28]
	subs xzr, x27, x11
	b.ge #0x14
	orr x27, xzr, #0x10
	str x27, [x0, #0x40]
	ldr x27, [x0, #0x50]
	bl x27
`,
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.rootInstr = m.allocateInstr()
			m.rootInstr.asNop0()
			m.insertStackBoundsCheck(tc.requiredStackSize, m.rootInstr)
			err := m.Encode(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_CompileStackGrowCallSequence(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	_ = m.CompileStackGrowCallSequence()

	require.Equal(t, `
	str x1, [x0, #0x60]
	str x2, [x0, #0x70]
	str x3, [x0, #0x80]
	str x4, [x0, #0x90]
	str x5, [x0, #0xa0]
	str x6, [x0, #0xb0]
	str x7, [x0, #0xc0]
	str x19, [x0, #0xd0]
	str x20, [x0, #0xe0]
	str x21, [x0, #0xf0]
	str x22, [x0, #0x100]
	str x23, [x0, #0x110]
	str x24, [x0, #0x120]
	str x25, [x0, #0x130]
	str x26, [x0, #0x140]
	str x28, [x0, #0x150]
	str x30, [x0, #0x160]
	str q0, [x0, #0x170]
	str q1, [x0, #0x180]
	str q2, [x0, #0x190]
	str q3, [x0, #0x1a0]
	str q4, [x0, #0x1b0]
	str q5, [x0, #0x1c0]
	str q6, [x0, #0x1d0]
	str q7, [x0, #0x1e0]
	str q18, [x0, #0x1f0]
	str q19, [x0, #0x200]
	str q20, [x0, #0x210]
	str q21, [x0, #0x220]
	str q22, [x0, #0x230]
	str q23, [x0, #0x240]
	str q24, [x0, #0x250]
	str q25, [x0, #0x260]
	str q26, [x0, #0x270]
	str q27, [x0, #0x280]
	str q28, [x0, #0x290]
	str q29, [x0, #0x2a0]
	str q30, [x0, #0x2b0]
	str q31, [x0, #0x2c0]
	mov x27, sp
	str x27, [x0, #0x38]
	orr w17, wzr, #0x1
	str w17, [x0]
	adr x27, #0x20
	str x27, [x0, #0x30]
	exit_sequence x0
	ldr x1, [x0, #0x60]
	ldr x2, [x0, #0x70]
	ldr x3, [x0, #0x80]
	ldr x4, [x0, #0x90]
	ldr x5, [x0, #0xa0]
	ldr x6, [x0, #0xb0]
	ldr x7, [x0, #0xc0]
	ldr x19, [x0, #0xd0]
	ldr x20, [x0, #0xe0]
	ldr x21, [x0, #0xf0]
	ldr x22, [x0, #0x100]
	ldr x23, [x0, #0x110]
	ldr x24, [x0, #0x120]
	ldr x25, [x0, #0x130]
	ldr x26, [x0, #0x140]
	ldr x28, [x0, #0x150]
	ldr x30, [x0, #0x160]
	ldr q0, [x0, #0x170]
	ldr q1, [x0, #0x180]
	ldr q2, [x0, #0x190]
	ldr q3, [x0, #0x1a0]
	ldr q4, [x0, #0x1b0]
	ldr q5, [x0, #0x1c0]
	ldr q6, [x0, #0x1d0]
	ldr q7, [x0, #0x1e0]
	ldr q18, [x0, #0x1f0]
	ldr q19, [x0, #0x200]
	ldr q20, [x0, #0x210]
	ldr q21, [x0, #0x220]
	ldr q22, [x0, #0x230]
	ldr q23, [x0, #0x240]
	ldr q24, [x0, #0x250]
	ldr q25, [x0, #0x260]
	ldr q26, [x0, #0x270]
	ldr q27, [x0, #0x280]
	ldr q28, [x0, #0x290]
	ldr q29, [x0, #0x2a0]
	ldr q30, [x0, #0x2b0]
	ldr q31, [x0, #0x2c0]
	ret
`, m.Format())
}
