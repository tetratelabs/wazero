package amd64

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_machine_lowerToAddressMode(t *testing.T) {
	nextVReg, nextNextVReg := regalloc.VReg(100).SetRegType(regalloc.RegTypeInt), regalloc.VReg(101).SetRegType(regalloc.RegTypeInt)
	_, _ = nextVReg, nextNextVReg
	for _, tc := range []struct {
		name  string
		in    func(*mockCompiler, ssa.Builder, *machine) (ptr ssa.Value, offset uint32)
		insts []string
		am    amode
	}{
		{
			name: "iadd const, const; offset != 0",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				iconst1 := b.AllocateInstruction().AsIconst32(1).Insert(b)
				iconst2 := b.AllocateInstruction().AsIconst32(2).Insert(b)
				iadd := b.AllocateInstruction().AsIadd(iconst1.Return(), iconst2.Return()).Insert(b)
				ptr = iadd.Return()
				offset = 3
				ctx.definitions[iconst1.Return()] = &backend.SSAValueDefinition{Instr: iconst1}
				ctx.definitions[iconst2.Return()] = &backend.SSAValueDefinition{Instr: iconst2}
				ctx.definitions[ptr] = &backend.SSAValueDefinition{Instr: iadd}
				return
			},
			insts: []string{
				"movabsq $6, %r100?",
			},
			am: newAmodeImmReg(0, nextVReg),
		},
		{
			name: "iadd const, param; offset != 0",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				iconst1 := b.AllocateInstruction().AsIconst32(1).Insert(b)
				p := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				iadd := b.AllocateInstruction().AsIadd(iconst1.Return(), p).Insert(b)
				ptr = iadd.Return()
				offset = 3
				ctx.definitions[iconst1.Return()] = &backend.SSAValueDefinition{Instr: iconst1}
				ctx.definitions[p] = &backend.SSAValueDefinition{BlockParamValue: p, BlkParamVReg: raxVReg}
				ctx.definitions[ptr] = &backend.SSAValueDefinition{Instr: iadd}
				return
			},
			am: newAmodeImmReg(1+3, raxVReg),
		},
		{
			name: "iadd param, param; offset != 0",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				p1 := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				p2 := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				iadd := b.AllocateInstruction().AsIadd(p1, p2).Insert(b)
				ptr = iadd.Return()
				offset = 3
				ctx.definitions[p1] = &backend.SSAValueDefinition{BlockParamValue: p1, BlkParamVReg: raxVReg}
				ctx.definitions[p2] = &backend.SSAValueDefinition{BlockParamValue: p2, BlkParamVReg: rcxVReg}
				ctx.definitions[ptr] = &backend.SSAValueDefinition{Instr: iadd}
				return
			},
			am: newAmodeRegRegShift(3, raxVReg, rcxVReg, 0),
		},

		// The other iadd cases are covered by TestMachine_lowerAddendsToAmode.
		{
			name: "uextend const32",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				iconst32 := b.AllocateInstruction().AsIconst32(123).Insert(b)
				uextend := b.AllocateInstruction().AsUExtend(iconst32.Return(), 32, 64).Insert(b)
				ctx.definitions[iconst32.Return()] = &backend.SSAValueDefinition{Instr: iconst32}
				ctx.definitions[uextend.Return()] = &backend.SSAValueDefinition{Instr: uextend}
				return uextend.Return(), 0
			},
			insts: []string{
				"movabsq $123, %r100?",
			},
			am: newAmodeImmReg(0, nextVReg),
		},
		{
			name: "redundant uextend const64",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				iconst32 := b.AllocateInstruction().AsIconst64(123).Insert(b)
				uextend := b.AllocateInstruction().AsUExtend(iconst32.Return(), 64, 64).Insert(b)
				ctx.definitions[iconst32.Return()] = &backend.SSAValueDefinition{Instr: iconst32}
				ctx.definitions[uextend.Return()] = &backend.SSAValueDefinition{Instr: uextend}
				return uextend.Return(), 0
			},
			insts: []string{
				"movabsq $123, %r100?",
			},
			am: newAmodeImmReg(0, nextVReg),
		},
		{
			name: "redundant uextend param64",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				p := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				uextend := b.AllocateInstruction().AsUExtend(p, 64, 64).Insert(b)
				ctx.definitions[p] = &backend.SSAValueDefinition{BlockParamValue: p, BlkParamVReg: raxVReg}
				ctx.definitions[uextend.Return()] = &backend.SSAValueDefinition{Instr: uextend}
				return uextend.Return(), 1 << 30
			},
			am: newAmodeImmReg(1<<30, raxVReg),
		},
		{
			name: "Ishl param64, const",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				p := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				iconst64 := b.AllocateInstruction().AsIconst64(2).Insert(b)
				ishl := b.AllocateInstruction().AsIshl(p, iconst64.Return()).Insert(b)
				ctx.definitions[p] = &backend.SSAValueDefinition{BlockParamValue: p, BlkParamVReg: raxVReg}
				ctx.definitions[iconst64.Return()] = &backend.SSAValueDefinition{Instr: iconst64}
				ctx.definitions[ishl.Return()] = &backend.SSAValueDefinition{Instr: ishl}
				return ishl.Return(), 1 << 30
			},
			insts: []string{
				"xor %r100?, %r100?",
			},
			am: newAmodeRegRegShift(1<<30, nextVReg, raxVReg, 2),
		},
		{
			name: "add Iconst, (Ishl param64, const)",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) (ptr ssa.Value, offset uint32) {
				p1 := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				p2 := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				const2 := b.AllocateInstruction().AsIconst64(2).Insert(b)
				ishl := b.AllocateInstruction().AsIshl(p1, const2.Return()).Insert(b)
				iadd := b.AllocateInstruction().AsIadd(p2, ishl.Return()).Insert(b)
				ctx.definitions[p1] = &backend.SSAValueDefinition{BlockParamValue: p1, BlkParamVReg: raxVReg}
				ctx.definitions[p2] = &backend.SSAValueDefinition{BlockParamValue: p2, BlkParamVReg: rcxVReg}
				ctx.definitions[const2.Return()] = &backend.SSAValueDefinition{Instr: const2}
				ctx.definitions[ishl.Return()] = &backend.SSAValueDefinition{Instr: ishl}
				ctx.definitions[iadd.Return()] = &backend.SSAValueDefinition{Instr: iadd}
				return iadd.Return(), 1 << 30
			},
			am: newAmodeRegRegShift(1<<30, rcxVReg, raxVReg, 2),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			ctx.vRegCounter = int(nextVReg.ID()) - 1
			ptr, offset := tc.in(ctx, b, m)
			actual := m.lowerToAddressMode(ptr, offset)
			require.Equal(t, strings.Join(tc.insts, "\n"), formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, tc.am, actual, actual.String())
		})
	}
}

func TestMachine_lowerAddendFromInstr(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   func(*mockCompiler, ssa.Builder, *machine) *ssa.Instruction
		exp  addend64
	}{
		{
			name: "iconst64",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				return b.AllocateInstruction().AsIconst64(123 << 32).Insert(b)
			},
			exp: addend64{regalloc.VRegInvalid, 123 << 32, 0},
		},
		{
			name: "iconst32",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				return b.AllocateInstruction().AsIconst32(123).Insert(b)
			},
			exp: addend64{regalloc.VRegInvalid, 123, 0},
		},
		{
			name: "uextend const32",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				iconst32 := b.AllocateInstruction().AsIconst32(123).Insert(b)
				ctx.definitions[iconst32.Return()] = &backend.SSAValueDefinition{Instr: iconst32}
				return b.AllocateInstruction().AsUExtend(iconst32.Return(), 32, 64).Insert(b)
			},
			exp: addend64{regalloc.VRegInvalid, 123, 0},
		},
		{
			name: "uextend const64",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				p := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				ctx.definitions[p] = &backend.SSAValueDefinition{BlkParamVReg: raxVReg, BlockParamValue: p}
				return b.AllocateInstruction().AsUExtend(p, 32, 64).Insert(b)
			},
			exp: addend64{raxVReg, 0, 0},
		},
		{
			name: "uextend param i32",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				p := b.CurrentBlock().AddParam(b, ssa.TypeI32)
				ctx.definitions[p] = &backend.SSAValueDefinition{BlkParamVReg: raxVReg, BlockParamValue: p}
				return b.AllocateInstruction().AsUExtend(p, 32, 64).Insert(b)
			},
			exp: addend64{raxVReg, 0, 0},
		},
		{
			name: "sextend const32",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				iconst32 := b.AllocateInstruction().AsIconst32(123).Insert(b)
				ctx.definitions[iconst32.Return()] = &backend.SSAValueDefinition{Instr: iconst32}
				return b.AllocateInstruction().AsSExtend(iconst32.Return(), 32, 64).Insert(b)
			},
			exp: addend64{regalloc.VRegInvalid, 123, 0},
		},
		{
			name: "sextend const64",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				p := b.CurrentBlock().AddParam(b, ssa.TypeI64)
				ctx.definitions[p] = &backend.SSAValueDefinition{BlkParamVReg: raxVReg, BlockParamValue: p}
				return b.AllocateInstruction().AsSExtend(p, 32, 64).Insert(b)
			},
			exp: addend64{raxVReg, 0, 0},
		},
		{
			name: "sextend param i32",
			in: func(ctx *mockCompiler, b ssa.Builder, m *machine) *ssa.Instruction {
				p := b.CurrentBlock().AddParam(b, ssa.TypeI32)
				ctx.definitions[p] = &backend.SSAValueDefinition{BlkParamVReg: raxVReg, BlockParamValue: p}
				return b.AllocateInstruction().AsSExtend(p, 32, 64).Insert(b)
			},
			exp: addend64{raxVReg, 0, 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			a := m.lowerAddendFromInstr(tc.in(ctx, b, m))
			require.Equal(t, tc.exp, a)
		})
	}
}

func TestMachine_lowerAddendsToAmode(t *testing.T) {
	x1, x2, x3 := raxVReg, rcxVReg, rdxVReg
	_, _, _ = x1, x2, x3

	nextVReg, nextNextVReg := regalloc.VReg(100).SetRegType(regalloc.RegTypeInt), regalloc.VReg(101).SetRegType(regalloc.RegTypeInt)
	_ = nextNextVReg
	for _, tc := range []struct {
		name   string
		x, y   addend64
		offset int32
		exp    amode
		insts  []string
	}{
		{
			name: "only offset",
			x:    addend64{r: regalloc.VRegInvalid}, y: addend64{r: regalloc.VRegInvalid},
			offset: 4095,
			insts:  []string{"movabsq $4095, %r100?"},
			exp:    newAmodeImmReg(0, nextVReg),
		},
		{
			name: "only offset, offx, offy",
			x:    addend64{r: regalloc.VRegInvalid, off: 1}, y: addend64{r: regalloc.VRegInvalid, off: 2},
			offset: 4095,
			insts:  []string{"movabsq $4098, %r100?"},
			exp:    newAmodeImmReg(0, nextVReg),
		},
		{
			name: "only offset, offx, offy; not fitting",
			x:    addend64{r: regalloc.VRegInvalid, off: 1 << 30}, y: addend64{r: regalloc.VRegInvalid, off: 2 << 30},
			offset: 4095,
			insts:  []string{"movabsq $3221229567, %r100?"},
			exp:    newAmodeImmReg(0, nextVReg),
		},
		{
			name: "one a64 with imm32",
			x:    addend64{r: x1}, y: addend64{r: regalloc.VRegInvalid},
			offset: 4095,
			exp:    newAmodeImmReg(4095, x1),
		},
		{
			name: "one a64 with imm32",
			x:    addend64{r: x1}, y: addend64{r: regalloc.VRegInvalid},
			offset: 1 << 16,
			exp:    newAmodeImmReg(1<<16, x1),
		},
		{
			name: "two a64 with imm32",
			x:    addend64{r: x1}, y: addend64{r: x2},
			offset: 1 << 16,
			exp:    newAmodeRegRegShift(1<<16, x1, x2, 0),
		},
		{
			name: "two a64 with offset fitting",
			x:    addend64{r: x1}, y: addend64{r: x2},
			offset: 1 << 30,
			exp:    newAmodeRegRegShift(1<<30, x1, x2, 0),
		},
		{
			name: "rx with offset not fitting",
			x:    addend64{r: x1}, y: addend64{r: regalloc.VRegInvalid, off: 1 << 30},
			offset: 1 << 30,
			insts: []string{
				"movabsq $2147483648, %r100?",
			},
			exp: newAmodeRegRegShift(0, x1, nextVReg, 0),
		},
		{
			name: "ry with offset not fitting",
			x:    addend64{r: regalloc.VRegInvalid, off: 1 << 30}, y: addend64{r: x1},
			offset: 1 << 30,
			insts: []string{
				"movabsq $2147483648, %r100?",
			},
			exp: newAmodeRegRegShift(0, nextVReg, x1, 0),
		},
		{
			name: "rx with shift, ry with shift, offset != 0",
			x:    addend64{r: x1, shift: 2}, y: addend64{r: x2, shift: 3},
			offset: 1 << 30,
			insts: []string{
				"shlq $2, %rax",
			},
			exp: newAmodeRegRegShift(1<<30, x1, x2, 3),
		},
		{
			name: "rx, ry with shift, offset != 0",
			x:    addend64{r: x1}, y: addend64{r: x2, shift: 3},
			offset: 1 << 30,
			exp:    newAmodeRegRegShift(1<<30, x1, x2, 3),
		},
		{
			name: "rx with shift, ry, offset != 0",
			x:    addend64{r: x1, shift: 3}, y: addend64{r: x2},
			offset: 1 << 30,
			exp:    newAmodeRegRegShift(1<<30, x2, x1, 3),
		},
		{
			name: "rx with shift, ry invalid, offset != 0",
			x:    addend64{r: x1, shift: 3}, y: addend64{r: regalloc.VRegInvalid},
			offset: 1 << 30,
			insts: []string{
				"xor %r100?, %r100?",
			},
			exp: newAmodeRegRegShift(1<<30, nextVReg, x1, 3),
		},
		{
			name: "rx invalid, rx with shift, offset != 0",
			x:    addend64{r: regalloc.VRegInvalid}, y: addend64{r: x1, shift: 3},
			offset: 1 << 30,
			insts: []string{
				"xor %r100?, %r100?",
			},
			exp: newAmodeRegRegShift(1<<30, nextVReg, x1, 3),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, m := newSetupWithMockContext()
			ctx.vRegCounter = int(nextVReg.ID()) - 1
			actual := m.lowerAddendsToAmode(tc.x, tc.y, tc.offset)
			require.Equal(t, strings.Join(tc.insts, "\n"), formatEmittedInstructionsInCurrentBlock(m))
			require.Equal(t, tc.exp, actual, actual.String())
		})
	}
}
