package asm

import "github.com/twitchyliquid64/golang-asm/obj"

type GolangAsmNode struct {
	Prog *obj.Prog
}

func NewGolangAsmNode(p *obj.Prog) Node {
	return &GolangAsmNode{Prog: p}
}

func (n *GolangAsmNode) Pc() int64 {
	return n.Prog.Pc
}

func (n *GolangAsmNode) AssignJumpTarget(target Node) {
	b := target.(*GolangAsmNode)
	n.Prog.To.SetTarget(b.Prog)
}

func (n *GolangAsmNode) AssignDestinationConstant(value int64) {
	n.Prog.To.Offset = value
}
