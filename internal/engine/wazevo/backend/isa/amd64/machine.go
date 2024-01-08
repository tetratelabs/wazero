package amd64

import (
	"context"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// NewBackend returns a new backend for arm64.
func NewBackend() backend.Machine {
	ectx := backend.NewExecutableContextT[instruction](
		resetInstruction,
		setNext,
		setPrev,
		asNop,
	)
	return &machine{
		ectx:     ectx,
		regAlloc: regalloc.NewAllocator(regInfo),
	}
}

// machine implements backend.Machine for amd64.
type machine struct {
	c                        backend.Compiler
	ectx                     *backend.ExecutableContextT[instruction]
	stackBoundsCheckDisabled bool

	regAlloc        regalloc.Allocator
	regAllocFn      *backend.RegAllocFunction[*instruction, *machine]
	regAllocStarted bool

	spillSlotSize int64
	currentABI    *backend.FunctionABI
	clobberedRegs []regalloc.VReg
}

// Reset implements backend.Machine.
func (m *machine) Reset() {
	m.stackBoundsCheckDisabled = false
	m.ectx.Reset()

	m.regAllocFn.Reset()
	m.regAlloc.Reset()
	m.regAllocStarted = false
}

// ExecutableContext implements backend.Machine.
func (m *machine) ExecutableContext() backend.ExecutableContext { return m.ectx }

// DisableStackCheck implements backend.Machine.
func (m *machine) DisableStackCheck() { m.stackBoundsCheckDisabled = true }

// SetCompiler implements backend.Machine.
func (m *machine) SetCompiler(compiler backend.Compiler) { m.c = compiler }

// SetCurrentABI implements backend.Machine.
func (m *machine) SetCurrentABI(abi *backend.FunctionABI) {
	m.currentABI = abi
}

// RegAlloc implements backend.Machine.
func (m *machine) RegAlloc() {
	rf := m.regAllocFn
	for _, pos := range m.ectx.OrderedBlockLabels {
		rf.AddBlock(pos.SB, pos.L, pos.Begin, pos.End)
	}

	m.regAllocStarted = true
	m.regAlloc.DoAllocation(rf)
	// Now that we know the final spill slot size, we must align spillSlotSize to 16 bytes.
	m.spillSlotSize = (m.spillSlotSize + 15) &^ 15
}

// LowerSingleBranch implements backend.Machine.
func (m *machine) LowerSingleBranch(b *ssa.Instruction) {
	// TODO implement me
	panic("implement me")
}

// LowerConditionalBranch implements backend.Machine.
func (m *machine) LowerConditionalBranch(b *ssa.Instruction) {
	// TODO implement me
	panic("implement me")
}

// LowerInstr implements backend.Machine.
func (m *machine) LowerInstr(instruction *ssa.Instruction) {
	// TODO implement me
	panic("implement me")
}

// InsertMove implements backend.Machine.
func (m *machine) InsertMove(dst, src regalloc.VReg, typ ssa.Type) {
	// TODO implement me
	panic("implement me")
}

// InsertReturn implements backend.Machine.
func (m *machine) InsertReturn() {
	// TODO implement me
	panic("implement me")
}

// InsertLoadConstant implements backend.Machine.
func (m *machine) InsertLoadConstant(instr *ssa.Instruction, vr regalloc.VReg) {
	// TODO implement me
	panic("implement me")
}

// Format implements backend.Machine.
func (m *machine) Format() string {
	// TODO implement me
	panic("implement me")
}

// SetupPrologue implements backend.Machine.
func (m *machine) SetupPrologue() {
	// TODO implement me
	panic("implement me")
}

// SetupEpilogue implements backend.Machine.
func (m *machine) SetupEpilogue() {
	// TODO implement me
	panic("implement me")
}

// ResolveRelativeAddresses implements backend.Machine.
func (m *machine) ResolveRelativeAddresses(ctx context.Context) {
	// TODO implement me
	panic("implement me")
}

// ResolveRelocations implements backend.Machine.
func (m *machine) ResolveRelocations(refToBinaryOffset map[ssa.FuncRef]int, binary []byte, relocations []backend.RelocationInfo) {
	// TODO implement me
	panic("implement me")
}

// Encode implements backend.Machine.
func (m *machine) Encode() {
	// TODO implement me
	panic("implement me")
}

// CompileGoFunctionTrampoline implements backend.Machine.
func (m *machine) CompileGoFunctionTrampoline(exitCode wazevoapi.ExitCode, sig *ssa.Signature, needModuleContextPtr bool) []byte {
	// TODO implement me
	panic("implement me")
}

// CompileStackGrowCallSequence implements backend.Machine.
func (m *machine) CompileStackGrowCallSequence() []byte {
	// TODO implement me
	panic("implement me")
}

// CompileEntryPreamble implements backend.Machine.
func (m *machine) CompileEntryPreamble(signature *ssa.Signature) []byte {
	// TODO implement me
	panic("implement me")
}
