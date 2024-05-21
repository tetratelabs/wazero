package arm64

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestInstruction_String(t *testing.T) {
	for _, tc := range []struct {
		i   *instruction
		exp string
	}{
		{
			i: &instruction{
				kind: condBr,
				u1:   eq.asCond().asUint64(),
				u2:   uint64(label(1)),
			},
			exp: "b.eq L1",
		},
		{
			i: &instruction{
				kind: condBr,
				u1:   ne.asCond().asUint64(),
				u2:   uint64(label(100)),
			},
			exp: "b.ne L100",
		},
		{
			i: &instruction{
				kind: condBr,
				u1:   registerAsRegZeroCond(regToVReg(x0)).asUint64(),
				u2:   uint64(label(100)),
			},
			exp: "cbz w0, (L100)",
		},
		{
			i: &instruction{
				kind: condBr,
				u1:   registerAsRegNotZeroCond(regToVReg(x29)).asUint64(),
				u2:   uint64(label(50)),
			},
			exp: "cbnz w29, L50",
		},
		{
			i: &instruction{
				kind: loadFpuConst32,
				u1:   uint64(math.Float32bits(3.0)),
				rd:   regalloc.VReg(0).SetRegType(regalloc.RegTypeFloat),
			},
			exp: "ldr s0?, #8; b 8; data.f32 3.000000",
		},
		{
			i: &instruction{
				kind: loadFpuConst64,
				u1:   math.Float64bits(12345.987491),
				rd:   regalloc.VReg(0).SetRegType(regalloc.RegTypeFloat),
			},
			exp: "ldr d0?, #8; b 16; data.f64 12345.987491",
		},
		{exp: "nop0", i: &instruction{kind: nop0}},
		{exp: "b L0", i: &instruction{kind: br, u1: uint64(label(0))}},
	} {
		t.Run(tc.exp, func(t *testing.T) { require.Equal(t, tc.exp, tc.i.String()) })
	}
}

func TestInstruction_isCopy(t *testing.T) {
	require.False(t, (&instruction{kind: mov32}).IsCopy())
	require.True(t, (&instruction{kind: mov64}).IsCopy())
	require.True(t, (&instruction{kind: fpuMov64}).IsCopy())
	require.True(t, (&instruction{kind: fpuMov128}).IsCopy())
}
