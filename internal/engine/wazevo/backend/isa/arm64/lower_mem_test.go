package arm64

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAddressMode_format(t *testing.T) {
	t.Run("addressModeKindRegScaledExtended", func(t *testing.T) {
		require.Equal(t,
			"[x1, w0, UXTW #0x1]",
			addressMode{
				kind:  addressModeKindRegScaledExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(16),
		)
		require.Equal(t,
			"[x1, w0, SXTW #0x1]",
			addressMode{
				kind:  addressModeKindRegScaledExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(16),
		)
		require.Equal(t,
			"[x1, w0, UXTW #0x2]",
			addressMode{
				kind:  addressModeKindRegScaledExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(32),
		)
		require.Equal(t,
			"[x1, w0, SXTW #0x2]",
			addressMode{
				kind:  addressModeKindRegScaledExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(32),
		)
		require.Equal(t,
			"[x1, w0, UXTW #0x3]",
			addressMode{
				kind:  addressModeKindRegScaledExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(64),
		)
		require.Equal(t,
			"[x1, w0, SXTW #0x3]",
			addressMode{
				kind:  addressModeKindRegScaledExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(64),
		)
	})
	t.Run("addressModeKindRegScaled", func(t *testing.T) {
		require.Equal(t,
			"[x1, w0, lsl #0x1]",
			addressMode{
				kind:  addressModeKindRegScaled,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(16),
		)
		require.Equal(t,
			"[x1, w0, lsl #0x1]",
			addressMode{
				kind:  addressModeKindRegScaled,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(16),
		)
		require.Equal(t,
			"[x1, w0, lsl #0x2]",
			addressMode{
				kind:  addressModeKindRegScaled,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(32),
		)
		require.Equal(t,
			"[x1, w0, lsl #0x2]",
			addressMode{
				kind:  addressModeKindRegScaled,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(32),
		)
		require.Equal(t,
			"[x1, w0, lsl #0x3]",
			addressMode{
				kind:  addressModeKindRegScaled,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(64),
		)
		require.Equal(t,
			"[x1, w0, lsl #0x3]",
			addressMode{
				kind:  addressModeKindRegScaled,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(64),
		)
	})
	t.Run("addressModeKindRegExtended", func(t *testing.T) {
		require.Equal(t,
			"[x1, w0, UXTW]",
			addressMode{
				kind:  addressModeKindRegExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpUXTW,
				imm:   0,
			}.format(16),
		)
		require.Equal(t,
			"[x1, w0, SXTW]",
			addressMode{
				kind:  addressModeKindRegExtended,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x0, regalloc.RegTypeInt),
				extOp: extendOpSXTW,
				imm:   0,
			}.format(64),
		)
	})
	t.Run("addressModeKindRegReg", func(t *testing.T) {
		require.Equal(t,
			"[x1, w29]",
			addressMode{
				kind:  addressModeKindRegReg,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x29, regalloc.RegTypeInt),
				extOp: extendOpUXTW, // To indicate that the index reg is 32-bit.
				imm:   0,
			}.format(64),
		)
		require.Equal(t,
			"[x1, x29]",
			addressMode{
				kind:  addressModeKindRegReg,
				rn:    regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				rm:    regalloc.FromRealReg(x29, regalloc.RegTypeInt),
				extOp: extendOpUXTX, // To indicate that the index reg is 64-bit.
				imm:   0,
			}.format(64),
		)
	})
	t.Run("addressModeKindRegSignedImm9", func(t *testing.T) {
		require.Equal(t,
			"[x1]",
			addressMode{
				kind: addressModeKindRegSignedImm9,
				rn:   regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				imm:  0,
			}.format(64),
		)
		require.Equal(t,
			"[x1, #-0x100]",
			addressMode{
				kind: addressModeKindRegSignedImm9,
				rn:   regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				imm:  math.MinInt8 << 1,
			}.format(64),
		)
		require.Equal(t,
			"[x1, #0xff]",
			addressMode{
				kind: addressModeKindRegSignedImm9,
				rn:   regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				imm:  (math.MaxInt8 << 1) + 1,
			}.format(64),
		)
	})

	t.Run("addressModeKindRegUnsignedImm12", func(t *testing.T) {
		require.Equal(t,
			"[x1]",
			addressMode{
				kind: addressModeKindRegUnsignedImm12,
				rn:   regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				imm:  0,
			}.format(64),
		)
		require.Equal(t,
			"[x1, #0xfff]",
			addressMode{
				kind: addressModeKindRegUnsignedImm12,
				rn:   regalloc.FromRealReg(x1, regalloc.RegTypeInt),
				imm:  4095,
			}.format(64),
		)
	})
}

func Test_offsetFitsInAddressModeKindRegUnsignedImm12(t *testing.T) {
	for _, tc := range []struct {
		dstSizeInBits byte
		offset        int64
		exp           bool
	}{
		{dstSizeInBits: 8, offset: -1, exp: false},
		{dstSizeInBits: 8, offset: 0, exp: false},
		{dstSizeInBits: 8, offset: 1, exp: true},
		{dstSizeInBits: 8, offset: 2, exp: true},
		{dstSizeInBits: 8, offset: 3, exp: true},
		{dstSizeInBits: 8, offset: 4095, exp: true},
		{dstSizeInBits: 8, offset: 4096, exp: false},
		{dstSizeInBits: 16, offset: -2, exp: false},
		{dstSizeInBits: 16, offset: -1, exp: false},
		{dstSizeInBits: 16, offset: 0, exp: false},
		{dstSizeInBits: 16, offset: 1, exp: false},
		{dstSizeInBits: 16, offset: 2, exp: true},
		{dstSizeInBits: 16, offset: 3, exp: false},
		{dstSizeInBits: 16, offset: 4095, exp: false},
		{dstSizeInBits: 16, offset: 4096, exp: true},
		{dstSizeInBits: 16, offset: 4095 * 2, exp: true},
		{dstSizeInBits: 16, offset: 4095*2 + 1, exp: false},
		{dstSizeInBits: 32, offset: -4, exp: false},
		{dstSizeInBits: 32, offset: -1, exp: false},
		{dstSizeInBits: 32, offset: 0, exp: false},
		{dstSizeInBits: 32, offset: 1, exp: false},
		{dstSizeInBits: 32, offset: 2, exp: false},
		{dstSizeInBits: 32, offset: 3, exp: false},
		{dstSizeInBits: 32, offset: 4, exp: true},
		{dstSizeInBits: 32, offset: 4095, exp: false},
		{dstSizeInBits: 32, offset: 4096, exp: true},
		{dstSizeInBits: 32, offset: 4095 * 4, exp: true},
		{dstSizeInBits: 32, offset: 4095*4 + 1, exp: false},
		{dstSizeInBits: 64, offset: -8, exp: false},
		{dstSizeInBits: 64, offset: -1, exp: false},
		{dstSizeInBits: 64, offset: 0, exp: false},
		{dstSizeInBits: 64, offset: 1, exp: false},
		{dstSizeInBits: 64, offset: 2, exp: false},
		{dstSizeInBits: 64, offset: 3, exp: false},
		{dstSizeInBits: 64, offset: 4, exp: false},
		{dstSizeInBits: 64, offset: 8, exp: true},
		{dstSizeInBits: 64, offset: 4095, exp: false},
		{dstSizeInBits: 64, offset: 4096, exp: true},
		{dstSizeInBits: 64, offset: 4095 * 8, exp: true},
		{dstSizeInBits: 64, offset: 4095*8 + 1, exp: false},
	} {
		require.Equal(
			t, tc.exp,
			offsetFitsInAddressModeKindRegUnsignedImm12(tc.dstSizeInBits, tc.offset),
			fmt.Sprintf("dstSizeInBits=%d, offset=%d", tc.dstSizeInBits, tc.offset),
		)
	}
}

func Test_offsetFitsInAddressModeKindRegSignedImm9(t *testing.T) {
	require.Equal(t, true, offsetFitsInAddressModeKindRegSignedImm9(0))
	require.Equal(t, false, offsetFitsInAddressModeKindRegSignedImm9(-257))
	require.Equal(t, true, offsetFitsInAddressModeKindRegSignedImm9(-256))
	require.Equal(t, true, offsetFitsInAddressModeKindRegSignedImm9(255))
	require.Equal(t, false, offsetFitsInAddressModeKindRegSignedImm9(256))
}

func TestMachine_collectAddends(t *testing.T) {
	v1000, v2000 := regalloc.VReg(1000).SetRegType(regalloc.RegTypeInt), regalloc.VReg(2000).SetRegType(regalloc.RegTypeInt)
	addParam := func(ctx *mockCompiler, b ssa.Builder, typ ssa.Type) ssa.Value {
		p := b.CurrentBlock().AddParam(b, typ)
		ctx.vRegMap[p] = v1000
		ctx.definitions[p] = backend.SSAValueDefinition{V: p}
		return p
	}
	insertI32Const := func(m *mockCompiler, b ssa.Builder, v uint32) *ssa.Instruction {
		inst := b.AllocateInstruction()
		inst.AsIconst32(v)
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = backend.SSAValueDefinition{Instr: inst}
		return inst
	}
	insertI64Const := func(m *mockCompiler, b ssa.Builder, v uint64) *ssa.Instruction {
		inst := b.AllocateInstruction()
		inst.AsIconst64(v)
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = backend.SSAValueDefinition{Instr: inst, V: inst.Return()}
		return inst
	}
	insertIadd := func(m *mockCompiler, b ssa.Builder, lhs, rhs ssa.Value) *ssa.Instruction {
		inst := b.AllocateInstruction()
		inst.AsIadd(lhs, rhs)
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = backend.SSAValueDefinition{Instr: inst, V: inst.Return()}
		return inst
	}
	insertExt := func(m *mockCompiler, b ssa.Builder, v ssa.Value, from, to byte, signed bool) *ssa.Instruction {
		inst := b.AllocateInstruction()
		if signed {
			inst.AsSExtend(v, from, to)
		} else {
			inst.AsUExtend(v, from, to)
		}
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = backend.SSAValueDefinition{Instr: inst, V: v}
		return inst
	}

	for _, tc := range []struct {
		name   string
		setup  func(*mockCompiler, ssa.Builder, *machine) (ptr ssa.Value, verify func(t *testing.T))
		exp32s []addend32
		exp64s []regalloc.VReg
		offset int64
	}{
		{
			name: "non merged",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				ptr = addParam(ctx, b, ssa.TypeI64)
				return ptr, func(t *testing.T) {}
			},
			exp64s: []regalloc.VReg{v1000},
		},
		{
			name: "i32 constant folded",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				minus1 := int32(-1)
				c1, c2, c3, c4 := insertI32Const(ctx, b, 1), insertI32Const(ctx, b, 2), insertI32Const(ctx, b, 3), insertI32Const(ctx, b, uint32(minus1))
				iadd1, iadd2 := insertIadd(ctx, b, c1.Return(), c2.Return()), insertIadd(ctx, b, c3.Return(), c4.Return())
				iadd3 := insertIadd(ctx, b, iadd1.Return(), iadd2.Return())
				return iadd3.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{iadd1, iadd2, iadd3} {
						require.True(t, instr.Lowered())
					}
				}
			},
			offset: 1 + 2 + 3 - 1,
		},
		{
			name: "i64 constant folded",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				minus1 := int32(-1)
				c1, c2, c3, c4 := insertI64Const(ctx, b, 1), insertI64Const(ctx, b, 2), insertI64Const(ctx, b, 3), insertI64Const(ctx, b, uint64(minus1))
				iadd1, iadd2 := insertIadd(ctx, b, c1.Return(), c2.Return()), insertIadd(ctx, b, c3.Return(), c4.Return())
				iadd3 := insertIadd(ctx, b, iadd1.Return(), iadd2.Return())
				return iadd3.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{iadd1, iadd2, iadd3} {
						require.True(t, instr.Lowered())
					}
				}
			},
			offset: 1 + 2 + 3 - 1,
		},
		{
			name: "constant folded with one 32 value",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				param := addParam(ctx, b, ssa.TypeI32)
				minus1 := int32(-1)
				c1, c2, c3, c4 := insertI32Const(ctx, b, 1), insertI32Const(ctx, b, 2), insertI32Const(ctx, b, 3), insertI32Const(ctx, b, uint32(minus1))
				iadd1, iadd2 := insertIadd(ctx, b, c1.Return(), c2.Return()), insertIadd(ctx, b, c3.Return(), c4.Return())
				iadd3 := insertIadd(ctx, b, iadd1.Return(), iadd2.Return())
				iadd4 := insertIadd(ctx, b, param, iadd3.Return())

				return iadd4.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{iadd1, iadd2, iadd3, iadd4} {
						require.True(t, instr.Lowered())
					}
					// Param must be zero-extended.
					require.Equal(t, "uxtw x1?, w1000?", formatEmittedInstructionsInCurrentBlock(m))
				}
			},
			exp64s: []regalloc.VReg{regalloc.VReg(1).SetRegType(regalloc.RegTypeInt)},
			offset: 1 + 2 + 3 - 1,
		},
		{
			name: "one 64 value + sign-extended (32->64) instr",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				param := addParam(ctx, b, ssa.TypeI64)
				c1, c2 := insertI32Const(ctx, b, 1), insertI32Const(ctx, b, 2)
				iadd1 := insertIadd(ctx, b, c1.Return(), c2.Return())
				ext := insertExt(ctx, b, iadd1.Return(), 32, 64, true)
				ctx.vRegMap[ext.Arg()] = v2000
				iadd4 := insertIadd(ctx, b, param, ext.Return())
				return iadd4.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{ext, iadd4} {
						require.True(t, instr.Lowered())
					}
				}
			},
			exp64s: []regalloc.VReg{v1000 /* == param */},
			exp32s: []addend32{{v2000, extendOpSXTW} /* sign-extended iadd1 */},
		},
		{
			name: "one 64 value + sign-extended (32->64) const",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				param := addParam(ctx, b, ssa.TypeI64)
				minus1 := int32(-1)
				c1 := insertI32Const(ctx, b, uint32(minus1))
				ext := insertExt(ctx, b, c1.Return(), 32, 64, true)
				ctx.vRegMap[ext.Arg()] = v2000
				iadd4 := insertIadd(ctx, b, param, ext.Return())
				return iadd4.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{ext, iadd4} {
						require.True(t, instr.Lowered())
					}
				}
			},
			exp64s: []regalloc.VReg{v1000 /* == param */},
			offset: -1,
		},
		{
			name: "one 64 value + zero-extended (32->64) const",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				param := addParam(ctx, b, ssa.TypeI64)
				minus1 := int32(-1)
				c1 := insertI32Const(ctx, b, uint32(minus1))
				ext := insertExt(ctx, b, c1.Return(), 32, 64, false)
				ctx.vRegMap[ext.Arg()] = v2000
				iadd4 := insertIadd(ctx, b, param, ext.Return())
				return iadd4.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{ext, iadd4} {
						require.True(t, instr.Lowered())
					}
				}
			},
			exp64s: []regalloc.VReg{v1000 /* == param */},
			offset: math.MaxUint32, // zero-extended -1
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			ptr, verify := tc.setup(ctx, b, m)
			actual32sQ, actual64sQ, actualOffset := m.collectAddends(ptr)
			require.Equal(t, tc.exp32s, actual32sQ.Data)
			require.Equal(t, tc.exp64s, actual64sQ.Data)
			require.Equal(t, tc.offset, actualOffset)
			verify(t)
		})
	}
}

func TestMachine_addConstToReg64(t *testing.T) {
	const nextVRegID = 100
	t.Run("positive imm12", func(t *testing.T) {
		c := int64(0xaaa)
		ctx, _, m := newSetupWithMockContext()
		ctx.vRegCounter = nextVRegID - 1
		m.addConstToReg64(regalloc.FromRealReg(x15, regalloc.RegTypeInt), c)
		require.Equal(t, `add x100?, x15, #0xaaa`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("negative imm12", func(t *testing.T) {
		c := int64(-0xaaa)
		ctx, _, m := newSetupWithMockContext()
		ctx.vRegCounter = nextVRegID - 1
		m.addConstToReg64(regalloc.FromRealReg(x15, regalloc.RegTypeInt), c)
		require.Equal(t, `sub x100?, x15, #0xaaa`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("non imm12", func(t *testing.T) {
		c := int64(1<<32 | 1)
		ctx, _, m := newSetupWithMockContext()
		ctx.vRegCounter = nextVRegID - 1
		m.addConstToReg64(regalloc.FromRealReg(x15, regalloc.RegTypeInt), c)
		require.Equal(t, `movz x101?, #0x1, lsl 0
movk x101?, #0x1, lsl 32
add x100?, x15, x101?`, formatEmittedInstructionsInCurrentBlock(m))
	})
}

func TestMachine_addReg64ToReg64(t *testing.T) {
	const nextVRegID = 100
	for _, tc := range []struct {
		exp    string
		rn, rm regalloc.VReg
	}{
		{
			exp: "add x100?, x0, x1",
			rn:  regalloc.FromRealReg(x0, regalloc.RegTypeInt),
			rm:  regalloc.FromRealReg(x1, regalloc.RegTypeInt),
		},
		{
			exp: "add x100?, x10, x12",
			rn:  regalloc.FromRealReg(x10, regalloc.RegTypeInt),
			rm:  regalloc.FromRealReg(x12, regalloc.RegTypeInt),
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			ctx.vRegCounter = nextVRegID - 1
			rd := m.addReg64ToReg64(tc.rn, tc.rm)
			require.Equal(t, tc.exp, formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, rd, regalloc.VReg(nextVRegID).SetRegType(regalloc.RegTypeInt))
		})
	}
}

func TestMachine_addRegToReg64Ext(t *testing.T) {
	const nextVRegID = 100
	for _, tc := range []struct {
		exp    string
		rn, rm regalloc.VReg
		ext    extendOp
	}{
		{
			exp: "add x100?, x0, w1 UXTW",
			rn:  regalloc.FromRealReg(x0, regalloc.RegTypeInt),
			rm:  regalloc.FromRealReg(x1, regalloc.RegTypeInt),
			ext: extendOpUXTW,
		},
		{
			exp: "add x100?, x0, w1 SXTW",
			rn:  regalloc.FromRealReg(x0, regalloc.RegTypeInt),
			rm:  regalloc.FromRealReg(x1, regalloc.RegTypeInt),
			ext: extendOpSXTW,
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			ctx.vRegCounter = nextVRegID - 1
			rd := m.addRegToReg64Ext(tc.rn, tc.rm, tc.ext)
			require.Equal(t, tc.exp, formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, rd, regalloc.VReg(nextVRegID).SetRegType(regalloc.RegTypeInt))
		})
	}
}

func TestMachine_lowerToAddressModeFromAddends(t *testing.T) {
	x1, x2, x3 := regalloc.FromRealReg(x1, regalloc.RegTypeInt), regalloc.FromRealReg(x2, regalloc.RegTypeInt), regalloc.FromRealReg(x3, regalloc.RegTypeInt)
	x4, x5, x6 := regalloc.FromRealReg(x4, regalloc.RegTypeInt), regalloc.FromRealReg(x5, regalloc.RegTypeInt), regalloc.FromRealReg(x6, regalloc.RegTypeInt)

	nextVReg, nextNextVeg := regalloc.VReg(100).SetRegType(regalloc.RegTypeInt), regalloc.VReg(101).SetRegType(regalloc.RegTypeInt)
	for _, tc := range []struct {
		name          string
		a32s          []addend32
		a64s          []regalloc.VReg
		dstSizeInBits byte
		offset        int64
		exp           addressMode
		insts         []string
	}{
		{
			name:   "only offset",
			offset: 4095,
			insts:  []string{"orr x100?, xzr, #0xfff"},
			exp:    addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 0},
		},
		{
			name:   "only offset",
			offset: 4095 << 12,
			insts:  []string{"orr x100?, xzr, #0xfff000"},
			exp:    addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 0},
		},
		{
			name:          "one a64 with imm12",
			a64s:          []regalloc.VReg{x1},
			offset:        4095,
			dstSizeInBits: 8,
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: x1, imm: 4095},
		},
		{
			name:          "one a64 with imm12",
			a64s:          []regalloc.VReg{x1},
			offset:        4095 * 2,
			dstSizeInBits: 16,
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: x1, imm: 4095 * 2},
		},
		{
			name:          "one a64 with imm12",
			a64s:          []regalloc.VReg{x1},
			offset:        4095 * 4,
			dstSizeInBits: 32,
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: x1, imm: 4095 * 4},
		},
		{
			name:          "one a64 with imm12",
			a64s:          []regalloc.VReg{x1},
			offset:        4095 * 8,
			dstSizeInBits: 64,
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: x1, imm: 4095 * 8},
		},
		{
			name:          "one a64 with imm9",
			a64s:          []regalloc.VReg{x1},
			dstSizeInBits: 64,
			exp:           addressMode{kind: addressModeKindRegSignedImm9, rn: x1, imm: 0},
		},
		{
			name:          "one a64 with imm9",
			a64s:          []regalloc.VReg{x1},
			offset:        -256,
			dstSizeInBits: 64,
			exp:           addressMode{kind: addressModeKindRegSignedImm9, rn: x1, imm: -256},
		},
		{
			name:          "one a64 with offset not fitting",
			a64s:          []regalloc.VReg{x1},
			offset:        1 << 16,
			dstSizeInBits: 64,
			insts:         []string{"add x100?, x1, #0x10000"},
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 0},
		},
		{
			name:          "two a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2},
			offset:        4095,
			dstSizeInBits: 8,
			insts:         []string{"add x100?, x1, x2"},
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 4095},
		},
		{
			name:          "two a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2},
			offset:        4095 * 2,
			dstSizeInBits: 16,
			insts:         []string{"add x100?, x1, x2"},
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 4095 * 2},
		},
		{
			name:          "two a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2},
			offset:        4095 * 4,
			dstSizeInBits: 32,
			insts:         []string{"add x100?, x1, x2"},
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 4095 * 4},
		},
		{
			name:          "two a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2},
			offset:        4095 * 8,
			dstSizeInBits: 64,
			insts:         []string{"add x100?, x1, x2"},
			exp:           addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextVReg, imm: 4095 * 8},
		},
		{
			name:          "two a64 with imm9",
			a64s:          []regalloc.VReg{x1, x2},
			dstSizeInBits: 64,
			insts:         []string{"add x100?, x1, x2"},
			exp:           addressMode{kind: addressModeKindRegSignedImm9, rn: nextVReg, imm: 0},
		},
		{
			name:          "two a64 with imm9",
			a64s:          []regalloc.VReg{x1, x2},
			offset:        -256,
			dstSizeInBits: 64,
			insts:         []string{"add x100?, x1, x2"},
			exp:           addressMode{kind: addressModeKindRegSignedImm9, rn: nextVReg, imm: -256},
		},
		{
			name:          "two a64 with offset not fitting",
			a64s:          []regalloc.VReg{x1, x2},
			offset:        1 << 16,
			dstSizeInBits: 64,
			insts:         []string{"add x100?, x1, #0x10000"},
			exp:           addressMode{kind: addressModeKindRegReg, rn: nextVReg, rm: x2, extOp: extendOpUXTX},
		},
		{
			name:          "three a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2, x3},
			offset:        4095 * 2,
			dstSizeInBits: 16,
			insts: []string{
				"add x100?, x1, x2",
				"add x101?, x100?, x3",
			},
			exp: addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextNextVeg, imm: 4095 * 2},
		},
		{
			name:          "three a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2, x3},
			offset:        4095 * 4,
			dstSizeInBits: 32,
			insts: []string{
				"add x100?, x1, x2",
				"add x101?, x100?, x3",
			},
			exp: addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextNextVeg, imm: 4095 * 4},
		},
		{
			name:          "three a64 with imm12",
			a64s:          []regalloc.VReg{x1, x2, x3},
			offset:        4095 * 8,
			dstSizeInBits: 64,
			insts: []string{
				"add x100?, x1, x2",
				"add x101?, x100?, x3",
			},
			exp: addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextNextVeg, imm: 4095 * 8},
		},
		{
			name:          "three a64 with imm9",
			a64s:          []regalloc.VReg{x1, x2, x3},
			dstSizeInBits: 64,
			insts: []string{
				"add x100?, x1, x2",
				"add x101?, x100?, x3",
			},
			exp: addressMode{kind: addressModeKindRegSignedImm9, rn: nextNextVeg, imm: 0},
		},
		{
			name:          "three a64 with imm9",
			a64s:          []regalloc.VReg{x1, x2, x3},
			offset:        -256,
			dstSizeInBits: 64,
			insts: []string{
				"add x100?, x1, x2",
				"add x101?, x100?, x3",
			},
			exp: addressMode{kind: addressModeKindRegSignedImm9, rn: nextNextVeg, imm: -256},
		},
		{
			name:          "three a64 with offset not fitting",
			a64s:          []regalloc.VReg{x1, x2, x3},
			offset:        1 << 16,
			dstSizeInBits: 64,
			insts: []string{
				"add x100?, x1, #0x10000",
				"add x101?, x100?, x3",
			},
			exp: addressMode{kind: addressModeKindRegReg, rn: nextNextVeg, rm: x2, extOp: extendOpUXTX},
		},
		{
			name:          "three a32/a64 with offset",
			a64s:          []regalloc.VReg{x1, x2, x3},
			a32s:          []addend32{{r: x4, ext: extendOpSXTW}, {r: x5, ext: extendOpUXTW}, {r: x6, ext: extendOpSXTW}},
			offset:        1 << 16,
			dstSizeInBits: 64,
			insts: []string{
				"add x100?, x1, #0x10000",
				"add x101?, x100?, x2",
				"add x102?, x101?, x3",
				"add x103?, x102?, w5 UXTW",
				"add x104?, x103?, w6 SXTW",
			},
			exp: addressMode{
				kind: addressModeKindRegExtended,
				rn:   regalloc.VReg(104).SetRegType(regalloc.RegTypeInt),
				rm:   x4, extOp: extendOpSXTW,
			},
		},
		{
			name:   "one a32 with offset",
			a32s:   []addend32{{r: x1, ext: extendOpSXTW}},
			offset: 1 << 16,
			insts: []string{
				"sxtw x100?, w1",
				"add x101?, x100?, #0x10000",
			},
			exp: addressMode{kind: addressModeKindRegUnsignedImm12, rn: nextNextVeg, imm: 0},
		},
		{
			name:   "two a32s with offset",
			a32s:   []addend32{{r: x1, ext: extendOpSXTW}, {r: x2, ext: extendOpUXTW}},
			offset: 1 << 16,
			insts: []string{
				"sxtw x100?, w1",
				"add x101?, x100?, #0x10000",
			},
			exp: addressMode{
				kind:  addressModeKindRegExtended,
				rn:    regalloc.VReg(101).SetRegType(regalloc.RegTypeInt),
				rm:    x2,
				imm:   0,
				extOp: extendOpUXTW,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			ctx.vRegCounter = int(nextVReg.ID()) - 1

			var a32s wazevoapi.Queue[addend32]
			var a64s wazevoapi.Queue[regalloc.VReg]
			for _, a32 := range tc.a32s {
				a32s.Enqueue(a32)
			}
			for _, a64 := range tc.a64s {
				a64s.Enqueue(a64)
			}
			actual := m.lowerToAddressModeFromAddends(&a32s, &a64s, tc.dstSizeInBits, tc.offset)
			require.Equal(t, strings.Join(tc.insts, "\n"), formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, &tc.exp, actual, actual.format(tc.dstSizeInBits))
		})
	}
}

func Test_extLoadSizeSign(t *testing.T) {
	for _, tc := range []struct {
		op      ssa.Opcode
		expSize byte
		signed  bool
	}{
		{op: ssa.OpcodeUload8, expSize: 8, signed: false},
		{op: ssa.OpcodeUload16, expSize: 16, signed: false},
		{op: ssa.OpcodeUload32, expSize: 32, signed: false},
		{op: ssa.OpcodeSload8, expSize: 8, signed: true},
		{op: ssa.OpcodeSload16, expSize: 16, signed: true},
		{op: ssa.OpcodeSload32, expSize: 32, signed: true},
	} {
		size, signed := extLoadSignSize(tc.op)
		require.Equal(t, tc.expSize, size)
		require.Equal(t, tc.signed, signed)
	}
}
