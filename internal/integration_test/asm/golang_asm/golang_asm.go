package golang_asm

import (
	"encoding/binary"
	"fmt"

	goasm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"

	"github.com/heeus/hwazero/internal/asm"
)

// GolangAsmNode implements Node for golang-asm library.
type GolangAsmNode struct {
	prog *obj.Prog
}

func NewGolangAsmNode(p *obj.Prog) asm.Node {
	return &GolangAsmNode{prog: p}
}

// String implements fmt.Stringer.
func (n *GolangAsmNode) String() string {
	return n.prog.String()
}

// OffsetInBinary implements the same method as documented on asm.Node.
func (n *GolangAsmNode) OffsetInBinary() asm.NodeOffsetInBinary {
	return asm.NodeOffsetInBinary(n.prog.Pc)
}

// AssignJumpTarget implements the same method as documented on asm.Node.
func (n *GolangAsmNode) AssignJumpTarget(target asm.Node) {
	b := target.(*GolangAsmNode)
	n.prog.To.SetTarget(b.prog)
}

// AssignDestinationConstant implements the same method as documented on asm.Node.
func (n *GolangAsmNode) AssignDestinationConstant(value asm.ConstantValue) {
	n.prog.To.Offset = value
}

// AssignSourceConstant implements the same method as documented on asm.Node.
func (n *GolangAsmNode) AssignSourceConstant(value asm.ConstantValue) {
	n.prog.From.Offset = value
}

// GolangAsmBaseAssembler implements *part of* AssemblerBase for golang-asm library.
type GolangAsmBaseAssembler struct {
	b *goasm.Builder

	// setBranchTargetOnNextNodes holds branch kind instructions (BR, conditional BR, etc)
	// where we want to set the next coming instruction as the destination of these BR instructions.
	setBranchTargetOnNextNodes []asm.Node

	// onGenerateCallbacks holds the callbacks which are called after generating native code.
	onGenerateCallbacks []func(code []byte) error
}

func NewGolangAsmBaseAssembler(arch string) (*GolangAsmBaseAssembler, error) {
	b, err := goasm.NewBuilder(arch, 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}
	return &GolangAsmBaseAssembler{b: b}, nil
}

// Assemble implements the same method as documented on asm.AssemblerBase.
func (a *GolangAsmBaseAssembler) Assemble() ([]byte, error) {
	code := a.b.Assemble()
	for _, cb := range a.onGenerateCallbacks {
		if err := cb(code); err != nil {
			return nil, err
		}
	}
	return code, nil
}

// SetJumpTargetOnNext implements the same method as documented on asm.AssemblerBase.
func (a *GolangAsmBaseAssembler) SetJumpTargetOnNext(nodes ...asm.Node) {
	a.setBranchTargetOnNextNodes = append(a.setBranchTargetOnNextNodes, nodes...)
}

// AddOnGenerateCallBack implements the same method as documented on asm.AssemblerBase.
func (a *GolangAsmBaseAssembler) AddOnGenerateCallBack(cb func([]byte) error) {
	a.onGenerateCallbacks = append(a.onGenerateCallbacks, cb)
}

// BuildJumpTable implements the same method as documented on asm.AssemblerBase.
func (a *GolangAsmBaseAssembler) BuildJumpTable(table []byte, labelInitialInstructions []asm.Node) {
	a.AddOnGenerateCallBack(func(code []byte) error {
		// Build the offset table for each target.
		base := labelInitialInstructions[0].OffsetInBinary()
		for i, nop := range labelInitialInstructions {
			if uint64(nop.OffsetInBinary())-uint64(base) >= asm.JumpTableMaximumOffset {
				return fmt.Errorf("too large br_table")
			}
			// We store the offset from the beginning of the L0's initial instruction.
			binary.LittleEndian.PutUint32(table[i*4:(i+1)*4], uint32(nop.OffsetInBinary())-uint32(base))
		}
		return nil
	})
}

// AddInstruction is used in architecture specific assembler implementation for golang-asm.
func (a *GolangAsmBaseAssembler) AddInstruction(next *obj.Prog) {
	a.b.AddInstruction(next)
	for _, node := range a.setBranchTargetOnNextNodes {
		n := node.(*GolangAsmNode)
		n.prog.To.SetTarget(next)
	}
	a.setBranchTargetOnNextNodes = nil
}

// NewProg is used in architecture specific assembler implementation for golang-asm.
func (a *GolangAsmBaseAssembler) NewProg() (prog *obj.Prog) {
	prog = a.b.NewProg()
	return
}
