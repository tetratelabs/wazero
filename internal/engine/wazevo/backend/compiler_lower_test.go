package backend

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestCompiler_lowerBlockArguments(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(builder ssa.Builder) (c *compiler, args []ssa.Value, succ ssa.BasicBlock, verify func(t *testing.T))
	}{
		{
			name: "all consts",
			setup: func(builder ssa.Builder) (*compiler, []ssa.Value, ssa.BasicBlock, func(t *testing.T)) {
				entryBlk := builder.AllocateBasicBlock()
				builder.SetCurrentBlock(entryBlk)
				i1 := builder.AllocateInstruction()
				i1.AsIconst32(1)
				i2 := builder.AllocateInstruction()
				i2.AsIconst64(2)
				f1 := builder.AllocateInstruction()
				f1.AsF32const(3.0)
				f2 := builder.AllocateInstruction()
				f2.AsF64const(4.0)

				builder.InsertInstruction(i1)
				builder.InsertInstruction(i2)
				builder.InsertInstruction(f1)
				builder.InsertInstruction(f2)

				succ := builder.AllocateBasicBlock()
				succ.AddParam(builder, ssa.TypeI32)
				succ.AddParam(builder, ssa.TypeI64)
				succ.AddParam(builder, ssa.TypeF32)
				succ.AddParam(builder, ssa.TypeF64)

				var insertedConstInstructions []struct {
					instr  *ssa.Instruction
					target regalloc.VReg
				}
				m := &mockMachine{
					insertLoadConstant: func(instr *ssa.Instruction, vr regalloc.VReg) {
						insertedConstInstructions = append(insertedConstInstructions, struct {
							instr  *ssa.Instruction
							target regalloc.VReg
						}{instr: instr, target: vr})
					},
					argResultInts: []regalloc.RealReg{regalloc.RealReg(0), regalloc.RealReg(1)},
					argResultFloats: []regalloc.RealReg{
						regalloc.RealReg(2), regalloc.RealReg(3), regalloc.RealReg(4), regalloc.RealReg(5),
					},
				}

				c := newCompiler(context.Background(), m, builder)
				c.ssaValueToVRegs = []regalloc.VReg{0, 1, 2, 3, 4, 5, 6, 7}
				return c, []ssa.Value{i1.Return(), i2.Return(), f1.Return(), f2.Return()}, succ, func(t *testing.T) {
					require.Equal(t, 4, len(insertedConstInstructions))
					require.Equal(t, i1, insertedConstInstructions[0].instr)
					require.Equal(t, regalloc.VReg(4), insertedConstInstructions[0].target)
					require.Equal(t, i2, insertedConstInstructions[1].instr)
					require.Equal(t, regalloc.VReg(5), insertedConstInstructions[1].target)
					require.Equal(t, f1, insertedConstInstructions[2].instr)
					require.Equal(t, regalloc.VReg(6), insertedConstInstructions[2].target)
					require.Equal(t, f2, insertedConstInstructions[3].instr)
					require.Equal(t, regalloc.VReg(7), insertedConstInstructions[3].target)
				}
			},
		},
		{
			name: "overlap",
			setup: func(builder ssa.Builder) (*compiler, []ssa.Value, ssa.BasicBlock, func(t *testing.T)) {
				blk := builder.AllocateBasicBlock()
				v1 := blk.AddParam(builder, ssa.TypeI32)
				v2 := blk.AddParam(builder, ssa.TypeI32)
				v3 := blk.AddParam(builder, ssa.TypeF32)

				var insertMoves []struct{ src, dst regalloc.VReg }
				m := &mockMachine{insertMove: func(dst, src regalloc.VReg) {
					insertMoves = append(insertMoves, struct{ src, dst regalloc.VReg }{src: src, dst: dst})
				}}
				c := newCompiler(context.Background(), m, builder)
				c.ssaValueToVRegs = []regalloc.VReg{0, 1, 2, 3}
				c.nextVRegID = 100 // Temporary reg should start with 100.
				return c, []ssa.Value{v2, v1, v3 /* Swaps v1, v2 and pass v3 as-is. */}, blk, func(t *testing.T) {
					require.Equal(t, 6, len(insertMoves)) // Three values are overlapped.
					mov1, mov2, mov3, mov4, mov5, mov6 := insertMoves[0], insertMoves[1], insertMoves[2], insertMoves[3], insertMoves[4], insertMoves[5]
					// Save the values to the temporary registers.
					require.Equal(t, regalloc.VRegID(1), mov1.src.ID())
					require.Equal(t, regalloc.VRegID(100), mov1.dst.ID())
					require.Equal(t, regalloc.VRegID(0), mov2.src.ID())
					require.Equal(t, regalloc.VRegID(101), mov2.dst.ID())
					require.Equal(t, regalloc.VRegID(2), mov3.src.ID())
					require.Equal(t, regalloc.VRegID(102), mov3.dst.ID())
					// Then move back to the original place.
					require.Equal(t, regalloc.VRegID(100), mov4.src.ID())
					require.Equal(t, regalloc.VRegID(0), mov4.dst.ID())
					require.Equal(t, regalloc.VRegID(101), mov5.src.ID())
					require.Equal(t, regalloc.VRegID(1), mov5.dst.ID())
					require.Equal(t, regalloc.VRegID(102), mov6.src.ID())
					require.Equal(t, regalloc.VRegID(2), mov6.dst.ID())
				}
			},
		},
		{
			name: "no overlap",
			setup: func(builder ssa.Builder) (*compiler, []ssa.Value, ssa.BasicBlock, func(t *testing.T)) {
				blk := builder.AllocateBasicBlock()
				builder.SetCurrentBlock(blk)
				i32 := blk.AddParam(builder, ssa.TypeI32)
				add := builder.AllocateInstruction()
				add.AsIadd(i32, i32)
				builder.InsertInstruction(add)

				var insertMoves []struct{ src, dst regalloc.VReg }
				m := &mockMachine{insertMove: func(dst, src regalloc.VReg) {
					insertMoves = append(insertMoves, struct{ src, dst regalloc.VReg }{src: src, dst: dst})
				}}
				c := newCompiler(context.Background(), m, builder)
				c.ssaValueToVRegs = []regalloc.VReg{0, 1}
				return c, []ssa.Value{add.Return()}, blk, func(t *testing.T) {
					require.Equal(t, 1, len(insertMoves))
					require.Equal(t, regalloc.VRegID(1), insertMoves[0].src.ID())
					require.Equal(t, regalloc.VRegID(0), insertMoves[0].dst.ID())
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := ssa.NewBuilder()
			c, args, succ, verify := tc.setup(builder)
			c.lowerBlockArguments(args, succ)
			verify(t)
		})
	}
}
