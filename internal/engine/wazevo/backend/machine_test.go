package backend

import (
	"context"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// mockMachine implements Machine for testing.
type mockMachine struct {
	argResultInts, argResultFloats []regalloc.RealReg
	startLoweringFunction          func(id ssa.BasicBlockID)
	startBlock                     func(block ssa.BasicBlock)
	lowerSingleBranch              func(b *ssa.Instruction)
	lowerConditionalBranch         func(b *ssa.Instruction)
	lowerInstr                     func(instruction *ssa.Instruction)
	endBlock                       func()
	endLoweringFunction            func()
	reset                          func()
	insertMove                     func(dst, src regalloc.VReg)
	insertLoadConstant             func(instr *ssa.Instruction, vr regalloc.VReg)
	format                         func() string
	linkAdjacentBlocks             func(prev, next ssa.BasicBlock)
}

func (m mockMachine) ArgsResultsRegs() (argResultInts, argResultFloats []regalloc.RealReg) {
	return m.argResultInts, m.argResultFloats
}

func (m mockMachine) RegAlloc() { panic("implement me") }

func (m mockMachine) LowerParams(params []ssa.Value) { panic("implement me") }

func (m mockMachine) LowerReturns(returns []ssa.Value) { panic("implement me") }

func (m mockMachine) ExecutableContext() ExecutableContext { panic("implement me") }

func (m mockMachine) CompileEntryPreamble(signature *ssa.Signature) []byte {
	panic("TODO")
}

func (m mockMachine) CompileStackGrowCallSequence() []byte {
	panic("TODO")
}

// CompileGoFunctionTrampoline implements Machine.CompileGoFunctionTrampoline.
func (m mockMachine) CompileGoFunctionTrampoline(wazevoapi.ExitCode, *ssa.Signature, bool) []byte {
	panic("TODO")
}

// Encode implements Machine.Encode.
func (m mockMachine) Encode(ctx context.Context) {}

// ResolveRelocations implements Machine.ResolveRelocations.
func (m mockMachine) ResolveRelocations(map[ssa.FuncRef]int, []byte, []RelocationInfo) {}

// UpdateRelocationInfo implements Machine.UpdateRelocationInfo.
func (m mockMachine) UpdateRelocationInfo(refToBinaryOffset map[ssa.FuncRef]int, trampolineOffset int, r RelocationInfo) (RelocationInfo, int) {
	return r, 0
}

// RelocationTrampolineSize implements Machine.RelocationTrampolineSize.
func (m mockMachine) RelocationTrampolineSize(rels []RelocationInfo) int {
	return 0
}

// PostRegAlloc implements Machine.SetupPrologue.
func (m mockMachine) PostRegAlloc() {}

// InsertReturn implements Machine.InsertReturn.
func (m mockMachine) InsertReturn() { panic("TODO") }

// LinkAdjacentBlocks implements Machine.LinkAdjacentBlocks.
func (m mockMachine) LinkAdjacentBlocks(prev, next ssa.BasicBlock) { m.linkAdjacentBlocks(prev, next) }

// SetCurrentABI implements Machine.SetCurrentABI.
func (m mockMachine) SetCurrentABI(*FunctionABI) {}

// SetCompiler implements Machine.SetCompiler.
func (m mockMachine) SetCompiler(Compiler) {}

// StartLoweringFunction implements Machine.StartLoweringFunction.
func (m mockMachine) StartLoweringFunction(id ssa.BasicBlockID) {
	m.startLoweringFunction(id)
}

// StartBlock implements Machine.StartBlock.
func (m mockMachine) StartBlock(block ssa.BasicBlock) {
	m.startBlock(block)
}

// LowerSingleBranch implements Machine.LowerSingleBranch.
func (m mockMachine) LowerSingleBranch(b *ssa.Instruction) {
	m.lowerSingleBranch(b)
}

// LowerConditionalBranch implements Machine.LowerConditionalBranch.
func (m mockMachine) LowerConditionalBranch(b *ssa.Instruction) {
	m.lowerConditionalBranch(b)
}

// LowerInstr implements Machine.LowerInstr.
func (m mockMachine) LowerInstr(instruction *ssa.Instruction) {
	m.lowerInstr(instruction)
}

// EndBlock implements Machine.EndBlock.
func (m mockMachine) EndBlock() {
	m.endBlock()
}

// EndLoweringFunction implements Machine.EndLoweringFunction.
func (m mockMachine) EndLoweringFunction() {
	m.endLoweringFunction()
}

// Reset implements Machine.Reset.
func (m mockMachine) Reset() {
	m.reset()
}

// FlushPendingInstructions implements Machine.FlushPendingInstructions.
func (m mockMachine) FlushPendingInstructions() {}

// InsertMove implements Machine.InsertMove.
func (m mockMachine) InsertMove(dst, src regalloc.VReg, typ ssa.Type) {
	m.insertMove(dst, src)
}

// InsertLoadConstantBlockArg implements Machine.InsertLoadConstantBlockArg.
func (m mockMachine) InsertLoadConstantBlockArg(instr *ssa.Instruction, vr regalloc.VReg) {
	m.insertLoadConstant(instr, vr)
}

// Format implements Machine.Format.
func (m mockMachine) Format() string {
	return m.format()
}

// DisableStackCheck implements Machine.DisableStackCheck.
func (m mockMachine) DisableStackCheck() {}

var _ Machine = (*mockMachine)(nil)
