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
	m := &machine{}
	return m
}

// machine implements backend.Machine for amd64.
type machine struct {
	stackBoundsCheckDisabled bool
}

// DisableStackCheck implements backend.Machine.
func (m *machine) DisableStackCheck() {
	m.stackBoundsCheckDisabled = true
}

// RegisterInfo implements backend.Machine.
func (m *machine) RegisterInfo() *regalloc.RegisterInfo {
	// TODO implement me
	panic("implement me")
}

// InitializeABI implements backend.Machine.
func (m *machine) InitializeABI(sig *ssa.Signature) {
	// TODO implement me
	panic("implement me")
}

// ABI implements backend.Machine.
func (m *machine) ABI() backend.FunctionABI {
	// TODO implement me
	panic("implement me")
}

// SetCompiler implements backend.Machine.
func (m *machine) SetCompiler(compiler backend.Compiler) {
	// TODO implement me
	panic("implement me")
}

// StartLoweringFunction implements backend.Machine.
func (m *machine) StartLoweringFunction(maximumBlockID ssa.BasicBlockID) {
	// TODO implement me
	panic("implement me")
}

// StartBlock implements backend.Machine.
func (m *machine) StartBlock(block ssa.BasicBlock) {
	// TODO implement me
	panic("implement me")
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

// EndBlock implements backend.Machine.
func (m *machine) EndBlock() {
	// TODO implement me
	panic("implement me")
}

// LinkAdjacentBlocks implements backend.Machine.
func (m *machine) LinkAdjacentBlocks(prev, next ssa.BasicBlock) {
	// TODO implement me
	panic("implement me")
}

// EndLoweringFunction implements backend.Machine.
func (m *machine) EndLoweringFunction() {
	// TODO implement me
	panic("implement me")
}

// Reset implements backend.Machine.
func (m *machine) Reset() {
	// TODO implement me
	panic("implement me")
}

// FlushPendingInstructions implements backend.Machine.
func (m *machine) FlushPendingInstructions() {
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

// Function implements backend.Machine.
func (m *machine) Function() regalloc.Function {
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
