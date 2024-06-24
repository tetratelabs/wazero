package arm64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestRegAllocFunctionImpl_ReloadRegisterAfter(t *testing.T) {
	ctx, _, m := newSetupWithMockContext()

	ctx.typeOf = map[regalloc.VRegID]ssa.Type{x1VReg.ID(): ssa.TypeI64, v1VReg.ID(): ssa.TypeF64}
	i1, i2 := m.allocateNop(), m.allocateNop()
	i1.next = i2
	i2.prev = i1

	m.insertReloadRegisterAt(x1VReg, i1, true)
	m.insertReloadRegisterAt(v1VReg, i1, true)

	require.NotEqual(t, i1, i2.prev)
	require.NotEqual(t, i1.next, i2)
	fload, iload := i1.next, i1.next.next
	require.Equal(t, fload.prev, i1)
	require.Equal(t, i1, fload.prev)
	require.Equal(t, iload.next, i2)
	require.Equal(t, iload, i2.prev)

	require.Equal(t, iload.kind, uLoad64)
	require.Equal(t, fload.kind, fpuLoad64)

	m.rootInstr = i1
	require.Equal(t, `
	ldr d1, [sp, #0x18]
	ldr x1, [sp, #0x10]
`, m.Format())
}

func TestRegAllocFunctionImpl_StoreRegisterBefore(t *testing.T) {
	ctx, _, m := newSetupWithMockContext()

	ctx.typeOf = map[regalloc.VRegID]ssa.Type{x1VReg.ID(): ssa.TypeI64, v1VReg.ID(): ssa.TypeF64}
	i1, i2 := m.allocateNop(), m.allocateNop()
	i1.next = i2
	i2.prev = i1

	m.insertStoreRegisterAt(x1VReg, i2, false)
	m.insertStoreRegisterAt(v1VReg, i2, false)

	require.NotEqual(t, i1, i2.prev)
	require.NotEqual(t, i1.next, i2)
	iload, fload := i1.next, i1.next.next
	require.Equal(t, iload.prev, i1)
	require.Equal(t, i1, iload.prev)
	require.Equal(t, fload.next, i2)
	require.Equal(t, fload, i2.prev)

	require.Equal(t, iload.kind, store64)
	require.Equal(t, fload.kind, fpuStore64)

	m.rootInstr = i1
	require.Equal(t, `
	str x1, [sp, #0x10]
	str d1, [sp, #0x18]
`, m.Format())
}

func TestMachine_insertStoreRegisterAt(t *testing.T) {
	for _, tc := range []struct {
		spillSlotSize int64
		expected      string
	}{
		{
			spillSlotSize: 0,
			expected: `
	udf
	str x1, [sp, #0x10]
	str d1, [sp, #0x18]
	exit_sequence x30
`,
		},
		{
			spillSlotSize: 0xffff,
			expected: `
	udf
	movz x27, #0xf, lsl 0
	movk x27, #0x1, lsl 16
	str x1, [sp, x27]
	movz x27, #0x17, lsl 0
	movk x27, #0x1, lsl 16
	str d1, [sp, x27]
	exit_sequence x30
`,
		},
		{
			spillSlotSize: 0xffff_00,
			expected: `
	udf
	movz x27, #0xff10, lsl 0
	movk x27, #0xff, lsl 16
	str x1, [sp, x27]
	movz x27, #0xff18, lsl 0
	movk x27, #0xff, lsl 16
	str d1, [sp, x27]
	exit_sequence x30
`,
		},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			m.spillSlotSize = tc.spillSlotSize

			for _, after := range []bool{false, true} {
				var name string
				if after {
					name = "after"
				} else {
					name = "before"
				}
				t.Run(name, func(t *testing.T) {
					ctx.typeOf = map[regalloc.VRegID]ssa.Type{x1VReg.ID(): ssa.TypeI64, v1VReg.ID(): ssa.TypeF64}
					i1, i2 := m.allocateInstr().asUDF(), m.allocateInstr().asExitSequence(x30VReg)
					i1.next = i2
					i2.prev = i1

					if after {
						m.insertStoreRegisterAt(v1VReg, i1, after)
						m.insertStoreRegisterAt(x1VReg, i1, after)
					} else {
						m.insertStoreRegisterAt(x1VReg, i2, after)
						m.insertStoreRegisterAt(v1VReg, i2, after)
					}
					m.rootInstr = i1
					require.Equal(t, tc.expected, m.Format())
				})
			}
		})
	}
}

func TestMachine_insertReloadRegisterAt(t *testing.T) {
	for _, tc := range []struct {
		spillSlotSize int64
		expected      string
	}{
		{
			spillSlotSize: 0,
			expected: `
	udf
	ldr x1, [sp, #0x10]
	ldr d1, [sp, #0x18]
	exit_sequence x30
`,
		},
		{
			spillSlotSize: 0xffff,
			expected: `
	udf
	movz x27, #0xf, lsl 0
	movk x27, #0x1, lsl 16
	ldr x1, [sp, x27]
	movz x27, #0x17, lsl 0
	movk x27, #0x1, lsl 16
	ldr d1, [sp, x27]
	exit_sequence x30
`,
		},
		{
			spillSlotSize: 0xffff_00,
			expected: `
	udf
	movz x27, #0xff10, lsl 0
	movk x27, #0xff, lsl 16
	ldr x1, [sp, x27]
	movz x27, #0xff18, lsl 0
	movk x27, #0xff, lsl 16
	ldr d1, [sp, x27]
	exit_sequence x30
`,
		},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			m.spillSlotSize = tc.spillSlotSize

			for _, after := range []bool{false, true} {
				var name string
				if after {
					name = "after"
				} else {
					name = "before"
				}
				t.Run(name, func(t *testing.T) {
					ctx.typeOf = map[regalloc.VRegID]ssa.Type{x1VReg.ID(): ssa.TypeI64, v1VReg.ID(): ssa.TypeF64}
					i1, i2 := m.allocateInstr().asUDF(), m.allocateInstr().asExitSequence(x30VReg)
					i1.next = i2
					i2.prev = i1

					if after {
						m.insertReloadRegisterAt(v1VReg, i1, after)
						m.insertReloadRegisterAt(x1VReg, i1, after)
					} else {
						m.insertReloadRegisterAt(x1VReg, i2, after)
						m.insertReloadRegisterAt(v1VReg, i2, after)
					}
					m.rootInstr = i1

					require.Equal(t, tc.expected, m.Format())
				})
			}
		})
	}
}

func TestRegMachine_ClobberedRegisters(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	m.regAllocFn.ClobberedRegisters([]regalloc.VReg{v19VReg, v19VReg, v19VReg, v19VReg})
	require.Equal(t, []regalloc.VReg{v19VReg, v19VReg, v19VReg, v19VReg}, m.clobberedRegs)
}

func TestMachineMachineswap(t *testing.T) {
	for _, tc := range []struct {
		x1, x2, tmp regalloc.VReg
		expected    string
	}{
		{
			x1:  x18VReg,
			x2:  x19VReg,
			tmp: x20VReg,
			expected: `
	udf
	mov x20, x18
	mov x18, x19
	mov x19, x20
	exit_sequence x30
`,
		},
		{
			x1: x18VReg,
			x2: x19VReg,
			// Tmp not given.
			expected: `
	udf
	mov x27, x18
	mov x18, x19
	mov x19, x27
	exit_sequence x30
`,
		},
		{
			x1:  v18VReg,
			x2:  v19VReg,
			tmp: v11VReg,
			expected: `
	udf
	mov v11.16b, v18.16b
	mov v18.16b, v19.16b
	mov v19.16b, v11.16b
	exit_sequence x30
`,
		},
		{
			x1: v18VReg,
			x2: v19VReg,
			// Tmp not given.
			expected: `
	udf
	str d18, [sp, #0x10]
	mov v18.16b, v19.16b
	ldr d19, [sp, #0x10]
	exit_sequence x30
`,
		},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()

			ctx.typeOf = map[regalloc.VRegID]ssa.Type{
				x18VReg.ID(): ssa.TypeI64, x19VReg.ID(): ssa.TypeI64,
				v18VReg.ID(): ssa.TypeF64, v19VReg.ID(): ssa.TypeF64,
			}
			cur, i2 := m.allocateInstr().asUDF(), m.allocateInstr().asExitSequence(x30VReg)
			cur.next = i2
			i2.prev = cur

			m.swap(cur, tc.x1, tc.x2, tc.tmp)
			m.rootInstr = cur

			require.Equal(t, tc.expected, m.Format())
		})
	}
}
