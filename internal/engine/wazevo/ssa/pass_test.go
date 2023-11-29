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
					brz.AsBrz(value, nil, middle1)
					b.InsertInstruction(brz)

					jmp := b.AllocateInstruction()
					jmp.AsJump(nil, middle2)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(middle1)
				{
					jmp := b.AllocateInstruction()
					jmp.AsJump(nil, end)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(middle2)
				{
					jmp := b.AllocateInstruction()
					jmp.AsJump(nil, end)
					b.InsertInstruction(jmp)
				}

				{
					unreachable := b.AllocateBasicBlock()
					b.SetCurrentBlock(unreachable)
					jmp := b.AllocateInstruction()
					jmp.AsJump(nil, end)
					b.InsertInstruction(jmp)
				}

				b.SetCurrentBlock(end)
				{
					jmp := b.AllocateInstruction()
					jmp.AsJump(nil, middle1)
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
					jmp.AsJump([]Value{iConst}, loopHeader)
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

					brz := b.AllocateInstruction()
					brz.AsBrz(value, []Value{tmp}, loopHeader) // Loop to itself.
					b.InsertInstruction(brz)

					jmp := b.AllocateInstruction()
					jmp.AsJump(nil, end)
					b.InsertInstruction(jmp)
				}
				b.Seal(loopHeader)

				b.SetCurrentBlock(end)
				{
					ret := b.AllocateInstruction()
					ret.AsReturn(nil)
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
				jmp.AsJump(nil, end)
				b.InsertInstruction(jmp)

				b.SetCurrentBlock(end)
				aliasedRefOnceVal := b.allocateValue(refOnceVal.Type())
				b.alias(aliasedRefOnceVal, refOnceVal)

				add := b.AllocateInstruction()
				add.AsIadd(aliasedRefOnceVal, refThriceVal)
				b.InsertInstruction(add)

				addRes := add.Return()

				ret := b.AllocateInstruction()
				ret.AsReturn([]Value{addRes})
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

				// Iadd32 x + 0 should resolve to const.
				zeroI32 := b.AllocateInstruction().AsIconst32(0).Insert(b).Return()
				nopIadd32 := b.AllocateInstruction().AsIadd(i32Param, zeroI32).Insert(b).Return()

				// Iadd32 0 + x should resolve to const.
				zeroI32_2 := b.AllocateInstruction().AsIconst32(0).Insert(b).Return()
				nopIadd32_2 := b.AllocateInstruction().AsIadd(zeroI32_2, i32Param).Insert(b).Return()

				// Iadd64 x + 0 should resolve to const.
				zeroI64 := b.AllocateInstruction().AsIconst64(0).Insert(b).Return()
				nopIadd64 := b.AllocateInstruction().AsIadd(i64Param, zeroI64).Insert(b).Return()

				// Iadd64 0 + x should resolve to const.
				zeroI64_2 := b.AllocateInstruction().AsIconst64(0).Insert(b).Return()
				nopIadd64_2 := b.AllocateInstruction().AsIadd(zeroI64_2, i64Param).Insert(b).Return()

				ret := b.AllocateInstruction()
				ret.AsReturn([]Value{nopIshl, nopUshr, nonZeroIshl, nonZeroSshr, nopIadd32, nopIadd32_2, nopIadd64, nopIadd64_2})
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
	v10:i32 = Iconst_32 0x0
	v11:i32 = Iadd v0, v10
	v12:i32 = Iconst_32 0x0
	v13:i32 = Iadd v12, v0
	v14:i64 = Iconst_64 0x0
	v15:i64 = Iadd v1, v14
	v16:i64 = Iconst_64 0x0
	v17:i64 = Iadd v16, v1
	Return v3, v5, v7, v9, v11, v13, v15, v17
`,
			after: `
blk0: (v0:i32, v1:i64)
	v6:i32 = Iconst_32 0x1ea1
	v7:i32 = Ishl v0, v6
	v8:i64 = Iconst_64 0x3d41
	v9:i64 = Sshr v1, v8
	Return v0, v1, v7, v9, v0, v0, v1, v1
`,
		},
		{
			name:     "const folding",
			pass:     passConstFoldingOpt,
			postPass: passDeadCodeEliminationOpt,
			setup: func(b *builder) (verifier func(t *testing.T)) {
				entry := b.AllocateBasicBlock()
				b.SetCurrentBlock(entry)

				// Iadd32 const1 + const2 should resolve to Const (const1 + const2).
				nonZeroI32_1 := b.AllocateInstruction().AsIconst32(0x1).Insert(b).Return()
				nonZeroI32_2 := b.AllocateInstruction().AsIconst32(0x2).Insert(b).Return()
				foldIaddI32_1 := b.AllocateInstruction().AsIadd(nonZeroI32_1, nonZeroI32_2).Insert(b).Return()

				// Iadd32 foldedConst1, const3 should resolve to Const (foldedConst1, const3).
				nonZeroI32_3 := b.AllocateInstruction().AsIconst32(0x3).Insert(b).Return()
				foldIaddI32_2 := b.AllocateInstruction().AsIadd(foldIaddI32_1, nonZeroI32_3).Insert(b).Return()

				// Isub32 foldedConst1, const3 should resolve to Const (const4, foldedConst2).
				nonZeroI32_4 := b.AllocateInstruction().AsIconst32(0x4).Insert(b).Return()
				foldIsubI32_1 := b.AllocateInstruction().AsIsub(nonZeroI32_4, foldIaddI32_2).Insert(b).Return()

				// Imul32 foldedConst, foldedConst should resolve to IConst32 (foldedConst * foldedConst).
				foldImulI32_1 := b.AllocateInstruction().AsImul(foldIsubI32_1, foldIsubI32_1).Insert(b).Return()

				// Iadd64 const1 + const2 should resolve to Const (const1 + const2).
				nonZeroI64_1 := b.AllocateInstruction().AsIconst64(0x1).Insert(b).Return()
				nonZeroI64_2 := b.AllocateInstruction().AsIconst64(0x2).Insert(b).Return()
				foldIaddI64_1 := b.AllocateInstruction().AsIadd(nonZeroI64_1, nonZeroI64_2).Insert(b).Return()

				// Iadd64 foldedConst1, const3 should resolve to Const (foldedConst1, const3).
				nonZeroI64_3 := b.AllocateInstruction().AsIconst64(0x3).Insert(b).Return()
				foldIaddI64_2 := b.AllocateInstruction().AsIadd(foldIaddI64_1, nonZeroI64_3).Insert(b).Return()

				// Isub64 const4, foldedConst1 should resolve to Const (const4, foldedConst2).
				nonZeroI64_4 := b.AllocateInstruction().AsIconst64(0x4).Insert(b).Return()
				foldIsubI64_1 := b.AllocateInstruction().AsIsub(nonZeroI64_4, foldIaddI64_2).Insert(b).Return()

				// Imul64 foldedConst, foldedConst should resolve to IConst64 (foldedConst * foldedConst).
				foldImulI64_1 := b.AllocateInstruction().AsImul(foldIsubI64_1, foldIsubI64_1).Insert(b).Return()

				// Fadd32 const1 + const2 should resolve to Const (const1 + const2).
				nonZeroF32_1 := b.AllocateInstruction().AsF32const(1.0).Insert(b).Return()
				nonZeroF32_2 := b.AllocateInstruction().AsF32const(2.0).Insert(b).Return()
				foldFaddF32_1 := b.AllocateInstruction().AsFadd(nonZeroF32_1, nonZeroF32_2).Insert(b).Return()

				// Fadd32 foldedConst1, const3 should resolve to Const (foldedConst1 + const3).
				nonZeroF32_3 := b.AllocateInstruction().AsF32const(3.0).Insert(b).Return()
				foldIaddF32_2 := b.AllocateInstruction().AsFadd(foldFaddF32_1, nonZeroF32_3).Insert(b).Return()

				// Fsub32 const4, foldedConst1 should resolve to Const (const4 - foldedConst2).
				nonZeroF32_4 := b.AllocateInstruction().AsF32const(4.0).Insert(b).Return()
				foldIsubF32_1 := b.AllocateInstruction().AsFsub(nonZeroF32_4, foldIaddF32_2).Insert(b).Return()

				// Fmul32 foldedConst, foldedConst should resolve to FConst32 (foldedConst * foldedConst).
				foldFmulF32_1 := b.AllocateInstruction().AsFmul(foldIsubF32_1, foldIsubF32_1).Insert(b).Return()

				// Fadd64 const1 + const2 should resolve to FConst64 (const1 + const2).
				nonZeroF64_1 := b.AllocateInstruction().AsF64const(1.0).Insert(b).Return()
				nonZeroF64_2 := b.AllocateInstruction().AsF64const(2.0).Insert(b).Return()
				// This intermediate value won't be dropped because it is referenced in the result.
				foldFaddF64_1 := b.AllocateInstruction().AsFadd(nonZeroF64_1, nonZeroF64_2).Insert(b).Return()

				// Fadd64 foldedConst1, const3 should resolve to FConst64 (foldedConst1 + const3).
				nonZeroF64_3 := b.AllocateInstruction().AsF64const(3.0).Insert(b).Return()
				foldFaddF64_2 := b.AllocateInstruction().AsFadd(foldFaddF64_1, nonZeroF64_3).Insert(b).Return()

				// Fsub64 const4, foldedConst1 should resolve to FConst64 (const4 - foldedConst2).
				nonZeroF64_4 := b.AllocateInstruction().AsF64const(4.0).Insert(b).Return()
				foldFsubF64_1 := b.AllocateInstruction().AsFsub(nonZeroF64_4, foldFaddF64_2).Insert(b).Return()

				// Fmul64 foldedConst, foldedConst should resolve to FConst64 (foldedConst * foldedConst).
				foldFmulF64_1 := b.AllocateInstruction().AsFmul(foldFsubF64_1, foldFsubF64_1).Insert(b).Return()

				ret := b.AllocateInstruction()
				ret.AsReturn([]Value{
					foldImulI32_1,
					foldIsubI64_1,
					foldImulI64_1,
					foldIsubF32_1,
					foldFmulF32_1,
					foldFaddF64_1,
					foldFsubF64_1,
					foldFmulF64_1,
				})
				b.InsertInstruction(ret)
				return nil
			},
			before: `
blk0: ()
	v0:i32 = Iconst_32 0x1
	v1:i32 = Iconst_32 0x2
	v2:i32 = Iadd v0, v1
	v3:i32 = Iconst_32 0x3
	v4:i32 = Iadd v2, v3
	v5:i32 = Iconst_32 0x4
	v6:i32 = Isub v5, v4
	v7:i32 = Imul v6, v6
	v8:i64 = Iconst_64 0x1
	v9:i64 = Iconst_64 0x2
	v10:i64 = Iadd v8, v9
	v11:i64 = Iconst_64 0x3
	v12:i64 = Iadd v10, v11
	v13:i64 = Iconst_64 0x4
	v14:i64 = Isub v13, v12
	v15:i64 = Imul v14, v14
	v16:f32 = F32const 1.000000
	v17:f32 = F32const 2.000000
	v18:f32 = Fadd v16, v17
	v19:f32 = F32const 3.000000
	v20:f32 = Fadd v18, v19
	v21:f32 = F32const 4.000000
	v22:f32 = Fsub v21, v20
	v23:f32 = Fmul v22, v22
	v24:f64 = F64const 1.000000
	v25:f64 = F64const 2.000000
	v26:f64 = Fadd v24, v25
	v27:f64 = F64const 3.000000
	v28:f64 = Fadd v26, v27
	v29:f64 = F64const 4.000000
	v30:f64 = Fsub v29, v28
	v31:f64 = Fmul v30, v30
	Return v7, v14, v15, v22, v23, v26, v30, v31
`,
			after: `
blk0: ()
	v7:i32 = Iconst_32 0x4
	v14:i64 = Iconst_64 0xfffffffffffffffe
	v15:i64 = Iconst_64 0x4
	v22:f32 = F32const -2.000000
	v23:f32 = F32const 4.000000
	v26:f64 = F64const 3.000000
	v30:f64 = F64const -2.000000
	v31:f64 = F64const 4.000000
	Return v7, v14, v15, v22, v23, v26, v30, v31
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
