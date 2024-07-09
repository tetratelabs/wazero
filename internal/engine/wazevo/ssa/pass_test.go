package ssa

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestBuilder_passes(t *testing.T) {
	for _, tc := range []struct {
		name string
		// pass is the optimization pass to run.
		pass,
		// postPass is run after the pass is executed, and can be used to test a pass that depends on another pass.
		postPass func(b *builder)
		// setup creates the SSA function in the given *builder.
		// TODO: when we have the text SSA IR parser, we can eliminate this `setup`,
		// 	we could directly decode the *builder from the `before` string. I am still
		//  constantly changing the format, so let's keep setup for now.
		// `verifier` is executed after executing pass, and can be used to
		// do the additional verification of the state of SSA function in addition to `after` text result.
		setup func(*builder) (verifier func(t *testing.T))
		// before is the expected SSA function after `setup` is executed.
		before,
		// after is the expected output after optimization pass.
		after string
	}{
		{
			name: "dead block",
			pass: passDeadBlockEliminationOpt,
			setup: func(b *builder) func(*testing.T) {
				entry := b.AllocateBasicBlock()
				value := entry.AddParam(b, TypeI32)

				middle1, middle2 := b.AllocateBasicBlock(), b.AllocateBasicBlock()
				end := b.AllocateBasicBlock()

				b.SetCurrentBlock(entry)
				{
					brz := b.AllocateInstruction()
					brz.AsBrz(value, ValuesNil, middle1)
					b.InsertInstruction(brz)

					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, middle2)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(middle1)
				{
					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, end)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(middle2)
				{
					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, end)
					b.InsertInstruction(jmp)
				}

				{
					unreachable := b.AllocateBasicBlock()
					b.SetCurrentBlock(unreachable)
					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, end)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(end)
				{
					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, middle1)
					b.InsertInstruction(jmp)
				}

				b.Seal(entry)
				b.Seal(middle1)
				b.Seal(middle2)
				b.Seal(end)
				return nil
			},
			before: `
blk0: (v0:i32)
	Brz v0, blk1
	Jump blk2

blk1: () <-- (blk0,blk3)
	Jump blk3

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk1,blk2,blk4)
	Jump blk1

blk4: ()
	Jump blk3
`,
			after: `
blk0: (v0:i32)
	Brz v0, blk1
	Jump blk2

blk1: () <-- (blk0,blk3)
	Jump blk3

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk1,blk2)
	Jump blk1
`,
		},
		{
			name: "redundant phis",
			pass: passRedundantPhiEliminationOpt,
			setup: func(b *builder) func(*testing.T) {
				entry, loopHeader, end := b.AllocateBasicBlock(), b.AllocateBasicBlock(), b.AllocateBasicBlock()

				loopHeader.AddParam(b, TypeI32)
				var1 := b.DeclareVariable(TypeI32)

				b.SetCurrentBlock(entry)
				{
					constInst := b.AllocateInstruction()
					constInst.AsIconst32(0xff)
					b.InsertInstruction(constInst)
					iConst := constInst.Return()
					b.DefineVariable(var1, iConst, entry)

					jmp := b.AllocateInstruction()
					args := b.varLengthPool.Allocate(1)
					args = args.Append(&b.varLengthPool, iConst)
					jmp.AsJump(args, loopHeader)
					b.InsertInstruction(jmp)
				}
				b.Seal(entry)

				b.SetCurrentBlock(loopHeader)
				{
					// At this point, loop is not sealed, so PHI will be added to this header. However, the only
					// input to the PHI is iConst above, so there must be an alias to iConst from the PHI value.
					value := b.MustFindValue(var1)

					tmpInst := b.AllocateInstruction()
					tmpInst.AsIconst32(0xff)
					b.InsertInstruction(tmpInst)
					tmp := tmpInst.Return()

					args := b.varLengthPool.Allocate(0)
					args = args.Append(&b.varLengthPool, tmp)
					brz := b.AllocateInstruction()
					brz.AsBrz(value, args, loopHeader) // Loop to itself.
					b.InsertInstruction(brz)

					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, end)
					b.InsertInstruction(jmp)
				}
				b.Seal(loopHeader)

				b.SetCurrentBlock(end)
				{
					ret := b.AllocateInstruction()
					ret.AsReturn(ValuesNil)
					b.InsertInstruction(ret)
				}

				// passRedundantPhiEliminationOpt requires the reverse post-order traversal to be calculated.
				passCalculateImmediateDominators(b)
				return nil
			},
			before: `
blk0: ()
	v1:i32 = Iconst_32 0xff
	Jump blk1, v1, v1

blk1: (v0:i32,v2:i32) <-- (blk0,blk1)
	v3:i32 = Iconst_32 0xff
	Brz v2, blk1, v3, v2
	Jump blk2

blk2: () <-- (blk1)
	Return
`,
			after: `
blk0: ()
	v1:i32 = Iconst_32 0xff
	Jump blk1, v1

blk1: (v0:i32) <-- (blk0,blk1)
	v3:i32 = Iconst_32 0xff
	Brz v1, blk1, v3
	Jump blk2

blk2: () <-- (blk1)
	Return
`,
		},
		{
			name: "dead code",
			pass: passDeadCodeEliminationOpt,
			setup: func(b *builder) func(*testing.T) {
				entry, end := b.AllocateBasicBlock(), b.AllocateBasicBlock()

				b.SetCurrentBlock(entry)
				iconstRefThriceInst := b.AllocateInstruction()
				iconstRefThriceInst.AsIconst32(3)
				b.InsertInstruction(iconstRefThriceInst)
				refThriceVal := iconstRefThriceInst.Return()

				// This has side effect.
				store := b.AllocateInstruction()
				store.AsStore(OpcodeStore, refThriceVal, refThriceVal, 0)
				b.InsertInstruction(store)

				iconstDeadInst := b.AllocateInstruction()
				iconstDeadInst.AsIconst32(0)
				b.InsertInstruction(iconstDeadInst)

				iconstRefOnceInst := b.AllocateInstruction()
				iconstRefOnceInst.AsIconst32(1)
				b.InsertInstruction(iconstRefOnceInst)
				refOnceVal := iconstRefOnceInst.Return()

				jmp := b.AllocateInstruction()
				jmp.AsJump(ValuesNil, end)
				b.InsertInstruction(jmp)

				b.SetCurrentBlock(end)
				aliasedRefOnceVal := b.allocateValue(refOnceVal.Type())
				b.alias(aliasedRefOnceVal, refOnceVal)

				add := b.AllocateInstruction()
				add.AsIadd(aliasedRefOnceVal, refThriceVal)
				b.InsertInstruction(add)

				addRes := add.Return()

				ret := b.AllocateInstruction()
				args := b.varLengthPool.Allocate(1)
				args = args.Append(&b.varLengthPool, addRes)
				ret.AsReturn(args)
				b.InsertInstruction(ret)
				return func(t *testing.T) {
					// Group IDs.
					const gid0, gid1, gid2 InstructionGroupID = 0, 1, 2
					require.Equal(t, gid0, iconstRefThriceInst.gid)
					require.Equal(t, gid0, store.gid)
					require.Equal(t, gid1, iconstDeadInst.gid)
					require.Equal(t, gid1, iconstRefOnceInst.gid)
					require.Equal(t, gid1, jmp.gid)
					// Different blocks have different gids.
					require.Equal(t, gid2, add.gid)
					require.Equal(t, gid2, ret.gid)

					// Dead or Alive...
					require.False(t, iconstDeadInst.live)
					require.True(t, iconstRefOnceInst.live)
					require.True(t, iconstRefThriceInst.live)
					require.True(t, add.live)
					require.True(t, jmp.live)
					require.True(t, ret.live)

					require.Equal(t, uint32(1), b.valuesInfo[refOnceVal.ID()].RefCount)
					require.Equal(t, uint32(1), b.valuesInfo[addRes.ID()].RefCount)
					require.Equal(t, uint32(3), b.valuesInfo[refThriceVal.ID()].RefCount)
				}
			},
			before: `
blk0: ()
	v0:i32 = Iconst_32 0x3
	Store v0, v0, 0x0
	v1:i32 = Iconst_32 0x0
	v2:i32 = Iconst_32 0x1
	Jump blk1

blk1: () <-- (blk0)
	v4:i32 = Iadd v3, v0
	Return v4
`,
			after: `
blk0: ()
	v0:i32 = Iconst_32 0x3
	Store v0, v0, 0x0
	v2:i32 = Iconst_32 0x1
	Jump blk1

blk1: () <-- (blk0)
	v4:i32 = Iadd v2, v0
	Return v4
`,
		},
		{
			name:     "nop elimination",
			pass:     passNopInstElimination,
			postPass: passDeadCodeEliminationOpt,
			setup: func(b *builder) (verifier func(t *testing.T)) {
				entry := b.AllocateBasicBlock()
				b.SetCurrentBlock(entry)

				i32Param := entry.AddParam(b, TypeI32)
				i64Param := entry.AddParam(b, TypeI64)

				// 32-bit shift.
				moduleZeroI32 := b.AllocateInstruction().AsIconst32(32 * 245).Insert(b).Return()
				nopIshl := b.AllocateInstruction().AsIshl(i32Param, moduleZeroI32).Insert(b).Return()

				// 64-bit shift.
				moduleZeroI64 := b.AllocateInstruction().AsIconst64(64 * 245).Insert(b).Return()
				nopUshr := b.AllocateInstruction().AsUshr(i64Param, moduleZeroI64).Insert(b).Return()

				// Non zero shift amount should not be eliminated.
				nonZeroI32 := b.AllocateInstruction().AsIconst32(32*245 + 1).Insert(b).Return()
				nonZeroIshl := b.AllocateInstruction().AsIshl(i32Param, nonZeroI32).Insert(b).Return()

				nonZeroI64 := b.AllocateInstruction().AsIconst64(64*245 + 1).Insert(b).Return()
				nonZeroSshr := b.AllocateInstruction().AsSshr(i64Param, nonZeroI64).Insert(b).Return()

				ret := b.AllocateInstruction()
				args := b.varLengthPool.Allocate(4)
				args = args.Append(&b.varLengthPool, nopIshl)
				args = args.Append(&b.varLengthPool, nopUshr)
				args = args.Append(&b.varLengthPool, nonZeroIshl)
				args = args.Append(&b.varLengthPool, nonZeroSshr)
				ret.AsReturn(args)
				b.InsertInstruction(ret)
				return nil
			},
			before: `
blk0: (v0:i32, v1:i64)
	v2:i32 = Iconst_32 0x1ea0
	v3:i32 = Ishl v0, v2
	v4:i64 = Iconst_64 0x3d40
	v5:i64 = Ushr v1, v4
	v6:i32 = Iconst_32 0x1ea1
	v7:i32 = Ishl v0, v6
	v8:i64 = Iconst_64 0x3d41
	v9:i64 = Sshr v1, v8
	Return v3, v5, v7, v9
`,
			after: `
blk0: (v0:i32, v1:i64)
	v6:i32 = Iconst_32 0x1ea1
	v7:i32 = Ishl v0, v6
	v8:i64 = Iconst_64 0x3d41
	v9:i64 = Sshr v1, v8
	Return v0, v1, v7, v9
`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder().(*builder)
			verifier := tc.setup(b)
			require.Equal(t, tc.before, b.Format())
			tc.pass(b)
			if verifier != nil {
				verifier(t)
			}
			if tc.postPass != nil {
				tc.postPass(b)
			}
			require.Equal(t, tc.after, b.Format())
		})
	}
}
