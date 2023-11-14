package arm64

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_lowerConstant(t *testing.T) {
	t.Run("zero i32", func(t *testing.T) {
		ssaB, m := newSetup()
		ssaConstInstr := ssaB.AllocateInstruction()
		ssaConstInstr.AsIconst32(0)
		ssaB.InsertInstruction(ssaConstInstr)

		vr := m.lowerConstant(ssaConstInstr)
		machInstr := getPendingInstr(m)
		require.Equal(t, regalloc.VRegIDNonReservedBegin, vr.ID())
		require.Equal(t, regalloc.RegTypeInt, vr.RegType())
		require.Equal(t, mov64, machInstr.kind)
		require.Equal(t, "mov x128?, xzr", formatEmittedInstructionsInCurrentBlock(m))
	})

	t.Run("zero i64", func(t *testing.T) {
		ssaB, m := newSetup()
		ssaConstInstr := ssaB.AllocateInstruction()
		ssaConstInstr.AsIconst64(0)
		ssaB.InsertInstruction(ssaConstInstr)

		vr := m.lowerConstant(ssaConstInstr)
		machInstr := getPendingInstr(m)
		require.Equal(t, regalloc.VRegIDNonReservedBegin, vr.ID())
		require.Equal(t, regalloc.RegTypeInt, vr.RegType())
		require.Equal(t, mov64, machInstr.kind)
		require.Equal(t, "mov x128?, xzr", formatEmittedInstructionsInCurrentBlock(m))
	})

	t.Run("TypeF32", func(t *testing.T) {
		ssaB, m := newSetup()
		ssaConstInstr := ssaB.AllocateInstruction()
		ssaConstInstr.AsF32const(1.1234)
		ssaB.InsertInstruction(ssaConstInstr)

		vr := m.lowerConstant(ssaConstInstr)
		machInstr := getPendingInstr(m)
		require.Equal(t, regalloc.VRegIDNonReservedBegin, vr.ID())
		require.Equal(t, regalloc.RegTypeFloat, vr.RegType())
		require.Equal(t, loadFpuConst32, machInstr.kind)
		require.Equal(t, uint64(math.Float32bits(1.1234)), machInstr.u1)

		require.Equal(t, "ldr s128?, #8; b 8; data.f32 1.123400", formatEmittedInstructionsInCurrentBlock(m))
	})

	t.Run("TypeF64", func(t *testing.T) {
		ssaB, m := newSetup()
		ssaConstInstr := ssaB.AllocateInstruction()
		ssaConstInstr.AsF64const(-9471.2)
		ssaB.InsertInstruction(ssaConstInstr)

		vr := m.lowerConstant(ssaConstInstr)
		machInstr := getPendingInstr(m)
		require.Equal(t, regalloc.VRegIDNonReservedBegin, vr.ID())
		require.Equal(t, regalloc.RegTypeFloat, vr.RegType())
		require.Equal(t, loadFpuConst64, machInstr.kind)
		require.Equal(t, math.Float64bits(-9471.2), machInstr.u1)

		require.Equal(t, "ldr d128?, #8; b 16; data.f64 -9471.200000", formatEmittedInstructionsInCurrentBlock(m))
	})
}

func TestMachine_lowerConstantI32(t *testing.T) {
	for _, tc := range []struct {
		val uint32
		exp []string
	}{
		{val: 0, exp: []string{"movz w0, #0x0, lsl 0"}},
		{val: 0xffff, exp: []string{"movz w0, #0xffff, lsl 0"}},
		{val: 0xffff_0000, exp: []string{"movz w0, #0xffff, lsl 16"}},
		{val: 0xffff_fffe, exp: []string{"movn w0, #0x1, lsl 0"}},
		{val: 0x2, exp: []string{"orr w0, wzr, #0x2"}},
		{val: 0x80000001, exp: []string{
			"movz w0, #0x1, lsl 0",
			"movk w0, #0x8000, lsl 16",
		}},
		{val: 0xf00000f, exp: []string{
			"movz w0, #0xf, lsl 0",
			"movk w0, #0xf00, lsl 16",
		}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x", tc.val), func(t *testing.T) {
			_, m := newSetup()
			m.lowerConstantI32(regToVReg(x0), int32(tc.val))
			exp := strings.Join(tc.exp, "\n")
			require.Equal(t, exp, formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_lowerConstantI64(t *testing.T) {
	invert := func(v uint64) uint64 { return ^v }
	for _, tc := range []struct {
		val uint64
		exp []string
	}{
		{val: 0x0, exp: []string{"movz x0, #0x0, lsl 0"}},
		{val: 0x1, exp: []string{"orr x0, xzr, #0x1"}},
		{val: 0x3, exp: []string{"orr x0, xzr, #0x3"}},
		{val: 0xfff000, exp: []string{"orr x0, xzr, #0xfff000"}},
		{val: 0x8001 << 16, exp: []string{"movz x0, #0x8001, lsl 16"}},
		{val: 0x8001 << 32, exp: []string{"movz x0, #0x8001, lsl 32"}},
		{val: 0x8001 << 48, exp: []string{"movz x0, #0x8001, lsl 48"}},
		{val: invert(0x8001 << 16), exp: []string{"movn x0, #0x8001, lsl 16"}},
		{val: invert(0x8001 << 32), exp: []string{"movn x0, #0x8001, lsl 32"}},
		{val: invert(0x8001 << 48), exp: []string{"movn x0, #0x8001, lsl 48"}},
		{val: 0x80000001 << 16, exp: []string{
			"movz x0, #0x1, lsl 16",
			"movk x0, #0x8000, lsl 32",
		}},
		{val: 0x40000001, exp: []string{
			"movz x0, #0x1, lsl 0",
			"movk x0, #0x4000, lsl 16",
		}},
		{val: 0xffffffffff001000, exp: []string{
			"movn x0, #0xefff, lsl 0",
			"movk x0, #0xff00, lsl 16",
		}},
		{val: 0xffff0000c466361f, exp: []string{
			"movz x0, #0x361f, lsl 0",
			"movk x0, #0xc466, lsl 16",
			"movk x0, #0xffff, lsl 48",
		}},
		{val: 0x89705f4136b4a598, exp: []string{
			"movz x0, #0xa598, lsl 0",
			"movk x0, #0x36b4, lsl 16",
			"movk x0, #0x5f41, lsl 32",
			"movk x0, #0x8970, lsl 48",
		}},
		{val: 0xffff_0001_0001_0001, exp: []string{
			"movn x0, #0xfffe, lsl 0",
			"movk x0, #0x1, lsl 16",
			"movk x0, #0x1, lsl 32",
		}},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%#x", tc.val), func(t *testing.T) {
			_, m := newSetup()
			m.lowerConstantI64(regToVReg(x0), int64(tc.val))
			exp := strings.Join(tc.exp, "\n")
			require.Equal(t, exp, formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}
