package arm64

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_insertAtHead(t *testing.T) {
	t.Run("no head", func(t *testing.T) {
		m := &machine{}
		i := &instruction{kind: condBr}
		m.insertAtPerBlockHead(i)
		require.Equal(t, i, m.perBlockHead)
		require.Equal(t, i, m.perBlockEnd)
	})
	t.Run("has head", func(t *testing.T) {
		prevHead := &instruction{kind: br}
		m := &machine{perBlockHead: prevHead, perBlockEnd: prevHead}
		i := &instruction{kind: condBr}
		m.insertAtPerBlockHead(i)
		require.Equal(t, i, m.perBlockHead)
		require.Equal(t, prevHead, m.perBlockEnd)
		require.Equal(t, nil, prevHead.next)
		require.Equal(t, i, prevHead.prev)
		require.Equal(t, prevHead, i.next)
		require.Equal(t, nil, i.prev)
	})
}

func TestMachine_resolveAddressingMode(t *testing.T) {
	t.Run("imm12/arg", func(t *testing.T) {
		m := &machine{}
		i := &instruction{}
		i.asULoad(operandNR(x17VReg), addressMode{
			kind: addressModeKindArgStackSpace,
			rn:   spVReg,
			imm:  128,
		}, 64)
		m.resolveAddressingMode(1024, 0, i)
		require.Equal(t, addressModeKindRegUnsignedImm12, i.amode.kind)
		require.Equal(t, int64(128+1024), i.amode.imm)
	})
	t.Run("imm12/result", func(t *testing.T) {
		m := &machine{}
		i := &instruction{}
		i.asULoad(operandNR(x17VReg), addressMode{
			kind: addressModeKindResultStackSpace,
			rn:   spVReg,
			imm:  128,
		}, 64)
		m.resolveAddressingMode(0, 256, i)
		require.Equal(t, addressModeKindRegUnsignedImm12, i.amode.kind)
		require.Equal(t, int64(128+256), i.amode.imm)
	})

	t.Run("tmp reg", func(t *testing.T) {
		m := &machine{instrPool: wazevoapi.NewPool[instruction](resetInstruction)}
		root := &instruction{kind: udf}
		i := &instruction{prev: root}
		i.asULoad(operandNR(x17VReg), addressMode{
			kind: addressModeKindResultStackSpace,
			rn:   spVReg,
		}, 64)
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
		currentABI: &abiImpl{argStackSize: 180},
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

			originLabelPos := m.allocateLabelPosition(originLabel)
			originLabelPos.begin = cbr
			originLabelPos.end = linkInstr(cbr, end)
			originNextLabelPos := m.allocateLabelPosition(originLabelNext)
			originNextLabelPos.begin = originalEndNext
			linkInstr(originLabelPos.end, originalEndNext)

			m.labelPositions[originLabel] = originLabelPos
			m.labelPositions[originLabelNext] = originNextLabelPos

			m.rootInstr = cbr
			fmt.Println(originLabelPos.begin.String())
			require.Equal(t, tc.expBefore, m.Format())

			m.nextLabel = 9999999
			m.insertConditionalJumpTrampoline(cbr, originLabelPos, originLabelNext)

			fmt.Println(originLabelPos.begin.String())

			require.Equal(t, tc.expAfter, m.Format())
		})
	}
}
