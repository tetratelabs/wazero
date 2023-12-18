package backend

import (
	"context"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// mockMachine implements Machine for testing.
type mockMachine struct {
	abi                    mockABI
	startLoweringFunction  func(id ssa.BasicBlockID)
	startBlock             func(block ssa.BasicBlock)
	lowerSingleBranch      func(b *ssa.Instruction)
	lowerConditionalBranch func(b *ssa.Instruction)
	lowerInstr             func(instruction *ssa.Instruction)
	endBlock               func()
	endLoweringFunction    func()
	reset                  func()
	insertMove             func(dst, src regalloc.VReg)
	insertLoadConstant     func(instr *ssa.Instruction, vr regalloc.VReg)
	format                 func() string
	linkAdjacentBlocks     func(prev, next ssa.BasicBlock)
	rinfo                  *regalloc.RegisterInfo
}

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
func (m mockMachine) Encode() {}

// ResolveRelocations implements Machine.ResolveRelocations.
func (m mockMachine) ResolveRelocations(map[ssa.FuncRef]int, []byte, []RelocationInfo) {}

// SetupPrologue implements Machine.SetupPrologue.
func (m mockMachine) SetupPrologue() {}

// SetupEpilogue implements Machine.SetupEpilogue.
func (m mockMachine) SetupEpilogue() {}

// ResolveRelativeAddresses implements Machine.ResolveRelativeAddresses.
func (m mockMachine) ResolveRelativeAddresses(ctx context.Context) {}

// Function implements Machine.Function.
func (m mockMachine) Function() (f regalloc.Function) { return }

// RegisterInfo implements Machine.RegisterInfo.
func (m mockMachine) RegisterInfo() *regalloc.RegisterInfo {
	if m.rinfo != nil {
		return m.rinfo
	}
	return &regalloc.RegisterInfo{}
}

// InsertReturn implements Machine.InsertReturn.
func (m mockMachine) InsertReturn() { panic("TODO") }

// LinkAdjacentBlocks implements Machine.LinkAdjacentBlocks.
func (m mockMachine) LinkAdjacentBlocks(prev, next ssa.BasicBlock) { m.linkAdjacentBlocks(prev, next) }

// InitializeABI implements Machine.InitializeABI.
func (m mockMachine) InitializeABI(*ssa.Signature) {}

// ABI implements Machine.ABI.
func (m mockMachine) ABI() FunctionABI { return m.abi }

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

// InsertLoadConstant implements Machine.InsertLoadConstant.
func (m mockMachine) InsertLoadConstant(instr *ssa.Instruction, vr regalloc.VReg) {
	m.insertLoadConstant(instr, vr)
}

// Format implements Machine.Format.
func (m mockMachine) Format() string {
	return m.format()
}

// DisableStackCheck implements Machine.DisableStackCheck.
func (m mockMachine) DisableStackCheck() {}

var _ Machine = (*mockMachine)(nil)

// mockABI implements ABI for testing.
type mockABI struct{}

func (m mockABI) EmitGoEntryPreamble() {}

func (m mockABI) CalleeGenFunctionArgsToVRegs(regs []ssa.Value) {
	panic("TODO")
}

func (m mockABI) CalleeGenVRegsToFunctionReturns(regs []ssa.Value) {
	panic("TODO")
}

var _ FunctionABI = (*mockABI)(nil)
