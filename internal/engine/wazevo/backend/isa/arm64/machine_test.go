package arm64

import (
	"testing"

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
		// 0x89705f4136b4a598
		m := &machine{instrPool: wazevoapi.NewPool[instruction]()}
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
	movz x27, #0x1, LSL 0
	movk x27, #0x4000, LSL 16
	ldr x17, [sp, x27]
`, m.Format())
	})
}
