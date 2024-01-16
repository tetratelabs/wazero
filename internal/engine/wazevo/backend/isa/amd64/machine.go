package amd64

import (
	"context"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// NewBackend returns a new backend for arm64.
func NewBackend() backend.Machine {
	ectx := backend.NewExecutableContextT[instruction](
		resetInstruction,
		setNext,
		setPrev,
		asNop,
	)
	return &machine{
		spillSlots: make(map[regalloc.VRegID]int64),
		ectx:       ectx,
		regAlloc:   regalloc.NewAllocator(regInfo),
	}
}

type (
	// machine implements backend.Machine for amd64.
	machine struct {
		c                        backend.Compiler
		ectx                     *backend.ExecutableContextT[instruction]
		stackBoundsCheckDisabled bool

		regAlloc        regalloc.Allocator
		regAllocFn      *backend.RegAllocFunction[*instruction, *machine]
		regAllocStarted bool

		spillSlotSize int64
		spillSlots    map[regalloc.VRegID]int64 // regalloc.VRegID to offset.
		currentABI    *backend.FunctionABI
		clobberedRegs []regalloc.VReg

		maxRequiredStackSizeForCalls int64

		labelResolutionPends []labelResolutionPend
	}

	labelResolutionPend struct {
		instr  *instruction
		offset int64
	}
)

// Reset implements backend.Machine.
func (m *machine) Reset() {
	m.stackBoundsCheckDisabled = false
	m.ectx.Reset()

	m.regAllocFn.Reset()
	m.regAlloc.Reset()
	m.regAllocStarted = false
}

// ExecutableContext implements backend.Machine.
func (m *machine) ExecutableContext() backend.ExecutableContext { return m.ectx }

// DisableStackCheck implements backend.Machine.
func (m *machine) DisableStackCheck() { m.stackBoundsCheckDisabled = true }

// SetCompiler implements backend.Machine.
func (m *machine) SetCompiler(c backend.Compiler) {
	m.c = c
	m.regAllocFn = backend.NewRegAllocFunction[*instruction, *machine](m, c.SSABuilder(), c)
}

// SetCurrentABI implements backend.Machine.
func (m *machine) SetCurrentABI(abi *backend.FunctionABI) {
	m.currentABI = abi
}

// RegAlloc implements backend.Machine.
func (m *machine) RegAlloc() {
	rf := m.regAllocFn
	for _, pos := range m.ectx.OrderedBlockLabels {
		rf.AddBlock(pos.SB, pos.L, pos.Begin, pos.End)
	}

	m.regAllocStarted = true
	m.regAlloc.DoAllocation(rf)
	// Now that we know the final spill slot size, we must align spillSlotSize to 16 bytes.
	m.spillSlotSize = (m.spillSlotSize + 15) &^ 15
}

// InsertReturn implements backend.Machine.
func (m *machine) InsertReturn() {
	i := m.allocateInstr().asRet(m.currentABI)
	m.insert(i)
}

func (m *machine) getVRegSpillSlotOffsetFromSP(id regalloc.VRegID, size byte) int64 {
	offset, ok := m.spillSlots[id]
	if !ok {
		offset = m.spillSlotSize
		// TODO: this should be aligned depending on the `size` to use Imm12 offset load/store as much as possible.
		m.spillSlots[id] = offset
		m.spillSlotSize += int64(size)
	}
	return offset + 16 // spill slot starts above the clobbered registers and the frame size.
}

// LowerSingleBranch implements backend.Machine.
func (m *machine) LowerSingleBranch(b *ssa.Instruction) {
	ectx := m.ectx
	switch b.Opcode() {
	case ssa.OpcodeJump:
		_, _, targetBlk := b.BranchData()
		if b.IsFallthroughJump() {
			return
		}
		jmp := m.allocateInstr()
		target := ectx.GetOrAllocateSSABlockLabel(targetBlk)
		if target == backend.LabelReturn {
			jmp.asRet(m.currentABI)
		} else {
			jmp.asJmp(newOperandLabel(target))
		}
		m.insert(jmp)
	case ssa.OpcodeBrTable:
		panic("TODO: implement me")
	default:
		panic("BUG: unexpected branch opcode" + b.Opcode().String())
	}
}

// LowerConditionalBranch implements backend.Machine.
func (m *machine) LowerConditionalBranch(b *ssa.Instruction) {
	// TODO implement me
	panic("implement me")
}

// LowerInstr implements backend.Machine.
func (m *machine) LowerInstr(instr *ssa.Instruction) {
	switch op := instr.Opcode(); op {
	case ssa.OpcodeBrz, ssa.OpcodeBrnz, ssa.OpcodeJump, ssa.OpcodeBrTable:
		panic("BUG: branching instructions are handled by LowerBranches")
	case ssa.OpcodeReturn:
		panic("BUG: return must be handled by backend.Compiler")
	case ssa.OpcodeIconst, ssa.OpcodeF32const, ssa.OpcodeF64const: // Constant instructions are inlined.
	case ssa.OpcodeCall:
		m.lowerCall(instr)
	case ssa.OpcodeStore:
		m.lowerStore(instr)
	case ssa.OpcodeIadd:
		m.lowerIadd(instr)
	default:
		panic("TODO: lowering " + op.String())
	}
}

func (m *machine) lowerStore(si *ssa.Instruction) {
	value, ptr, offset, storeSizeInBits := si.StoreData()
	rm := m.c.VRegOf(value)
	base := m.c.VRegOf(ptr)

	store := m.allocateInstr()
	store.asMovRM(rm, newOperandMem(newAmodeImmReg(offset, base)), storeSizeInBits/8)
	m.insert(store)
}

func (m *machine) lowerCall(si *ssa.Instruction) {
	isDirectCall := si.Opcode() == ssa.OpcodeCall
	var indirectCalleePtr ssa.Value
	var directCallee ssa.FuncRef
	var sigID ssa.SignatureID
	var args []ssa.Value
	if isDirectCall {
		directCallee, sigID, args = si.CallData()
	} else {
		indirectCalleePtr, sigID, args = si.CallIndirectData()
	}
	calleeABI := m.c.GetFunctionABI(m.c.SSABuilder().ResolveSignature(sigID))

	stackSlotSize := calleeABI.AlignedArgResultStackSlotSize()
	if m.maxRequiredStackSizeForCalls < stackSlotSize+16 {
		m.maxRequiredStackSizeForCalls = stackSlotSize + 16 // return address frame.
	}

	for i, arg := range args {
		reg := m.c.VRegOf(arg)
		def := m.c.ValueDefinition(arg)
		m.callerGenVRegToFunctionArg(calleeABI, i, reg, def, stackSlotSize)
	}

	if isDirectCall {
		call := m.allocateInstr()
		call.asCall(directCallee, calleeABI)
		m.insert(call)
	} else {
		_ = indirectCalleePtr
		panic("TODO")
	}

	var index int
	r1, rs := si.Returns()
	if r1.Valid() {
		m.callerGenFunctionReturnVReg(calleeABI, 0, m.c.VRegOf(r1), stackSlotSize)
		index++
	}

	for _, r := range rs {
		m.callerGenFunctionReturnVReg(calleeABI, index, m.c.VRegOf(r), stackSlotSize)
		index++
	}
}

func (m *machine) lowerIadd(si *ssa.Instruction) {
	x, y := si.Arg2()
	if !x.Type().IsInt() {
		panic("BUG?")
	}

	_64 := x.Type().Bits() == 64

	xDef := m.c.ValueDefinition(x)
	yDef := m.c.ValueDefinition(y)

	rn := m.getOperandGP(xDef)
	rm := m.getOperandGP(yDef)

	tmp := m.c.VRegOf(si.Return())
	mov := m.allocateInstr()
	mov.asMovRR(rn.r, tmp, _64)

	alu := m.allocateInstr()
	alu.asAluRmiR(aluRmiROpcodeAdd, rm, tmp, _64)
	m.insert(alu)
}

func (m *machine) getOperandGP(def *backend.SSAValueDefinition) operand {
	var v regalloc.VReg
	if def.IsFromBlockParam() {
		v = def.BlkParamVReg
	} else {
		instr := def.Instr
		if instr.Constant() {
			panic("TODO: instr.Constant")
		} else {
			if n := def.N; n == 0 {
				v = m.c.VRegOf(instr.Return())
			} else {
				_, rs := instr.Returns()
				v = m.c.VRegOf(rs[n-1])
			}
		}
	}

	r := v

	//switch inBits := def.SSAValue().Type().Bits(); {
	//case mode == extModeNone:
	//case inBits == 32 && (mode == extModeZeroExtend32 || mode == extModeSignExtend32):
	//case inBits == 32 && mode == extModeZeroExtend64:
	//	extended := m.compiler.AllocateVReg(ssa.TypeI64)
	//	ext := m.allocateInstr()
	//	ext.asExtend(extended, v, 32, 64, false)
	//	m.insert(ext)
	//	r = extended
	//case inBits == 32 && mode == extModeSignExtend64:
	//	extended := m.compiler.AllocateVReg(ssa.TypeI64)
	//	ext := m.allocateInstr()
	//	ext.asExtend(extended, v, 32, 64, true)
	//	m.insert(ext)
	//	r = extended
	//case inBits == 64 && (mode == extModeZeroExtend64 || mode == extModeSignExtend64):
	//}
	//return operandNR(r)

	return newOperandReg(r)
}

// callerGenVRegToFunctionArg is the opposite of GenFunctionArgToVReg, which is used to generate the
// caller side of the function call.
func (m *machine) callerGenVRegToFunctionArg(a *backend.FunctionABI, argIndex int, reg regalloc.VReg, def *backend.SSAValueDefinition, slotBegin int64) {
	arg := &a.Args[argIndex]
	if def != nil && def.IsFromInstr() {
		// Constant instructions are inlined.
		if inst := def.Instr; inst.Constant() {
			m.InsertLoadConstant(inst, reg)
		}
	}
	if arg.Kind == backend.ABIArgKindReg {
		m.InsertMove(arg.Reg, reg, arg.Type)
	} else {
		// TODO: we could use pair store if there's consecutive stores for the same type.
		//
		// Note that at this point, stack pointer is already adjusted.
		bits := arg.Type.Bits()
		am := m.resolveAddressModeForOffset(arg.Offset-slotBegin, bits, rspVReg)
		store := m.allocateInstr()
		store.asMovRM(reg, newOperandMem(am), bits/8)
		m.insert(store)
	}
}

func (m *machine) callerGenFunctionReturnVReg(a *backend.FunctionABI, retIndex int, reg regalloc.VReg, slotBegin int64) {
	r := &a.Rets[retIndex]
	if r.Kind == backend.ABIArgKindReg {
		m.InsertMove(reg, r.Reg, r.Type)
	} else {
		// TODO: we could use pair load if there's consecutive loads for the same type.
		dst := m.resolveAddressModeForOffset(a.ArgStackSize+r.Offset-slotBegin, r.Type.Bits(), rspVReg)
		ldr := m.allocateInstr()
		switch r.Type {
		case ssa.TypeI32, ssa.TypeI64:
			ldr.asLEA(dst, reg)
		case ssa.TypeF32, ssa.TypeF64, ssa.TypeV128:
			panic("TODO") // ldr.asFpuLoad(operandNR(reg), amode, r.Type.Bits())
		default:
			panic("BUG")
		}
		m.insert(ldr)
	}
}

func (m *machine) resolveAddressModeForOffset(offset int64, dstBits byte, rn regalloc.VReg) amode {
	if rn.RegType() != regalloc.RegTypeInt {
		panic("BUG: rn should be a pointer: " + formatVRegSized(rn, dstBits == 64))
	}
	var am amode
	if offsetFitsInAddressModeKindRegUnsignedImm12(dstBits, offset) {
		am = newAmodeImmReg(uint32(offset), rn)
	} else {
		panic("TODO")
	}
	return am
}

func (m *machine) resolveAddressModeForOffsetAndInsert(cur *instruction, offset int64, dstBits byte, rn regalloc.VReg) (*instruction, amode) {
	exct := m.ectx
	exct.PendingInstructions = exct.PendingInstructions[:0]
	mode := m.resolveAddressModeForOffset(offset, dstBits, rn)
	for _, instr := range exct.PendingInstructions {
		cur = linkInstr(cur, instr)
	}
	return cur, mode
}

func offsetFitsInAddressModeKindRegUnsignedImm12(dstSizeInBits byte, offset int64) bool {
	divisor := int64(dstSizeInBits) / 8
	return 0 < offset && offset%divisor == 0 && offset/divisor < 4096
}

// InsertMove implements backend.Machine.
func (m *machine) InsertMove(dst, src regalloc.VReg, typ ssa.Type) {
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		i := m.allocateInstr().asMovRR(src, dst, typ.Bits() == 64)
		m.insert(i)
	case ssa.TypeF32, ssa.TypeF64, ssa.TypeV128:
		var op sseOpcode
		switch typ {
		case ssa.TypeF32:
			op = sseOpcodeMovss
		case ssa.TypeF64:
			op = sseOpcodeMovsd
		case ssa.TypeV128:
			op = sseOpcodeMovdqa
		}
		i := m.allocateInstr().asXmmUnaryRmR(op, operand{kind: operandKindReg, r: src}, dst, typ.Bits() == 64)
		m.insert(i)
	default:
		panic("BUG")
	}
}

// Format implements backend.Machine.
func (m *machine) Format() string {
	ectx := m.ectx
	begins := map[*instruction]backend.Label{}
	for l, pos := range ectx.LabelPositions {
		begins[pos.Begin] = l
	}

	irBlocks := map[backend.Label]ssa.BasicBlockID{}
	for i, l := range ectx.SsaBlockIDToLabels {
		irBlocks[l] = ssa.BasicBlockID(i)
	}

	var lines []string
	for cur := ectx.RootInstr; cur != nil; cur = cur.next {
		if l, ok := begins[cur]; ok {
			var labelStr string
			if blkID, ok := irBlocks[l]; ok {
				labelStr = fmt.Sprintf("%s (SSA Block: %s):", l, blkID)
			} else {
				labelStr = fmt.Sprintf("%s:", l)
			}
			lines = append(lines, labelStr)
		}
		if cur.kind == nop0 {
			continue
		}
		lines = append(lines, "\t"+cur.String())
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

func (m *machine) encodeWithoutRelResolution(root *instruction) {
	for cur := root; cur != nil; cur = cur.next {
		cur.encode(m.c)
	}
}

// Encode implements backend.Machine Encode.
func (m *machine) Encode(context.Context) {
	ectx := m.ectx
	bufPtr := m.c.BufPtr()

	m.labelResolutionPends = m.labelResolutionPends[:0]
	for _, pos := range ectx.OrderedBlockLabels {
		offset := int64(len(*bufPtr))
		pos.BinaryOffset = offset
		for cur := pos.Begin; cur != pos.End.next; cur = cur.next {
			offset := int64(len(*bufPtr))
			if cur.kind == nop0 {
				l := cur.nop0Label()
				if pos, ok := ectx.LabelPositions[l]; ok {
					pos.BinaryOffset = offset
				}
			}

			needLabelResolution := cur.encode(m.c)
			if needLabelResolution {
				m.labelResolutionPends = append(m.labelResolutionPends,
					labelResolutionPend{instr: cur, offset: int64(offset)},
				)
			}
		}
	}

	for i := range m.labelResolutionPends {
		p := &m.labelResolutionPends[i]
		switch p.instr.kind {
		case jmp:
			panic("TODO")
		case jmpIf:
			panic("TODO")
		default:
			panic("BUG")
		}
	}
}

// ResolveRelocations implements backend.Machine.
func (m *machine) ResolveRelocations(refToBinaryOffset map[ssa.FuncRef]int, binary []byte, relocations []backend.RelocationInfo) {
	// TODO implement me
	panic("implement me")
}

// allocateInstr allocates an instruction.
func (m *machine) allocateInstr() *instruction {
	instr := m.ectx.InstructionPool.Allocate()
	if !m.regAllocStarted {
		instr.addedBeforeRegAlloc = true
	}
	return instr
}

func (m *machine) allocateNop() *instruction {
	instr := m.allocateInstr()
	instr.kind = nop0
	return instr
}

func (m *machine) insert(i *instruction) {
	ectx := m.ectx
	ectx.PendingInstructions = append(ectx.PendingInstructions, i)
}

func (m *machine) allocateBrTarget() (nop *instruction, l backend.Label) { //nolint
	ectx := m.ectx
	l = ectx.AllocateLabel()
	nop = m.allocateInstr()
	nop.asNop0WithLabel(l)
	pos := ectx.AllocateLabelPosition(l)
	pos.Begin, pos.End = nop, nop
	ectx.LabelPositions[l] = pos
	return
}
