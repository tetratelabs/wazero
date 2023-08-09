package arm64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestRegAllocFunctionImpl_addBlock(t *testing.T) {
	ssab := ssa.NewBuilder()
	sb1, sb2 := ssab.AllocateBasicBlock(), ssab.AllocateBasicBlock()
	p1, p2 := &labelPosition{}, &labelPosition{}

	f := &regAllocFunctionImpl{labelToRegAllocBlockIndex: map[label]int{}}
	f.addBlock(sb1, label(10), p1)
	f.addBlock(sb2, label(20), p2)

	require.Equal(t, 2, len(f.labelToRegAllocBlockIndex))
	require.Equal(t, 2, len(f.reversePostOrderBlocks))

	rb1, rb2 := &f.reversePostOrderBlocks[0], &f.reversePostOrderBlocks[1]
	require.Equal(t, f, rb1.f)
	require.Equal(t, f, rb2.f)
	require.Equal(t, f, rb1.instrImpl.f)
	require.Equal(t, f, rb2.instrImpl.f)

	require.Equal(t, p1, rb1.pos)
	require.Equal(t, p2, rb2.pos)

	require.Equal(t, label(10), rb1.l)
	require.Equal(t, label(20), rb2.l)

	require.Equal(t, sb1, rb1.sb)
	require.Equal(t, sb2, rb2.sb)
}

func TestRegAllocFunctionImpl_PostOrderBlockIterator(t *testing.T) {
	f := &regAllocFunctionImpl{reversePostOrderBlocks: []regAllocBlockImpl{{}, {}, {}}}
	blk := f.PostOrderBlockIteratorBegin()
	require.Equal(t, blk, &f.reversePostOrderBlocks[2])
	blk = f.PostOrderBlockIteratorNext()
	require.Equal(t, blk, &f.reversePostOrderBlocks[1])
	blk = f.PostOrderBlockIteratorNext()
	require.Equal(t, blk, &f.reversePostOrderBlocks[0])
	blk = f.PostOrderBlockIteratorNext()
	require.Nil(t, blk)
}

func TestRegAllocFunctionImpl_ReversePostOrderBlockIterator(t *testing.T) {
	f := &regAllocFunctionImpl{reversePostOrderBlocks: []regAllocBlockImpl{{}, {}, {}}}
	blk := f.ReversePostOrderBlockIteratorBegin()
	require.Equal(t, blk, &f.reversePostOrderBlocks[0])
	blk = f.ReversePostOrderBlockIteratorNext()
	require.Equal(t, blk, &f.reversePostOrderBlocks[1])
	blk = f.ReversePostOrderBlockIteratorNext()
	require.Equal(t, blk, &f.reversePostOrderBlocks[2])
	blk = f.ReversePostOrderBlockIteratorNext()
	require.Nil(t, blk)
}

func TestRegAllocFunctionImpl_ReloadRegisterAfter(t *testing.T) {
	ctx, _, m := newSetupWithMockContext()
	m.clobberedRegs = make([]regalloc.VReg, 4) // This will make the beginning of the spill slot at 4 * 16 bytes = 64.

	ctx.typeOf = map[regalloc.VReg]ssa.Type{x1VReg: ssa.TypeI64, v1VReg: ssa.TypeF64}
	i1, i2 := m.allocateNop(), m.allocateNop()
	i1.next = i2
	i2.prev = i1

	f := &regAllocFunctionImpl{m: m}
	f.ReloadRegisterAfter(x1VReg, &regAllocInstrImpl{i: i1})
	f.ReloadRegisterAfter(v1VReg, &regAllocInstrImpl{i: i1})

	require.NotEqual(t, i1, i2.prev)
	require.NotEqual(t, i1.next, i2)
	fload, iload := i1.next, i1.next.next
	require.Equal(t, fload.prev, i1)
	require.Equal(t, i1, fload.prev)
	require.Equal(t, iload.next, i2)
	require.Equal(t, iload, i2.prev)

	require.Equal(t, iload.kind, uLoad64)
	require.Equal(t, fload.kind, fpuLoad64)

	m.rootInstr = i1
	require.Equal(t, `
	ldr d1, [sp, #0x48]
	ldr x1, [sp, #0x40]
`, m.Format())
}

func TestRegAllocFunctionImpl_StoreRegisterBefore(t *testing.T) {
	ctx, _, m := newSetupWithMockContext()
	m.clobberedRegs = make([]regalloc.VReg, 4) // This will make the beginning of the spill slot at 4 * 16 bytes = 64.

	ctx.typeOf = map[regalloc.VReg]ssa.Type{x1VReg: ssa.TypeI64, v1VReg: ssa.TypeF64}
	i1, i2 := m.allocateNop(), m.allocateNop()
	i1.next = i2
	i2.prev = i1

	f := &regAllocFunctionImpl{m: m}
	f.StoreRegisterBefore(x1VReg, &regAllocInstrImpl{i: i2})
	f.StoreRegisterBefore(v1VReg, &regAllocInstrImpl{i: i2})

	require.NotEqual(t, i1, i2.prev)
	require.NotEqual(t, i1.next, i2)
	iload, fload := i1.next, i1.next.next
	require.Equal(t, iload.prev, i1)
	require.Equal(t, i1, iload.prev)
	require.Equal(t, fload.next, i2)
	require.Equal(t, fload, i2.prev)

	require.Equal(t, iload.kind, store64)
	require.Equal(t, fload.kind, fpuStore64)

	m.rootInstr = i1
	require.Equal(t, `
	str x1, [sp, #0x40]
	str d1, [sp, #0x48]
`, m.Format())
}

func TestRegAllocFunctionImpl_ClobberedRegisters(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	f := &regAllocFunctionImpl{m: m}
	f.ClobberedRegisters([]regalloc.VReg{v19VReg, v19VReg, v19VReg, v19VReg})
	require.Equal(t, []regalloc.VReg{v19VReg, v19VReg, v19VReg, v19VReg}, m.clobberedRegs)
}
