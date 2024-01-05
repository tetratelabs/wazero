package backend

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

type FunctionABIRegInfo interface {
	// ArgsResultsRegs returns the registers used for passing parameters.
	ArgsResultsRegs() (argInts, argFloats, resultInt, resultFloats []regalloc.RealReg)
}

type (
	FunctionABI[R FunctionABIRegInfo] struct {
		r           R
		Initialized bool

		Args, Rets                 []ABIArg
		ArgStackSize, RetStackSize int64

		ArgRealRegs []regalloc.VReg
		RetRealRegs []regalloc.VReg
	}

	// ABIArg represents either argument or return value's location.
	ABIArg struct {
		// Index is the index of the argument.
		Index int
		// Kind is the kind of the argument.
		Kind ABIArgKind
		// Reg is valid if Kind == ABIArgKindReg.
		// This VReg must be based on RealReg.
		Reg regalloc.VReg
		// Offset is valid if Kind == ABIArgKindStack.
		// This is the offset from the beginning of either arg or ret stack slot.
		Offset int64
		// Type is the type of the argument.
		Type ssa.Type
	}

	// ABIArgKind is the kind of ABI argument.
	ABIArgKind byte
)

const (
	// ABIArgKindReg represents an argument passed in a register.
	ABIArgKindReg = iota
	// ABIArgKindStack represents an argument passed in the stack.
	ABIArgKindStack
)

// String implements fmt.Stringer.
func (a *ABIArg) String() string {
	return fmt.Sprintf("args[%d]: %s", a.Index, a.Kind)
}

// String implements fmt.Stringer.
func (a ABIArgKind) String() string {
	switch a {
	case ABIArgKindReg:
		return "reg"
	case ABIArgKindStack:
		return "stack"
	default:
		panic("BUG")
	}
}

// Init initializes the abiImpl for the given signature.
func (a *FunctionABI[M]) Init(sig *ssa.Signature) {
	argInts, argFloats, resultInts, resultFloats := a.r.ArgsResultsRegs()

	if len(a.Rets) < len(sig.Results) {
		a.Rets = make([]ABIArg, len(sig.Results))
	}
	a.Rets = a.Rets[:len(sig.Results)]
	a.RetStackSize = a.setABIArgs(a.Rets, sig.Results, argInts, argFloats)
	if argsNum := len(sig.Params); len(a.Args) < argsNum {
		a.Args = make([]ABIArg, argsNum)
	}
	a.Args = a.Args[:len(sig.Params)]
	a.ArgStackSize = a.setABIArgs(a.Args, sig.Params, resultInts, resultFloats)

	// Gather the real registers usages in arg/return.
	a.RetRealRegs = a.RetRealRegs[:0]
	for i := range a.Rets {
		r := &a.Rets[i]
		if r.Kind == ABIArgKindReg {
			a.RetRealRegs = append(a.RetRealRegs, r.Reg)
		}
	}
	a.ArgRealRegs = a.ArgRealRegs[:0]
	for i := range a.Args {
		arg := &a.Args[i]
		if arg.Kind == ABIArgKindReg {
			reg := arg.Reg
			a.ArgRealRegs = append(a.ArgRealRegs, reg)
		}
	}

	a.Initialized = true
}

// setABIArgs sets the ABI arguments in the given slice. This assumes that len(s) >= len(types)
// where if len(s) > len(types), the last elements of s is for the multi-return slot.
func (a *FunctionABI[M]) setABIArgs(s []ABIArg, types []ssa.Type, ints, floats []regalloc.RealReg) (stackSize int64) {
	il, fl := len(ints), len(floats)

	var stackOffset int64
	intParamIndex, floatParamIndex := 0, 0
	for i, typ := range types {
		arg := &s[i]
		arg.Index = i
		arg.Type = typ
		if typ.IsInt() {
			if intParamIndex >= il {
				arg.Kind = ABIArgKindStack
				const slotSize = 8 // Align 8 bytes.
				arg.Offset = stackOffset
				stackOffset += slotSize
			} else {
				arg.Kind = ABIArgKindReg
				arg.Reg = regalloc.FromRealReg(ints[intParamIndex], regalloc.RegTypeInt)
				intParamIndex++
			}
		} else {
			if floatParamIndex >= fl {
				arg.Kind = ABIArgKindStack
				slotSize := int64(8)   // Align at least 8 bytes.
				if typ.Bits() == 128 { // Vector.
					slotSize = 16
				}
				arg.Offset = stackOffset
				stackOffset += slotSize
			} else {
				arg.Kind = ABIArgKindReg
				arg.Reg = regalloc.FromRealReg(floats[floatParamIndex], regalloc.RegTypeFloat)
				floatParamIndex++
			}
		}
	}
	return stackOffset
}

func (a *FunctionABI[M]) AlignedArgResultStackSlotSize() int64 {
	stackSlotSize := a.RetStackSize + a.ArgStackSize
	// Align stackSlotSize to 16 bytes.
	stackSlotSize = (stackSlotSize + 15) &^ 15
	return stackSlotSize
}
