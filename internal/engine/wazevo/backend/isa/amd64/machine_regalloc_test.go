package amd64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_insertStoreRegisterAt(t *testing.T) {
	for _, tc := range []struct {
		spillSlotSize int64
		expected      string
	}{
		{
			spillSlotSize: 16,
			expected: `
	ud2
	movsd %xmm1, 16(%rsp)
	mov.q %rax, 24(%rsp)
	ret
`,
		},
		{
			spillSlotSize: 160,
			expected: `
	ud2
	movsd %xmm1, 160(%rsp)
	mov.q %rax, 168(%rsp)
	ret
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
					ctx.typeOf = map[regalloc.VRegID]ssa.Type{
						raxVReg.ID(): ssa.TypeI64, xmm1VReg.ID(): ssa.TypeF64,
					}
					i1, i2 := m.allocateInstr().asUD2(), m.allocateInstr().asRet()
					i1.next = i2
					i2.prev = i1

					if after {
						m.insertStoreRegisterAt(raxVReg, i1, after)
						m.insertStoreRegisterAt(xmm1VReg, i1, after)
					} else {
						m.insertStoreRegisterAt(xmm1VReg, i2, after)
						m.insertStoreRegisterAt(raxVReg, i2, after)
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
			spillSlotSize: 16,
			expected: `
	ud2
	movq 16(%rsp), %rax
	movdqu 24(%rsp), %xmm1
	ret
`,
		},
		{
			spillSlotSize: 160,
			expected: `
	ud2
	movq 160(%rsp), %rax
	movdqu 168(%rsp), %xmm1
	ret
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
					ctx.typeOf = map[regalloc.VRegID]ssa.Type{
						raxVReg.ID(): ssa.TypeI64, xmm1VReg.ID(): ssa.TypeV128,
					}
					i1, i2 := m.allocateInstr().asUD2(), m.allocateInstr().asRet()
					i1.next = i2
					i2.prev = i1

					if after {
						m.insertReloadRegisterAt(xmm1VReg, i1, after)
						m.insertReloadRegisterAt(raxVReg, i1, after)
					} else {
						m.insertReloadRegisterAt(raxVReg, i2, after)
						m.insertReloadRegisterAt(xmm1VReg, i2, after)
					}
					m.rootInstr = i1
					require.Equal(t, tc.expected, m.Format())
				})
			}
		})
	}
}

func TestMachine_InsertMoveBefore(t *testing.T) {
	for _, tc := range []struct {
		src, dst regalloc.VReg
		expected string
	}{
		{
			src: raxVReg,
			dst: rdxVReg,
			expected: `
	ud2
	movq %rax, %rdx
	ret
`,
		},
		{
			src: xmm1VReg,
			dst: xmm2VReg,
			expected: `
	ud2
	movdqu %xmm1, %xmm2
	ret
`,
		},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			i1, i2 := m.allocateInstr().asUD2(), m.allocateInstr().asRet()
			i1.next = i2
			i2.prev = i1

			m.insertMoveBefore(tc.dst, tc.src, i2)
			m.rootInstr = i1
			require.Equal(t, tc.expected, m.Format())
		})
	}
}

func TestMachineSwap(t *testing.T) {
	for _, tc := range []struct {
		x1, x2, tmp regalloc.VReg
		expected    string
	}{
		{
			x1:  r15VReg,
			x2:  raxVReg,
			tmp: rdiVReg,
			expected: `
	ud2
	xchg.q %r15, %rax
	exit_sequence %rsi
`,
		},
		{
			x1: r15VReg,
			x2: raxVReg,
			expected: `
	ud2
	xchg.q %r15, %rax
	exit_sequence %rsi
`,
		},
		{
			x1:  xmm1VReg,
			x2:  xmm12VReg,
			tmp: xmm11VReg,
			expected: `
	ud2
	movdqu %xmm1, %xmm11
	movdqu %xmm12, %xmm1
	movdqu %xmm11, %xmm12
	exit_sequence %rsi
`,
		},
		{
			x1: xmm1VReg,
			x2: xmm12VReg,
			// Tmp not given.
			expected: `
	ud2
	movsd %xmm1, (%rsp)
	movdqa %xmm12, %xmm1
	movsd (%rsp), %xmm12
	exit_sequence %rsi
`,
		},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()

			ctx.typeOf = map[regalloc.VRegID]ssa.Type{
				r15VReg.ID(): ssa.TypeI64, raxVReg.ID(): ssa.TypeI64,
				xmm1VReg.ID(): ssa.TypeF64, xmm12VReg.ID(): ssa.TypeF64,
			}
			cur, i2 := m.allocateInstr().asUD2(), m.allocateExitSeq(rsiVReg)
			cur.next = i2
			i2.prev = cur

			m.swap(cur, tc.x1, tc.x2, tc.tmp)
			m.rootInstr = cur

			require.Equal(t, tc.expected, m.Format())

			m.encodeWithoutSSA(cur)
		})
	}
}
