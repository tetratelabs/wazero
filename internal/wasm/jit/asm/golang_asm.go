package asm

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"

	goasm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
)

type golangAsmNode struct {
	prog *obj.Prog
}

func NewGolangAsmNode(p *obj.Prog) Node {
	return &golangAsmNode{prog: p}
}

func (n *golangAsmNode) Pc() int64 {
	return n.prog.Pc
}

func (n *golangAsmNode) AssignJumpTarget(target Node) {
	b := target.(*golangAsmNode)
	n.prog.To.SetTarget(b.prog)
}

func (n *golangAsmNode) AssignDestinationConstant(value int64) {
	n.prog.To.Offset = value
}

func (n *golangAsmNode) AssignSourceConstant(value int64) {
	n.prog.From.Offset = value
}

type GolangAsmBaseAssembler struct {
	b                          *goasm.Builder
	setBranchTargetOnNextNodes []Node
	// onGenerateCallbacks holds the callbacks which are called after generating native code.
	onGenerateCallbacks []func(code []byte) error
}

var _ AssemblerBase = &GolangAsmBaseAssembler{}

func NewGolangAsmBaseAssembler() (*GolangAsmBaseAssembler, error) {
	b, err := goasm.NewBuilder(runtime.GOARCH, 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}
	return &GolangAsmBaseAssembler{b: b}, nil
}

// Assemble implements AssemblerBase.Assemble
func (a *GolangAsmBaseAssembler) Assemble() ([]byte, error) {
	code := a.b.Assemble()
	for _, cb := range a.onGenerateCallbacks {
		if err := cb(code); err != nil {
			return nil, err
		}
	}
	return code, nil
}

// SetBranchTargetOnNext implements AssemblerBase.SetBranchTargetOnNext
func (a *GolangAsmBaseAssembler) SetBranchTargetOnNext(nodes ...Node) {
	a.setBranchTargetOnNextNodes = append(a.setBranchTargetOnNextNodes, nodes...)
}

func (a *GolangAsmBaseAssembler) NewProg() (prog *obj.Prog) {
	prog = a.b.NewProg()
	return
}

// AddOnGenerateCallBack implements AssemblerBase.AddOnGenerateCallBack
func (a *GolangAsmBaseAssembler) AddOnGenerateCallBack(cb func([]byte) error) {
	a.onGenerateCallbacks = append(a.onGenerateCallbacks, cb)
}

// BuildJumpTable implements AssemblerBase.BuildJumpTable
func (a *GolangAsmBaseAssembler) BuildJumpTable(table []byte, labelInitialInstructions []Node) {
	a.AddOnGenerateCallBack(func(code []byte) error {
		// Build the offset table for each target including default one.
		base := labelInitialInstructions[0].Pc() // This corresponds to the L0's address in the example.
		for i, nop := range labelInitialInstructions {
			if uint64(nop.Pc())-uint64(base) >= math.MaxUint32 {
				// TODO: this happens when users try loading an extremely large webassembly binary
				// which contains a br_table statement with approximately 4294967296 (2^32) targets.
				// We would like to support that binary, but realistically speaking, that kind of binary
				// could result in more than ten giga bytes of native JITed code where we have to care about
				// huge stacks whose height might exceed 32-bit range, and such huge stack doesn't work with the
				// current implementation.
				return fmt.Errorf("too large br_table")
			}
			// We store the offset from the beginning of the L0's initial instruction.
			binary.LittleEndian.PutUint32(table[i*4:(i+1)*4], uint32(nop.Pc())-uint32(base))
		}
		return nil
	})
}

func (a *GolangAsmBaseAssembler) AddInstruction(next *obj.Prog) {
	a.b.AddInstruction(next)
	for _, node := range a.setBranchTargetOnNextNodes {
		n := node.(*golangAsmNode)
		n.prog.To.SetTarget(next)
	}
	a.setBranchTargetOnNextNodes = nil
}
