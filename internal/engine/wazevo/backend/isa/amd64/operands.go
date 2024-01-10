package amd64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
)

type operand struct {
	kind  operandKind
	r     regalloc.VReg
	imm32 uint32
	amode amode
}

type operandKind byte

const (
	// operandKindReg is an operand which is an integer Register.
	operandKindReg operandKind = iota + 1

	// operandKindMem is an operand which is either an integer Register or a value in Memory.  This can denote an 8, 16,
	// 32, 64, or 128 bit value.
	operandKindMem

	// operandKindRegMemImm is either an integer Register, a value in Memory or an Immediate.
	operandImm32
)

func (o *operand) format(_64 bool) string {
	switch o.kind {
	case operandKindReg:
		return formatVRegSized(o.r, _64)
	case operandKindMem:
		return o.amode.String()
	case operandImm32:
		return fmt.Sprintf("$%d", int32(o.imm32))
	default:
		panic("BUG: invalid operand kind")
	}
}

func newOperandReg(r regalloc.VReg) operand {
	return operand{kind: operandKindReg, r: r}
}

func newOperandImm32(imm32 uint32) operand {
	return operand{kind: operandImm32, imm32: imm32}
}

// nolint
type amode struct {
	kind  amodeKind
	imm32 uint32

	// For amodeRegRegShit:

	base  regalloc.VReg
	index regalloc.VReg
	shift byte // 0, 1, 2, 3
}

type amodeKind byte

const (

	// immediate sign-extended and a Register.
	amodeImmReg amodeKind = iota + 1

	// sign-extend-32-to-64(Immediate) + Register1 + (Register2 << Shift)
	amodeRegRegShit
)

func (a *amode) String() string {
	switch a.kind {
	case amodeImmReg:
		panic("TODO")
	case amodeRegRegShit:
		panic("TODO")
	}
	panic("BUG: invalid amode kind")
}
