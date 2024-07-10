package amd64

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_asImm32(t *testing.T) {
	v, ok := asImm32(0xffffffff, true)
	require.True(t, ok)
	require.Equal(t, uint32(0xffffffff), v)

	v, ok = asImm32(0xffffffff, false)
	require.False(t, ok)
	require.Equal(t, uint32(0), v)

	v, ok = asImm32(0x80000000, true)
	require.True(t, ok)
	require.Equal(t, uint32(0x80000000), v)

	v, ok = asImm32(0x80000000, false)
	require.False(t, ok)
	require.Equal(t, uint32(0), v)

	v, ok = asImm32(0xffffffff<<1, true)
	require.False(t, ok)
	require.Equal(t, uint32(0), v)
}

func TestMachine_getOperand_Reg(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) backend.SSAValueDefinition
		exp          operand
		instructions []string
	}{
		{
			name: "block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				ctx.vRegMap[1234] = raxVReg
				return backend.SSAValueDefinition{V: 1234, Instr: nil}
			},
			exp: newOperandReg(raxVReg),
		},

		{
			name: "const instr",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				instr := builder.AllocateInstruction()
				instr.AsIconst32(0xf00000f)
				builder.InsertInstruction(instr)
				ctx.vRegCounter = 99
				return backend.SSAValueDefinition{Instr: instr}
			},
			exp:          newOperandReg(regalloc.VReg(100).SetRegType(regalloc.RegTypeInt)),
			instructions: []string{"movl $251658255, %r100d?"},
		},
		{
			name: "non const instr (single-return)",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				c := builder.AllocateInstruction()
				sig := &ssa.Signature{Results: []ssa.Type{ssa.TypeI64}}
				builder.DeclareSignature(sig)
				c.AsCall(ssa.FuncRef(0), sig, ssa.ValuesNil)
				builder.InsertInstruction(c)
				r := c.Return()
				ctx.vRegMap[r] = regalloc.VReg(50)
				return backend.SSAValueDefinition{V: r}
			},
			exp: newOperandReg(regalloc.VReg(50)),
		},
		{
			name: "non const instr (multi-return)",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				c := builder.AllocateInstruction()
				sig := &ssa.Signature{Results: []ssa.Type{ssa.TypeI64, ssa.TypeF64, ssa.TypeF64}}
				builder.DeclareSignature(sig)
				c.AsCall(ssa.FuncRef(0), sig, ssa.ValuesNil)
				builder.InsertInstruction(c)
				_, rs := c.Returns()
				ctx.vRegMap[rs[1]] = regalloc.VReg(50)
				return backend.SSAValueDefinition{V: rs[1]}
			},
			exp: newOperandReg(regalloc.VReg(50)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def := tc.setup(ctx, b, m)
			actual := m.getOperand_Reg(def)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_getOperand_Imm32_Reg(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) backend.SSAValueDefinition
		exp          operand
		instructions []string
	}{
		{
			name: "block param falls back to getOperand_Reg",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				ctx.vRegMap[1234] = raxVReg
				return backend.SSAValueDefinition{V: 1234, Instr: nil}
			},
			exp: newOperandReg(raxVReg),
		},
		{
			name: "const imm 32",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				instr := builder.AllocateInstruction()
				instr.AsIconst32(0xf00000f)
				builder.InsertInstruction(instr)
				ctx.vRegCounter = 99
				ctx.currentGID = 0xff // const can be merged anytime, regardless of the group id.
				return backend.SSAValueDefinition{Instr: instr}
			},
			exp: newOperandImm32(0xf00000f),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def := tc.setup(ctx, b, m)
			actual := m.getOperand_Imm32_Reg(def)
			require.Equal(t, tc.exp, actual)
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func Test_machine_getOperand_Mem_Imm32_Reg(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	defer func() {
		runtime.KeepAlive(m)
	}()
	newAmodeImmReg := m.newAmodeImmReg

	for _, tc := range []struct {
		name         string
		setup        func(*mockCompiler, ssa.Builder, *machine) backend.SSAValueDefinition
		exp          operand
		instructions []string
	}{
		{
			name: "block param falls back to getOperand_Imm32_Reg",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				ctx.vRegMap[1234] = raxVReg
				return backend.SSAValueDefinition{V: 1234, Instr: nil}
			},
			exp: newOperandReg(raxVReg),
		},
		{
			name: "amode with block param",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				blk := builder.CurrentBlock()
				ptr := blk.AddParam(builder, ssa.TypeI64)
				ctx.vRegMap[ptr] = raxVReg
				ctx.definitions[ptr] = backend.SSAValueDefinition{V: ptr}
				instr := builder.AllocateInstruction()
				instr.AsLoad(ptr, 123, ssa.TypeI64).Insert(builder)
				return backend.SSAValueDefinition{Instr: instr}
			},
			exp: newOperandMem(newAmodeImmReg(123, raxVReg)),
		},
		{
			name: "amode with iconst",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				iconst := builder.AllocateInstruction().AsIconst64(456).Insert(builder)
				instr := builder.AllocateInstruction()
				instr.AsLoad(iconst.Return(), 123, ssa.TypeI64).Insert(builder)
				ctx.definitions[iconst.Return()] = backend.SSAValueDefinition{Instr: iconst}
				return backend.SSAValueDefinition{Instr: instr}
			},
			instructions: []string{
				"movabsq $579, %r1?", // r1 := 123+456
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
		{
			name: "amode with iconst and extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				iconst := builder.AllocateInstruction().AsIconst32(0xffffff).Insert(builder)
				uextend := builder.AllocateInstruction().AsUExtend(iconst.Return(), 32, 64).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(uextend.Return(), 123, ssa.TypeI64).Insert(builder)

				ctx.definitions[uextend.Return()] = backend.SSAValueDefinition{Instr: uextend}
				ctx.definitions[iconst.Return()] = backend.SSAValueDefinition{Instr: iconst}

				return backend.SSAValueDefinition{Instr: instr}
			},
			instructions: []string{
				fmt.Sprintf("movabsq $%d, %%r1?", 0xffffff+123),
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
		{
			name: "amode with iconst and extend",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				iconst := builder.AllocateInstruction().AsIconst32(456).Insert(builder)
				uextend := builder.AllocateInstruction().AsUExtend(iconst.Return(), 32, 64).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(uextend.Return(), 123, ssa.TypeI64).Insert(builder)

				ctx.definitions[uextend.Return()] = backend.SSAValueDefinition{Instr: uextend}
				ctx.definitions[iconst.Return()] = backend.SSAValueDefinition{Instr: iconst}

				return backend.SSAValueDefinition{Instr: instr}
			},
			instructions: []string{
				fmt.Sprintf("movabsq $%d, %%r1?", 456+123),
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
		{
			name: "amode with iconst and add",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				p := builder.CurrentBlock().AddParam(builder, ssa.TypeI64)
				iconst := builder.AllocateInstruction().AsIconst64(456).Insert(builder)
				iadd := builder.AllocateInstruction().AsIadd(iconst.Return(), p).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(iadd.Return(), 789, ssa.TypeI64).Insert(builder)

				ctx.vRegMap[p] = raxVReg
				ctx.definitions[p] = backend.SSAValueDefinition{V: p}
				ctx.definitions[iconst.Return()] = backend.SSAValueDefinition{Instr: iconst}
				ctx.definitions[iadd.Return()] = backend.SSAValueDefinition{Instr: iadd}

				return backend.SSAValueDefinition{Instr: instr}
			},
			exp: newOperandMem(newAmodeImmReg(456+789, raxVReg)),
		},
		{
			name: "amode with iconst, block param and add",
			setup: func(ctx *mockCompiler, builder ssa.Builder, m *machine) backend.SSAValueDefinition {
				iconst1 := builder.AllocateInstruction().AsIconst64(456).Insert(builder)
				iconst2 := builder.AllocateInstruction().AsIconst64(123).Insert(builder)
				iadd := builder.AllocateInstruction().AsIadd(iconst1.Return(), iconst2.Return()).Insert(builder)

				instr := builder.AllocateInstruction()
				instr.AsLoad(iadd.Return(), 789, ssa.TypeI64).Insert(builder)

				ctx.definitions[iconst1.Return()] = backend.SSAValueDefinition{Instr: iconst1}
				ctx.definitions[iconst2.Return()] = backend.SSAValueDefinition{Instr: iconst2}
				ctx.definitions[iadd.Return()] = backend.SSAValueDefinition{Instr: iadd}

				return backend.SSAValueDefinition{Instr: instr}
			},
			instructions: []string{
				fmt.Sprintf("movabsq $%d, %%r1?", 123+456+789),
			},
			exp: newOperandMem(newAmodeImmReg(0, regalloc.VReg(1).SetRegType(regalloc.RegTypeInt))),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			def := tc.setup(ctx, b, m)
			actual := m.getOperand_Mem_Imm32_Reg(def)
			if tc.exp.kind == operandKindMem {
				require.Equal(t, tc.exp.addressMode(), actual.addressMode())
			} else {
				require.Equal(t, tc.exp, actual)
			}
			require.Equal(t, strings.Join(tc.instructions, "\n"), formatEmittedInstructionsInCurrentBlock(m))
		})
	}
}

func TestMachine_lowerExitWithCode(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	m.nextLabel = 1
	m.lowerExitWithCode(r15VReg, wazevoapi.ExitCodeUnreachable)
	m.insert(m.allocateInstr().asUD2())
	m.FlushPendingInstructions()
	m.rootInstr = m.perBlockHead
	require.Equal(t, `
	mov.q %rsp, 56(%r15)
	mov.q %rbp, 1152(%r15)
	movl $3, %ebp
	mov.l %rbp, (%r15)
L1:
	lea L1, %rbp
	mov.q %rbp, 48(%r15)
	exit_sequence %r15
L2:
	ud2
`, m.Format())
}

func Test_machine_lowerClz(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(*mockCompiler, ssa.Builder, *machine) backend.SSAValueDefinition
		cpuFlags platform.CpuFeatureFlags
		typ      ssa.Type
		exp      string
	}{
		{
			name:     "no extra flags (64)",
			cpuFlags: &mockCpuFlags{},
			typ:      ssa.TypeI64,
			exp: `
	testq %rax, %rax
	jnz L1
	movabsq $64, %r1?
	jmp L2
L1:
	bsrq %rax, %r1?
	xor $63, %r1?
L2:
	movq %r1?, %rcx
`,
		},
		{
			name:     "ABM (64)",
			cpuFlags: &mockCpuFlags{extraFlags: platform.CpuExtraFeatureAmd64ABM},
			typ:      ssa.TypeI64,
			exp: `
	lzcntq %rax, %rcx
`,
		},
		{
			name:     "no extra flags (32)",
			cpuFlags: &mockCpuFlags{},
			typ:      ssa.TypeI32,
			exp: `
	testl %eax, %eax
	jnz L1
	movl $32, %r1d?
	jmp L2
L1:
	bsrl %eax, %r1d?
	xor $31, %r1d?
L2:
	movq %r1?, %rcx
`,
		},
		{
			name:     "ABM (32)",
			cpuFlags: &mockCpuFlags{extraFlags: platform.CpuExtraFeatureAmd64ABM},
			typ:      ssa.TypeI32,
			exp: `
	lzcntl %eax, %ecx
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			m.nextLabel = 1
			p := b.CurrentBlock().AddParam(b, tc.typ)
			m.cpuFeatures = tc.cpuFlags

			ctx.vRegMap[p] = raxVReg
			ctx.definitions[p] = backend.SSAValueDefinition{V: p}
			ctx.vRegMap[0] = rcxVReg
			instr := &ssa.Instruction{}
			instr.AsClz(p)
			m.lowerClz(instr)
			m.FlushPendingInstructions()
			m.rootInstr = m.perBlockHead
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

func TestMachine_lowerCtz(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(*mockCompiler, ssa.Builder, *machine) backend.SSAValueDefinition
		cpuFlags platform.CpuFeatureFlags
		typ      ssa.Type
		exp      string
	}{
		{
			name:     "no extra flags (64)",
			cpuFlags: &mockCpuFlags{},
			typ:      ssa.TypeI64,
			exp: `
	testq %rax, %rax
	jnz L1
	movabsq $64, %r1?
	jmp L2
L1:
	bsfq %rax, %r1?
L2:
	movq %r1?, %rcx
`,
		},
		{
			name:     "ABM (64)",
			cpuFlags: &mockCpuFlags{extraFlags: platform.CpuExtraFeatureAmd64ABM},
			typ:      ssa.TypeI64,
			exp: `
	tzcntq %rax, %rcx
`,
		},
		{
			name:     "no extra flags (32)",
			cpuFlags: &mockCpuFlags{},
			typ:      ssa.TypeI32,
			exp: `
	testl %eax, %eax
	jnz L1
	movl $32, %r1d?
	jmp L2
L1:
	bsfl %eax, %r1d?
L2:
	movq %r1?, %rcx
`,
		},
		{
			name:     "ABM (32)",
			cpuFlags: &mockCpuFlags{extraFlags: platform.CpuExtraFeatureAmd64ABM},
			typ:      ssa.TypeI32,
			exp: `
	tzcntl %eax, %ecx
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, b, m := newSetupWithMockContext()
			m.nextLabel = 1
			p := b.CurrentBlock().AddParam(b, tc.typ)
			m.cpuFeatures = tc.cpuFlags

			ctx.vRegMap[p] = raxVReg
			ctx.definitions[p] = backend.SSAValueDefinition{V: p}
			ctx.vRegMap[0] = rcxVReg
			instr := &ssa.Instruction{}
			instr.AsCtz(p)
			m.lowerCtz(instr)
			m.FlushPendingInstructions()
			m.rootInstr = m.perBlockHead
			require.Equal(t, tc.exp, m.Format())
		})
	}
}

// mockCpuFlags implements platform.CpuFeatureFlags.
type mockCpuFlags struct {
	flags      platform.CpuFeature
	extraFlags platform.CpuFeature
}

// Has implements the method of the same name in platform.CpuFeatureFlags.
func (f *mockCpuFlags) Has(flag platform.CpuFeature) bool {
	return (f.flags & flag) != 0
}

// HasExtra implements the method of the same name in platform.CpuFeatureFlags.
func (f *mockCpuFlags) HasExtra(flag platform.CpuFeature) bool {
	return (f.extraFlags & flag) != 0
}

// Raw implements the method of the same name in platform.CpuFeatureFlags.
func (f *mockCpuFlags) Raw() uint64 { return 0 }
