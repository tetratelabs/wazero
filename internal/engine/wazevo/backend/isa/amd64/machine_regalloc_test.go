package amd64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_InsertStoreRegisterAt(t *testing.T) {
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
					i1, i2 := m.allocateInstr().asUD2(), m.allocateInstr().asRet(nil)
					i1.next = i2
					i2.prev = i1

					if after {
						m.InsertStoreRegisterAt(raxVReg, i1, after)
						m.InsertStoreRegisterAt(xmm1VReg, i1, after)
					} else {
						m.InsertStoreRegisterAt(xmm1VReg, i2, after)
						m.InsertStoreRegisterAt(raxVReg, i2, after)
					}
					m.ectx.RootInstr = i1
					require.Equal(t, tc.expected, m.Format())
				})
			}
		})
	}
}

func TestMachine_InsertReloadRegisterAt(t *testing.T) {
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
					i1, i2 := m.allocateInstr().asUD2(), m.allocateInstr().asRet(nil)
					i1.next = i2
					i2.prev = i1

					if after {
						m.InsertReloadRegisterAt(xmm1VReg, i1, after)
						m.InsertReloadRegisterAt(raxVReg, i1, after)
					} else {
						m.InsertReloadRegisterAt(raxVReg, i2, after)
						m.InsertReloadRegisterAt(xmm1VReg, i2, after)
					}
					m.ectx.RootInstr = i1
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
			i1, i2 := m.allocateInstr().asUD2(), m.allocateInstr().asRet(nil)
			i1.next = i2
			i2.prev = i1

			m.InsertMoveBefore(tc.dst, tc.src, i2)
			m.ectx.RootInstr = i1
			require.Equal(t, tc.expected, m.Format())
		})
	}
}
