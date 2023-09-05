package arm64

import (
	"context"
	"strings"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

func getPendingInstr(m *machine) *instruction {
	return m.pendingInstructions[0]
}

func formatEmittedInstructionsInCurrentBlock(m *machine) string {
	m.FlushPendingInstructions()
	var strs []string
	for cur := m.perBlockHead; cur != nil; cur = cur.next {
		strs = append(strs, cur.String())
	}
	return strings.Join(strs, "\n")
}

func newSetup() (ssa.Builder, *machine) {
	m := NewBackend().(*machine)
	ssaB := ssa.NewBuilder()
	backend.NewCompiler(context.Background(), m, ssaB)
	blk := ssaB.AllocateBasicBlock()
	ssaB.SetCurrentBlock(blk)
	return ssaB, m
}

func newSetupWithMockContext() (*mockCompiler, ssa.Builder, *machine) {
	ctx := newMockCompilationContext()
	m := NewBackend().(*machine)
	m.SetCompiler(ctx)
	ssaB := ssa.NewBuilder()
	blk := ssaB.AllocateBasicBlock()
	ssaB.SetCurrentBlock(blk)
	return ctx, ssaB, m
}

func regToVReg(reg regalloc.RealReg) regalloc.VReg {
	return regalloc.VReg(0).SetRealReg(reg).SetRegType(regalloc.RegTypeInt)
}

func intToVReg(i int) regalloc.VReg {
	return regalloc.VReg(i).SetRegType(regalloc.RegTypeInt)
}

// mockCompiler implements backend.Compiler for testing.
type mockCompiler struct {
	currentGID  ssa.InstructionGroupID
	vRegCounter int
	vRegMap     map[ssa.Value]regalloc.VReg
	definitions map[ssa.Value]*backend.SSAValueDefinition
	lowered     map[*ssa.Instruction]bool
	sigs        map[ssa.SignatureID]*ssa.Signature
	typeOf      map[regalloc.VReg]ssa.Type
	relocs      []backend.RelocationInfo
	buf         []byte
}

func (m *mockCompiler) AddRelocationInfo(funcRef ssa.FuncRef) {
	m.relocs = append(m.relocs, backend.RelocationInfo{FuncRef: funcRef, Offset: int64(len(m.buf))})
}

func (m *mockCompiler) Emit4Bytes(b uint32) {
	m.buf = append(m.buf, byte(b), byte(b>>8), byte(b>>16), byte(b>>24))
}

func (m *mockCompiler) Encode() int                                                      { return 0 }
func (m *mockCompiler) Buf() []byte                                                      { return m.buf }
func (m *mockCompiler) AllocateVRegWithSSAType(regalloc.RegType, ssa.Type) regalloc.VReg { return 0 }
func (m *mockCompiler) TypeOf(v regalloc.VReg) (ret ssa.Type) {
	return m.typeOf[v]
}
func (m *mockCompiler) Finalize()      {}
func (m *mockCompiler) RegAlloc()      {}
func (m *mockCompiler) Lower()         {}
func (m *mockCompiler) Format() string { return "" }
func (m *mockCompiler) Init(bool)      {}

func newMockCompilationContext() *mockCompiler {
	return &mockCompiler{
		vRegMap:     make(map[ssa.Value]regalloc.VReg),
		definitions: make(map[ssa.Value]*backend.SSAValueDefinition),
		lowered:     make(map[*ssa.Instruction]bool),
	}
}

// ResolveSignature implements backend.Compiler.
func (m *mockCompiler) ResolveSignature(id ssa.SignatureID) *ssa.Signature {
	return m.sigs[id]
}

// AllocateVReg implements backend.Compiler.
func (m *mockCompiler) AllocateVReg(regType regalloc.RegType) regalloc.VReg {
	m.vRegCounter++
	return regalloc.VReg(m.vRegCounter).SetRegType(regType)
}

// MarkLowered implements backend.Compiler.
func (m *mockCompiler) MarkLowered(inst *ssa.Instruction) {
	m.lowered[inst] = true
}

// ValueDefinition implements backend.Compiler.
func (m *mockCompiler) ValueDefinition(value ssa.Value) *backend.SSAValueDefinition {
	definition, exists := m.definitions[value]
	if !exists {
		return nil
	}
	return definition
}

// VRegOf implements backend.Compiler.
func (m *mockCompiler) VRegOf(value ssa.Value) regalloc.VReg {
	vReg, exists := m.vRegMap[value]
	if !exists {
		panic("Value does not exist")
	}
	return vReg
}

// MatchInstr implements backend.Compiler.
func (m *mockCompiler) MatchInstr(def *backend.SSAValueDefinition, opcode ssa.Opcode) bool {
	instr := def.Instr
	return def.IsFromInstr() &&
		instr.Opcode() == opcode &&
		instr.GroupID() == m.currentGID &&
		def.RefCount < 2
}

// MatchInstrOneOf implements backend.Compiler.
func (m *mockCompiler) MatchInstrOneOf(def *backend.SSAValueDefinition, opcodes []ssa.Opcode) ssa.Opcode {
	for _, opcode := range opcodes {
		if m.MatchInstr(def, opcode) {
			return opcode
		}
	}
	return ssa.OpcodeInvalid
}

// Compile implements backend.Compiler.
func (m *mockCompiler) Compile(context.Context) (_ []byte, _ []backend.RelocationInfo, goPreambleSize int, _ error) {
	return
}
