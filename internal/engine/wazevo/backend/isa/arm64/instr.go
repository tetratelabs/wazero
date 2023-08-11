package arm64

import (
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

type (
	// instruction represents either a real instruction in arm64, or the meta instructions
	// that are convenient for code generation. For example, inline constants are also treated
	// as instructions.
	//
	// Basically, each instruction knows how to get encoded in binaries. Hence, the final output of compilation
	// can be considered equivalent to the sequence of such instructions.
	//
	// Each field is interpreted depending on the kind.
	//
	// TODO: optimize the layout later once the impl settles.
	instruction struct {
		kind               instructionKind
		prev, next         *instruction
		u1, u2, u3         uint64
		rd, rm, rn, ra     operand
		amode              addressMode
		abi                *abiImpl
		addedAfterLowering bool
	}

	// instructionKind represents the kind of instruction.
	// This controls how the instruction struct is interpreted.
	instructionKind int
)

type defKind byte

const (
	defKindNone defKind = iota + 1
	defKindRD
	defKindCall
)

var defKinds = [numInstructionKinds]defKind{
	aluRRR:          defKindRD,
	aluRRRR:         defKindRD,
	aluRRImm12:      defKindRD,
	aluRRBitmaskImm: defKindRD,
	aluRRRShift:     defKindRD,
	aluRRImmShift:   defKindRD,
	aluRRRExtend:    defKindRD,
	bitRR:           defKindRD,
	movZ:            defKindRD,
	movN:            defKindRD,
	mov32:           defKindRD,
	mov64:           defKindRD,
	fpuMov64:        defKindRD,
	fpuMov128:       defKindRD,
	fpuRRR:          defKindRD,
	nop0:            defKindNone,
	call:            defKindCall,
	callInd:         defKindCall,
	ret:             defKindNone,
	store8:          defKindNone,
	store16:         defKindNone,
	store32:         defKindNone,
	store64:         defKindNone,
	exitSequence:    defKindNone,
	condBr:          defKindNone,
	br:              defKindNone,
	cSet:            defKindRD,
	extend:          defKindRD,
	fpuCmp:          defKindNone,
	uLoad8:          defKindRD,
	uLoad16:         defKindRD,
	uLoad32:         defKindRD,
	sLoad8:          defKindRD,
	sLoad16:         defKindRD,
	sLoad32:         defKindRD,
	uLoad64:         defKindRD,
	fpuLoad32:       defKindRD,
	fpuLoad64:       defKindRD,
	fpuLoad128:      defKindRD,
	loadFpuConst32:  defKindRD,
	loadFpuConst64:  defKindRD,
	fpuStore32:      defKindNone,
	fpuStore64:      defKindNone,
	fpuStore128:     defKindNone,
	udf:             defKindNone,
	cSel:            defKindRD,
	fpuCSel32:       defKindRD,
	fpuCSel64:       defKindRD,
}

// defs returns the list of regalloc.VReg that are defined by the instruction.
// In order to reduce the number of allocations, the caller can pass the slice to be used.
func (i *instruction) defs(regs []regalloc.VReg) []regalloc.VReg {
	switch defKinds[i.kind] {
	case defKindNone:
	case defKindRD:
		regs = append(regs, i.rd.nr())
	case defKindCall:
		regs = append(regs, i.abi.retRealRegs...)
	default:
		panic(fmt.Sprintf("defKind for %v not defined", i))
	}
	return regs
}

func (i *instruction) assignDef(reg regalloc.VReg) {
	switch defKinds[i.kind] {
	case defKindNone:
	case defKindRD:
		i.rd = i.rd.assignReg(reg)
	case defKindCall:
		panic("BUG: call instructions shouldn't be assigned")
	default:
		panic(fmt.Sprintf("defKind for %v not defined", i))
	}
}

type useKind byte

const (
	useKindNone useKind = iota + 1
	useKindRN
	useKindRNRM
	useKindRNRMRA
	useKindRet
	useKindCall
	useKindCallInd
	useKindAMode
	useKindRNAMode
	useKindCond
)

var useKinds = [numInstructionKinds]useKind{
	udf:             useKindNone,
	aluRRR:          useKindRNRM,
	aluRRRR:         useKindRNRMRA,
	aluRRImm12:      useKindRN,
	aluRRBitmaskImm: useKindRN,
	aluRRRShift:     useKindRNRM,
	aluRRImmShift:   useKindRN,
	aluRRRExtend:    useKindRNRM,
	bitRR:           useKindRN,
	movZ:            useKindNone,
	movN:            useKindNone,
	mov32:           useKindRN,
	mov64:           useKindRN,
	fpuMov64:        useKindRN,
	fpuMov128:       useKindRN,
	fpuRRR:          useKindRNRM,
	nop0:            useKindNone,
	call:            useKindCall,
	callInd:         useKindCallInd,
	ret:             useKindRet,
	store8:          useKindRNAMode,
	store16:         useKindRNAMode,
	store32:         useKindRNAMode,
	store64:         useKindRNAMode,
	exitSequence:    useKindRN,
	condBr:          useKindCond,
	br:              useKindNone,
	cSet:            useKindNone,
	extend:          useKindRN,
	fpuCmp:          useKindRNRM,
	uLoad8:          useKindAMode,
	uLoad16:         useKindAMode,
	uLoad32:         useKindAMode,
	sLoad8:          useKindAMode,
	sLoad16:         useKindAMode,
	sLoad32:         useKindAMode,
	uLoad64:         useKindAMode,
	fpuLoad32:       useKindAMode,
	fpuLoad64:       useKindAMode,
	fpuLoad128:      useKindAMode,
	fpuStore32:      useKindRNAMode,
	fpuStore64:      useKindRNAMode,
	fpuStore128:     useKindRNAMode,
	loadFpuConst32:  useKindNone,
	loadFpuConst64:  useKindNone,
	cSel:            useKindRNRM,
	fpuCSel32:       useKindRNRM,
	fpuCSel64:       useKindRNRM,
}

// uses returns the list of regalloc.VReg that are used by the instruction.
// In order to reduce the number of allocations, the caller can pass the slice to be used.
func (i *instruction) uses(regs []regalloc.VReg) []regalloc.VReg {
	switch useKinds[i.kind] {
	case useKindNone:
	case useKindRN:
		if rn := i.rn.reg(); rn.Valid() {
			regs = append(regs, rn)
		}
	case useKindRNRM:
		if rn := i.rn.reg(); rn.Valid() {
			regs = append(regs, rn)
		}
		if rm := i.rm.reg(); rm.Valid() {
			regs = append(regs, rm)
		}
	case useKindRNRMRA:
		if rn := i.rn.reg(); rn.Valid() {
			regs = append(regs, rn)
		}
		if rm := i.rm.reg(); rm.Valid() {
			regs = append(regs, rm)
		}
		if ra := i.ra.reg(); ra.Valid() {
			regs = append(regs, ra)
		}
	case useKindRet:
		regs = append(regs, i.abi.retRealRegs...)
	case useKindAMode:
		if amodeRN := i.amode.rn; amodeRN.Valid() {
			regs = append(regs, amodeRN)
		}
		if amodeRM := i.amode.rm; amodeRM.Valid() {
			regs = append(regs, amodeRM)
		}
	case useKindRNAMode:
		regs = append(regs, i.rn.reg())
		if amodeRN := i.amode.rn; amodeRN.Valid() {
			regs = append(regs, amodeRN)
		}
		if amodeRM := i.amode.rm; amodeRM.Valid() {
			regs = append(regs, amodeRM)
		}
	case useKindCond:
		cnd := cond(i.u1)
		if cnd.kind() != condKindCondFlagSet {
			regs = append(regs, cnd.register())
		}
	case useKindCall:
		regs = append(regs, i.abi.argRealRegs...)
	case useKindCallInd:
		regs = append(regs, i.rn.nr())
		regs = append(regs, i.abi.argRealRegs...)
	default:
		panic(fmt.Sprintf("useKind for %v not defined", i))
	}
	return regs
}

func (i *instruction) assignUses(regs []regalloc.VReg) {
	switch useKinds[i.kind] {
	case useKindNone:
	case useKindRN:
		if rn := i.rn.reg(); rn.Valid() {
			i.rn = i.rn.assignReg(regs[0])
		}
	case useKindRNRM:
		if rn := i.rn.reg(); rn.Valid() {
			i.rn = i.rn.assignReg(regs[0])
		}
		if rm := i.rm.reg(); rm.Valid() {
			i.rm = i.rm.assignReg(regs[1])
		}
	case useKindRNRMRA:
		if rn := i.rn.reg(); rn.Valid() {
			i.rn = i.rn.assignReg(regs[0])
		}
		if rm := i.rm.reg(); rm.Valid() {
			i.rm = i.rm.assignReg(regs[1])
		}
		if ra := i.ra.reg(); ra.Valid() {
			i.ra = i.ra.assignReg(regs[2])
		}
	case useKindRet:
		panic("BUG: ret instructions shouldn't be assigned")
	case useKindAMode:
		if amodeRN := i.amode.rn; amodeRN.Valid() {
			i.amode.rn = regs[0]
		}
		if amodeRM := i.amode.rm; amodeRM.Valid() {
			i.amode.rm = regs[1]
		}
	case useKindRNAMode:
		i.rn = i.rn.assignReg(regs[0])
		if amodeRN := i.amode.rn; amodeRN.Valid() {
			i.amode.rn = regs[1]
		}
		if amodeRM := i.amode.rm; amodeRM.Valid() {
			i.amode.rm = regs[2]
		}
	case useKindCond:
		c := cond(i.u1)
		switch c.kind() {
		case condKindRegisterZero:
			i.u1 = uint64(registerAsRegZeroCond(regs[0]))
		case condKindRegisterNotZero:
			i.u1 = uint64(registerAsRegNotZeroCond(regs[0]))
		}
	case useKindCall:
		panic("BUG: call instructions shouldn't be assigned")
	case useKindCallInd:
		i.rn = i.rn.assignReg(regs[0])
	default:
		panic(fmt.Sprintf("useKind for %v not defined", i))
	}
}

func (i *instruction) asCall(ref ssa.FuncRef, abi *abiImpl) {
	i.kind = call
	i.u1 = uint64(ref)
	i.abi = abi
}

func (i *instruction) asCallImm(imm int64) {
	i.kind = call
	i.u2 = uint64(imm)
}

func (i *instruction) asCallIndirect(ptr regalloc.VReg, abi *abiImpl) {
	i.kind = callInd
	i.rn = operandNR(ptr)
	i.abi = abi
}

func (i *instruction) callFuncRef() ssa.FuncRef {
	return ssa.FuncRef(i.u1)
}

// shift must be divided by 16 and must be in range 0-3 (if dst64bit is true) or 0-1 (if dst64bit is false)
func (i *instruction) asMOVZ(dst regalloc.VReg, imm uint64, shift uint64, dst64bit bool) {
	i.kind = movZ
	i.rd = operandNR(dst)
	i.u1 = imm
	i.u2 = shift
	if dst64bit {
		i.u3 = 1
	}
}

// shift must be divided by 16 and must be in range 0-3 (if dst64bit is true) or 0-1 (if dst64bit is false)
func (i *instruction) asMOVK(dst regalloc.VReg, imm uint64, shift uint64, dst64bit bool) {
	i.kind = movK
	i.rd = operandNR(dst)
	i.u1 = imm
	i.u2 = shift
	if dst64bit {
		i.u3 = 1
	}
}

// shift must be divided by 16 and must be in range 0-3 (if dst64bit is true) or 0-1 (if dst64bit is false)
func (i *instruction) asMOVN(dst regalloc.VReg, imm uint64, shift uint64, dst64bit bool) {
	i.kind = movN
	i.rd = operandNR(dst)
	i.u1 = imm
	i.u2 = shift
	if dst64bit {
		i.u3 = 1
	}
}

func (i *instruction) asNop0() {
	i.kind = nop0
}

func (i *instruction) asRet(abi *abiImpl) {
	i.kind = ret
	i.abi = abi
}

func (i *instruction) asStorePair64(src1, src2 regalloc.VReg, amode addressMode) {
	i.kind = storeP64
	i.rn = operandNR(src1)
	i.rm = operandNR(src2)
	i.amode = amode
}

func (i *instruction) asLoadPair64(src1, src2 regalloc.VReg, amode addressMode) {
	i.kind = loadP64
	i.rn = operandNR(src1)
	i.rm = operandNR(src2)
	i.amode = amode
}

func (i *instruction) asStore(src operand, amode addressMode, sizeInBits byte) {
	switch sizeInBits {
	case 8:
		i.kind = store8
	case 16:
		i.kind = store16
	case 32:
		if src.reg().RegType() == regalloc.RegTypeInt {
			i.kind = store32
		} else {
			i.kind = fpuStore32
		}
	case 64:
		if src.reg().RegType() == regalloc.RegTypeInt {
			i.kind = store64
		} else {
			i.kind = fpuStore64
		}
	case 128:
		i.kind = fpuStore128
	}
	i.rn = src
	i.amode = amode
}

func (i *instruction) asSLoad(dst operand, amode addressMode, sizeInBits byte) {
	switch sizeInBits {
	case 8:
		i.kind = sLoad8
	case 16:
		i.kind = sLoad16
	case 32:
		i.kind = sLoad32
	default:
		panic("BUG")
	}
	i.rd = dst
	i.amode = amode
}

func (i *instruction) asULoad(dst operand, amode addressMode, sizeInBits byte) {
	switch sizeInBits {
	case 8:
		i.kind = uLoad8
	case 16:
		i.kind = uLoad16
	case 32:
		i.kind = uLoad32
	case 64:
		i.kind = uLoad64
	}
	i.rd = dst
	i.amode = amode
}

func (i *instruction) asFpuLoad(dst operand, amode addressMode, sizeInBits byte) {
	switch sizeInBits {
	case 32:
		i.kind = fpuLoad32
	case 64:
		i.kind = fpuLoad64
	case 128:
		i.kind = fpuLoad128
	}
	i.rd = dst
	i.amode = amode
}

func (i *instruction) asCSet(rd regalloc.VReg, c condFlag) {
	i.kind = cSet
	i.rd = operandNR(rd)
	i.u1 = uint64(c)
}

func (i *instruction) asCSel(rd, rn, rm operand, c condFlag, _64bit bool) {
	i.kind = cSel
	i.rd = rd
	i.rn = rn
	i.rm = rm
	i.u1 = uint64(c)
	if _64bit {
		i.u3 = 1
	}
}

func (i *instruction) asFpuCSel(rd, rn, rm operand, c condFlag, _64bit bool) {
	if _64bit {
		i.kind = fpuCSel64
	} else {
		i.kind = fpuCSel32
	}
	i.rd = rd
	i.rn = rn
	i.rm = rm
	i.u1 = uint64(c)
}

func (i *instruction) asBr(target label) {
	if target == returnLabel {
		panic("BUG: call site should special case for returnLabel")
	}
	i.kind = br
	i.u1 = uint64(target)
}

func (i *instruction) brLabel() label {
	return label(i.u1)
}

// brOffsetResolved is called when the target label is resolved.
func (i *instruction) brOffsetResolved(offset int64) {
	i.u2 = uint64(offset)
	i.u3 = 1 // indicate that the offset is resolved, for debugging.
}

func (i *instruction) brOffset() int64 {
	return int64(i.u2)
}

// asCondBr encodes a conditional branch instruction. is64bit is only needed when cond is not flag.
func (i *instruction) asCondBr(c cond, target label, is64bit bool) {
	i.kind = condBr
	i.u1 = c.asUint64()
	i.u2 = uint64(target)
	if is64bit {
		i.u3 = 1
	}
}

func (i *instruction) condBrLabel() label {
	return label(i.u2)
}

// condBrOffsetResolve is called when the target label is resolved.
func (i *instruction) condBrOffsetResolve(offset int64) {
	i.rd.data = uint64(offset)
	i.rd.data2 = 1 // indicate that the offset is resolved, for debugging.
}

// condBrOffsetResolved returns true if condBrOffsetResolve is already called.
func (i *instruction) condBrOffsetResolved() bool {
	return i.rd.data2 == 1
}

func (i *instruction) condBrOffset() int64 {
	return int64(i.rd.data)
}

func (i *instruction) condBrCond() cond {
	return cond(i.u1)
}

func (i *instruction) condBr64bit() bool {
	return i.u3 == 1
}

func (i *instruction) asLoadFpuConst32(rd regalloc.VReg, raw uint64) {
	i.kind = loadFpuConst32
	i.u1 = raw
	i.rd = operandNR(rd)
}

func (i *instruction) asLoadFpuConst64(rd regalloc.VReg, raw uint64) {
	i.kind = loadFpuConst64
	i.u1 = raw
	i.rd = operandNR(rd)
}

func (i *instruction) asFpuCmp(rn, rm operand, is64bit bool) {
	i.kind = fpuCmp
	i.rn, i.rm = rn, rm
	if is64bit {
		i.u3 = 1
	}
}

// asALU setups a basic ALU instruction.
func (i *instruction) asALU(aluOp aluOp, rd, rn, rm operand, dst64bit bool) {
	switch rm.kind {
	case operandKindNR:
		i.kind = aluRRR
	case operandKindSR:
		i.kind = aluRRRShift
	case operandKindER:
		i.kind = aluRRRExtend
	case operandKindImm12:
		i.kind = aluRRImm12
	default:
		panic("BUG")
	}
	i.u1 = uint64(aluOp)
	i.rd, i.rn, i.rm = rd, rn, rm
	if dst64bit {
		i.u3 = 1
	}
}

// asALU setups a basic ALU instruction.
func (i *instruction) asALURRRR(aluOp aluOp, rd, rn, rm, ra operand, dst64bit bool) {
	i.kind = aluRRRR
	i.u1 = uint64(aluOp)
	i.rd, i.rn, i.rm, i.ra = rd, rn, rm, ra
	if dst64bit {
		i.u3 = 1
	}
}

// asALUShift setups a shift based ALU instruction.
func (i *instruction) asALUShift(aluOp aluOp, rd, rn, rm operand, dst64bit bool) {
	switch rm.kind {
	case operandKindNR:
		i.kind = aluRRR // If the shift amount op is a register, then the instruction is encoded as a normal ALU instruction with two register operands.
	case operandKindShiftImm:
		i.kind = aluRRImmShift
	default:
		panic("BUG")
	}
	i.u1 = uint64(aluOp)
	i.rd, i.rn, i.rm = rd, rn, rm
	if dst64bit {
		i.u3 = 1
	}
}

func (i *instruction) asALUBitmaskImm(aluOp aluOp, rn, rd regalloc.VReg, imm uint64, dst64bit bool) {
	i.kind = aluRRBitmaskImm
	i.u1 = uint64(aluOp)
	i.rn, i.rd = operandNR(rn), operandNR(rd)
	i.u2 = imm
	if dst64bit {
		i.u3 = 1
	}
}

func (i *instruction) asBitRR(bitOp bitOp, rd, rn regalloc.VReg, is64bit bool) {
	i.kind = bitRR
	i.rn, i.rd = operandNR(rn), operandNR(rd)
	i.u1 = uint64(bitOp)
	if is64bit {
		i.u2 = 1
	}
}

func (i *instruction) asFpuRRR(op fpuBinOp, rd, rn, rm operand, dst64bit bool) {
	i.kind = fpuRRR
	i.u1 = uint64(op)
	i.rd, i.rn, i.rm = rd, rn, rm
	if dst64bit {
		i.u3 = 1
	}
}

func (i *instruction) asExtend(rd, rn regalloc.VReg, fromBits, toBits byte, signed bool) {
	i.kind = extend
	i.rn, i.rd = operandNR(rn), operandNR(rd)
	i.u1 = uint64(fromBits)
	i.u2 = uint64(toBits)
	if signed {
		i.u3 = 1
	}
}

func (i *instruction) asMove32(rd, rn regalloc.VReg) {
	i.kind = mov32
	i.rn, i.rd = operandNR(rn), operandNR(rd)
}

func (i *instruction) asMove64(rd, rn regalloc.VReg) {
	i.kind = mov64
	i.rn, i.rd = operandNR(rn), operandNR(rd)
}

func (i *instruction) asFpuMov64(rd, rn regalloc.VReg) {
	i.kind = fpuMov64
	i.rn, i.rd = operandNR(rn), operandNR(rd)
}

func (i *instruction) asFpuMov128(rd, rn regalloc.VReg) {
	i.kind = fpuMov128
	i.rn, i.rd = operandNR(rn), operandNR(rd)
}

func (i *instruction) isCopy() bool {
	op := i.kind
	return op == mov64 || op == mov32 || op == fpuMov64 || op == fpuMov128
}

// String implements fmt.Stringer.
func (i *instruction) String() (str string) {
	is64SizeBitToSize := func(u3 uint64) byte {
		if u3 == 0 {
			return 32
		}
		return 64
	}

	switch i.kind {
	case nop0:
		str = "nop0"
	case nop4:
		panic("TODO")
	case aluRRR:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("%s %s, %s, %s", aluOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size), formatVRegSized(i.rn.nr(), size), formatVRegSized(i.rm.nr(), size))
	case aluRRRR:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("%s %s, %s, %s, %s", aluOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size), formatVRegSized(i.rn.nr(), size), formatVRegSized(i.rm.nr(), size), formatVRegSized(i.ra.nr(), size))
	case aluRRImm12:
		size := is64SizeBitToSize(i.u3)
		v, shiftBit := i.rm.imm12()
		if shiftBit == 1 {
			str = fmt.Sprintf("%s %s, %s, #%#x", aluOp(i.u1).String(),
				formatVRegSized(i.rd.nr(), size), formatVRegSized(i.rn.nr(), size), uint64(v)<<12)
		} else {
			str = fmt.Sprintf("%s %s, %s, #%#x", aluOp(i.u1).String(),
				formatVRegSized(i.rd.nr(), size), formatVRegSized(i.rn.nr(), size), v)
		}
	case aluRRBitmaskImm:
		size := is64SizeBitToSize(i.u3)
		rd, rn := formatVRegSized(i.rd.nr(), size), formatVRegSized(i.rn.nr(), size)
		if size == 32 {
			str = fmt.Sprintf("%s %s, %s, #%#x", aluOp(i.u1).String(), rd, rn, uint32(i.u2))
		} else {
			str = fmt.Sprintf("%s %s, %s, #%#x", aluOp(i.u1).String(), rd, rn, i.u2)
		}
	case aluRRImmShift:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("%s %s, %s, %#x",
			aluOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size),
			formatVRegSized(i.rn.nr(), size),
			i.rm.shiftImm(),
		)
	case aluRRRShift:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("%s %s, %s, %s",
			aluOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size),
			formatVRegSized(i.rn.nr(), size),
			formatVRegSized(i.rm.nr(), size),
		)
	case aluRRRExtend:
		rm, e, _ := i.rm.er()
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("%s %s, %s, %s %s", aluOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size),
			formatVRegSized(i.rn.nr(), size),
			formatVRegSized(rm, e.srcBits()),
			e,
		)
	case bitRR:
		size := is64SizeBitToSize(i.u2)
		str = fmt.Sprintf("%s %s, %s",
			bitOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size),
			formatVRegSized(i.rn.nr(), size),
		)
	case uLoad8:
		str = fmt.Sprintf("ldrb %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case sLoad8:
		str = fmt.Sprintf("ldrsb %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case uLoad16:
		str = fmt.Sprintf("ldrh %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case sLoad16:
		str = fmt.Sprintf("ldrsh %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case uLoad32:
		str = fmt.Sprintf("ldr %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case sLoad32:
		str = fmt.Sprintf("ldrs %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case uLoad64:
		str = fmt.Sprintf("ldr %s, %s", formatVRegSized(i.rd.nr(), 64), i.amode.format(64))
	case store8:
		str = fmt.Sprintf("strb %s, %s", formatVRegSized(i.rn.nr(), 32), i.amode.format(8))
	case store16:
		str = fmt.Sprintf("strh %s, %s", formatVRegSized(i.rn.nr(), 32), i.amode.format(16))
	case store32:
		str = fmt.Sprintf("str %s, %s", formatVRegSized(i.rn.nr(), 32), i.amode.format(32))
	case store64:
		str = fmt.Sprintf("str %s, %s", formatVRegSized(i.rn.nr(), 64), i.amode.format(64))
	case storeP64:
		str = fmt.Sprintf("stp %s, %s, %s",
			formatVRegSized(i.rn.nr(), 64), formatVRegSized(i.rm.nr(), 64), i.amode.format(64))
	case loadP64:
		str = fmt.Sprintf("ldp %s, %s, %s",
			formatVRegSized(i.rn.nr(), 64), formatVRegSized(i.rm.nr(), 64), i.amode.format(64))
	case mov64:
		str = fmt.Sprintf("mov %s, %s",
			formatVRegSized(i.rd.nr(), 64),
			formatVRegSized(i.rn.nr(), 64))
	case mov32:
		str = fmt.Sprintf("mov %s, %s", formatVRegSized(i.rd.nr(), 32), formatVRegSized(i.rn.nr(), 32))
	case movZ:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("movz %s, #%#x, LSL %d", formatVRegSized(i.rd.nr(), size), uint16(i.u1), i.u2*16)
	case movN:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("movn %s, #%#x, LSL %d", formatVRegSized(i.rd.nr(), size), uint16(i.u1), i.u2*16)
	case movK:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("movk %s, #%#x, LSL %d", formatVRegSized(i.rd.nr(), size), uint16(i.u1), i.u2*16)
	case extend:
		fromBits, toBits := byte(i.u1), byte(i.u2)

		var signedStr string
		if i.u3 == 1 {
			signedStr = "s"
		} else {
			signedStr = "u"
		}
		var fromStr string
		switch fromBits {
		case 8:
			fromStr = "b"
		case 16:
			fromStr = "h"
		case 32:
			fromStr = "w"
		}
		str = fmt.Sprintf("%sxt%s %s, %s", signedStr, fromStr, formatVRegSized(i.rd.nr(), toBits), formatVRegSized(i.rn.nr(), 32))
	case cSel:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("csel %s, %s, %s, %s",
			formatVRegSized(i.rd.nr(), size),
			formatVRegSized(i.rn.nr(), size),
			formatVRegSized(i.rm.nr(), size),
			condFlag(i.u1),
		)
	case cSet:
		str = fmt.Sprintf("cset %s, %s", formatVRegSized(i.rd.nr(), 64), condFlag(i.u1))
	case cCmpImm:
		panic("TODO")
	case fpuMov64:
		str = fmt.Sprintf("mov %s.8b, %s.8b", formatVRegSized(i.rd.nr(), 128), formatVRegSized(i.rn.nr(), 128))
	case fpuMov128:
		str = fmt.Sprintf("mov %s.16b, %s.16b", formatVRegSized(i.rd.nr(), 128), formatVRegSized(i.rn.nr(), 128))
	case fpuMovFromVec:
		panic("TODO")
	case fpuRR:
		panic("TODO")
	case fpuRRR:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("%s %s, %s, %s", fpuBinOp(i.u1).String(),
			formatVRegSized(i.rd.nr(), size), formatVRegSized(i.rn.nr(), size), formatVRegSized(i.rm.nr(), size))
	case fpuRRI:
		panic("TODO")
	case fpuRRRR:
		panic("TODO")
	case fpuCmp:
		size := is64SizeBitToSize(i.u3)
		str = fmt.Sprintf("fcmp %s, %s",
			formatVRegSized(i.rn.nr(), size), formatVRegSized(i.rm.nr(), size))
	case fpuLoad32:
		str = fmt.Sprintf("ldr %s, %s", formatVRegSized(i.rd.nr(), 32), i.amode.format(32))
	case fpuStore32:
		str = fmt.Sprintf("str %s, %s", formatVRegSized(i.rn.nr(), 32), i.amode.format(64))
	case fpuLoad64:
		str = fmt.Sprintf("ldr %s, %s", formatVRegSized(i.rd.nr(), 64), i.amode.format(64))
	case fpuStore64:
		str = fmt.Sprintf("str %s, %s", formatVRegSized(i.rn.nr(), 64), i.amode.format(64))
	case fpuLoad128:
		str = fmt.Sprintf("ldr %s, %s", formatVRegSized(i.rd.nr(), 128), i.amode.format(64))
	case fpuStore128:
		str = fmt.Sprintf("str %s, %s", formatVRegSized(i.rn.nr(), 128), i.amode.format(64))
	case loadFpuConst32:
		str = fmt.Sprintf("ldr %s, #8; b 8; data.f32 %f", formatVRegSized(i.rd.nr(), 32), math.Float32frombits(uint32(i.u1)))
	case loadFpuConst64:
		str = fmt.Sprintf("ldr %s, #8; b 16; data.f64 %f", formatVRegSized(i.rd.nr(), 64), math.Float64frombits(i.u1))
	case loadFpuConst128:
		panic("TODO")
	case fpuToInt:
		panic("TODO")
	case intToFpu:
		panic("TODO")
	case fpuCSel32:
		str = fmt.Sprintf("fcsel %s, %s, %s, %s",
			formatVRegSized(i.rd.nr(), 32),
			formatVRegSized(i.rn.nr(), 32),
			formatVRegSized(i.rm.nr(), 32),
			condFlag(i.u1),
		)
	case fpuCSel64:
		str = fmt.Sprintf("fcsel %s, %s, %s, %s",
			formatVRegSized(i.rd.nr(), 64),
			formatVRegSized(i.rn.nr(), 64),
			formatVRegSized(i.rm.nr(), 64),
			condFlag(i.u1),
		)
	case fpuRound:
		panic("TODO")
	case movToFpu:
		panic("TODO")
	case movToVec:
		panic("TODO")
	case movFromVec:
		panic("TODO")
	case movFromVecSigned:
		panic("TODO")
	case vecDup:
		panic("TODO")
	case vecDupFromFpu:
		panic("TODO")
	case vecExtend:
		panic("TODO")
	case vecMovElement:
		panic("TODO")
	case vecMiscNarrow:
		panic("TODO")
	case vecRRR:
		panic("TODO")
	case vecMisc:
		panic("TODO")
	case vecLanes:
		panic("TODO")
	case vecTbl:
		panic("TODO")
	case vecTbl2:
		panic("TODO")
	case movToNZCV:
		panic("TODO")
	case movFromNZCV:
		panic("TODO")
	case call:
		if i.u2 > 0 {
			str = fmt.Sprintf("bl #%#x", i.u2)
		} else {
			str = fmt.Sprintf("bl %s", ssa.FuncRef(i.u1))
		}
	case callInd:
		str = fmt.Sprintf("bl %s", formatVRegSized(i.rn.nr(), 32))
	case ret:
		str = "ret"
	case epiloguePlaceholder:
		panic("TODO")
	case br:
		target := label(i.u1)
		if i.u3 != 0 {
			str = fmt.Sprintf("b #%#x (%s)", i.brOffset(), target.String())
		} else {
			str = fmt.Sprintf("b %s", target.String())
		}
	case condBr:
		size := is64SizeBitToSize(i.u3)
		c := cond(i.u1)
		target := label(i.u2)
		switch c.kind() {
		case condKindRegisterZero:
			if !i.condBrOffsetResolved() {
				str = fmt.Sprintf("cbz %s, (%s)", formatVRegSized(c.register(), size), target.String())
			} else {
				str = fmt.Sprintf("cbz %s, #%#x %s", formatVRegSized(c.register(), size), i.condBrOffset(), target.String())
			}
		case condKindRegisterNotZero:
			if offset := i.condBrOffset(); offset != 0 {
				str = fmt.Sprintf("cbnz %s, #%#x (%s)", formatVRegSized(c.register(), size), offset, target.String())
			} else {
				str = fmt.Sprintf("cbnz %s, %s", formatVRegSized(c.register(), size), target.String())
			}
		case condKindCondFlagSet:
			if offset := i.condBrOffset(); offset != 0 {
				if target == invalidLabel {
					str = fmt.Sprintf("b.%s #%#x", c.flag(), offset)
				} else {
					str = fmt.Sprintf("b.%s #%#x, (%s)", c.flag(), offset, target.String())
				}
			} else {
				str = fmt.Sprintf("b.%s %s", c.flag(), target.String())
			}
		}
	case indirectBr:
		panic("TODO")
	case adr:
		str = fmt.Sprintf("adr %s, #%#x", formatVRegSized(i.rd.nr(), 64), int64(i.u1))
	case word4:
		panic("TODO")
	case word8:
		panic("TODO")
	case jtSequence:
		panic("TODO")
	case loadAddr:
		panic("TODO")
	case exitSequence:
		str = fmt.Sprintf("exit_sequence %s", formatVRegSized(i.rn.nr(), 64))
	case udf:
		str = "udf"
	default:
		panic(i.kind)
	}
	return
}

func (i *instruction) asAdr(rd regalloc.VReg, offset int64) {
	i.kind = adr
	i.rd = operandNR(rd)
	i.u1 = uint64(offset)
}

// TODO: delete unnecessary things.
const (
	// nop0 represents a no-op of zero size.
	nop0 instructionKind = iota + 1
	// nop4 represents a no-op that is one instruction large.
	nop4
	// aluRRR represents an ALU operation with two register sources and a register destination.
	aluRRR
	// aluRRRR represents an ALU operation with three register sources and a register destination.
	aluRRRR
	// aluRRImm12 represents an ALU operation with a register source and an immediate-12 source, with a register destination.
	aluRRImm12
	// aluRRBitmaskImm represents an ALU operation with a register source and a bitmask immediate, with a register destination.
	aluRRBitmaskImm
	// aluRRImmShift represents an ALU operation with a register source and an immediate-shifted source, with a register destination.
	aluRRImmShift
	// aluRRRShift represents an ALU operation with two register sources, one of which can be shifted, with a register destination.
	aluRRRShift
	// aluRRRExtend represents an ALU operation with two register sources, one of which can be extended, with a register destination.
	aluRRRExtend
	// bitRR represents a bit op instruction with a single register source.
	bitRR
	// uLoad8 represents an unsigned 8-bit load.
	uLoad8
	// sLoad8 represents a signed 8-bit load into 64-bit register.
	sLoad8
	// uLoad16 represents an unsigned 16-bit load into 64-bit register.
	uLoad16
	// sLoad16 represents a signed 16-bit load into 64-bit register.
	sLoad16
	// uLoad32 represents an unsigned 32-bit load into 64-bit register.
	uLoad32
	// sLoad32 represents a signed 32-bit load into 64-bit register.
	sLoad32
	// uLoad64 represents a 64-bit load.
	uLoad64
	// store8 represents an 8-bit store.
	store8
	// store16 represents a 16-bit store.
	store16
	// store32 represents a 32-bit store.
	store32
	// store64 represents a 64-bit store.
	store64
	// storeP64 represents a store of a pair of registers.
	storeP64
	// loadP64 represents a load of a pair of registers.
	loadP64
	// mov64 represents a MOV instruction. These are encoded as ORR's but we keep them separate for better handling.
	mov64
	// mov32 represents a 32-bit MOV. This zeroes the top 32 bits of the destination.
	mov32
	// movZ represents a MOVZ with a 16-bit immediate.
	movZ
	// movN represents a MOVN with a 16-bit immediate.
	movN
	// movK represents a MOVK with a 16-bit immediate.
	movK
	// extend represents a sign- or zero-extend operation.
	extend
	// cSel represents a conditional-select operation.
	cSel
	// cSet represents a conditional-set operation.
	cSet
	// cCmpImm represents a conditional comparison with an immediate.
	cCmpImm
	// fpuMov64 represents a FPU move. Distinct from a vector-register move; moving just 64 bits appears to be significantly faster.
	fpuMov64
	// fpuMov128 represents a vector register move.
	fpuMov128
	// fpuMovFromVec represents a move to scalar from a vector element.
	fpuMovFromVec
	// fpuRR represents a 1-op FPU instruction.
	fpuRR
	// fpuRRR represents a 2-op FPU instruction.
	fpuRRR
	// fpuRRI represents a 2-op FPU instruction with immediate value.
	fpuRRI
	// fpuRRRR represents a 3-op FPU instruction.
	fpuRRRR
	// fpuCmp represents a FPU comparison, either 32 or 64 bit.
	fpuCmp
	// fpuLoad32 represents a floating-point load, single-precision (32 bit).
	fpuLoad32
	// fpuStore32 represents a floating-point store, single-precision (32 bit).
	fpuStore32
	// fpuLoad64 represents a floating-point load, double-precision (64 bit).
	fpuLoad64
	// fpuStore64 represents a floating-point store, double-precision (64 bit).
	fpuStore64
	// fpuLoad128 represents a floating-point/vector load, 128 bit.
	fpuLoad128
	// fpuStore128 represents a floating-point/vector store, 128 bit.
	fpuStore128
	// loadFpuConst32 represents a load of a 32-bit floating-point constant.
	loadFpuConst32
	// loadFpuConst64 represents a load of a 64-bit floating-point constant.
	loadFpuConst64
	// loadFpuConst128 represents a load of a 128-bit floating-point constant.
	loadFpuConst128
	// fpuToInt represents a conversion from FP to integer.
	fpuToInt
	// intToFpu represents a conversion from integer to FP.
	intToFpu
	// fpuCSel32 represents a 32-bit FP conditional select.
	fpuCSel32
	// fpuCSel64 represents a 64-bit FP conditional select.
	fpuCSel64
	// fpuRound represents a rounding to integer operation.
	fpuRound
	// movToFpu represents a move from a GPR to a scalar FP register.
	movToFpu
	// movToVec represents a move to a vector element from a GPR.
	movToVec
	// movFromVec represents an unsigned move from a vector element to a GPR.
	movFromVec
	// movFromVecSigned represents a signed move from a vector element to a GPR.
	movFromVecSigned
	// vecDup represents a duplication of general-purpose register to vector.
	vecDup
	// vecDupFromFpu represents a duplication of scalar to vector.
	vecDupFromFpu
	// vecExtend represents a vector extension operation.
	vecExtend
	// vecMovElement represents a move vector element to another vector element operation.
	vecMovElement
	// vecMiscNarrow represents a vector narrowing operation.
	vecMiscNarrow
	// vecRRR represents a vector ALU operation.
	vecRRR
	// vecMisc represents a vector two register miscellaneous instruction.
	vecMisc
	// vecLanes represents a vector instruction across lanes.
	vecLanes
	// vecTbl represents a table vector lookup - single register table.
	vecTbl
	// vecTbl2 represents a table vector lookup - two register table.
	vecTbl2
	// movToNZCV represents a move to the NZCV flags.
	movToNZCV
	// movFromNZCV represents a move from the NZCV flags.
	movFromNZCV
	// call represents a machine call instruction.
	call
	// callInd represents a machine indirect-call instruction.
	callInd
	// ret represents a machine return instruction.
	ret
	// epiloguePlaceholder is a placeholder instruction, generating no code, meaning that a function epilogue must be
	// inserted there.
	epiloguePlaceholder
	// br represents an unconditional branch.
	br
	// condBr represents a conditional branch.
	condBr
	// trapIf represents a conditional trap.
	// indirectBr represents an indirect branch through a register.
	indirectBr
	// adr represents a compute the address (using a PC-relative offset) of a memory location.
	adr
	// word4 represents a raw 32-bit word.
	word4
	// word8 represents a raw 64-bit word.
	word8
	// jtSequence represents a jump-table sequence.
	jtSequence
	// loadAddr represents a load address instruction.
	loadAddr
	// exitSequence consists of multiple instructions, and exits the execution immediately.
	// See encodeExitSequence.
	exitSequence
	// UDF is the undefined instruction. For debugging only.
	udf

	// ------------------- do not define below this line -------------------
	numInstructionKinds
)

func (i *instruction) asUDF() {
	i.kind = udf
}

func (i *instruction) asExitSequence(ctx regalloc.VReg) {
	i.kind = exitSequence
	i.rn = operandNR(ctx)
}

// aluOp determines the type of ALU operation. Instructions whose kind is one of
// aluRRR, aluRRRR, aluRRImm12, aluRRBitmaskImm, aluRRImmShift, aluRRRShift and aluRRRExtend
// would use this type.
type aluOp int

func (a aluOp) String() string {
	switch a {
	case aluOpAdd:
		return "add"
	case aluOpSub:
		return "sub"
	case aluOpOrr:
		return "orr"
	case aluOpAnd:
		return "and"
	case aluOpBic:
		return "bic"
	case aluOpEor:
		return "eor"
	case aluOpAddS:
		return "adds"
	case aluOpSubS:
		return "subs"
	case aluOpSMulH:
		return "sMulH"
	case aluOpUMulH:
		return "uMulH"
	case aluOpSDiv64:
		return "sDiv64"
	case aluOpUDiv64:
		return "uDiv64"
	case aluOpRotR:
		return "rotR"
	case aluOpLsr:
		return "lsr"
	case aluOpAsr:
		return "asr"
	case aluOpLsl:
		return "lsl"
	case aluOpMAdd:
		return "madd"
	case aluOpMSub:
		return "msub"
	}
	panic(int(a))
}

const (
	// 32/64-bit Add.
	aluOpAdd aluOp = iota
	// 32/64-bit Subtract.
	aluOpSub
	// 32/64-bit Bitwise OR.
	aluOpOrr
	// 32/64-bit Bitwise AND.
	aluOpAnd
	// 32/64-bit Bitwise AND NOT.
	aluOpBic
	// 32/64-bit Bitwise XOR (Exclusive OR).
	aluOpEor
	// 32/64-bit Add setting flags.
	aluOpAddS
	// 32/64-bit Subtract setting flags.
	aluOpSubS
	// Signed multiply, high-word result.
	aluOpSMulH
	// Unsigned multiply, high-word result.
	aluOpUMulH
	// 64-bit Signed divide.
	aluOpSDiv64
	// 64-bit Unsigned divide.
	aluOpUDiv64
	// 32/64-bit Rotate right.
	aluOpRotR
	// 32/64-bit Logical shift right.
	aluOpLsr
	// 32/64-bit Arithmetic shift right.
	aluOpAsr
	// 32/64-bit Logical shift left.
	aluOpLsl /// Multiply-add

	// MAdd and MSub are only applicable for aluRRRR.
	aluOpMAdd
	aluOpMSub
)

// bitOp determines the type of bitwise operation. Instructions whose kind is one of
// bitOpRbit and bitOpClz would use this type.
type bitOp int

func (b bitOp) String() string {
	switch b {
	case bitOpRbit:
		return "rbit"
	case bitOpClz:
		return "clz"
	}
	panic(int(b))
}

const (
	// 32/64-bit Clz.
	bitOpRbit bitOp = iota
	bitOpClz
)

// fpuBinOp represents a binary floating-point unit (FPU) operation.
type fpuBinOp byte

const (
	fpuBinOpAdd = iota
	fpuBinOpSub
	fpuBinOpMul
	fpuBinOpDiv
	fpuBinOpMax
	fpuBinOpMin
)

// String implements the fmt.Stringer.
func (f fpuBinOp) String() string {
	switch f {
	case fpuBinOpAdd:
		return "fadd"
	case fpuBinOpSub:
		return "fsub"
	case fpuBinOpMul:
		return "fmul"
	case fpuBinOpDiv:
		return "fdiv"
	case fpuBinOpMax:
		return "fmax"
	case fpuBinOpMin:
		return "fmin"
	}
	panic(int(f))
}

// extMode represents the mode of a register operand extension.
// For example, aluRRRExtend instructions need this info to determine the extensions.
type extMode byte

const (
	extModeNone extMode = iota
	// extModeZeroExtend64 suggests a zero-extension to 32 bits if the original bit size is less than 32.
	extModeZeroExtend32
	// extModeSignExtend64 stands for a sign-extension to 32 bits if the original bit size is less than 32.
	extModeSignExtend32
	// extModeZeroExtend64 suggests a zero-extension to 64 bits if the original bit size is less than 64.
	extModeZeroExtend64
	// extModeSignExtend64 stands for a sign-extension to 64 bits if the original bit size is less than 64.
	extModeSignExtend64
)

func (e extMode) bits() byte {
	switch e {
	case extModeZeroExtend32, extModeSignExtend32:
		return 32
	case extModeZeroExtend64, extModeSignExtend64:
		return 64
	default:
		return 0
	}
}

func (e extMode) signed() bool {
	switch e {
	case extModeSignExtend32, extModeSignExtend64:
		return true
	default:
		return false
	}
}

func extModeOf(t ssa.Type, signed bool) extMode {
	switch t.Bits() {
	case 32:
		if signed {
			return extModeSignExtend32
		}
		return extModeZeroExtend32
	case 64:
		if signed {
			return extModeSignExtend64
		}
		return extModeZeroExtend64
	default:
		panic("TODO? do we need narrower than 32 bits?")
	}
}

type extendOp byte

const (
	extendOpUXTB extendOp = 0b000
	extendOpUXTH extendOp = 0b001
	extendOpUXTW extendOp = 0b010
	// extendOpUXTX does nothing, but convenient symbol that officially exists. See:
	// https://stackoverflow.com/questions/72041372/what-do-the-uxtx-and-sxtx-extensions-mean-for-32-bit-aarch64-adds-instruct
	extendOpUXTX extendOp = 0b011
	extendOpSXTB extendOp = 0b100
	extendOpSXTH extendOp = 0b101
	extendOpSXTW extendOp = 0b110
	// extendOpSXTX does nothing, but convenient symbol that officially exists. See:
	// https://stackoverflow.com/questions/72041372/what-do-the-uxtx-and-sxtx-extensions-mean-for-32-bit-aarch64-adds-instruct
	extendOpSXTX extendOp = 0b111
	extendOpNone extendOp = 0xff
)

func (e extendOp) srcBits() byte {
	switch e {
	case extendOpUXTB, extendOpSXTB:
		return 8
	case extendOpUXTH, extendOpSXTH:
		return 16
	case extendOpUXTW, extendOpSXTW:
		return 32
	case extendOpUXTX, extendOpSXTX:
		return 64
	}
	panic(int(e))
}

func (e extendOp) String() string {
	switch e {
	case extendOpUXTB:
		return "UXTB"
	case extendOpUXTH:
		return "UXTH"
	case extendOpUXTW:
		return "UXTW"
	case extendOpUXTX:
		return "UXTX"
	case extendOpSXTB:
		return "SXTB"
	case extendOpSXTH:
		return "SXTH"
	case extendOpSXTW:
		return "SXTW"
	case extendOpSXTX:
		return "SXTX"
	}
	panic(int(e))
}

func extendOpFrom(signed bool, from byte) extendOp {
	switch from {
	case 8:
		if signed {
			return extendOpSXTB
		}
		return extendOpUXTB
	case 16:
		if signed {
			return extendOpSXTH
		}
		return extendOpUXTH
	case 32:
		if signed {
			return extendOpSXTW
		}
		return extendOpUXTW
	case 64:
		if signed {
			return extendOpSXTX
		}
		return extendOpUXTX
	}
	panic("invalid extendOpFrom")
}

type shiftOp byte

const (
	shiftOpLSL shiftOp = 0b00
	shiftOpLSR shiftOp = 0b01
	shiftOpASR shiftOp = 0b10
	shiftOpROR shiftOp = 0b11
)

func (s shiftOp) String() string {
	switch s {
	case shiftOpLSL:
		return "LSL"
	case shiftOpLSR:
		return "LSR"
	case shiftOpASR:
		return "ASR"
	case shiftOpROR:
		return "ROR"
	}
	panic(int(s))
}

func binarySize(begin, end *instruction) (size int64) {
	for cur := begin; ; cur = cur.next {
		size += cur.size()
		if cur == end {
			break
		}
	}
	return size
}

const exitSequenceSize = 5 * 4 // 5 instructions as in encodeExitSequence.

// size returns the size of the instruction in encoded bytes.
func (i *instruction) size() int64 {
	switch i.kind {
	case exitSequence:
		return exitSequenceSize // 5 instructions as in encodeExitSequence.
	case nop0:
		return 0
	case loadFpuConst32:
		return 4 + 4 + 4
	case loadFpuConst64:
		return 4 + 4 + 8
	case loadFpuConst128:
		return 4 + 4 + 12
	default:
		return 4
	}
}
