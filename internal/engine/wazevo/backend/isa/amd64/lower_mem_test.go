package amd64

import (
	"math"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_lowerToAddressModeFromAddends(t *testing.T) {
	x1, x2, x3 := raxVReg, rcxVReg, rdxVReg

	nextVReg, nextNextVReg := regalloc.VReg(100).SetRegType(regalloc.RegTypeInt), regalloc.VReg(101).SetRegType(regalloc.RegTypeInt)
	_ = nextNextVReg
	for _, tc := range []struct {
		name   string
		a64s   []regalloc.VReg
		offset int64
		exp    amode
		insts  []string
	}{
		{
			name:   "only offset",
			offset: 4095,
			insts:  []string{"movabsq $4095, %r100?"},
			exp:    newAmodeImmReg(0, nextVReg),
		},
		{
			name:   "only offset",
			offset: 4095 << 12,
			insts:  []string{"movabsq $16773120, %r100?"},
			exp:    newAmodeImmReg(0, nextVReg),
		},
		{
			name:   "one a64 with imm32",
			a64s:   []regalloc.VReg{x1},
			offset: 4095,
			exp:    newAmodeImmReg(4095, x1),
		},
		{
			name:   "one a64 with imm32",
			a64s:   []regalloc.VReg{x1},
			offset: 1 << 16,
			exp:    newAmodeImmReg(1<<16, x1),
		},
		{
			name:   "two a64 with imm32",
			a64s:   []regalloc.VReg{x1, x2},
			offset: 1 << 16,
			insts:  []string{"add %rcx, %rax"},
			exp:    newAmodeImmReg(1<<16, x1),
		},
		{
			name:   "two a64 with offset not fitting",
			a64s:   []regalloc.VReg{x1, x2},
			offset: 1 << 48,
			insts: []string{
				"movabsq $281474976710656, %r100?",
				"add %r100?, %rax",
			},
			exp: newAmodeRegRegShift(0, x1, x2, 0),
		},
		{
			name:   "three a64 with imm32",
			a64s:   []regalloc.VReg{x1, x2, x3},
			offset: 1 << 16,
			insts: []string{
				"add %rcx, %rax",
				"add %rdx, %rax",
			},
			exp: newAmodeImmReg(1<<16, x1),
		},
		{
			name:   "three a64 with offset not fitting",
			a64s:   []regalloc.VReg{x1, x2, x3},
			offset: 1 << 32,
			insts: []string{
				"movabsq $4294967296, %r100?",
				"add %r100?, %rax",
				"add %rdx, %rax",
			},
			exp: newAmodeRegRegShift(0, x1, x2, 0),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			ctx.vRegCounter = int(nextVReg.ID()) - 1

			var a64s queue[regalloc.VReg]
			for _, a64 := range tc.a64s {
				a64s.enqueue(a64)
			}
			actual := m.lowerToAddressModeFromAddends(&a64s, tc.offset)
			require.Equal(t, strings.Join(tc.insts, "\n"), formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, tc.exp, actual, actual.String())
		})
	}
}

func TestMachine_collectAddends(t *testing.T) {
	v1000, v2000 := regalloc.VReg(1000).SetRegType(regalloc.RegTypeInt), regalloc.VReg(2000).SetRegType(regalloc.RegTypeInt)
	addParam := func(ctx *mockCompiler, b ssa.Builder, typ ssa.Type) ssa.Value {
		p := b.CurrentBlock().AddParam(b, typ)
		ctx.definitions[p] = &backend.SSAValueDefinition{BlockParamValue: p, BlkParamVReg: v1000}
		return p
	}
	insertI32Const := func(m *mockCompiler, b ssa.Builder, v uint32) *ssa.Instruction {
		inst := b.AllocateInstruction()
		inst.AsIconst32(v)
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = &backend.SSAValueDefinition{Instr: inst}
		return inst
	}
	insertI64Const := func(m *mockCompiler, b ssa.Builder, v uint64) *ssa.Instruction {
		inst := b.AllocateInstruction()
		inst.AsIconst64(v)
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = &backend.SSAValueDefinition{Instr: inst}
		return inst
	}
	insertIadd := func(m *mockCompiler, b ssa.Builder, lhs, rhs ssa.Value) *ssa.Instruction {
		inst := b.AllocateInstruction()
		inst.AsIadd(lhs, rhs)
		b.InsertInstruction(inst)
		m.definitions[inst.Return()] = &backend.SSAValueDefinition{Instr: inst}
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
		m.definitions[inst.Return()] = &backend.SSAValueDefinition{Instr: inst}
		return inst
	}

	for _, tc := range []struct {
		name   string
		setup  func(*mockCompiler, ssa.Builder, *machine) (ptr ssa.Value, verify func(t *testing.T))
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
				}
			},
			exp64s: []regalloc.VReg{v1000 /* == param */},
			offset: 1 + 2 + 3 - 1,
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
		{
			name: "one 64 value + redundant extension",
			setup: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, verify func(t *testing.T)) {
				param := addParam(ctx, b, ssa.TypeI64)
				ext := insertExt(ctx, b, param, 64, 64, true)
				ctx.vRegMap[ext.Arg()] = v2000
				iadd4 := insertIadd(ctx, b, param, ext.Return())
				return iadd4.Return(), func(t *testing.T) {
					for _, instr := range []*ssa.Instruction{ext, iadd4} {
						require.True(t, instr.Lowered())
					}
				}
			},
			exp64s: []regalloc.VReg{v1000, v1000},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			ptr, verify := tc.setup(ctx, b, m)
			actual64sQ, actualOffset := m.collectAddends(ptr)
			require.Equal(t, tc.exp64s, actual64sQ.data)
			require.Equal(t, tc.offset, actualOffset)
			verify(t)
		})
	}
}

func TestMachine_addReg64ToReg64(t *testing.T) {
	for _, tc := range []struct {
		exp    string
		rn, rm regalloc.VReg
	}{
		{
			exp: "add %rcx, %rax",
			rn:  raxVReg,
			rm:  rcxVReg,
		},
		{
			exp: "add %rbx, %rdx",
			rn:  rdxVReg,
			rm:  rbxVReg,
		},
	} {
		tc := tc
		t.Run(tc.exp, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			rd := m.addReg64ToReg64(tc.rn, tc.rm)
			require.Equal(t, tc.exp, formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, rd, tc.rn)
		})
	}
}

func TestMachine_addConstToReg64(t *testing.T) {
	t.Run("imm32", func(t *testing.T) {
		c := int64(1 << 30)
		_, _, m := newSetupWithMockContext()
		m.addConstToReg64(raxVReg, c)
		require.Equal(t, `add $1073741824, %rax`, formatEmittedInstructionsInCurrentBlock(m))
	})
	t.Run("non imm32", func(t *testing.T) {
		c := int64(1 << 32)
		_, _, m := newSetupWithMockContext()
		m.addConstToReg64(raxVReg, c)
		require.Equal(t, `movabsq $4294967296, %r1?
add %r1?, %rax`, formatEmittedInstructionsInCurrentBlock(m))
	})
}
