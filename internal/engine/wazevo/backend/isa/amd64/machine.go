package amd64

import (
	"context"
	"fmt"
	"strings"

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

type (
	// machine implements backend.Machine for amd64.
	machine struct {
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
)

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

// InsertReturn implements backend.Machine.
func (m *machine) InsertReturn() {
	i := m.allocateInstr().asRet(m.currentABI)
	m.insert(i)
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
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		i := m.allocateInstr().asMovRR(src, dst, typ.Bits() == 64)
		m.insert(i)
	case ssa.TypeF32, ssa.TypeF64, ssa.TypeV128:
		var op sseOpcode
		switch typ {
		case ssa.TypeF32:
			op = sseOpcodeMovss
		case ssa.TypeF64:
			op = sseOpcodeMovsd
		case ssa.TypeV128:
			op = sseOpcodeMovdqa
		}
		i := m.allocateInstr().asXmmUnaryRmR(op, operand{kind: operandKindReg, r: src}, dst, typ.Bits() == 64)
		m.insert(i)
	default:
		panic("BUG")
	}
}

// Format implements backend.Machine.
func (m *machine) Format() string {
	ectx := m.ectx
	begins := map[*instruction]backend.Label{}
	for l, pos := range ectx.LabelPositions {
		begins[pos.Begin] = l
	}

	irBlocks := map[backend.Label]ssa.BasicBlockID{}
	for i, l := range ectx.SsaBlockIDToLabels {
		irBlocks[l] = ssa.BasicBlockID(i)
	}

	var lines []string
	for cur := ectx.RootInstr; cur != nil; cur = cur.next {
		if l, ok := begins[cur]; ok {
			var labelStr string
			if blkID, ok := irBlocks[l]; ok {
				labelStr = fmt.Sprintf("%s (SSA Block: %s):", l, blkID)
			} else {
				labelStr = fmt.Sprintf("%s:", l)
			}
			lines = append(lines, labelStr)
		}
		if cur.kind == nop0 {
			continue
		}
		lines = append(lines, "\t"+cur.String())
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
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

// allocateInstr allocates an instruction.
func (m *machine) allocateInstr() *instruction {
	instr := m.ectx.InstructionPool.Allocate()
	if !m.regAllocStarted {
		instr.addedBeforeRegAlloc = true
	}
	return instr
}

func (m *machine) insert(i *instruction) {
	ectx := m.ectx
	ectx.PendingInstructions = append(ectx.PendingInstructions, i)
}
