package ssa

import (
	"fmt"
	"math"
	"strings"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// Opcode represents a SSA instruction.
type Opcode uint32

// Instruction represents an instruction whose opcode is specified by
// Opcode. Since Go doesn't have union type, we use this flattened type
// for all instructions, and therefore each field has different meaning
// depending on Opcode.
type Instruction struct {
	opcode     Opcode
	u64        uint64
	v          Value
	v2         Value
	vs         []Value
	typ        Type
	blk        BasicBlock
	prev, next *Instruction

	rValue  Value
	rValues []Value
	gid     InstructionGroupID
	live    bool
}

// Opcode returns the opcode of this instruction.
func (i *Instruction) Opcode() Opcode {
	return i.opcode
}

// GroupID returns the InstructionGroupID of this instruction.
func (i *Instruction) GroupID() InstructionGroupID {
	return i.gid
}

// reset resets this instruction to the initial state.
func (i *Instruction) reset() {
	*i = Instruction{}
	i.v = ValueInvalid
	i.v2 = ValueInvalid
	i.rValue = ValueInvalid
	i.typ = typeInvalid
}

// InstructionGroupID is assigned to each instruction and represents a group of instructions
// where each instruction is interchangeable with others except for the last instruction
// in the group which has side effects. In short, InstructionGroupID is determined by the side effects of instructions.
// That means, if there's an instruction with side effect between two instructions, then these two instructions
// will have different instructionGroupID. Note that each block always ends with branching, which is with side effects,
// therefore, instructions in different blocks always have different InstructionGroupID(s).
//
// The notable application of this is used in lowering SSA-level instruction to a ISA specific instruction,
// where we eagerly try to merge multiple instructions into single operation etc. Such merging cannot be done
// if these instruction have different InstructionGroupID since it will change the semantics of a program.
//
// See passDeadCodeElimination.
type InstructionGroupID uint32

// Returns Value(s) produced by this instruction if any.
// The `first` is the first return value, and `rest` is the rest of the values.
func (i *Instruction) Returns() (first Value, rest []Value) {
	return i.rValue, i.rValues
}

// Return returns a Value(s) produced by this instruction if any.
// If there's multiple return values, only the first one is returned.
func (i *Instruction) Return() (first Value) {
	return i.rValue
}

// Args returns the arguments to this instruction.
func (i *Instruction) Args() (v1, v2 Value, vs []Value) {
	return i.v, i.v2, i.vs
}

// Arg returns the first argument to this instruction.
func (i *Instruction) Arg() Value {
	return i.v
}

// Arg2 returns the first two argument2 to this instruction.
func (i *Instruction) Arg2() (Value, Value) {
	return i.v, i.v2
}

// Next returns the next instruction laid out next to itself.
func (i *Instruction) Next() *Instruction {
	return i.next
}

// Prev returns the previous instruction laid out prior to itself.
func (i *Instruction) Prev() *Instruction {
	return i.prev
}

// IsBranching returns true if this instruction is a branching instruction.
func (i *Instruction) IsBranching() bool {
	switch i.opcode {
	case OpcodeJump, OpcodeBrz, OpcodeBrnz, OpcodeBrTable:
		return true
	default:
		return false
	}
}

// TODO: complete opcode comments.
// TODO: there should be unnecessary opcodes.
const (
	OpcodeInvalid Opcode = iota

	// OpcodeUndefined is a placeholder for undefined opcode. This can be used for debugging to intentionally
	// cause a crash at certain point.
	OpcodeUndefined

	// OpcodeJump takes the list of args to the `block` and unconditionally jumps to it.
	OpcodeJump

	// OpcodeBrz branches into `blk` with `args`  if the value `c` equals zero: `Brz c, blk, args`.
	OpcodeBrz

	// OpcodeBrnz branches into `blk` with `args`  if the value `c` is not zero: `Brnz c, blk, args`.
	OpcodeBrnz

	// OpcodeBrTable ...
	// `BrTable x, block, JT`.
	OpcodeBrTable

	// OpcodeExitWithCode exit the execution immediately.
	OpcodeExitWithCode

	// OpcodeExitIfTrueWithCode exits the execution immediately if the value `c` is not zero.
	OpcodeExitIfTrueWithCode

	// OpcodeReturn returns from the function: `return rvalues`.
	OpcodeReturn

	// OpcodeCall calls a function specified by the symbol FN with arguments `args`: `returnvals = Call FN, args...`
	// This is a "near" call, which means the call target is known at compile time, and the target is relatively close
	// to this function. If the target cannot be reached by near call, the backend fails to compile.
	OpcodeCall

	// OpcodeCallIndirect calls a function specified by `callee` which is a function address: `returnvals = call_indirect SIG, callee, args`.
	// Note that this is different from call_indirect in Wasm, which also does type checking, etc.
	OpcodeCallIndirect

	// OpcodeFuncAddr ...
	// `addr = func_addr FN`.
	OpcodeFuncAddr

	// OpcodeSplat ...
	// `v = splat x`.
	OpcodeSplat

	// OpcodeSwizzle ...
	// `v = swizzle x, y`.
	OpcodeSwizzle

	// OpcodeInsertlane ...
	// `v = insertlane x, y, Idx`. (TernaryImm8)
	OpcodeInsertlane

	// OpcodeExtractlane ...
	// `v = extractlane x, Idx`. (BinaryImm8)
	OpcodeExtractlane

	// OpcodeSmin ...
	// `v = smin x, y`.
	OpcodeSmin

	// OpcodeUmin ...
	// `v = umin x, y`.
	OpcodeUmin

	// OpcodeSmax ...
	// `v = smax x, y`.
	OpcodeSmax

	// OpcodeUmax ...
	// `v = umax x, y`.
	OpcodeUmax

	// OpcodeAvgRound ...
	// `v = avg_round x, y`.
	OpcodeAvgRound

	// OpcodeUaddSat ...
	// `v = uadd_sat x, y`.
	OpcodeUaddSat

	// OpcodeSaddSat ...
	// `v = sadd_sat x, y`.
	OpcodeSaddSat

	// OpcodeUsubSat ...
	// `v = usub_sat x, y`.
	OpcodeUsubSat

	// OpcodeSsubSat ...
	// `v = ssub_sat x, y`.
	OpcodeSsubSat

	// OpcodeLoad loads a Type value from the [base + offset] address: `v = Load base, offset`.
	OpcodeLoad

	// OpcodeStore stores a Type value to the [base + offset] address: `Store v, base, offset`.
	OpcodeStore

	// OpcodeUload8 loads the 8-bit value from the [base + offset] address, zero-extended to 64 bits: `v = Uload8 base, offset`.
	OpcodeUload8

	// OpcodeSload8 loads the 8-bit value from the [base + offset] address, sign-extended to 64 bits: `v = Sload8 base, offset`.
	OpcodeSload8

	// OpcodeIstore8 stores the 8-bit value to the [base + offset] address, sign-extended to 64 bits: `Istore8 v, base, offset`.
	OpcodeIstore8

	// OpcodeUload16 loads the 16-bit value from the [base + offset] address, zero-extended to 64 bits: `v = Uload16 base, offset`.
	OpcodeUload16

	// OpcodeSload16 loads the 16-bit value from the [base + offset] address, sign-extended to 64 bits: `v = Sload16 base, offset`.
	OpcodeSload16

	// OpcodeIstore16 stores the 16-bit value to the [base + offset] address, zero-extended to 64 bits: `Istore16 v, base, offset`.
	OpcodeIstore16

	// OpcodeUload32 loads the 32-bit value from the [base + offset] address, zero-extended to 64 bits: `v = Uload32 base, offset`.
	OpcodeUload32

	// OpcodeSload32 loads the 32-bit value from the [base + offset] address, sign-extended to 64 bits: `v = Sload32 base, offset`.
	OpcodeSload32

	// OpcodeIstore32 stores the 32-bit value to the [base + offset] address, zero-extended to 64 bits: `Istore16 v, base, offset`.
	OpcodeIstore32

	// OpcodeUload8x8 ...
	// `v = uload8x8 MemFlags, p, Offset`.
	OpcodeUload8x8

	// OpcodeSload8x8 ...
	// `v = sload8x8 MemFlags, p, Offset`.
	OpcodeSload8x8

	// OpcodeUload16x4 ...
	// `v = uload16x4 MemFlags, p, Offset`.
	OpcodeUload16x4

	// OpcodeSload16x4 ...
	// `v = sload16x4 MemFlags, p, Offset`.
	OpcodeSload16x4

	// OpcodeUload32x2 ...
	// `v = uload32x2 MemFlags, p, Offset`.
	OpcodeUload32x2

	// OpcodeSload32x2 ...
	// `v = sload32x2 MemFlags, p, Offset`.
	OpcodeSload32x2

	// OpcodeGlobalValue ...
	// `v = global_value GV`.
	OpcodeGlobalValue

	// OpcodeSymbolValue ...
	// `v = symbol_value GV`.
	OpcodeSymbolValue

	// OpcodeHeapAddr ...
	// `addr = heap_addr H, index, Offset, Size`.
	OpcodeHeapAddr

	// OpcodeHeapLoad ...
	// `v = heap_load heap_imm, index`.
	OpcodeHeapLoad

	// OpcodeHeapStore ...
	// `heap_store heap_imm, index, a`.
	OpcodeHeapStore

	// OpcodeGetReturnAddress ...
	// `addr = get_return_address`.
	OpcodeGetReturnAddress

	// OpcodeTableAddr ...
	// `addr = table_addr T, p, Offset`.
	OpcodeTableAddr

	// OpcodeIconst represents the integer const.
	OpcodeIconst

	// OpcodeF32const ...
	// `v = f32const N`. (UnaryIeee32)
	OpcodeF32const

	// OpcodeF64const ...
	// `v = f64const N`. (UnaryIeee64)
	OpcodeF64const

	// OpcodeVconst ...
	// `v = vconst N`.
	OpcodeVconst

	// OpcodeShuffle ...
	// `v = shuffle a, b, mask`.
	OpcodeShuffle

	// OpcodeNull ...
	// `v = null`.
	OpcodeNull

	// OpcodeNop ...
	// `nop`.
	OpcodeNop

	// OpcodeSelect chooses between two values based on a condition `c`: `v = Select c, x, y`.
	OpcodeSelect

	// OpcodeBitselect ...
	// `v = bitselect c, x, y`.
	OpcodeBitselect

	// OpcodeVsplit ...
	// `lo, hi = vsplit x`.
	OpcodeVsplit

	// OpcodeVconcat ...
	// `v = vconcat x, y`.
	OpcodeVconcat

	// OpcodeVselect ...
	// `v = vselect c, x, y`.
	OpcodeVselect

	// OpcodeVanyTrue ...
	// `s = vany_true a`.
	OpcodeVanyTrue

	// OpcodeVallTrue ...
	// `s = vall_true a`.
	OpcodeVallTrue

	// OpcodeVhighBits ...
	// `x = vhigh_bits a`.
	OpcodeVhighBits

	// OpcodeIcmp compares two integer values with the given condition: `v = icmp Cond, x, y`.
	OpcodeIcmp

	// OpcodeIcmpImm compares an integer value with the immediate value on the given condition: `v = icmp_imm Cond, x, Y`.
	OpcodeIcmpImm

	// OpcodeIadd performs an integer addition: `v = Iadd x, y`.
	OpcodeIadd

	// OpcodeIsub performs an integer subtraction: `v = Isub x, y`.
	OpcodeIsub

	// OpcodeIneg ...
	// `v = ineg x`.
	OpcodeIneg

	// OpcodeIabs ...
	// `v = iabs x`.
	OpcodeIabs

	// OpcodeImul performs an integer multiplication: `v = Imul x, y`.
	OpcodeImul

	// OpcodeUmulhi ...
	// `v = umulhi x, y`.
	OpcodeUmulhi

	// OpcodeSmulhi ...
	// `v = smulhi x, y`.
	OpcodeSmulhi

	// OpcodeSqmulRoundSat ...
	// `v = sqmul_round_sat x, y`.
	OpcodeSqmulRoundSat

	// OpcodeUdiv ...
	// `v = udiv x, y`.
	OpcodeUdiv

	// OpcodeSdiv ...
	// `v = sdiv x, y`.
	OpcodeSdiv

	// OpcodeUrem ...
	// `v = urem x, y`.
	OpcodeUrem

	// OpcodeSrem ...
	// `v = srem x, y`.
	OpcodeSrem

	// OpcodeIaddImm ...
	// `v = iadd_imm x, Y`. (BinaryImm64)
	OpcodeIaddImm

	// OpcodeImulImm ...
	// `v = imul_imm x, Y`. (BinaryImm64)
	OpcodeImulImm

	// OpcodeUdivImm ...
	// `v = udiv_imm x, Y`. (BinaryImm64)
	OpcodeUdivImm

	// OpcodeSdivImm ...
	// `v = sdiv_imm x, Y`. (BinaryImm64)
	OpcodeSdivImm

	// OpcodeUremImm ...
	// `v = urem_imm x, Y`. (BinaryImm64)
	OpcodeUremImm

	// OpcodeSremImm ...
	// `v = srem_imm x, Y`. (BinaryImm64)
	OpcodeSremImm

	// OpcodeIrsubImm ...
	// `v = irsub_imm x, Y`. (BinaryImm64)
	OpcodeIrsubImm

	// OpcodeIaddCin ...
	// `v = iadd_cin x, y, c_in`.
	OpcodeIaddCin

	// OpcodeIaddIfcin ...
	// `v = iadd_ifcin x, y, c_in`.
	OpcodeIaddIfcin

	// OpcodeIaddCout ...
	// `a, c_out = iadd_cout x, y`.
	OpcodeIaddCout

	// OpcodeIaddIfcout ...
	// `a, c_out = iadd_ifcout x, y`.
	OpcodeIaddIfcout

	// OpcodeIaddCarry ...
	// `a, c_out = iadd_carry x, y, c_in`.
	OpcodeIaddCarry

	// OpcodeIaddIfcarry ...
	// `a, c_out = iadd_ifcarry x, y, c_in`.
	OpcodeIaddIfcarry

	// OpcodeUaddOverflowTrap ...
	// `v = uadd_overflow_trap x, y, code`.
	OpcodeUaddOverflowTrap

	// OpcodeIsubBin ...
	// `v = isub_bin x, y, b_in`.
	OpcodeIsubBin

	// OpcodeIsubIfbin ...
	// `v = isub_ifbin x, y, b_in`.
	OpcodeIsubIfbin

	// OpcodeIsubBout ...
	// `a, b_out = isub_bout x, y`.
	OpcodeIsubBout

	// OpcodeIsubIfbout ...
	// `a, b_out = isub_ifbout x, y`.
	OpcodeIsubIfbout

	// OpcodeIsubBorrow ...
	// `a, b_out = isub_borrow x, y, b_in`.
	OpcodeIsubBorrow

	// OpcodeIsubIfborrow ...
	// `a, b_out = isub_ifborrow x, y, b_in`.
	OpcodeIsubIfborrow

	// OpcodeBand ...
	// `v = band x, y`.
	OpcodeBand

	// OpcodeBor ...
	// `v = bor x, y`.
	OpcodeBor

	// OpcodeBxor ...
	// `v = bxor x, y`.
	OpcodeBxor

	// OpcodeBnot ...
	// `v = bnot x`.
	OpcodeBnot

	// OpcodeBandNot ...
	// `v = band_not x, y`.
	OpcodeBandNot

	// OpcodeBorNot ...
	// `v = bor_not x, y`.
	OpcodeBorNot

	// OpcodeBxorNot ...
	// `v = bxor_not x, y`.
	OpcodeBxorNot

	// OpcodeBandImm ...
	// `v = band_imm x, Y`. (BinaryImm64)
	OpcodeBandImm

	// OpcodeBorImm ...
	// `v = bor_imm x, Y`. (BinaryImm64)
	OpcodeBorImm

	// OpcodeBxorImm ...
	// `v = bxor_imm x, Y`. (BinaryImm64)
	OpcodeBxorImm

	// OpcodeRotl ...
	// `v = rotl x, y`.
	OpcodeRotl

	// OpcodeRotr ...
	// `v = rotr x, y`.
	OpcodeRotr

	// OpcodeRotlImm ...
	// `v = rotl_imm x, Y`. (BinaryImm64)
	OpcodeRotlImm

	// OpcodeRotrImm ...
	// `v = rotr_imm x, Y`. (BinaryImm64)
	OpcodeRotrImm

	// OpcodeIshl ...
	// `v = ishl x, y`.
	OpcodeIshl

	// OpcodeUshr ...
	// `v = ushr x, y`.
	OpcodeUshr

	// OpcodeSshr ...
	// `v = sshr x, y`.
	OpcodeSshr

	// OpcodeIshlImm ...
	// `v = ishl_imm x, Y`. (BinaryImm64)
	OpcodeIshlImm

	// OpcodeUshrImm ...
	// `v = ushr_imm x, Y`. (BinaryImm64)
	OpcodeUshrImm

	// OpcodeSshrImm ...
	// `v = sshr_imm x, Y`. (BinaryImm64)
	OpcodeSshrImm

	// OpcodeBitrev ...
	// `v = bitrev x`.
	OpcodeBitrev

	// OpcodeClz ...
	// `v = clz x`.
	OpcodeClz

	// OpcodeCls ...
	// `v = cls x`.
	OpcodeCls

	// OpcodeCtz ...
	// `v = ctz x`.
	OpcodeCtz

	// OpcodeBswap ...
	// `v = bswap x`.
	OpcodeBswap

	// OpcodePopcnt ...
	// `v = popcnt x`.
	OpcodePopcnt

	// OpcodeFcmp compares two floating point values: `v = fcmp Cond, x, y`.
	OpcodeFcmp

	// OpcodeFadd performs an floating point addition.
	// `v = Fadd x, y`.
	OpcodeFadd

	// OpcodeFsub performs an floating point subtraction.
	// `v = Fsub x, y`.
	OpcodeFsub

	// OpcodeFmul ...
	// `v = fmul x, y`.
	OpcodeFmul

	// OpcodeFdiv ...
	// `v = fdiv x, y`.
	OpcodeFdiv

	// OpcodeSqrt ...
	// `v = sqrt x`.
	OpcodeSqrt

	// OpcodeFma ...
	// `v = fma x, y, z`.
	OpcodeFma

	// OpcodeFneg ...
	// `v = fneg x`.
	OpcodeFneg

	// OpcodeFabs ...
	// `v = fabs x`.
	OpcodeFabs

	// OpcodeFcopysign ...
	// `v = fcopysign x, y`.
	OpcodeFcopysign

	// OpcodeFmin ...
	// `v = fmin x, y`.
	OpcodeFmin

	// OpcodeFminPseudo ...
	// `v = fmin_pseudo x, y`.
	OpcodeFminPseudo

	// OpcodeFmax ...
	// `v = fmax x, y`.
	OpcodeFmax

	// OpcodeFmaxPseudo ...
	// `v = fmax_pseudo x, y`.
	OpcodeFmaxPseudo

	// OpcodeCeil ...
	// `v = ceil x`.
	OpcodeCeil

	// OpcodeFloor ...
	// `v = floor x`.
	OpcodeFloor

	// OpcodeTrunc ...
	// `v = trunc x`.
	OpcodeTrunc

	// OpcodeNearest ...
	// `v = nearest x`.
	OpcodeNearest

	// OpcodeIsNull ...
	// `v = is_null x`.
	OpcodeIsNull

	// OpcodeIsInvalid ...
	// `v = is_invalid x`.
	OpcodeIsInvalid

	// OpcodeBitcast ...
	// `v = bitcast MemFlags, x`.
	OpcodeBitcast

	// OpcodeScalarToVector ...
	// `v = scalar_to_vector s`.
	OpcodeScalarToVector

	// OpcodeBmask ...
	// `v = bmask x`.
	OpcodeBmask

	// OpcodeIreduce ...
	// `v = ireduce x`.
	OpcodeIreduce
	// `v = snarrow x, y`.

	// OpcodeSnarrow ...
	OpcodeSnarrow
	// `v = unarrow x, y`.

	// OpcodeUnarrow ...
	OpcodeUnarrow
	// `v = uunarrow x, y`.

	// OpcodeUunarrow ...
	OpcodeUunarrow
	// `v = swiden_low x`.

	// OpcodeSwidenLow ...
	OpcodeSwidenLow
	// `v = swiden_high x`.

	// OpcodeSwidenHigh ...
	OpcodeSwidenHigh
	// `v = uwiden_low x`.

	// OpcodeUwidenLow ...
	OpcodeUwidenLow
	// `v = uwiden_high x`.

	// OpcodeUwidenHigh ...
	OpcodeUwidenHigh
	// `v = iadd_pairwise x, y`.

	// OpcodeIaddPairwise ...
	OpcodeIaddPairwise

	// OpcodeWideningPairwiseDotProductS ...
	// `v = widening_pairwise_dot_product_s x, y`.
	OpcodeWideningPairwiseDotProductS

	// OpcodeUExtend zero-extends the given integer: `v = UExtend x, from->to`.
	OpcodeUExtend

	// OpcodeSExtend sign-extends the given integer: `v = SExtend x, from->to`.
	OpcodeSExtend

	// OpcodeFpromote ...
	// `v = fpromote x`.
	OpcodeFpromote

	// OpcodeFdemote ...
	// `v = fdemote x`.
	OpcodeFdemote

	// OpcodeFvdemote ...
	// `v = fvdemote x`.
	OpcodeFvdemote

	// OpcodeFvpromoteLow ...
	// `x = fvpromote_low a`.
	OpcodeFvpromoteLow

	// OpcodeFcvtToUint ...
	// `v = fcvt_to_uint x`.
	OpcodeFcvtToUint

	// OpcodeFcvtToSint ...
	// `v = fcvt_to_sint x`.
	OpcodeFcvtToSint

	// OpcodeFcvtToUintSat ...
	// `v = fcvt_to_uint_sat x`.
	OpcodeFcvtToUintSat

	// OpcodeFcvtToSintSat ...
	// `v = fcvt_to_sint_sat x`.
	OpcodeFcvtToSintSat

	// OpcodeFcvtFromUint ...
	// `v = fcvt_from_uint x`.
	OpcodeFcvtFromUint

	// OpcodeFcvtFromSint ...
	// `v = fcvt_from_sint x`.
	OpcodeFcvtFromSint

	// OpcodeFcvtLowFromSint ...
	// `v = fcvt_low_from_sint x`.
	OpcodeFcvtLowFromSint

	// OpcodeIsplit ...
	// `lo, hi = isplit x`.
	OpcodeIsplit

	// OpcodeIconcat ...
	// `v = iconcat lo, hi`.
	OpcodeIconcat

	// OpcodeAtomicRmw ...
	// `v = atomic_rmw MemFlags, AtomicRmwOp, p, x`.
	OpcodeAtomicRmw

	// OpcodeAtomicCas ...
	// `v = atomic_cas MemFlags, p, e, x`.
	OpcodeAtomicCas

	// OpcodeAtomicLoad ...
	// `v = atomic_load MemFlags, p`.
	OpcodeAtomicLoad

	// OpcodeAtomicStore ...
	// `atomic_store MemFlags, x, p`.
	OpcodeAtomicStore

	// OpcodeFence ...
	// `fence`.
	OpcodeFence

	// OpcodeExtractVector ...
	// `v = extract_vector x, y`. (BinaryImm8)
	OpcodeExtractVector

	// opcodeEnd marks the end of the opcode list.
	opcodeEnd
)

// returnTypesFn provides the info to determine the type of instruction.
// t1 is the type of the first result, ts are the types of the remaining results.
type returnTypesFn func(b *builder, instr *Instruction) (t1 Type, ts []Type)

var (
	returnTypesFnNoReturns returnTypesFn = func(b *builder, instr *Instruction) (t1 Type, ts []Type) { return typeInvalid, nil }
	returnTypesFnSingle                  = func(b *builder, instr *Instruction) (t1 Type, ts []Type) { return instr.typ, nil }
	returnTypesFnI32                     = func(b *builder, instr *Instruction) (t1 Type, ts []Type) { return TypeI32, nil }
	returnTypesFnF32                     = func(b *builder, instr *Instruction) (t1 Type, ts []Type) { return TypeF32, nil }
	returnTypesFnF64                     = func(b *builder, instr *Instruction) (t1 Type, ts []Type) { return TypeF64, nil }
)

type sideEffect int

const (
	sideEffectUnknown sideEffect = iota
	sideEffectTrue
	sideEffectFalse
)

// instructionSideEffects provides the info to determine if an instruction has side effects.
// Instructions with side effects must not be eliminated regardless whether the result is used or not.
var instructionSideEffects = [opcodeEnd]sideEffect{
	OpcodeUndefined:          sideEffectTrue,
	OpcodeJump:               sideEffectTrue,
	OpcodeIconst:             sideEffectFalse,
	OpcodeCall:               sideEffectTrue,
	OpcodeCallIndirect:       sideEffectTrue,
	OpcodeIadd:               sideEffectFalse,
	OpcodeImul:               sideEffectFalse,
	OpcodeIsub:               sideEffectFalse,
	OpcodeIcmp:               sideEffectFalse,
	OpcodeFcmp:               sideEffectFalse,
	OpcodeFadd:               sideEffectFalse,
	OpcodeClz:                sideEffectFalse,
	OpcodeCtz:                sideEffectFalse,
	OpcodeLoad:               sideEffectFalse,
	OpcodeUload8:             sideEffectFalse,
	OpcodeUload16:            sideEffectFalse,
	OpcodeUload32:            sideEffectFalse,
	OpcodeSload8:             sideEffectFalse,
	OpcodeSload16:            sideEffectFalse,
	OpcodeSload32:            sideEffectFalse,
	OpcodeSExtend:            sideEffectFalse,
	OpcodeUExtend:            sideEffectFalse,
	OpcodeFsub:               sideEffectFalse,
	OpcodeF32const:           sideEffectFalse,
	OpcodeF64const:           sideEffectFalse,
	OpcodeIshl:               sideEffectFalse,
	OpcodeSshr:               sideEffectFalse,
	OpcodeUshr:               sideEffectFalse,
	OpcodeStore:              sideEffectTrue,
	OpcodeIstore8:            sideEffectTrue,
	OpcodeIstore16:           sideEffectTrue,
	OpcodeIstore32:           sideEffectTrue,
	OpcodeExitWithCode:       sideEffectTrue,
	OpcodeExitIfTrueWithCode: sideEffectTrue,
	OpcodeReturn:             sideEffectTrue,
	OpcodeBrz:                sideEffectTrue,
	OpcodeBrnz:               sideEffectTrue,
	OpcodeFdiv:               sideEffectFalse,
	OpcodeFmul:               sideEffectFalse,
	OpcodeFmax:               sideEffectFalse,
	OpcodeSelect:             sideEffectFalse,
	OpcodeFmin:               sideEffectFalse,
}

// HasSideEffects returns true if this instruction has side effects.
func (i *Instruction) HasSideEffects() bool {
	if e := instructionSideEffects[i.opcode]; e == sideEffectUnknown {
		panic("BUG: side effect info not registered for " + i.opcode.String())
	} else {
		return e == sideEffectTrue
	}
}

// instructionReturnTypes provides the function to determine the return types of an instruction.
var instructionReturnTypes = [opcodeEnd]returnTypesFn{
	OpcodeIshl:      returnTypesFnSingle,
	OpcodeSshr:      returnTypesFnSingle,
	OpcodeUshr:      returnTypesFnSingle,
	OpcodeJump:      returnTypesFnNoReturns,
	OpcodeUndefined: returnTypesFnNoReturns,
	OpcodeIconst:    returnTypesFnSingle,
	OpcodeSelect:    returnTypesFnSingle,
	OpcodeSExtend:   returnTypesFnSingle,
	OpcodeUExtend:   returnTypesFnSingle,
	OpcodeCallIndirect: func(b *builder, instr *Instruction) (t1 Type, ts []Type) {
		sigID := SignatureID(instr.v)
		sig, ok := b.signatures[sigID]
		if !ok {
			panic("BUG")
		}
		switch len(sig.Results) {
		case 0:
			t1 = typeInvalid
		case 1:
			t1 = sig.Results[0]
		default:
			t1, ts = sig.Results[0], sig.Results[1:]
		}
		return
	},
	OpcodeCall: func(b *builder, instr *Instruction) (t1 Type, ts []Type) {
		sigID := SignatureID(instr.v)
		sig, ok := b.signatures[sigID]
		if !ok {
			panic("BUG")
		}
		switch len(sig.Results) {
		case 0:
			t1 = typeInvalid
		case 1:
			t1 = sig.Results[0]
		default:
			t1, ts = sig.Results[0], sig.Results[1:]
		}
		return
	},
	OpcodeLoad:               returnTypesFnSingle,
	OpcodeIadd:               returnTypesFnSingle,
	OpcodeIsub:               returnTypesFnSingle,
	OpcodeImul:               returnTypesFnSingle,
	OpcodeIcmp:               returnTypesFnI32,
	OpcodeFcmp:               returnTypesFnI32,
	OpcodeFadd:               returnTypesFnSingle,
	OpcodeFsub:               returnTypesFnSingle,
	OpcodeFdiv:               returnTypesFnSingle,
	OpcodeFmul:               returnTypesFnSingle,
	OpcodeFmax:               returnTypesFnSingle,
	OpcodeFmin:               returnTypesFnSingle,
	OpcodeF32const:           returnTypesFnF32,
	OpcodeF64const:           returnTypesFnF64,
	OpcodeClz:                returnTypesFnSingle,
	OpcodeCtz:                returnTypesFnSingle,
	OpcodeStore:              returnTypesFnNoReturns,
	OpcodeIstore8:            returnTypesFnNoReturns,
	OpcodeIstore16:           returnTypesFnNoReturns,
	OpcodeIstore32:           returnTypesFnNoReturns,
	OpcodeExitWithCode:       returnTypesFnNoReturns,
	OpcodeExitIfTrueWithCode: returnTypesFnNoReturns,
	OpcodeReturn:             returnTypesFnNoReturns,
	OpcodeBrz:                returnTypesFnNoReturns,
	OpcodeBrnz:               returnTypesFnNoReturns,
	OpcodeUload8:             returnTypesFnSingle,
	OpcodeUload16:            returnTypesFnSingle,
	OpcodeUload32:            returnTypesFnSingle,
	OpcodeSload8:             returnTypesFnSingle,
	OpcodeSload16:            returnTypesFnSingle,
	OpcodeSload32:            returnTypesFnSingle,
}

// AsLoad initializes this instruction as a store instruction with OpcodeLoad.
func (i *Instruction) AsLoad(ptr Value, offset uint32, typ Type) {
	i.opcode = OpcodeLoad
	i.v = ptr
	i.u64 = uint64(offset)
	i.typ = typ
}

// AsExtLoad initializes this instruction as a store instruction with OpcodeLoad.
func (i *Instruction) AsExtLoad(op Opcode, ptr Value, offset uint32, dst64bit bool) {
	i.opcode = op
	i.v = ptr
	i.u64 = uint64(offset)
	if dst64bit {
		i.typ = TypeI64
	} else {
		i.typ = TypeI32
	}
}

// LoadData returns the operands for a load instruction.
func (i *Instruction) LoadData() (ptr Value, offset uint32, typ Type) {
	return i.v, uint32(i.u64), i.typ
}

// AsStore initializes this instruction as a store instruction with OpcodeStore.
func (i *Instruction) AsStore(storeOp Opcode, value, ptr Value, offset uint32) {
	i.opcode = storeOp
	i.v = value
	i.v2 = ptr

	var dstSize uint64
	switch storeOp {
	case OpcodeStore:
		dstSize = uint64(value.Type().Bits())
	case OpcodeIstore8:
		dstSize = 8
	case OpcodeIstore16:
		dstSize = 16
	case OpcodeIstore32:
		dstSize = 32
	default:
		panic("invalid store opcode" + storeOp.String())
	}
	i.u64 = uint64(offset) | dstSize<<32
}

// StoreData returns the operands for a store instruction.
func (i *Instruction) StoreData() (value, ptr Value, offset uint32, storeSizeInBits byte) {
	return i.v, i.v2, uint32(i.u64), byte(i.u64 >> 32)
}

// AsIconst64 initializes this instruction as a 64-bit integer constant instruction with OpcodeIconst.
func (i *Instruction) AsIconst64(v uint64) {
	i.opcode = OpcodeIconst
	i.typ = TypeI64
	i.u64 = v
}

// AsIconst32 initializes this instruction as a 32-bit integer constant instruction with OpcodeIconst.
func (i *Instruction) AsIconst32(v uint32) {
	i.opcode = OpcodeIconst
	i.typ = TypeI32
	i.u64 = uint64(v)
}

// BinaryData return the operands for a binary instruction.
func (i *Instruction) BinaryData() (x, y Value) {
	return i.v, i.v2
}

// AsIadd initializes this instruction as an integer addition instruction with OpcodeIadd.
func (i *Instruction) AsIadd(x, y Value) {
	i.opcode = OpcodeIadd
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsImul initializes this instruction as an integer addition instruction with OpcodeImul.
func (i *Instruction) AsImul(x, y Value) {
	i.opcode = OpcodeImul
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsIsub initializes this instruction as an integer subtraction instruction with OpcodeIsub.
func (i *Instruction) AsIsub(x, y Value) {
	i.opcode = OpcodeIsub
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsIcmp initializes this instruction as an integer comparison instruction with OpcodeIcmp.
func (i *Instruction) AsIcmp(x, y Value, c IntegerCmpCond) {
	i.opcode = OpcodeIcmp
	i.v = x
	i.v2 = y
	i.u64 = uint64(c)
	i.typ = TypeI32
}

// AsFcmp initializes this instruction as an integer comparison instruction with OpcodeFcmp.
func (i *Instruction) AsFcmp(x, y Value, c FloatCmpCond) {
	i.opcode = OpcodeFcmp
	i.v = x
	i.v2 = y
	i.u64 = uint64(c)
	i.typ = TypeI32
}

// AsIshl initializes this instruction as an integer shift left instruction with OpcodeIshl.
func (i *Instruction) AsIshl(x, amount Value) {
	i.opcode = OpcodeIshl
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsUshr initializes this instruction as an integer unsigned shift right (logical shift right) instruction with OpcodeUshr.
func (i *Instruction) AsUshr(x, amount Value) {
	i.opcode = OpcodeUshr
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsSshr initializes this instruction as an integer signed shift right (arithmetic shift right) instruction with OpcodeSshr.
func (i *Instruction) AsSshr(x, amount Value) {
	i.opcode = OpcodeSshr
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// IcmpData returns the operands and comparison condition of this integer comparison instruction.
func (i *Instruction) IcmpData() (x, y Value, c IntegerCmpCond) {
	return i.v, i.v2, IntegerCmpCond(i.u64)
}

// FcmpData returns the operands and comparison condition of this floating-point comparison instruction.
func (i *Instruction) FcmpData() (x, y Value, c FloatCmpCond) {
	return i.v, i.v2, FloatCmpCond(i.u64)
}

// AsFadd initializes this instruction as a floating-point addition instruction with OpcodeFadd.
func (i *Instruction) AsFadd(x, y Value) {
	i.opcode = OpcodeFadd
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsFsub initializes this instruction as a floating-point subtraction instruction with OpcodeFsub.
func (i *Instruction) AsFsub(x, y Value) {
	i.opcode = OpcodeFsub
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsFmul initializes this instruction as a floating-point multiplication instruction with OpcodeFmul.
func (i *Instruction) AsFmul(x, y Value) {
	i.opcode = OpcodeFmul
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsFdiv initializes this instruction as a floating-point division instruction with OpcodeFdiv.
func (i *Instruction) AsFdiv(x, y Value) {
	i.opcode = OpcodeFdiv
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsFmin initializes this instruction to take the minimum of two floating-points with OpcodeFmin.
func (i *Instruction) AsFmin(x, y Value) {
	i.opcode = OpcodeFmin
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsFmax initializes this instruction to take the maximum of two floating-points with OpcodeFmax.
func (i *Instruction) AsFmax(x, y Value) {
	i.opcode = OpcodeFmax
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

// AsF32const initializes this instruction as a 32-bit floating-point constant instruction with OpcodeF32const.
func (i *Instruction) AsF32const(f float32) {
	i.opcode = OpcodeF32const
	i.typ = TypeF64
	i.u64 = uint64(math.Float32bits(f))
}

// AsF64const initializes this instruction as a 64-bit floating-point constant instruction with OpcodeF64const.
func (i *Instruction) AsF64const(f float64) {
	i.opcode = OpcodeF64const
	i.typ = TypeF64
	i.u64 = math.Float64bits(f)
}

// AsReturn initializes this instruction as a return instruction with OpcodeReturn.
func (i *Instruction) AsReturn(vs []Value) {
	i.opcode = OpcodeReturn
	i.vs = vs
}

// ReturnVals returns the return values of OpcodeReturn.
func (i *Instruction) ReturnVals() []Value {
	return i.vs
}

// AsExitWithCode initializes this instruction as a trap instruction with OpcodeExitWithCode.
func (i *Instruction) AsExitWithCode(ctx Value, code wazevoapi.ExitCode) {
	i.opcode = OpcodeExitWithCode
	i.v = ctx
	i.u64 = uint64(code)
}

// AsExitIfTrueWithCode initializes this instruction as a trap instruction with OpcodeExitIfTrueWithCode.
func (i *Instruction) AsExitIfTrueWithCode(ctx, c Value, code wazevoapi.ExitCode) {
	i.opcode = OpcodeExitIfTrueWithCode
	i.v = ctx
	i.v2 = c
	i.u64 = uint64(code)
}

// ExitWithCodeData returns the context and exit code of OpcodeExitWithCode.
func (i *Instruction) ExitWithCodeData() (ctx Value, code wazevoapi.ExitCode) {
	return i.v, wazevoapi.ExitCode(i.u64)
}

// ExitIfTrueWithCodeData returns the context and exit code of OpcodeExitWithCode.
func (i *Instruction) ExitIfTrueWithCodeData() (ctx, c Value, code wazevoapi.ExitCode) {
	return i.v, i.v2, wazevoapi.ExitCode(i.u64)
}

// InvertBrx inverts either OpcodeBrz or OpcodeBrnz to the other.
func (i *Instruction) InvertBrx() {
	switch i.opcode {
	case OpcodeBrz:
		i.opcode = OpcodeBrnz
	case OpcodeBrnz:
		i.opcode = OpcodeBrz
	default:
		panic("BUG")
	}
}

// BranchData returns the branch data for this instruction necessary for backends.
func (i *Instruction) BranchData() (condVal Value, blockArgs []Value, target BasicBlock) {
	switch i.opcode {
	case OpcodeJump:
		condVal = ValueInvalid
	case OpcodeBrz, OpcodeBrnz:
		condVal = i.v
	default:
		panic("BUG")
	}
	blockArgs = i.vs
	target = i.blk
	return
}

// AsJump initializes this instruction as a jump instruction with OpcodeJump.
func (i *Instruction) AsJump(vs []Value, target BasicBlock) {
	i.opcode = OpcodeJump
	i.vs = vs
	i.blk = target
}

// IsFallthroughJump returns true if this instruction is a fallthrough jump.
func (i *Instruction) IsFallthroughJump() bool {
	if i.opcode != OpcodeJump {
		panic("BUG: IsFallthrough only available for OpcodeJump")
	}
	return i.opcode == OpcodeJump && i.u64 != 0
}

// AsFallthroughJump marks this instruction as a fallthrough jump.
func (i *Instruction) AsFallthroughJump() {
	if i.opcode != OpcodeJump {
		panic("BUG: AsFallthroughJump only available for OpcodeJump")
	}
	i.u64 = 1
}

// AsBrz initializes this instruction as a branch-if-zero instruction with OpcodeBrz.
func (i *Instruction) AsBrz(v Value, args []Value, target BasicBlock) {
	i.opcode = OpcodeBrz
	i.v = v
	i.vs = args
	i.blk = target
}

// AsBrnz initializes this instruction as a branch-if-not-zero instruction with OpcodeBrnz.
func (i *Instruction) AsBrnz(v Value, args []Value, target BasicBlock) {
	i.opcode = OpcodeBrnz
	i.v = v
	i.vs = args
	i.blk = target
}

// AsCall initializes this instruction as a call instruction with OpcodeCall.
func (i *Instruction) AsCall(ref FuncRef, sig *Signature, args []Value) {
	i.opcode = OpcodeCall
	i.u64 = uint64(ref)
	i.vs = args
	i.v = Value(sig.ID)
	sig.used = true
}

// CallData returns the call data for this instruction necessary for backends.
func (i *Instruction) CallData() (ref FuncRef, sigID SignatureID, args []Value) {
	if i.opcode != OpcodeCall {
		panic("BUG: CallData only available for OpcodeCall")
	}
	ref = FuncRef(i.u64)
	sigID = SignatureID(i.v)
	args = i.vs
	return
}

// AsCallIndirect initializes this instruction as a call-indirect instruction with OpcodeCallIndirect.
func (i *Instruction) AsCallIndirect(funcPtr Value, sig *Signature, args []Value) {
	i.opcode = OpcodeCallIndirect
	i.typ = TypeF64
	i.vs = args
	i.v = Value(sig.ID)
	i.v2 = funcPtr
	sig.used = true
}

// CallIndirectData returns the call indirect data for this instruction necessary for backends.
func (i *Instruction) CallIndirectData() (funcPtr Value, sigID SignatureID, args []Value) {
	if i.opcode != OpcodeCallIndirect {
		panic("BUG: CallIndirectData only available for OpcodeCallIndirect")
	}
	funcPtr = i.v2
	sigID = SignatureID(i.v)
	args = i.vs
	return
}

// AsClz initializes this instruction as a Count Leading Zeroes instruction with OpcodeClz.
func (i *Instruction) AsClz(x Value) {
	i.opcode = OpcodeClz
	i.v = x
	i.typ = x.Type()
}

// AsCtz initializes this instruction as a Count Trailing Zeroes instruction with OpcodeCtz.
func (i *Instruction) AsCtz(x Value) {
	i.opcode = OpcodeCtz
	i.v = x
	i.typ = x.Type()
}

// AsPopcnt initializes this instruction as an Integer Population Count instruction with OpcodePopcnt.
func (i *Instruction) AsPopcnt(x Value) {
	i.opcode = OpcodePopcnt
	i.v = x
	i.typ = x.Type()
}

// UnaryData return the operand for a unary instruction.
func (i *Instruction) UnaryData() Value {
	return i.v
}

// AsSExtend initializes this instruction as a sign extension instruction with OpcodeSExtend.
func (i *Instruction) AsSExtend(v Value, from, to byte) {
	i.opcode = OpcodeSExtend
	i.v = v
	i.u64 = uint64(from)<<8 | uint64(to)
	if to == 64 {
		i.typ = TypeI64
	} else {
		i.typ = TypeI32
	}
}

// AsUExtend initializes this instruction as an unsigned extension instruction with OpcodeUExtend.
func (i *Instruction) AsUExtend(v Value, from, to byte) {
	i.opcode = OpcodeUExtend
	i.v = v
	i.u64 = uint64(from)<<8 | uint64(to)
	if to == 64 {
		i.typ = TypeI64
	} else {
		i.typ = TypeI32
	}
}

func (i *Instruction) ExtendData() (from, to byte, signed bool) {
	if i.opcode != OpcodeSExtend && i.opcode != OpcodeUExtend {
		panic("BUG: ExtendData only available for OpcodeSExtend and OpcodeUExtend")
	}
	from = byte(i.u64 >> 8)
	to = byte(i.u64)
	signed = i.opcode == OpcodeSExtend
	return
}

// AsSelect initializes this instruction as an unsigned extension instruction with OpcodeSelect.
func (i *Instruction) AsSelect(c, x, y Value) {
	i.opcode = OpcodeSelect
	i.v = c
	i.v2 = x
	i.u64 = uint64(y)
	i.typ = x.Type()
}

// SelectData returns the select data for this instruction necessary for backends.
func (i *Instruction) SelectData() (c, x, y Value) {
	c = i.v
	x = i.v2
	y = Value(i.u64)
	return
}

// ExtendFromToBits returns the from and to bit size for the extension instruction.
func (i *Instruction) ExtendFromToBits() (from, to byte) {
	from = byte(i.u64 >> 8)
	to = byte(i.u64)
	return
}

// Format returns a string representation of this instruction with the given builder.
// For debugging purposes only.
func (i *Instruction) Format(b Builder) string {
	var instSuffix string
	switch i.opcode {
	case OpcodeExitWithCode:
		instSuffix = fmt.Sprintf(" %s, %s", i.v.Format(b), wazevoapi.ExitCode(i.u64))
	case OpcodeExitIfTrueWithCode:
		instSuffix = fmt.Sprintf(" %s, %s, %s", i.v2.Format(b), i.v.Format(b), wazevoapi.ExitCode(i.u64))
	case OpcodeIadd, OpcodeIsub, OpcodeImul, OpcodeFadd, OpcodeFsub, OpcodeFmin, OpcodeFmax, OpcodeFdiv, OpcodeFmul:
		instSuffix = fmt.Sprintf(" %s, %s", i.v.Format(b), i.v2.Format(b))
	case OpcodeIcmp:
		instSuffix = fmt.Sprintf(" %s, %s, %s", IntegerCmpCond(i.u64), i.v.Format(b), i.v2.Format(b))
	case OpcodeFcmp:
		instSuffix = fmt.Sprintf(" %s, %s, %s", FloatCmpCond(i.u64), i.v.Format(b), i.v2.Format(b))
	case OpcodeSExtend, OpcodeUExtend:
		instSuffix = fmt.Sprintf(" %s, %d->%d", i.v.Format(b), i.u64>>8, i.u64&0xff)
	case OpcodeCall, OpcodeCallIndirect:
		vs := make([]string, len(i.vs))
		for idx := range vs {
			vs[idx] = i.vs[idx].Format(b)
		}
		if i.opcode == OpcodeCallIndirect {
			instSuffix = fmt.Sprintf(" %s:%s, %s", i.v2.Format(b), SignatureID(i.v), strings.Join(vs, ", "))
		} else {
			instSuffix = fmt.Sprintf(" %s:%s, %s", FuncRef(i.u64), SignatureID(i.v), strings.Join(vs, ", "))
		}
	case OpcodeStore, OpcodeIstore8, OpcodeIstore16, OpcodeIstore32:
		instSuffix = fmt.Sprintf(" %s, %s, %#x", i.v.Format(b), i.v2.Format(b), int32(i.u64))
	case OpcodeLoad:
		instSuffix = fmt.Sprintf(" %s, %#x", i.v.Format(b), int32(i.u64))
	case OpcodeUload8, OpcodeUload16, OpcodeUload32, OpcodeSload8, OpcodeSload16, OpcodeSload32:
		instSuffix = fmt.Sprintf(" %s, %#x", i.v.Format(b), int32(i.u64))
	case OpcodeSelect:
		instSuffix = fmt.Sprintf(" %s, %s, %s", i.v.Format(b), i.v2.Format(b), Value(i.u64).Format(b))
	case OpcodeIconst:
		switch i.typ {
		case TypeI32:
			instSuffix = fmt.Sprintf("_32 %#x", uint32(i.u64))
		case TypeI64:
			instSuffix = fmt.Sprintf("_64 %#x", i.u64)
		}
	case OpcodeF32const:
		instSuffix = fmt.Sprintf(" %f", math.Float32frombits(uint32(i.u64)))
	case OpcodeF64const:
		instSuffix = fmt.Sprintf(" %f", math.Float64frombits(i.u64))
	case OpcodeReturn:
		if len(i.vs) == 0 {
			break
		}
		vs := make([]string, len(i.vs))
		for idx := range vs {
			vs[idx] = i.vs[idx].Format(b)
		}
		instSuffix = fmt.Sprintf(" %s", strings.Join(vs, ", "))
	case OpcodeJump:
		vs := make([]string, len(i.vs)+1)
		if i.IsFallthroughJump() {
			vs[0] = " fallthrough"
		} else {
			vs[0] = " " + i.blk.(*basicBlock).Name()
		}
		for idx := range i.vs {
			vs[idx+1] = i.vs[idx].Format(b)
		}

		instSuffix = strings.Join(vs, ", ")
	case OpcodeBrz, OpcodeBrnz:
		vs := make([]string, len(i.vs)+2)
		vs[0] = " " + i.v.Format(b)
		vs[1] = i.blk.(*basicBlock).Name()
		for idx := range i.vs {
			vs[idx+2] = i.vs[idx].Format(b)
		}
		instSuffix = strings.Join(vs, ", ")
	case OpcodeIshl, OpcodeSshr, OpcodeUshr:
		instSuffix = fmt.Sprintf(" %s, %s", i.v.Format(b), i.v2.Format(b))
	case OpcodeUndefined:
	case OpcodeClz, OpcodeCtz, OpcodePopcnt:
		instSuffix = " " + i.v.Format(b)
	default:
		panic(fmt.Sprintf("TODO: format for %s", i.opcode))
	}

	instr := i.opcode.String() + instSuffix

	var rvs []string
	if rv := i.rValue; rv.Valid() {
		rvs = append(rvs, rv.formatWithType(b))
	}

	for _, v := range i.rValues {
		rvs = append(rvs, v.formatWithType(b))
	}

	if len(rvs) > 0 {
		return fmt.Sprintf("%s = %s", strings.Join(rvs, ", "), instr)
	} else {
		return instr
	}
}

// addArgumentBranchInst adds an argument to this instruction.
func (i *Instruction) addArgumentBranchInst(v Value) {
	switch i.opcode {
	case OpcodeJump, OpcodeBrz, OpcodeBrnz:
		i.vs = append(i.vs, v)
	default:
		panic("BUG: " + i.typ.String())
	}
}

// Constant returns true if this instruction is a constant instruction.
func (i *Instruction) Constant() bool {
	switch i.opcode {
	case OpcodeIconst, OpcodeF32const, OpcodeF64const:
		return true
	}
	return false
}

// ConstantVal returns the constant value of this instruction.
// How to interpret the return value depends on the opcode.
func (i *Instruction) ConstantVal() (ret uint64) {
	switch i.opcode {
	case OpcodeIconst, OpcodeF32const, OpcodeF64const:
		ret = i.u64
	default:
		panic("TODO")
	}
	return
}

// String implements fmt.Stringer.
func (o Opcode) String() (ret string) {
	switch o {
	case OpcodeInvalid:
		return "invalid"
	case OpcodeUndefined:
		return "Undefined"
	case OpcodeJump:
		return "Jump"
	case OpcodeBrz:
		return "Brz"
	case OpcodeBrnz:
		return "Brnz"
	case OpcodeBrTable:
		return "BrTable"
	case OpcodeExitWithCode:
		return "Exit"
	case OpcodeExitIfTrueWithCode:
		return "ExitIfTrue"
	case OpcodeReturn:
		return "Return"
	case OpcodeCall:
		return "Call"
	case OpcodeCallIndirect:
		return "CallIndirect"
	case OpcodeFuncAddr:
		return "FuncAddr"
	case OpcodeSplat:
		return "Splat"
	case OpcodeSwizzle:
		return "Swizzle"
	case OpcodeInsertlane:
		return "Insertlane"
	case OpcodeExtractlane:
		return "Extractlane"
	case OpcodeSmin:
		return "Smin"
	case OpcodeUmin:
		return "Umin"
	case OpcodeSmax:
		return "Smax"
	case OpcodeUmax:
		return "Umax"
	case OpcodeAvgRound:
		return "AvgRound"
	case OpcodeUaddSat:
		return "UaddSat"
	case OpcodeSaddSat:
		return "SaddSat"
	case OpcodeUsubSat:
		return "UsubSat"
	case OpcodeSsubSat:
		return "SsubSat"
	case OpcodeLoad:
		return "Load"
	case OpcodeStore:
		return "Store"
	case OpcodeUload8:
		return "Uload8"
	case OpcodeSload8:
		return "Sload8"
	case OpcodeIstore8:
		return "Istore8"
	case OpcodeUload16:
		return "Uload16"
	case OpcodeSload16:
		return "Sload16"
	case OpcodeIstore16:
		return "Istore16"
	case OpcodeUload32:
		return "Uload32"
	case OpcodeSload32:
		return "Sload32"
	case OpcodeIstore32:
		return "Istore32"
	case OpcodeUload8x8:
		return "Uload8x8"
	case OpcodeSload8x8:
		return "Sload8x8"
	case OpcodeUload16x4:
		return "Uload16x4"
	case OpcodeSload16x4:
		return "Sload16x4"
	case OpcodeUload32x2:
		return "Uload32x2"
	case OpcodeSload32x2:
		return "Sload32x2"
	case OpcodeGlobalValue:
		return "GlobalValue"
	case OpcodeSymbolValue:
		return "SymbolValue"
	case OpcodeHeapAddr:
		return "HeapAddr"
	case OpcodeHeapLoad:
		return "HeapLoad"
	case OpcodeHeapStore:
		return "HeapStore"
	case OpcodeGetReturnAddress:
		return "GetReturnAddress"
	case OpcodeTableAddr:
		return "TableAddr"
	case OpcodeIconst:
		return "Iconst"
	case OpcodeF32const:
		return "F32const"
	case OpcodeF64const:
		return "F64const"
	case OpcodeVconst:
		return "Vconst"
	case OpcodeShuffle:
		return "Shuffle"
	case OpcodeNull:
		return "Null"
	case OpcodeNop:
		return "Nop"
	case OpcodeSelect:
		return "Select"
	case OpcodeBitselect:
		return "Bitselect"
	case OpcodeVsplit:
		return "Vsplit"
	case OpcodeVconcat:
		return "Vconcat"
	case OpcodeVselect:
		return "Vselect"
	case OpcodeVanyTrue:
		return "VanyTrue"
	case OpcodeVallTrue:
		return "VallTrue"
	case OpcodeVhighBits:
		return "VhighBits"
	case OpcodeIcmp:
		return "Icmp"
	case OpcodeIcmpImm:
		return "IcmpImm"
	case OpcodeIadd:
		return "Iadd"
	case OpcodeIsub:
		return "Isub"
	case OpcodeIneg:
		return "Ineg"
	case OpcodeIabs:
		return "Iabs"
	case OpcodeImul:
		return "Imul"
	case OpcodeUmulhi:
		return "Umulhi"
	case OpcodeSmulhi:
		return "Smulhi"
	case OpcodeSqmulRoundSat:
		return "SqmulRoundSat"
	case OpcodeUdiv:
		return "Udiv"
	case OpcodeSdiv:
		return "Sdiv"
	case OpcodeUrem:
		return "Urem"
	case OpcodeSrem:
		return "Srem"
	case OpcodeIaddImm:
		return "IaddImm"
	case OpcodeImulImm:
		return "ImulImm"
	case OpcodeUdivImm:
		return "UdivImm"
	case OpcodeSdivImm:
		return "SdivImm"
	case OpcodeUremImm:
		return "UremImm"
	case OpcodeSremImm:
		return "SremImm"
	case OpcodeIrsubImm:
		return "IrsubImm"
	case OpcodeIaddCin:
		return "IaddCin"
	case OpcodeIaddIfcin:
		return "IaddIfcin"
	case OpcodeIaddCout:
		return "IaddCout"
	case OpcodeIaddIfcout:
		return "IaddIfcout"
	case OpcodeIaddCarry:
		return "IaddCarry"
	case OpcodeIaddIfcarry:
		return "IaddIfcarry"
	case OpcodeUaddOverflowTrap:
		return "UaddOverflowTrap"
	case OpcodeIsubBin:
		return "IsubBin"
	case OpcodeIsubIfbin:
		return "IsubIfbin"
	case OpcodeIsubBout:
		return "IsubBout"
	case OpcodeIsubIfbout:
		return "IsubIfbout"
	case OpcodeIsubBorrow:
		return "IsubBorrow"
	case OpcodeIsubIfborrow:
		return "IsubIfborrow"
	case OpcodeBand:
		return "Band"
	case OpcodeBor:
		return "Bor"
	case OpcodeBxor:
		return "Bxor"
	case OpcodeBnot:
		return "Bnot"
	case OpcodeBandNot:
		return "BandNot"
	case OpcodeBorNot:
		return "BorNot"
	case OpcodeBxorNot:
		return "BxorNot"
	case OpcodeBandImm:
		return "BandImm"
	case OpcodeBorImm:
		return "BorImm"
	case OpcodeBxorImm:
		return "BxorImm"
	case OpcodeRotl:
		return "Rotl"
	case OpcodeRotr:
		return "Rotr"
	case OpcodeRotlImm:
		return "RotlImm"
	case OpcodeRotrImm:
		return "RotrImm"
	case OpcodeIshl:
		return "Ishl"
	case OpcodeUshr:
		return "Ushr"
	case OpcodeSshr:
		return "Sshr"
	case OpcodeIshlImm:
		return "IshlImm"
	case OpcodeUshrImm:
		return "UshrImm"
	case OpcodeSshrImm:
		return "SshrImm"
	case OpcodeBitrev:
		return "Bitrev"
	case OpcodeClz:
		return "Clz"
	case OpcodeCls:
		return "Cls"
	case OpcodeCtz:
		return "Ctz"
	case OpcodeBswap:
		return "Bswap"
	case OpcodePopcnt:
		return "Popcnt"
	case OpcodeFcmp:
		return "Fcmp"
	case OpcodeFadd:
		return "Fadd"
	case OpcodeFsub:
		return "Fsub"
	case OpcodeFmul:
		return "Fmul"
	case OpcodeFdiv:
		return "Fdiv"
	case OpcodeSqrt:
		return "Sqrt"
	case OpcodeFma:
		return "Fma"
	case OpcodeFneg:
		return "Fneg"
	case OpcodeFabs:
		return "Fabs"
	case OpcodeFcopysign:
		return "Fcopysign"
	case OpcodeFmin:
		return "Fmin"
	case OpcodeFminPseudo:
		return "FminPseudo"
	case OpcodeFmax:
		return "Fmax"
	case OpcodeFmaxPseudo:
		return "FmaxPseudo"
	case OpcodeCeil:
		return "Ceil"
	case OpcodeFloor:
		return "Floor"
	case OpcodeTrunc:
		return "Trunc"
	case OpcodeNearest:
		return "Nearest"
	case OpcodeIsNull:
		return "IsNull"
	case OpcodeIsInvalid:
		return "IsInvalid"
	case OpcodeBitcast:
		return "Bitcast"
	case OpcodeScalarToVector:
		return "ScalarToVector"
	case OpcodeBmask:
		return "Bmask"
	case OpcodeIreduce:
		return "Ireduce"
	case OpcodeSnarrow:
		return "Snarrow"
	case OpcodeUnarrow:
		return "Unarrow"
	case OpcodeUunarrow:
		return "Uunarrow"
	case OpcodeSwidenLow:
		return "SwidenLow"
	case OpcodeSwidenHigh:
		return "SwidenHigh"
	case OpcodeUwidenLow:
		return "UwidenLow"
	case OpcodeUwidenHigh:
		return "UwidenHigh"
	case OpcodeIaddPairwise:
		return "IaddPairwise"
	case OpcodeWideningPairwiseDotProductS:
		return "WideningPairwiseDotProductS"
	case OpcodeUExtend:
		return "UExtend"
	case OpcodeSExtend:
		return "SExtend"
	case OpcodeFpromote:
		return "Fpromote"
	case OpcodeFdemote:
		return "Fdemote"
	case OpcodeFvdemote:
		return "Fvdemote"
	case OpcodeFvpromoteLow:
		return "FvpromoteLow"
	case OpcodeFcvtToUint:
		return "FcvtToUint"
	case OpcodeFcvtToSint:
		return "FcvtToSint"
	case OpcodeFcvtToUintSat:
		return "FcvtToUintSat"
	case OpcodeFcvtToSintSat:
		return "FcvtToSintSat"
	case OpcodeFcvtFromUint:
		return "FcvtFromUint"
	case OpcodeFcvtFromSint:
		return "FcvtFromSint"
	case OpcodeFcvtLowFromSint:
		return "FcvtLowFromSint"
	case OpcodeIsplit:
		return "Isplit"
	case OpcodeIconcat:
		return "Iconcat"
	case OpcodeAtomicRmw:
		return "AtomicRmw"
	case OpcodeAtomicCas:
		return "AtomicCas"
	case OpcodeAtomicLoad:
		return "AtomicLoad"
	case OpcodeAtomicStore:
		return "AtomicStore"
	case OpcodeFence:
		return "Fence"
	case OpcodeExtractVector:
		return "ExtractVector"
	}
	panic(fmt.Sprintf("unknown opcode %d", o))
}
