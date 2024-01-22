package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// lowerToAddressMode converts a pointer to an addressMode that can be used as an operand for load/store instructions.
func (m *machine) lowerToAddressMode(ptr ssa.Value, offsetBase uint32) (am amode) {
	a64s, offset := m.collectAddends(ptr)
	offset += int64(offsetBase)
	return m.lowerToAddressModeFromAddends(a64s, offset)
}

// lowerToAddressModeFromAddends creates an addressMode from a list of addends collected by collectAddends.
// During the construction, this might emit additional instructions.
//
// Extracted as a separate function for easy testing.
func (m *machine) lowerToAddressModeFromAddends(a64s *queue[regalloc.VReg], offset int64) (am amode) {
	if a64s.empty() {
		// Only static offsets.
		tmpReg := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmpReg, uint64(offset), true)
		am = newAmodeImmReg(0, tmpReg)
		offset = 0
	} else if base := a64s.dequeue(); a64s.empty() {
		if lower32willSignExtendTo64(uint64(offset)) {
			// Absorb the offset into the amode with no index.
			am = newAmodeImmReg(uint32(offset), base)
			offset = 0
		} else {
			// Offset is too large to be absorbed into the amode, will be added later.
			am = newAmodeImmReg(0, base)
		}
	} else if index := a64s.dequeue(); lower32willSignExtendTo64(uint64(offset)) {
		// Absorb the offset into the amode with an index.
		am = newAmodeRegRegShift(uint32(offset), base, index, 0)
		offset = 0
	} else {
		// Offset is too large to be absorbed into the amode, will be added later.
		am = newAmodeRegRegShift(0, base, index, 0)
	}

	baseReg := am.base
	if offset > 0 {
		baseReg = m.addConstToReg64(baseReg, offset) // baseReg += offset
	}

	for !a64s.empty() {
		a64 := a64s.dequeue()
		baseReg = m.addReg64ToReg64(baseReg, a64) // baseReg += a64
	}

	am.base = baseReg
	return
}

var addendsMatchOpcodes = [4]ssa.Opcode{ssa.OpcodeUExtend, ssa.OpcodeSExtend, ssa.OpcodeIadd, ssa.OpcodeIconst}

func (m *machine) collectAddends(ptr ssa.Value) (addends64 *queue[regalloc.VReg], offset int64) {
	m.addendsWorkQueue.reset()
	m.addends64.reset()
	m.addendsWorkQueue.enqueue(ptr)

	for !m.addendsWorkQueue.empty() {
		v := m.addendsWorkQueue.dequeue()

		def := m.c.ValueDefinition(v)
		switch op := m.c.MatchInstrOneOf(def, addendsMatchOpcodes[:]); op {
		case ssa.OpcodeIadd:
			// If the addend is an add, we recursively collect its operands.
			x, y := def.Instr.Arg2()
			m.addendsWorkQueue.enqueue(x)
			m.addendsWorkQueue.enqueue(y)
			def.Instr.MarkLowered()
		case ssa.OpcodeIconst:
			// If the addend is constant, we just statically merge it into the offset.
			ic := def.Instr
			u64 := ic.ConstantVal()
			if ic.Return().Type().Bits() == 32 {
				offset += int64(int32(u64)) // sign-extend.
			} else {
				offset += int64(u64)
			}
			def.Instr.MarkLowered()
		case ssa.OpcodeUExtend, ssa.OpcodeSExtend:
			switch input := def.Instr.Arg(); input.Type().Bits() {
			case 64:
				// If the input is already 64-bit, this extend is a no-op. TODO: shouldn't this be optimized out at much earlier stage? no?
				m.addends64.enqueue(m.getOperand_Reg(m.c.ValueDefinition(input)).r)
				def.Instr.MarkLowered()
				continue
			case 32:
				inputDef := m.c.ValueDefinition(input)
				constInst := inputDef.IsFromInstr() && inputDef.Instr.Constant()
				switch {
				case constInst && op == ssa.OpcodeUExtend:
					// Zero-extension of a 32-bit constant can be merged into the offset.
					offset += int64(uint32(inputDef.Instr.ConstantVal()))
				case constInst && op == ssa.OpcodeSExtend:
					// Sign-extension of a 32-bit constant can be merged into the offset.
					offset += int64(int32(inputDef.Instr.ConstantVal())) // sign-extend!
				default:
					// Cannot fold into a constant, ignore.
					continue
				}
				def.Instr.MarkLowered()
				continue
			}
			// Note: case Ishl x, y could be handled too when the offset amount is <= 3.
		default:
			// If the addend is not one of them, we simply use it as-is.
			m.addends64.enqueue(m.getOperand_Reg(def).r)
		}
	}
	return &m.addends64, offset
}

// FIXME: this can be shared.
// queue is the resettable queue where the underlying slice is reused.
type queue[T any] struct {
	index int
	data  []T
}

func (m *machine) addConstToReg64(rd regalloc.VReg, c int64) regalloc.VReg {
	alu := m.allocateInstr()
	u64 := uint64(c)
	if imm32Op, ok := asImm32Operand(u64); ok {
		alu.asAluRmiR(aluRmiROpcodeAdd, imm32Op, rd, true)
	} else {
		tmp := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmp, u64, true)
		alu.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(tmp), rd, true)
	}
	m.insert(alu)
	return rd
}

func (m *machine) addReg64ToReg64(rd, rm regalloc.VReg) regalloc.VReg {
	alu := m.allocateInstr()
	alu.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(rm), rd, true)
	m.insert(alu)
	return rd
}

func (q *queue[T]) enqueue(v T) {
	q.data = append(q.data, v)
}

func (q *queue[T]) dequeue() (ret T) {
	ret = q.data[q.index]
	q.index++
	return
}

func (q *queue[T]) empty() bool {
	return q.index >= len(q.data)
}

func (q *queue[T]) reset() {
	q.index = 0
	q.data = q.data[:0]
}