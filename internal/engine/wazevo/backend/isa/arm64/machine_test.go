package arm64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_resolveAddressingMode(t *testing.T) {
	m := NewBackend().(*machine)
	t.Run("imm12/arg", func(t *testing.T) {
		i := &instruction{}
		amode := m.amodePool.Allocate()
		*amode = addressMode{
			kind: addressModeKindArgStackSpace,
			rn:   spVReg,
			imm:  128,
		}
		i.asULoad(x17VReg, amode, 64)
		m.resolveAddressingMode(1024, 0, i)
		require.Equal(t, addressModeKindRegUnsignedImm12, i.getAmode().kind)
		require.Equal(t, int64(128+1024), i.getAmode().imm)
	})
	t.Run("imm12/result", func(t *testing.T) {
		i := &instruction{}
		amode := m.amodePool.Allocate()
		*amode = addressMode{
			kind: addressModeKindResultStackSpace,
			rn:   spVReg,
			imm:  128,
		}
		i.asULoad(x17VReg, amode, 64)
		m.resolveAddressingMode(0, 256, i)
		require.Equal(t, addressModeKindRegUnsignedImm12, i.getAmode().kind)
		require.Equal(t, int64(128+256), i.getAmode().imm)
	})

	t.Run("tmp reg", func(t *testing.T) {
		root := &instruction{kind: udf}
		i := &instruction{prev: root}
		amode := m.amodePool.Allocate()
		*amode = addressMode{
			kind: addressModeKindResultStackSpace,
			rn:   spVReg,
		}
		i.asULoad(x17VReg, amode, 64)
		m.resolveAddressingMode(0, 0x40000001, i)

		m.rootInstr = root
		require.Equal(t, `
	udf
	movz x27, #0x1, lsl 0
	movk x27, #0x4000, lsl 16
	ldr x17, [sp, x27]
`, m.Format())
	})
}

func TestMachine_clobberedRegSlotSize(t *testing.T) {
	m := &machine{clobberedRegs: make([]regalloc.VReg, 10)}
	require.Equal(t, int64(160), m.clobberedRegSlotSize())
}

func TestMachine_frameSize(t *testing.T) {
	m := &machine{clobberedRegs: make([]regalloc.VReg, 10), spillSlotSize: 16 * 8}
	require.Equal(t, int64(16*18), m.frameSize())
}

func TestMachine_requiredStackSize(t *testing.T) {
	m := &machine{
		clobberedRegs: make([]regalloc.VReg, 10), spillSlotSize: 16 * 8,
		maxRequiredStackSizeForCalls: 320,
	}
	require.Equal(t, int64(16*18)+int64(320)+32, m.requiredStackSize())
}

func TestMachine_arg0OffsetFromSP(t *testing.T) {
	m := &machine{clobberedRegs: make([]regalloc.VReg, 10), spillSlotSize: 16 * 8}
	require.Equal(t, int64(16*18)+32, m.arg0OffsetFromSP())
}

func TestMachine_ret0OffsetFromSP(t *testing.T) {
	m := &machine{
		clobberedRegs: make([]regalloc.VReg, 10), spillSlotSize: 16 * 8,
		currentABI: &backend.FunctionABI{ArgStackSize: 180},
	}
	require.Equal(t, int64(16*18)+32+180, m.ret0OffsetFromSP())
}

func TestMachine_getVRegSpillSlotOffsetFromSP(t *testing.T) {
	m := &machine{spillSlots: make(map[regalloc.VRegID]int64)}
	id := regalloc.VRegID(1)
	offset := m.getVRegSpillSlotOffsetFromSP(id, 8)
	require.Equal(t, int64(16), offset)
	require.Equal(t, int64(8), m.spillSlotSize)
	_, ok := m.spillSlots[id]
	require.True(t, ok)

	id = 100
	offset = m.getVRegSpillSlotOffsetFromSP(id, 16)
	require.Equal(t, int64(16+8), offset)
	require.Equal(t, int64(24), m.spillSlotSize)
	_, ok = m.spillSlots[id]
	require.True(t, ok)
}

func TestMachine_insertConditionalJumpTrampoline(t *testing.T) {
	for _, tc := range []struct {
		brAtEnd             bool
		expBefore, expAfter string
	}{
		{
			brAtEnd: true,
			expBefore: `
L100:
	b.eq L12345
	b L888888888
L200:
	exit_sequence x0
`,
			expAfter: `
L100:
	b.eq L10000000
	b L888888888
L10000000:
	b L12345
L200:
	exit_sequence x0
`,
		},
		{
			brAtEnd: false,
			expBefore: `
L100:
	b.eq L12345
	udf
L200:
	exit_sequence x0
`,
			expAfter: `
L100:
	b.eq L10000000
	udf
	b L200
L10000000:
	b L12345
L200:
	exit_sequence x0
`,
		},
	} {
		var name string
		if tc.brAtEnd {
			name = "brAtEnd"
		} else {
			name = "brNotAtEnd"
		}

		t.Run(name, func(t *testing.T) {
			m := NewBackend().(*machine)
			m.maxSSABlockID, m.nextLabel = 0, 10000000
			const (
				originLabel     = 100
				originLabelNext = 200
				targetLabel     = 12345
			)

			cbr := m.allocateInstr()
			cbr.asCondBr(eq.asCond(), targetLabel, false)

			end := m.allocateInstr()
			if tc.brAtEnd {
				end.asBr(888888888)
			} else {
				end.asUDF()
			}

			originalEndNext := m.allocateInstr()
			originalEndNext.asExitSequence(x0VReg)

			originLabelPos := m.labelPositionPool.GetOrAllocate(originLabel)
			originLabelPos.begin = cbr
			originLabelPos.end = linkInstr(cbr, end)
			originNextLabelPos := m.labelPositionPool.GetOrAllocate(originLabelNext)
			originNextLabelPos.begin = originalEndNext
			linkInstr(originLabelPos.end, originalEndNext)

			m.rootInstr = cbr
			require.Equal(t, tc.expBefore, m.Format())

			m.insertConditionalJumpTrampoline(cbr, originLabelPos, originLabelNext)

			require.Equal(t, tc.expAfter, m.Format())

			// The original label position should be updated to the unconditional jump to the original target destination.
			require.Equal(t, "b L12345", originLabelPos.end.String())
		})
	}
}
