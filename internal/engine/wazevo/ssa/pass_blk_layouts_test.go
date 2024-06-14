package ssa

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_maybeInvertBranch(t *testing.T) {
	insertJump := func(b *builder, src, dst *basicBlock) {
		b.SetCurrentBlock(src)
		jump := b.AllocateInstruction()
		jump.AsJump(ValuesNil, dst)
		b.InsertInstruction(jump)
	}

	insertBrz := func(b *builder, src, dst *basicBlock) {
		b.SetCurrentBlock(src)
		vinst := b.AllocateInstruction()
		vinst.AsIconst32(0)
		b.InsertInstruction(vinst)
		v := vinst.Return()
		brz := b.AllocateInstruction()
		brz.AsBrz(v, ValuesNil, dst)
		b.InsertInstruction(brz)
	}

	for _, tc := range []struct {
		name  string
		setup func(b *builder) (now, next *basicBlock, verify func(t *testing.T))
		exp   bool
	}{
		{
			name: "ends with br_table",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next = b.allocateBasicBlock(), b.allocateBasicBlock()
				inst := b.AllocateInstruction()
				// TODO: we haven't implemented AsBrTable on Instruction.
				inst.opcode = OpcodeBrTable
				now.currentInstr = inst
				verify = func(t *testing.T) { require.Equal(t, OpcodeBrTable, inst.opcode) }
				return
			},
		},
		{
			name: "no conditional branch without previous instruction",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next = b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, now, next)
				verify = func(t *testing.T) {
					tail := now.currentInstr
					require.Equal(t, OpcodeJump, tail.opcode)
				}
				return
			},
		},
		{
			name: "no conditional branch with previous instruction",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next = b.allocateBasicBlock(), b.allocateBasicBlock()
				b.SetCurrentBlock(now)
				prev := b.AllocateInstruction()
				prev.AsIconst64(1)
				b.InsertInstruction(prev)
				insertJump(b, now, next)
				verify = func(t *testing.T) {
					tail := now.currentInstr
					require.Equal(t, OpcodeJump, tail.opcode)
					require.Equal(t, prev, tail.prev)
				}
				return
			},
		},
		{
			name: "tail target is already loop",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next, loopHeader, dummy := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				loopHeader.loopHeader = true
				insertBrz(b, now, dummy)
				insertJump(b, now, loopHeader)
				verify = func(t *testing.T) {
					tail := now.currentInstr
					conditionalBr := tail.prev
					require.Equal(t, OpcodeJump, tail.opcode)
					require.Equal(t, OpcodeBrz, conditionalBr.opcode) // intact.
					require.Equal(t, conditionalBr, tail.prev)
				}
				return
			},
		},
		{
			name: "tail target is already the next block",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next, dummy := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				insertBrz(b, now, dummy)
				insertJump(b, now, next)
				verify = func(t *testing.T) {
					tail := now.currentInstr
					conditionalBr := tail.prev
					require.Equal(t, OpcodeJump, tail.opcode)
					require.Equal(t, OpcodeBrz, conditionalBr.opcode) // intact.
					require.Equal(t, conditionalBr, tail.prev)
				}
				return
			},
		},
		{
			name: "conditional target is loop",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next, loopHeader := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				loopHeader.loopHeader = true
				insertBrz(b, now, loopHeader) // jump to loop, which needs inversion.
				insertJump(b, now, next)

				tail := now.currentInstr
				conditionalBr := tail.prev

				// Sanity check before inversion.
				require.Equal(t, conditionalBr, loopHeader.preds[0].branch)
				require.Equal(t, tail, next.preds[0].branch)
				verify = func(t *testing.T) {
					require.Equal(t, OpcodeJump, tail.opcode)
					require.Equal(t, OpcodeBrnz, conditionalBr.opcode)                       // inversion.
					require.Equal(t, loopHeader, b.basicBlock(BasicBlockID(tail.rValue)))    // swapped.
					require.Equal(t, next, b.basicBlock(BasicBlockID(conditionalBr.rValue))) // swapped.
					require.Equal(t, conditionalBr, tail.prev)

					// Predecessor info should correctly point to the inverted jump instruction.
					require.Equal(t, tail, loopHeader.preds[0].branch)
					require.Equal(t, conditionalBr, next.preds[0].branch)
				}
				return
			},
			exp: true,
		},
		{
			name: "conditional target is the next block",
			setup: func(b *builder) (now, next *basicBlock, verify func(t *testing.T)) {
				now, next = b.allocateBasicBlock(), b.allocateBasicBlock()
				nowTarget := b.allocateBasicBlock()
				insertBrz(b, now, next) // jump to the next block in conditional, which needs inversion.
				insertJump(b, now, nowTarget)

				tail := now.currentInstr
				conditionalBr := tail.prev

				// Sanity check before inversion.
				require.Equal(t, tail, nowTarget.preds[0].branch)
				require.Equal(t, conditionalBr, next.preds[0].branch)

				verify = func(t *testing.T) {
					require.Equal(t, OpcodeJump, tail.opcode)
					require.Equal(t, OpcodeBrnz, conditionalBr.opcode)                            // inversion.
					require.Equal(t, next, b.basicBlock(BasicBlockID(tail.rValue)))               // swapped.
					require.Equal(t, nowTarget, b.basicBlock(BasicBlockID(conditionalBr.rValue))) // swapped.
					require.Equal(t, conditionalBr, tail.prev)

					require.Equal(t, conditionalBr, nowTarget.preds[0].branch)
					require.Equal(t, tail, next.preds[0].branch)
				}
				return
			},
			exp: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder().(*builder)
			now, next, verify := tc.setup(b)
			actual := maybeInvertBranches(b, now, next)
			verify(t)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestBuilder_splitCriticalEdge(t *testing.T) {
	b := NewBuilder().(*builder)
	predBlk, dummyBlk, dummyBlk2 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
	predBlk.reversePostOrder = 100
	b.SetCurrentBlock(predBlk)
	inst := b.AllocateInstruction()
	inst.AsIconst32(1)
	b.InsertInstruction(inst)
	v := inst.Return()
	originalBrz := b.AllocateInstruction() // This is the split edge.
	originalBrz.AsBrz(v, ValuesNil, dummyBlk)
	b.InsertInstruction(originalBrz)
	dummyJump := b.AllocateInstruction()
	dummyJump.AsJump(ValuesNil, dummyBlk2)
	b.InsertInstruction(dummyJump)

	predInfo := &basicBlockPredecessorInfo{blk: predBlk, branch: originalBrz}
	trampoline := b.splitCriticalEdge(predBlk, dummyBlk, predInfo)
	require.NotNil(t, trampoline)
	require.Equal(t, int32(100), trampoline.reversePostOrder)

	require.Equal(t, trampoline, predInfo.blk)
	require.Equal(t, originalBrz, predInfo.branch)
	require.Equal(t, trampoline.rootInstr, predInfo.branch)
	require.Equal(t, trampoline.currentInstr, predInfo.branch)
	require.Equal(t, trampoline.success[0], dummyBlk)

	replacedBrz := predBlk.rootInstr.next
	require.Equal(t, OpcodeBrz, replacedBrz.opcode)
	require.Equal(t, trampoline, b.basicBlock(BasicBlockID(replacedBrz.rValue)))
}

func Test_swapInstruction(t *testing.T) {
	t.Run("swap root", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()

		dummy := b.AllocateInstruction()

		old := b.AllocateInstruction()
		old.next, dummy.prev = dummy, old
		newi := b.AllocateInstruction()
		blk.rootInstr = old
		swapInstruction(blk, old, newi)

		require.Equal(t, newi, blk.rootInstr)
		require.Equal(t, dummy, newi.next)
		require.Equal(t, dummy.prev, newi)
		require.Nil(t, old.next)
		require.Nil(t, old.prev)
	})
	t.Run("swap middle", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()
		b.SetCurrentBlock(blk)
		i1, i2, i3 := b.AllocateInstruction(), b.AllocateInstruction(), b.AllocateInstruction()
		i1.AsIconst32(1)
		i2.AsIconst32(2)
		i3.AsIconst32(3)
		b.InsertInstruction(i1)
		b.InsertInstruction(i2)
		b.InsertInstruction(i3)

		newi := b.AllocateInstruction()
		newi.AsIconst32(100)
		swapInstruction(blk, i2, newi)

		require.Equal(t, i1, blk.rootInstr)
		require.Equal(t, newi, i1.next)
		require.Equal(t, i3, newi.next)
		require.Equal(t, i1, newi.prev)
		require.Equal(t, newi, i3.prev)
		require.Nil(t, i2.next)
		require.Nil(t, i2.prev)
	})
	t.Run("swap tail", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()
		b.SetCurrentBlock(blk)
		i1, i2 := b.AllocateInstruction(), b.AllocateInstruction()
		i1.AsIconst32(1)
		i2.AsIconst32(2)
		b.InsertInstruction(i1)
		b.InsertInstruction(i2)

		newi := b.AllocateInstruction()
		newi.AsIconst32(100)
		swapInstruction(blk, i2, newi)

		require.Equal(t, i1, blk.rootInstr)
		require.Equal(t, newi, blk.currentInstr)
		require.Equal(t, newi, i1.next)
		require.Equal(t, i1, newi.prev)
		require.Nil(t, newi.next)
		require.Nil(t, i2.next)
		require.Nil(t, i2.prev)
	})
}

func TestBuilder_LayoutBlocks(t *testing.T) {
	insertJump := func(b *builder, src, dst *basicBlock, vs ...Value) {
		b.SetCurrentBlock(src)
		jump := b.AllocateInstruction()
		args := b.varLengthPool.Allocate(len(vs))
		args = args.Append(&b.varLengthPool, vs...)
		jump.AsJump(args, dst)
		b.InsertInstruction(jump)
	}

	insertBrz := func(b *builder, src, dst *basicBlock, condVal Value, vs ...Value) {
		b.SetCurrentBlock(src)
		vinst := b.AllocateInstruction().AsIconst32(0)
		b.InsertInstruction(vinst)
		brz := b.AllocateInstruction()
		args := b.varLengthPool.Allocate(len(vs))
		args = args.Append(&b.varLengthPool, vs...)
		brz.AsBrz(condVal, args, dst)
		b.InsertInstruction(brz)
	}

	for _, tc := range []struct {
		name  string
		setup func(b *builder)
		exp   []BasicBlockID
	}{
		{
			name: "sequential - no critical edge",
			setup: func(b *builder) {
				b1, b2, b3, b4 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, b1, b2)
				insertJump(b, b2, b3)
				insertJump(b, b3, b4)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
				b.Seal(b4)
			},
			exp: []BasicBlockID{0, 1, 2, 3},
		},
		{
			name: "sequential with unreachable predecessor",
			setup: func(b *builder) {
				b0, unreachable, b2 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, b0, b2)
				insertJump(b, unreachable, b2)
				unreachable.invalid = true
				b.Seal(b0)
				b.Seal(unreachable)
				b.Seal(b2)
			},
			exp: []BasicBlockID{0, 2},
		},
		{
			name: "merge - no critical edge",
			// 0 -> 1 -> 3
			// |         ^
			// v         |
			// 2 ---------
			setup: func(b *builder) {
				b0, b1, b2, b3 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				b.SetCurrentBlock(b0)
				c := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c)
				insertBrz(b, b0, b2, c.Return())
				insertJump(b, b0, b1)
				insertJump(b, b1, b3)
				insertJump(b, b2, b3)
				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
			},
			exp: []BasicBlockID{0, 1, 2, 3},
		},
		{
			name: "loop towards loop header in fallthrough",
			//    0
			//    v
			//    1<--+
			//    |   | <---- critical
			//    2---+
			//    v
			//    3
			//
			// ==>
			//
			//    0
			//    v
			//    1<---+
			//    |    |
			//    2--->4
			//    v
			//    3
			setup: func(b *builder) {
				b0, b1, b2, b3 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, b0, b1)
				insertJump(b, b1, b2)
				b.SetCurrentBlock(b2)
				c := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c)
				insertBrz(b, b2, b1, c.Return())
				insertJump(b, b2, b3)
				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
			},
			// The trampoline 4 is placed right after 2, which is the hot path of the loop.
			exp: []BasicBlockID{0, 1, 2, 4, 3},
		},
		{
			name: "loop - towards loop header in conditional branch",
			//    0
			//    v
			//    1<--+
			//    |   | <---- critical
			//    2---+
			//    v
			//    3
			//
			// ==>
			//
			//    0
			//    v
			//    1<---+
			//    |    |
			//    2--->4
			//    v
			//    3
			setup: func(b *builder) {
				b0, b1, b2, b3 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, b0, b1)
				insertJump(b, b1, b2)
				b.SetCurrentBlock(b2)
				c := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c)
				insertBrz(b, b2, b3, c.Return())
				insertJump(b, b2, b1)
				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
			},
			// The trampoline 4 is placed right after 2, which is the hot path of the loop.
			exp: []BasicBlockID{0, 1, 2, 4, 3},
		},
		{
			name: "loop with header is critical backward edge",
			//    0
			//    v
			//    1<--+
			//  / |   |
			// 3  2   | <--- critical
			//  \ |   |
			//    4---+
			//    v
			//    5
			//
			// ==>
			//
			//    0
			//    v
			//    1<----+
			//  / |     |
			// 3  2     |
			//  \ |     |
			//    4---->6
			//    v
			//    5
			setup: func(b *builder) {
				b0, b1, b2, b3, b4, b5 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(),
					b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, b0, b1)
				b.SetCurrentBlock(b0)
				c1 := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c1)
				insertBrz(b, b1, b2, c1.Return())
				insertJump(b, b1, b3)
				insertJump(b, b3, b4)
				insertJump(b, b2, b4)
				b.SetCurrentBlock(b4)
				c2 := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c2)
				insertBrz(b, b4, b1, c2.Return())
				insertJump(b, b4, b5)
				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
				b.Seal(b4)
				b.Seal(b5)
			},
			// The trampoline 6 is placed right after 4, which is the hot path of the loop.
			exp: []BasicBlockID{0, 1, 3, 2, 4, 6, 5},
		},
		{
			name: "multiple critical edges",
			//                   0
			//                   v
			//               +---1<--+
			//               |   v   | <---- critical
			// critical ---->|   2 --+
			//               |   | <-------- critical
			//               |   v
			//               +-->3--->4
			//
			// ==>
			//
			//                   0
			//                   v
			//               +---1<---+
			//               |   v    |
			//               5   2 -->6
			//               |   v
			//               |   7
			//               |   v
			//               +-->3--->4
			setup: func(b *builder) {
				b0, b1, b2, b3, b4 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(),
					b.allocateBasicBlock(), b.allocateBasicBlock()
				insertJump(b, b0, b1)
				b.SetCurrentBlock(b1)
				c1 := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c1)
				insertBrz(b, b1, b2, c1.Return())
				insertJump(b, b1, b3)

				b.SetCurrentBlock(b2)
				c2 := b.AllocateInstruction().AsIconst32(0)
				b.InsertInstruction(c2)
				insertBrz(b, b2, b1, c2.Return())
				insertJump(b, b2, b3)
				insertJump(b, b3, b4)

				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
				b.Seal(b4)
			},
			exp: []BasicBlockID{
				0, 1,
				// block 2 has loop header (1) as the conditional branch target, so it's inverted,
				// and the split edge trampoline is placed right after 2 which is the hot path of the loop.
				2, 6,
				// Then the placement iteration goes to 3, which has two (5, 7) unplaced trampolines as predecessors,
				// so they are placed before 3.
				5, 7, 3,
				// Then the final block.
				4,
			},
		},
		{
			name: "brz with arg",
			setup: func(b *builder) {
				b0, b1, b2 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()
				p := b0.AddParam(b, TypeI32)
				retval := b1.AddParam(b, TypeI32)

				b.SetCurrentBlock(b0)
				{
					arg := b.AllocateInstruction().AsIconst32(1000).Insert(b).Return()
					insertBrz(b, b0, b1, p, arg)
					insertJump(b, b0, b2)
				}
				b.SetCurrentBlock(b1)
				{
					args := b.varLengthPool.Allocate(1)
					args = args.Append(&b.varLengthPool, retval)
					b.AllocateInstruction().AsReturn(args).Insert(b)
				}
				b.SetCurrentBlock(b2)
				{
					arg := b.AllocateInstruction().AsIconst32(1).Insert(b).Return()
					insertJump(b, b2, b1, arg)
				}

				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
			},
			exp: []BasicBlockID{0x0, 0x3, 0x1, 0x2},
		},
		{
			name: "loop with output",
			exp:  []BasicBlockID{0x0, 0x2, 0x4, 0x1, 0x3, 0x6, 0x5},
			setup: func(b *builder) {
				b.currentSignature = &Signature{Results: []Type{TypeI32}}
				b0, b1, b2, b3 := b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock(), b.allocateBasicBlock()

				b.SetCurrentBlock(b0)
				funcParam := b0.AddParam(b, TypeI32)
				b2Param := b2.AddParam(b, TypeI32)
				insertJump(b, b0, b2, funcParam)

				b.SetCurrentBlock(b1)
				{
					returnParam := b1.AddParam(b, TypeI32)
					insertJump(b, b1, b.returnBlk, returnParam)
				}

				b.SetCurrentBlock(b2)
				{
					c := b.AllocateInstruction().AsIconst32(100).Insert(b)
					cmp := b.AllocateInstruction().
						AsIcmp(b2Param, c.Return(), IntegerCmpCondUnsignedLessThan).
						Insert(b)
					insertBrz(b, b2, b1, cmp.Return(), b2Param)
					insertJump(b, b2, b3)
				}

				b.SetCurrentBlock(b3)
				{
					one := b.AllocateInstruction().AsIconst32(1).Insert(b)
					minusOned := b.AllocateInstruction().AsIsub(b2Param, one.Return()).Insert(b)
					c := b.AllocateInstruction().AsIconst32(150).Insert(b)
					cmp := b.AllocateInstruction().
						AsIcmp(b2Param, c.Return(), IntegerCmpCondEqual).
						Insert(b)
					insertBrz(b, b3, b1, cmp.Return(), minusOned.Return())
					insertJump(b, b3, b2, minusOned.Return())
				}

				b.Seal(b0)
				b.Seal(b1)
				b.Seal(b2)
				b.Seal(b3)
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder().(*builder)
			tc.setup(b)

			b.runPreBlockLayoutPasses() // LayoutBlocks() must be called after RunPasses().
			b.runBlockLayoutPass()
			var actual []BasicBlockID
			for blk := b.BlockIteratorReversePostOrderBegin(); blk != nil; blk = b.BlockIteratorReversePostOrderNext() {
				actual = append(actual, blk.(*basicBlock).id)
			}
			require.Equal(t, tc.exp, actual)
		})
	}
}
