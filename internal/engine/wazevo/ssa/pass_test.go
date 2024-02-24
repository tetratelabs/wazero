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
	Brz v2, blk1, v3
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

					require.Equal(t, 1, b.valueRefCounts[refOnceVal.ID()])
					require.Equal(t, 1, b.valueRefCounts[addRes.ID()])
					require.Equal(t, 3, b.valueRefCounts[refThriceVal.ID()])
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
		{
			name: "tail duplication / pred only jump",
			pass: passTailDuplication,
			setup: func(b *builder) func(*testing.T) {
				entry := b.AllocateBasicBlock()
				switchVal := entry.AddParam(b, TypeI32)

				const srcBlockNum = 10
				var srcBlocks []BasicBlock
				for i := 0; i < srcBlockNum; i++ {
					srcBlocks = append(srcBlocks, b.allocateBasicBlock())
				}
				eliminationTargetBlock := b.allocateBasicBlock()
				elseBlock := b.allocateBasicBlock()
				dstBlock := b.allocateBasicBlock()

				var constValsInEntry []Value
				b.SetCurrentBlock(entry)
				{
					btable := b.AllocateInstruction()
					btable.AsBrTable(switchVal, append(srcBlocks, elseBlock))
					b.InsertInstruction(btable)
					for i := 0; i < 10; i++ {
						constVal := b.AllocateInstruction().AsIconst32(uint32(i)).Insert(b).Return()
						constValsInEntry = append(constValsInEntry, constVal)
					}
				}

				b.SetCurrentBlock(dstBlock)
				{
					var vs []Value
					for i := 0; i < 5; i++ {
						value := dstBlock.AddParam(b, TypeI32)
						vs = append(vs, value)
					}
					ret := b.AllocateInstruction()
					ret.AsReturn(b.AllocateVarLengthValues(vs...))
					b.InsertInstruction(ret)
				}

				b.SetCurrentBlock(elseBlock)
				{
					var vs []Value
					for i := 0; i < 5; i++ {
						constVal := b.AllocateInstruction().AsIconst32(uint32(i + 1234)).Insert(b).Return()
						vs = append(vs, constVal)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(vs...), dstBlock)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(eliminationTargetBlock)
				{
					var params []Value
					for i := 0; i < 10; i++ {
						value := eliminationTargetBlock.AddParam(b, TypeI32)
						params = append(params, value)
					}
					var vs []Value
					for i := 0; i < 5; i++ {
						added := b.AllocateInstruction().AsIadd(params[i], params[i+5]).Insert(b).Return()
						vs = append(vs, added)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(vs...), dstBlock)
					b.InsertInstruction(jmp)
				}

				for _, blk := range srcBlocks {
					b.SetCurrentBlock(blk)
					var argsToEliminationTarget []Value
					for i := 0; i < 10; i++ {
						argsToEliminationTarget = append(argsToEliminationTarget, constValsInEntry[(i+int(blk.ID()))%len(constValsInEntry)])
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(argsToEliminationTarget...), eliminationTargetBlock)
					b.InsertInstruction(jmp)
				}

				// Seal all blocks.
				b.Seal(entry)
				b.Seal(elseBlock)
				b.Seal(dstBlock)
				b.Seal(eliminationTargetBlock)
				for _, blk := range srcBlocks {
					b.Seal(blk)
				}

				b.runPreBlockLayoutPasses()
				return nil
			},
			before: `
blk0: (v0:i32)
	BrTable v0, [blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8, blk9, blk10, blk12]
	v1:i32 = Iconst_32 0x0
	v2:i32 = Iconst_32 0x1
	v3:i32 = Iconst_32 0x2
	v4:i32 = Iconst_32 0x3
	v5:i32 = Iconst_32 0x4
	v6:i32 = Iconst_32 0x5
	v7:i32 = Iconst_32 0x6
	v8:i32 = Iconst_32 0x7
	v9:i32 = Iconst_32 0x8
	v10:i32 = Iconst_32 0x9

blk1: () <-- (blk0)
	Jump blk11, v2, v3, v4, v5, v6, v7, v8, v9, v10, v1

blk2: () <-- (blk0)
	Jump blk11, v3, v4, v5, v6, v7, v8, v9, v10, v1, v2

blk3: () <-- (blk0)
	Jump blk11, v4, v5, v6, v7, v8, v9, v10, v1, v2, v3

blk4: () <-- (blk0)
	Jump blk11, v5, v6, v7, v8, v9, v10, v1, v2, v3, v4

blk5: () <-- (blk0)
	Jump blk11, v6, v7, v8, v9, v10, v1, v2, v3, v4, v5

blk6: () <-- (blk0)
	Jump blk11, v7, v8, v9, v10, v1, v2, v3, v4, v5, v6

blk7: () <-- (blk0)
	Jump blk11, v8, v9, v10, v1, v2, v3, v4, v5, v6, v7

blk8: () <-- (blk0)
	Jump blk11, v9, v10, v1, v2, v3, v4, v5, v6, v7, v8

blk9: () <-- (blk0)
	Jump blk11, v10, v1, v2, v3, v4, v5, v6, v7, v8, v9

blk10: () <-- (blk0)
	Jump blk11, v1, v2, v3, v4, v5, v6, v7, v8, v9, v10

blk11: (v21:i32,v22:i32,v23:i32,v24:i32,v25:i32,v26:i32,v27:i32,v28:i32,v29:i32,v30:i32) <-- (blk1,blk2,blk3,blk4,blk5,blk6,blk7,blk8,blk9,blk10)
	v31:i32 = Iadd v21, v26
	v32:i32 = Iadd v22, v27
	v33:i32 = Iadd v23, v28
	v34:i32 = Iadd v24, v29
	v35:i32 = Iadd v25, v30
	Jump blk13, v31, v32, v33, v34, v35

blk12: () <-- (blk0)
	v16:i32 = Iconst_32 0x4d2
	v17:i32 = Iconst_32 0x4d3
	v18:i32 = Iconst_32 0x4d4
	v19:i32 = Iconst_32 0x4d5
	v20:i32 = Iconst_32 0x4d6
	Jump blk13, v16, v17, v18, v19, v20

blk13: (v11:i32,v12:i32,v13:i32,v14:i32,v15:i32) <-- (blk12,blk11)
	Return v11, v12, v13, v14, v15
`,
			after: `
blk0: (v0:i32)
	BrTable v0, [blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8, blk9, blk10, blk12]
	v1:i32 = Iconst_32 0x0
	v2:i32 = Iconst_32 0x1
	v3:i32 = Iconst_32 0x2
	v4:i32 = Iconst_32 0x3
	v5:i32 = Iconst_32 0x4
	v6:i32 = Iconst_32 0x5
	v7:i32 = Iconst_32 0x6
	v8:i32 = Iconst_32 0x7
	v9:i32 = Iconst_32 0x8
	v10:i32 = Iconst_32 0x9

blk1: () <-- (blk0)
	v36:i32 = Iadd v2, v7
	v37:i32 = Iadd v3, v8
	v38:i32 = Iadd v4, v9
	v39:i32 = Iadd v5, v10
	v40:i32 = Iadd v6, v1
	Jump blk13, v36, v37, v38, v39, v40

blk2: () <-- (blk0)
	v41:i32 = Iadd v3, v8
	v42:i32 = Iadd v4, v9
	v43:i32 = Iadd v5, v10
	v44:i32 = Iadd v6, v1
	v45:i32 = Iadd v7, v2
	Jump blk13, v41, v42, v43, v44, v45

blk3: () <-- (blk0)
	v46:i32 = Iadd v4, v9
	v47:i32 = Iadd v5, v10
	v48:i32 = Iadd v6, v1
	v49:i32 = Iadd v7, v2
	v50:i32 = Iadd v8, v3
	Jump blk13, v46, v47, v48, v49, v50

blk4: () <-- (blk0)
	v51:i32 = Iadd v5, v10
	v52:i32 = Iadd v6, v1
	v53:i32 = Iadd v7, v2
	v54:i32 = Iadd v8, v3
	v55:i32 = Iadd v9, v4
	Jump blk13, v51, v52, v53, v54, v55

blk5: () <-- (blk0)
	v56:i32 = Iadd v6, v1
	v57:i32 = Iadd v7, v2
	v58:i32 = Iadd v8, v3
	v59:i32 = Iadd v9, v4
	v60:i32 = Iadd v10, v5
	Jump blk13, v56, v57, v58, v59, v60

blk6: () <-- (blk0)
	v61:i32 = Iadd v7, v2
	v62:i32 = Iadd v8, v3
	v63:i32 = Iadd v9, v4
	v64:i32 = Iadd v10, v5
	v65:i32 = Iadd v1, v6
	Jump blk13, v61, v62, v63, v64, v65

blk7: () <-- (blk0)
	v66:i32 = Iadd v8, v3
	v67:i32 = Iadd v9, v4
	v68:i32 = Iadd v10, v5
	v69:i32 = Iadd v1, v6
	v70:i32 = Iadd v2, v7
	Jump blk13, v66, v67, v68, v69, v70

blk8: () <-- (blk0)
	v71:i32 = Iadd v9, v4
	v72:i32 = Iadd v10, v5
	v73:i32 = Iadd v1, v6
	v74:i32 = Iadd v2, v7
	v75:i32 = Iadd v3, v8
	Jump blk13, v71, v72, v73, v74, v75

blk9: () <-- (blk0)
	v76:i32 = Iadd v10, v5
	v77:i32 = Iadd v1, v6
	v78:i32 = Iadd v2, v7
	v79:i32 = Iadd v3, v8
	v80:i32 = Iadd v4, v9
	Jump blk13, v76, v77, v78, v79, v80

blk10: () <-- (blk0)
	v81:i32 = Iadd v1, v6
	v82:i32 = Iadd v2, v7
	v83:i32 = Iadd v3, v8
	v84:i32 = Iadd v4, v9
	v85:i32 = Iadd v5, v10
	Jump blk13, v81, v82, v83, v84, v85

blk12: () <-- (blk0)
	v16:i32 = Iconst_32 0x4d2
	v17:i32 = Iconst_32 0x4d3
	v18:i32 = Iconst_32 0x4d4
	v19:i32 = Iconst_32 0x4d5
	v20:i32 = Iconst_32 0x4d6
	Jump blk13, v16, v17, v18, v19, v20

blk13: (v11:i32,v12:i32,v13:i32,v14:i32,v15:i32) <-- (blk12,blk1,blk2,blk3,blk4,blk5,blk6,blk7,blk8,blk9,blk10)
	Return v11, v12, v13, v14, v15
`,
		},
		{
			name: "tail duplication / target dominates dst",
			pass: passTailDuplication,
			setup: func(b *builder) func(*testing.T) {
				entry := b.AllocateBasicBlock()
				switchVal := entry.AddParam(b, TypeI32)

				const srcBlockNum = 10
				var srcBlocks []BasicBlock
				for i := 0; i < srcBlockNum; i++ {
					srcBlocks = append(srcBlocks, b.allocateBasicBlock())
				}
				eliminationTargetBlock := b.allocateBasicBlock()
				dstBlock := b.allocateBasicBlock()

				b.SetCurrentBlock(entry)
				{
					btable := b.AllocateInstruction()
					btable.AsBrTable(switchVal, srcBlocks)
					b.InsertInstruction(btable)
				}

				const dstParam = 3
				var valueInEliminationTarget []Value
				b.SetCurrentBlock(eliminationTargetBlock)
				{
					var params []Value
					for i := 0; i < 10; i++ {
						value := eliminationTargetBlock.AddParam(b, TypeI32)
						params = append(params, value)
					}
					for i := 0; i < dstParam; i++ {
						added := b.AllocateInstruction().AsIadd(params[i], params[i+dstParam]).Insert(b).Return()
						doubled := b.AllocateInstruction().AsIadd(added, added).Insert(b).Return()
						valueInEliminationTarget = append(valueInEliminationTarget, doubled)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(ValuesNil, dstBlock)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(dstBlock)
				{
					var vs []Value
					for i := 0; i < len(valueInEliminationTarget); i++ {
						value := b.AllocateInstruction().AsIadd(valueInEliminationTarget[i], valueInEliminationTarget[i]).Insert(b).Return()
						vs = append(vs, value)
					}
					ret := b.AllocateInstruction()
					ret.AsReturn(b.AllocateVarLengthValues(vs...))
					b.InsertInstruction(ret)
				}

				for _, blk := range srcBlocks {
					b.SetCurrentBlock(blk)
					var argsToEliminationTarget []Value
					for i := 0; i < 10; i++ {
						constVal := b.AllocateInstruction().AsIconst32(uint32(i) + uint32(blk.ID())).Insert(b).Return()
						argsToEliminationTarget = append(argsToEliminationTarget, constVal)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(argsToEliminationTarget...), eliminationTargetBlock)
					b.InsertInstruction(jmp)
				}

				// Seal all blocks.
				b.Seal(entry)
				b.Seal(dstBlock)
				b.Seal(eliminationTargetBlock)
				for _, blk := range srcBlocks {
					b.Seal(blk)
				}

				b.runPreBlockLayoutPasses()
				return nil
			},
			before: `
blk0: (v0:i32)
	BrTable v0, [blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8, blk9, blk10]

blk1: () <-- (blk0)
	v20:i32 = Iconst_32 0x1
	v21:i32 = Iconst_32 0x2
	v22:i32 = Iconst_32 0x3
	v23:i32 = Iconst_32 0x4
	v24:i32 = Iconst_32 0x5
	v25:i32 = Iconst_32 0x6
	v26:i32 = Iconst_32 0x7
	v27:i32 = Iconst_32 0x8
	v28:i32 = Iconst_32 0x9
	v29:i32 = Iconst_32 0xa
	Jump blk11, v20, v21, v22, v23, v24, v25, v26, v27, v28, v29

blk2: () <-- (blk0)
	v30:i32 = Iconst_32 0x2
	v31:i32 = Iconst_32 0x3
	v32:i32 = Iconst_32 0x4
	v33:i32 = Iconst_32 0x5
	v34:i32 = Iconst_32 0x6
	v35:i32 = Iconst_32 0x7
	v36:i32 = Iconst_32 0x8
	v37:i32 = Iconst_32 0x9
	v38:i32 = Iconst_32 0xa
	v39:i32 = Iconst_32 0xb
	Jump blk11, v30, v31, v32, v33, v34, v35, v36, v37, v38, v39

blk3: () <-- (blk0)
	v40:i32 = Iconst_32 0x3
	v41:i32 = Iconst_32 0x4
	v42:i32 = Iconst_32 0x5
	v43:i32 = Iconst_32 0x6
	v44:i32 = Iconst_32 0x7
	v45:i32 = Iconst_32 0x8
	v46:i32 = Iconst_32 0x9
	v47:i32 = Iconst_32 0xa
	v48:i32 = Iconst_32 0xb
	v49:i32 = Iconst_32 0xc
	Jump blk11, v40, v41, v42, v43, v44, v45, v46, v47, v48, v49

blk4: () <-- (blk0)
	v50:i32 = Iconst_32 0x4
	v51:i32 = Iconst_32 0x5
	v52:i32 = Iconst_32 0x6
	v53:i32 = Iconst_32 0x7
	v54:i32 = Iconst_32 0x8
	v55:i32 = Iconst_32 0x9
	v56:i32 = Iconst_32 0xa
	v57:i32 = Iconst_32 0xb
	v58:i32 = Iconst_32 0xc
	v59:i32 = Iconst_32 0xd
	Jump blk11, v50, v51, v52, v53, v54, v55, v56, v57, v58, v59

blk5: () <-- (blk0)
	v60:i32 = Iconst_32 0x5
	v61:i32 = Iconst_32 0x6
	v62:i32 = Iconst_32 0x7
	v63:i32 = Iconst_32 0x8
	v64:i32 = Iconst_32 0x9
	v65:i32 = Iconst_32 0xa
	v66:i32 = Iconst_32 0xb
	v67:i32 = Iconst_32 0xc
	v68:i32 = Iconst_32 0xd
	v69:i32 = Iconst_32 0xe
	Jump blk11, v60, v61, v62, v63, v64, v65, v66, v67, v68, v69

blk6: () <-- (blk0)
	v70:i32 = Iconst_32 0x6
	v71:i32 = Iconst_32 0x7
	v72:i32 = Iconst_32 0x8
	v73:i32 = Iconst_32 0x9
	v74:i32 = Iconst_32 0xa
	v75:i32 = Iconst_32 0xb
	v76:i32 = Iconst_32 0xc
	v77:i32 = Iconst_32 0xd
	v78:i32 = Iconst_32 0xe
	v79:i32 = Iconst_32 0xf
	Jump blk11, v70, v71, v72, v73, v74, v75, v76, v77, v78, v79

blk7: () <-- (blk0)
	v80:i32 = Iconst_32 0x7
	v81:i32 = Iconst_32 0x8
	v82:i32 = Iconst_32 0x9
	v83:i32 = Iconst_32 0xa
	v84:i32 = Iconst_32 0xb
	v85:i32 = Iconst_32 0xc
	v86:i32 = Iconst_32 0xd
	v87:i32 = Iconst_32 0xe
	v88:i32 = Iconst_32 0xf
	v89:i32 = Iconst_32 0x10
	Jump blk11, v80, v81, v82, v83, v84, v85, v86, v87, v88, v89

blk8: () <-- (blk0)
	v90:i32 = Iconst_32 0x8
	v91:i32 = Iconst_32 0x9
	v92:i32 = Iconst_32 0xa
	v93:i32 = Iconst_32 0xb
	v94:i32 = Iconst_32 0xc
	v95:i32 = Iconst_32 0xd
	v96:i32 = Iconst_32 0xe
	v97:i32 = Iconst_32 0xf
	v98:i32 = Iconst_32 0x10
	v99:i32 = Iconst_32 0x11
	Jump blk11, v90, v91, v92, v93, v94, v95, v96, v97, v98, v99

blk9: () <-- (blk0)
	v100:i32 = Iconst_32 0x9
	v101:i32 = Iconst_32 0xa
	v102:i32 = Iconst_32 0xb
	v103:i32 = Iconst_32 0xc
	v104:i32 = Iconst_32 0xd
	v105:i32 = Iconst_32 0xe
	v106:i32 = Iconst_32 0xf
	v107:i32 = Iconst_32 0x10
	v108:i32 = Iconst_32 0x11
	v109:i32 = Iconst_32 0x12
	Jump blk11, v100, v101, v102, v103, v104, v105, v106, v107, v108, v109

blk10: () <-- (blk0)
	v110:i32 = Iconst_32 0xa
	v111:i32 = Iconst_32 0xb
	v112:i32 = Iconst_32 0xc
	v113:i32 = Iconst_32 0xd
	v114:i32 = Iconst_32 0xe
	v115:i32 = Iconst_32 0xf
	v116:i32 = Iconst_32 0x10
	v117:i32 = Iconst_32 0x11
	v118:i32 = Iconst_32 0x12
	v119:i32 = Iconst_32 0x13
	Jump blk11, v110, v111, v112, v113, v114, v115, v116, v117, v118, v119

blk11: (v1:i32,v2:i32,v3:i32,v4:i32,v5:i32,v6:i32,v7:i32,v8:i32,v9:i32,v10:i32) <-- (blk1,blk2,blk3,blk4,blk5,blk6,blk7,blk8,blk9,blk10)
	v11:i32 = Iadd v1, v4
	v12:i32 = Iadd v11, v11
	v13:i32 = Iadd v2, v5
	v14:i32 = Iadd v13, v13
	v15:i32 = Iadd v3, v6
	v16:i32 = Iadd v15, v15
	Jump blk12

blk12: () <-- (blk11)
	v17:i32 = Iadd v12, v12
	v18:i32 = Iadd v14, v14
	v19:i32 = Iadd v16, v16
	Return v17, v18, v19
`,
			after: `
blk0: (v0:i32)
	BrTable v0, [blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8, blk9, blk10]

blk1: () <-- (blk0)
	v20:i32 = Iconst_32 0x1
	v21:i32 = Iconst_32 0x2
	v22:i32 = Iconst_32 0x3
	v23:i32 = Iconst_32 0x4
	v24:i32 = Iconst_32 0x5
	v25:i32 = Iconst_32 0x6
	v26:i32 = Iconst_32 0x7
	v27:i32 = Iconst_32 0x8
	v28:i32 = Iconst_32 0x9
	v29:i32 = Iconst_32 0xa
	v120:i32 = Iadd v20, v23
	v121:i32 = Iadd v120, v120
	v122:i32 = Iadd v21, v24
	v123:i32 = Iadd v122, v122
	v124:i32 = Iadd v22, v25
	v125:i32 = Iadd v124, v124
	Jump blk12, v120, v121, v122, v123, v124, v125

blk2: () <-- (blk0)
	v30:i32 = Iconst_32 0x2
	v31:i32 = Iconst_32 0x3
	v32:i32 = Iconst_32 0x4
	v33:i32 = Iconst_32 0x5
	v34:i32 = Iconst_32 0x6
	v35:i32 = Iconst_32 0x7
	v36:i32 = Iconst_32 0x8
	v37:i32 = Iconst_32 0x9
	v38:i32 = Iconst_32 0xa
	v39:i32 = Iconst_32 0xb
	v126:i32 = Iadd v30, v33
	v127:i32 = Iadd v126, v126
	v128:i32 = Iadd v31, v34
	v129:i32 = Iadd v128, v128
	v130:i32 = Iadd v32, v35
	v131:i32 = Iadd v130, v130
	Jump blk12, v126, v127, v128, v129, v130, v131

blk3: () <-- (blk0)
	v40:i32 = Iconst_32 0x3
	v41:i32 = Iconst_32 0x4
	v42:i32 = Iconst_32 0x5
	v43:i32 = Iconst_32 0x6
	v44:i32 = Iconst_32 0x7
	v45:i32 = Iconst_32 0x8
	v46:i32 = Iconst_32 0x9
	v47:i32 = Iconst_32 0xa
	v48:i32 = Iconst_32 0xb
	v49:i32 = Iconst_32 0xc
	v132:i32 = Iadd v40, v43
	v133:i32 = Iadd v132, v132
	v134:i32 = Iadd v41, v44
	v135:i32 = Iadd v134, v134
	v136:i32 = Iadd v42, v45
	v137:i32 = Iadd v136, v136
	Jump blk12, v132, v133, v134, v135, v136, v137

blk4: () <-- (blk0)
	v50:i32 = Iconst_32 0x4
	v51:i32 = Iconst_32 0x5
	v52:i32 = Iconst_32 0x6
	v53:i32 = Iconst_32 0x7
	v54:i32 = Iconst_32 0x8
	v55:i32 = Iconst_32 0x9
	v56:i32 = Iconst_32 0xa
	v57:i32 = Iconst_32 0xb
	v58:i32 = Iconst_32 0xc
	v59:i32 = Iconst_32 0xd
	v138:i32 = Iadd v50, v53
	v139:i32 = Iadd v138, v138
	v140:i32 = Iadd v51, v54
	v141:i32 = Iadd v140, v140
	v142:i32 = Iadd v52, v55
	v143:i32 = Iadd v142, v142
	Jump blk12, v138, v139, v140, v141, v142, v143

blk5: () <-- (blk0)
	v60:i32 = Iconst_32 0x5
	v61:i32 = Iconst_32 0x6
	v62:i32 = Iconst_32 0x7
	v63:i32 = Iconst_32 0x8
	v64:i32 = Iconst_32 0x9
	v65:i32 = Iconst_32 0xa
	v66:i32 = Iconst_32 0xb
	v67:i32 = Iconst_32 0xc
	v68:i32 = Iconst_32 0xd
	v69:i32 = Iconst_32 0xe
	v144:i32 = Iadd v60, v63
	v145:i32 = Iadd v144, v144
	v146:i32 = Iadd v61, v64
	v147:i32 = Iadd v146, v146
	v148:i32 = Iadd v62, v65
	v149:i32 = Iadd v148, v148
	Jump blk12, v144, v145, v146, v147, v148, v149

blk6: () <-- (blk0)
	v70:i32 = Iconst_32 0x6
	v71:i32 = Iconst_32 0x7
	v72:i32 = Iconst_32 0x8
	v73:i32 = Iconst_32 0x9
	v74:i32 = Iconst_32 0xa
	v75:i32 = Iconst_32 0xb
	v76:i32 = Iconst_32 0xc
	v77:i32 = Iconst_32 0xd
	v78:i32 = Iconst_32 0xe
	v79:i32 = Iconst_32 0xf
	v150:i32 = Iadd v70, v73
	v151:i32 = Iadd v150, v150
	v152:i32 = Iadd v71, v74
	v153:i32 = Iadd v152, v152
	v154:i32 = Iadd v72, v75
	v155:i32 = Iadd v154, v154
	Jump blk12, v150, v151, v152, v153, v154, v155

blk7: () <-- (blk0)
	v80:i32 = Iconst_32 0x7
	v81:i32 = Iconst_32 0x8
	v82:i32 = Iconst_32 0x9
	v83:i32 = Iconst_32 0xa
	v84:i32 = Iconst_32 0xb
	v85:i32 = Iconst_32 0xc
	v86:i32 = Iconst_32 0xd
	v87:i32 = Iconst_32 0xe
	v88:i32 = Iconst_32 0xf
	v89:i32 = Iconst_32 0x10
	v156:i32 = Iadd v80, v83
	v157:i32 = Iadd v156, v156
	v158:i32 = Iadd v81, v84
	v159:i32 = Iadd v158, v158
	v160:i32 = Iadd v82, v85
	v161:i32 = Iadd v160, v160
	Jump blk12, v156, v157, v158, v159, v160, v161

blk8: () <-- (blk0)
	v90:i32 = Iconst_32 0x8
	v91:i32 = Iconst_32 0x9
	v92:i32 = Iconst_32 0xa
	v93:i32 = Iconst_32 0xb
	v94:i32 = Iconst_32 0xc
	v95:i32 = Iconst_32 0xd
	v96:i32 = Iconst_32 0xe
	v97:i32 = Iconst_32 0xf
	v98:i32 = Iconst_32 0x10
	v99:i32 = Iconst_32 0x11
	v162:i32 = Iadd v90, v93
	v163:i32 = Iadd v162, v162
	v164:i32 = Iadd v91, v94
	v165:i32 = Iadd v164, v164
	v166:i32 = Iadd v92, v95
	v167:i32 = Iadd v166, v166
	Jump blk12, v162, v163, v164, v165, v166, v167

blk9: () <-- (blk0)
	v100:i32 = Iconst_32 0x9
	v101:i32 = Iconst_32 0xa
	v102:i32 = Iconst_32 0xb
	v103:i32 = Iconst_32 0xc
	v104:i32 = Iconst_32 0xd
	v105:i32 = Iconst_32 0xe
	v106:i32 = Iconst_32 0xf
	v107:i32 = Iconst_32 0x10
	v108:i32 = Iconst_32 0x11
	v109:i32 = Iconst_32 0x12
	v168:i32 = Iadd v100, v103
	v169:i32 = Iadd v168, v168
	v170:i32 = Iadd v101, v104
	v171:i32 = Iadd v170, v170
	v172:i32 = Iadd v102, v105
	v173:i32 = Iadd v172, v172
	Jump blk12, v168, v169, v170, v171, v172, v173

blk10: () <-- (blk0)
	v110:i32 = Iconst_32 0xa
	v111:i32 = Iconst_32 0xb
	v112:i32 = Iconst_32 0xc
	v113:i32 = Iconst_32 0xd
	v114:i32 = Iconst_32 0xe
	v115:i32 = Iconst_32 0xf
	v116:i32 = Iconst_32 0x10
	v117:i32 = Iconst_32 0x11
	v118:i32 = Iconst_32 0x12
	v119:i32 = Iconst_32 0x13
	v174:i32 = Iadd v110, v113
	v175:i32 = Iadd v174, v174
	v176:i32 = Iadd v111, v114
	v177:i32 = Iadd v176, v176
	v178:i32 = Iadd v112, v115
	v179:i32 = Iadd v178, v178
	Jump blk12, v174, v175, v176, v177, v178, v179

blk12: (v11:i32,v12:i32,v13:i32,v14:i32,v15:i32,v16:i32) <-- (blk1,blk2,blk3,blk4,blk5,blk6,blk7,blk8,blk9,blk10)
	v17:i32 = Iadd v12, v12
	v18:i32 = Iadd v14, v14
	v19:i32 = Iadd v16, v16
	Return v17, v18, v19
`,
		},
		{
			name: "tail duplication / target does not dominate dst",
			pass: passTailDuplication,
			setup: func(b *builder) func(*testing.T) {
				entry := b.AllocateBasicBlock()
				switchVal := entry.AddParam(b, TypeI32)

				const srcBlockNum = 10
				var srcBlocks []BasicBlock
				for i := 0; i < srcBlockNum; i++ {
					srcBlocks = append(srcBlocks, b.allocateBasicBlock())
				}
				eliminationTargetBlock := b.allocateBasicBlock()
				elseBlock := b.allocateBasicBlock()
				dstBlock := b.allocateBasicBlock()

				b.SetCurrentBlock(entry)
				{
					btable := b.AllocateInstruction()
					btable.AsBrTable(switchVal, append(srcBlocks, elseBlock))
					b.InsertInstruction(btable)
				}

				b.SetCurrentBlock(dstBlock)
				{
					var vs []Value
					for i := 0; i < 5; i++ {
						value := dstBlock.AddParam(b, TypeI32)
						vs = append(vs, value)
					}
					ret := b.AllocateInstruction()
					ret.AsReturn(b.AllocateVarLengthValues(vs...))
					b.InsertInstruction(ret)
				}

				b.SetCurrentBlock(elseBlock)
				{
					var vs []Value
					for i := 0; i < 5; i++ {
						constVal := b.AllocateInstruction().AsIconst32(uint32(i + 1234)).Insert(b).Return()
						vs = append(vs, constVal)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(vs...), dstBlock)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(eliminationTargetBlock)
				{
					var params []Value
					for i := 0; i < 10; i++ {
						value := eliminationTargetBlock.AddParam(b, TypeI32)
						params = append(params, value)
					}
					var vs []Value
					for i := 0; i < 5; i++ {
						added := b.AllocateInstruction().AsIadd(params[i], params[i+5]).Insert(b).Return()
						vs = append(vs, added)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(vs...), dstBlock)
					b.InsertInstruction(jmp)
				}

				for _, blk := range srcBlocks {
					b.SetCurrentBlock(blk)
					var argsToEliminationTarget []Value
					for i := 0; i < 10; i++ {
						constVal := b.AllocateInstruction().AsIconst32(uint32(i) + uint32(blk.ID())).Insert(b).Return()
						argsToEliminationTarget = append(argsToEliminationTarget, constVal)
					}
					jmp := b.AllocateInstruction()
					jmp.AsJump(b.AllocateVarLengthValues(argsToEliminationTarget...), eliminationTargetBlock)
					b.InsertInstruction(jmp)
				}

				// Seal all blocks.
				b.Seal(entry)
				b.Seal(elseBlock)
				b.Seal(dstBlock)
				b.Seal(eliminationTargetBlock)
				for _, blk := range srcBlocks {
					b.Seal(blk)
				}

				b.runPreBlockLayoutPasses()
				return nil
			},
			before: `
blk0: (v0:i32)
	BrTable v0, [blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8, blk9, blk10, blk12]

blk1: () <-- (blk0)
	v26:i32 = Iconst_32 0x1
	v27:i32 = Iconst_32 0x2
	v28:i32 = Iconst_32 0x3
	v29:i32 = Iconst_32 0x4
	v30:i32 = Iconst_32 0x5
	v31:i32 = Iconst_32 0x6
	v32:i32 = Iconst_32 0x7
	v33:i32 = Iconst_32 0x8
	v34:i32 = Iconst_32 0x9
	v35:i32 = Iconst_32 0xa
	Jump blk11, v26, v27, v28, v29, v30, v31, v32, v33, v34, v35

blk2: () <-- (blk0)
	v36:i32 = Iconst_32 0x2
	v37:i32 = Iconst_32 0x3
	v38:i32 = Iconst_32 0x4
	v39:i32 = Iconst_32 0x5
	v40:i32 = Iconst_32 0x6
	v41:i32 = Iconst_32 0x7
	v42:i32 = Iconst_32 0x8
	v43:i32 = Iconst_32 0x9
	v44:i32 = Iconst_32 0xa
	v45:i32 = Iconst_32 0xb
	Jump blk11, v36, v37, v38, v39, v40, v41, v42, v43, v44, v45

blk3: () <-- (blk0)
	v46:i32 = Iconst_32 0x3
	v47:i32 = Iconst_32 0x4
	v48:i32 = Iconst_32 0x5
	v49:i32 = Iconst_32 0x6
	v50:i32 = Iconst_32 0x7
	v51:i32 = Iconst_32 0x8
	v52:i32 = Iconst_32 0x9
	v53:i32 = Iconst_32 0xa
	v54:i32 = Iconst_32 0xb
	v55:i32 = Iconst_32 0xc
	Jump blk11, v46, v47, v48, v49, v50, v51, v52, v53, v54, v55

blk4: () <-- (blk0)
	v56:i32 = Iconst_32 0x4
	v57:i32 = Iconst_32 0x5
	v58:i32 = Iconst_32 0x6
	v59:i32 = Iconst_32 0x7
	v60:i32 = Iconst_32 0x8
	v61:i32 = Iconst_32 0x9
	v62:i32 = Iconst_32 0xa
	v63:i32 = Iconst_32 0xb
	v64:i32 = Iconst_32 0xc
	v65:i32 = Iconst_32 0xd
	Jump blk11, v56, v57, v58, v59, v60, v61, v62, v63, v64, v65

blk5: () <-- (blk0)
	v66:i32 = Iconst_32 0x5
	v67:i32 = Iconst_32 0x6
	v68:i32 = Iconst_32 0x7
	v69:i32 = Iconst_32 0x8
	v70:i32 = Iconst_32 0x9
	v71:i32 = Iconst_32 0xa
	v72:i32 = Iconst_32 0xb
	v73:i32 = Iconst_32 0xc
	v74:i32 = Iconst_32 0xd
	v75:i32 = Iconst_32 0xe
	Jump blk11, v66, v67, v68, v69, v70, v71, v72, v73, v74, v75

blk6: () <-- (blk0)
	v76:i32 = Iconst_32 0x6
	v77:i32 = Iconst_32 0x7
	v78:i32 = Iconst_32 0x8
	v79:i32 = Iconst_32 0x9
	v80:i32 = Iconst_32 0xa
	v81:i32 = Iconst_32 0xb
	v82:i32 = Iconst_32 0xc
	v83:i32 = Iconst_32 0xd
	v84:i32 = Iconst_32 0xe
	v85:i32 = Iconst_32 0xf
	Jump blk11, v76, v77, v78, v79, v80, v81, v82, v83, v84, v85

blk7: () <-- (blk0)
	v86:i32 = Iconst_32 0x7
	v87:i32 = Iconst_32 0x8
	v88:i32 = Iconst_32 0x9
	v89:i32 = Iconst_32 0xa
	v90:i32 = Iconst_32 0xb
	v91:i32 = Iconst_32 0xc
	v92:i32 = Iconst_32 0xd
	v93:i32 = Iconst_32 0xe
	v94:i32 = Iconst_32 0xf
	v95:i32 = Iconst_32 0x10
	Jump blk11, v86, v87, v88, v89, v90, v91, v92, v93, v94, v95

blk8: () <-- (blk0)
	v96:i32 = Iconst_32 0x8
	v97:i32 = Iconst_32 0x9
	v98:i32 = Iconst_32 0xa
	v99:i32 = Iconst_32 0xb
	v100:i32 = Iconst_32 0xc
	v101:i32 = Iconst_32 0xd
	v102:i32 = Iconst_32 0xe
	v103:i32 = Iconst_32 0xf
	v104:i32 = Iconst_32 0x10
	v105:i32 = Iconst_32 0x11
	Jump blk11, v96, v97, v98, v99, v100, v101, v102, v103, v104, v105

blk9: () <-- (blk0)
	v106:i32 = Iconst_32 0x9
	v107:i32 = Iconst_32 0xa
	v108:i32 = Iconst_32 0xb
	v109:i32 = Iconst_32 0xc
	v110:i32 = Iconst_32 0xd
	v111:i32 = Iconst_32 0xe
	v112:i32 = Iconst_32 0xf
	v113:i32 = Iconst_32 0x10
	v114:i32 = Iconst_32 0x11
	v115:i32 = Iconst_32 0x12
	Jump blk11, v106, v107, v108, v109, v110, v111, v112, v113, v114, v115

blk10: () <-- (blk0)
	v116:i32 = Iconst_32 0xa
	v117:i32 = Iconst_32 0xb
	v118:i32 = Iconst_32 0xc
	v119:i32 = Iconst_32 0xd
	v120:i32 = Iconst_32 0xe
	v121:i32 = Iconst_32 0xf
	v122:i32 = Iconst_32 0x10
	v123:i32 = Iconst_32 0x11
	v124:i32 = Iconst_32 0x12
	v125:i32 = Iconst_32 0x13
	Jump blk11, v116, v117, v118, v119, v120, v121, v122, v123, v124, v125

blk11: (v11:i32,v12:i32,v13:i32,v14:i32,v15:i32,v16:i32,v17:i32,v18:i32,v19:i32,v20:i32) <-- (blk1,blk2,blk3,blk4,blk5,blk6,blk7,blk8,blk9,blk10)
	v21:i32 = Iadd v11, v16
	v22:i32 = Iadd v12, v17
	v23:i32 = Iadd v13, v18
	v24:i32 = Iadd v14, v19
	v25:i32 = Iadd v15, v20
	Jump blk13, v21, v22, v23, v24, v25

blk12: () <-- (blk0)
	v6:i32 = Iconst_32 0x4d2
	v7:i32 = Iconst_32 0x4d3
	v8:i32 = Iconst_32 0x4d4
	v9:i32 = Iconst_32 0x4d5
	v10:i32 = Iconst_32 0x4d6
	Jump blk13, v6, v7, v8, v9, v10

blk13: (v1:i32,v2:i32,v3:i32,v4:i32,v5:i32) <-- (blk12,blk11)
	Return v1, v2, v3, v4, v5
`,
			after: `
blk0: (v0:i32)
	BrTable v0, [blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8, blk9, blk10, blk12]

blk1: () <-- (blk0)
	v26:i32 = Iconst_32 0x1
	v27:i32 = Iconst_32 0x2
	v28:i32 = Iconst_32 0x3
	v29:i32 = Iconst_32 0x4
	v30:i32 = Iconst_32 0x5
	v31:i32 = Iconst_32 0x6
	v32:i32 = Iconst_32 0x7
	v33:i32 = Iconst_32 0x8
	v34:i32 = Iconst_32 0x9
	v35:i32 = Iconst_32 0xa
	v126:i32 = Iadd v26, v31
	v127:i32 = Iadd v27, v32
	v128:i32 = Iadd v28, v33
	v129:i32 = Iadd v29, v34
	v130:i32 = Iadd v30, v35
	Jump blk13, v126, v127, v128, v129, v130

blk2: () <-- (blk0)
	v36:i32 = Iconst_32 0x2
	v37:i32 = Iconst_32 0x3
	v38:i32 = Iconst_32 0x4
	v39:i32 = Iconst_32 0x5
	v40:i32 = Iconst_32 0x6
	v41:i32 = Iconst_32 0x7
	v42:i32 = Iconst_32 0x8
	v43:i32 = Iconst_32 0x9
	v44:i32 = Iconst_32 0xa
	v45:i32 = Iconst_32 0xb
	v131:i32 = Iadd v36, v41
	v132:i32 = Iadd v37, v42
	v133:i32 = Iadd v38, v43
	v134:i32 = Iadd v39, v44
	v135:i32 = Iadd v40, v45
	Jump blk13, v131, v132, v133, v134, v135

blk3: () <-- (blk0)
	v46:i32 = Iconst_32 0x3
	v47:i32 = Iconst_32 0x4
	v48:i32 = Iconst_32 0x5
	v49:i32 = Iconst_32 0x6
	v50:i32 = Iconst_32 0x7
	v51:i32 = Iconst_32 0x8
	v52:i32 = Iconst_32 0x9
	v53:i32 = Iconst_32 0xa
	v54:i32 = Iconst_32 0xb
	v55:i32 = Iconst_32 0xc
	v136:i32 = Iadd v46, v51
	v137:i32 = Iadd v47, v52
	v138:i32 = Iadd v48, v53
	v139:i32 = Iadd v49, v54
	v140:i32 = Iadd v50, v55
	Jump blk13, v136, v137, v138, v139, v140

blk4: () <-- (blk0)
	v56:i32 = Iconst_32 0x4
	v57:i32 = Iconst_32 0x5
	v58:i32 = Iconst_32 0x6
	v59:i32 = Iconst_32 0x7
	v60:i32 = Iconst_32 0x8
	v61:i32 = Iconst_32 0x9
	v62:i32 = Iconst_32 0xa
	v63:i32 = Iconst_32 0xb
	v64:i32 = Iconst_32 0xc
	v65:i32 = Iconst_32 0xd
	v141:i32 = Iadd v56, v61
	v142:i32 = Iadd v57, v62
	v143:i32 = Iadd v58, v63
	v144:i32 = Iadd v59, v64
	v145:i32 = Iadd v60, v65
	Jump blk13, v141, v142, v143, v144, v145

blk5: () <-- (blk0)
	v66:i32 = Iconst_32 0x5
	v67:i32 = Iconst_32 0x6
	v68:i32 = Iconst_32 0x7
	v69:i32 = Iconst_32 0x8
	v70:i32 = Iconst_32 0x9
	v71:i32 = Iconst_32 0xa
	v72:i32 = Iconst_32 0xb
	v73:i32 = Iconst_32 0xc
	v74:i32 = Iconst_32 0xd
	v75:i32 = Iconst_32 0xe
	v146:i32 = Iadd v66, v71
	v147:i32 = Iadd v67, v72
	v148:i32 = Iadd v68, v73
	v149:i32 = Iadd v69, v74
	v150:i32 = Iadd v70, v75
	Jump blk13, v146, v147, v148, v149, v150

blk6: () <-- (blk0)
	v76:i32 = Iconst_32 0x6
	v77:i32 = Iconst_32 0x7
	v78:i32 = Iconst_32 0x8
	v79:i32 = Iconst_32 0x9
	v80:i32 = Iconst_32 0xa
	v81:i32 = Iconst_32 0xb
	v82:i32 = Iconst_32 0xc
	v83:i32 = Iconst_32 0xd
	v84:i32 = Iconst_32 0xe
	v85:i32 = Iconst_32 0xf
	v151:i32 = Iadd v76, v81
	v152:i32 = Iadd v77, v82
	v153:i32 = Iadd v78, v83
	v154:i32 = Iadd v79, v84
	v155:i32 = Iadd v80, v85
	Jump blk13, v151, v152, v153, v154, v155

blk7: () <-- (blk0)
	v86:i32 = Iconst_32 0x7
	v87:i32 = Iconst_32 0x8
	v88:i32 = Iconst_32 0x9
	v89:i32 = Iconst_32 0xa
	v90:i32 = Iconst_32 0xb
	v91:i32 = Iconst_32 0xc
	v92:i32 = Iconst_32 0xd
	v93:i32 = Iconst_32 0xe
	v94:i32 = Iconst_32 0xf
	v95:i32 = Iconst_32 0x10
	v156:i32 = Iadd v86, v91
	v157:i32 = Iadd v87, v92
	v158:i32 = Iadd v88, v93
	v159:i32 = Iadd v89, v94
	v160:i32 = Iadd v90, v95
	Jump blk13, v156, v157, v158, v159, v160

blk8: () <-- (blk0)
	v96:i32 = Iconst_32 0x8
	v97:i32 = Iconst_32 0x9
	v98:i32 = Iconst_32 0xa
	v99:i32 = Iconst_32 0xb
	v100:i32 = Iconst_32 0xc
	v101:i32 = Iconst_32 0xd
	v102:i32 = Iconst_32 0xe
	v103:i32 = Iconst_32 0xf
	v104:i32 = Iconst_32 0x10
	v105:i32 = Iconst_32 0x11
	v161:i32 = Iadd v96, v101
	v162:i32 = Iadd v97, v102
	v163:i32 = Iadd v98, v103
	v164:i32 = Iadd v99, v104
	v165:i32 = Iadd v100, v105
	Jump blk13, v161, v162, v163, v164, v165

blk9: () <-- (blk0)
	v106:i32 = Iconst_32 0x9
	v107:i32 = Iconst_32 0xa
	v108:i32 = Iconst_32 0xb
	v109:i32 = Iconst_32 0xc
	v110:i32 = Iconst_32 0xd
	v111:i32 = Iconst_32 0xe
	v112:i32 = Iconst_32 0xf
	v113:i32 = Iconst_32 0x10
	v114:i32 = Iconst_32 0x11
	v115:i32 = Iconst_32 0x12
	v166:i32 = Iadd v106, v111
	v167:i32 = Iadd v107, v112
	v168:i32 = Iadd v108, v113
	v169:i32 = Iadd v109, v114
	v170:i32 = Iadd v110, v115
	Jump blk13, v166, v167, v168, v169, v170

blk10: () <-- (blk0)
	v116:i32 = Iconst_32 0xa
	v117:i32 = Iconst_32 0xb
	v118:i32 = Iconst_32 0xc
	v119:i32 = Iconst_32 0xd
	v120:i32 = Iconst_32 0xe
	v121:i32 = Iconst_32 0xf
	v122:i32 = Iconst_32 0x10
	v123:i32 = Iconst_32 0x11
	v124:i32 = Iconst_32 0x12
	v125:i32 = Iconst_32 0x13
	v171:i32 = Iadd v116, v121
	v172:i32 = Iadd v117, v122
	v173:i32 = Iadd v118, v123
	v174:i32 = Iadd v119, v124
	v175:i32 = Iadd v120, v125
	Jump blk13, v171, v172, v173, v174, v175

blk12: () <-- (blk0)
	v6:i32 = Iconst_32 0x4d2
	v7:i32 = Iconst_32 0x4d3
	v8:i32 = Iconst_32 0x4d4
	v9:i32 = Iconst_32 0x4d5
	v10:i32 = Iconst_32 0x4d6
	Jump blk13, v6, v7, v8, v9, v10

blk13: (v1:i32,v2:i32,v3:i32,v4:i32,v5:i32) <-- (blk12,blk1,blk2,blk3,blk4,blk5,blk6,blk7,blk8,blk9,blk10)
	Return v1, v2, v3, v4, v5
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

func Test_isTailDuplicationTarget(t *testing.T) {
	require.False(t, isTailDuplicationTarget(&basicBlock{id: basicBlockIDReturnBlock}))
	require.False(t, isTailDuplicationTarget(&basicBlock{preds: make([]basicBlockPredecessorInfo, 1)}))
	require.False(t, isTailDuplicationTarget(&basicBlock{preds: make([]basicBlockPredecessorInfo, 100)}))
	t.Run("ok - jmp", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()
		blk.preds = make([]basicBlockPredecessorInfo, 100)
		b.SetCurrentBlock(blk)
		var vs []Value
		for i := 0; i < 5; i++ {
			param := blk.AddParam(b, TypeI32)
			doubled := b.AllocateInstruction().AsIadd(param, param).Insert(b).Return()
			vs = append(vs, doubled)
		}

		jmp := b.AllocateInstruction()
		jmp.AsJump(b.AllocateVarLengthValues(vs...), blk)
		b.InsertInstruction(jmp)
		require.True(t, isTailDuplicationTarget(blk))
	})
	t.Run("termination is not jump", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()
		blk.preds = make([]basicBlockPredecessorInfo, 100)
		b.SetCurrentBlock(blk)
		var vs []Value
		for i := 0; i < 5; i++ {
			param := blk.AddParam(b, TypeI32)
			doubled := b.AllocateInstruction().AsIadd(param, param).Insert(b).Return()
			vs = append(vs, doubled)
		}

		add := b.AllocateInstruction()
		add.AsIadd(vs[0], vs[1])
		b.InsertInstruction(add)
		require.False(t, isTailDuplicationTarget(blk))
	})
	t.Run("termination is jump but to return block.", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()
		blk.preds = make([]basicBlockPredecessorInfo, 100)
		b.SetCurrentBlock(blk)
		var vs []Value
		for i := 0; i < 5; i++ {
			param := blk.AddParam(b, TypeI32)
			doubled := b.AllocateInstruction().AsIadd(param, param).Insert(b).Return()
			vs = append(vs, doubled)
		}

		jmp := b.AllocateInstruction()
		jmp.AsJump(b.AllocateVarLengthValues(vs...), b.returnBlk)
		b.InsertInstruction(jmp)
		require.False(t, isTailDuplicationTarget(blk))
	})
	t.Run("too many instr", func(t *testing.T) {
		b := NewBuilder().(*builder)
		blk := b.allocateBasicBlock()
		blk.preds = make([]basicBlockPredecessorInfo, 100)
		b.SetCurrentBlock(blk)

		param := blk.AddParam(b, TypeI32)
		for i := 0; i < 100; i++ {
			b.AllocateInstruction().AsIadd(param, param).Insert(b).Return()
		}
		jmp := b.AllocateInstruction()
		jmp.AsJump(ValuesNil, blk)
		b.InsertInstruction(jmp)
		require.False(t, isTailDuplicationTarget(blk))
	})
}
