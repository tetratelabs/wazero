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
	u1, u2     uint64
	v          Value
	v2         Value
	v3         Value
	vs         []Value
	typ        Type
	blk        BasicBlock
	targets    []BasicBlock
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
	i.v3 = ValueInvalid
	i.rValue = ValueInvalid
	i.typ = typeInvalid
	i.vs = nil
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
func (i *Instruction) Args() (v1, v2, v3 Value, vs []Value) {
	return i.v, i.v2, i.v3, i.vs
}

// Arg returns the first argument to this instruction.
func (i *Instruction) Arg() Value {
	return i.v
}

// Arg2 returns the first two arguments to this instruction.
func (i *Instruction) Arg2() (Value, Value) {
	return i.v, i.v2
}

// ArgWithLane returns the first argument to this instruction, and the lane type.
func (i *Instruction) ArgWithLane() (Value, VecLane) {
	return i.v, VecLane(i.u1)
}

// Arg2WithLane returns the first two arguments to this instruction, and the lane type.
func (i *Instruction) Arg2WithLane() (Value, Value, VecLane) {
	return i.v, i.v2, VecLane(i.u1)
}

// Arg3 returns the first three arguments to this instruction.
func (i *Instruction) Arg3() (Value, Value, Value) {
	return i.v, i.v2, i.v3
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

	// OpcodeBrTable takes the index value `index`, and branches into `labelX`. If the `index` is out of range,
	// it branches into the last labelN: `BrTable index, [label1, label2, ... labelN]`.
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

	// OpcodeIconst represents the integer const.
	OpcodeIconst

	// OpcodeF32const represents the single-precision const.
	OpcodeF32const

	// OpcodeF64const represents the double-precision const.
	OpcodeF64const

	// OpcodeVconst represents the 128bit vector const.
	OpcodeVconst

	// OpcodeVbor computes binary or between two 128bit vectors: `v = bor x, y`.
	OpcodeVbor

	// OpcodeVbxor computes binary xor between two 128bit vectors: `v = bxor x, y`.
	OpcodeVbxor

	// OpcodeVband computes binary and between two 128bit vectors: `v = band x, y`.
	OpcodeVband

	// OpcodeVbandnot computes binary and-not between two 128bit vectors: `v = bandnot x, y`.
	OpcodeVbandnot

	// OpcodeVbnot negates a 128bit vector: `v = bnot x`.
	OpcodeVbnot

	// OpcodeVbitselect uses the bits in the control mask c to select the corresponding bit from x when 1
	// and y when 0: `v = bitselect c, x, y`.
	OpcodeVbitselect

	// OpcodeShuffle ...
	// `v = shuffle a, b, mask`.
	OpcodeShuffle

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

	// OpcodeVIadd performs an integer addition: `v = VIadd.lane x, y` on vector.
	OpcodeVIadd

	// OpcodeVSaddSat performs a signed saturating vector addition: `v = VSaddSat.lane x, y` on vector.
	OpcodeVSaddSat

	// OpcodeVUaddSat performs an unsigned saturating vector addition: `v = VUaddSat.lane x, y` on vector.
	OpcodeVUaddSat

	// OpcodeIsub performs an integer subtraction: `v = Isub x, y`.
	OpcodeIsub

	// OpcodeVIsub performs an integer subtraction: `v = VIsub.lane x, y` on vector.
	OpcodeVIsub

	// OpcodeVSsubSat performs a signed saturating vector subtraction: `v = VSsubSat.lane x, y` on vector.
	OpcodeVSsubSat

	// OpcodeVUsubSat performs an unsigned saturating vector subtraction: `v = VUsubSat.lane x, y` on vector.
	OpcodeVUsubSat

	// OpcodeVImin performs a signed integer min: `v = VImin.lane x, y` on vector.
	OpcodeVImin

	// OpcodeVUmin performs an unsigned integer min: `v = VUmin.lane x, y` on vector.
	OpcodeVUmin

	// OpcodeVImax performs a signed integer max: `v = VImax.lane x, y` on vector.
	OpcodeVImax

	// OpcodeVUmax performs an unsigned integer max: `v = VUmax.lane x, y` on vector.
	OpcodeVUmax

	// OpcodeVAvgRound performs an unsigned integer avg, truncating to zero: `v = VAvgRound.lane x, y` on vector.
	OpcodeVAvgRound

	// OpcodeVImul performs an integer multiplication: `v = VImul.lane x, y` on vector.
	OpcodeVImul

	// OpcodeVIneg negates the given vector value: `v = VIneg x`.
	OpcodeVIneg

	// OpcodeVIpopcnt counts the number of 1-bits in the given vector: `v = VIpopcnt x`.
	OpcodeVIpopcnt

	// OpcodeVIabs returns the absolute value for the given vector value: `v = VIabs x`.
	OpcodeVIabs

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

	// OpcodeUdiv performs the unsigned integer division `v = Udiv x, y`.
	OpcodeUdiv

	// OpcodeSdiv performs the signed integer division `v = Sdiv x, y`.
	OpcodeSdiv

	// OpcodeUrem computes the remainder of the unsigned integer division `v = Urem x, y`.
	OpcodeUrem

	// OpcodeSrem computes the remainder of the signed integer division `v = Srem x, y`.
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

	// OpcodeRotl rotates the given integer value to the left: `v = Rotl x, y`.
	OpcodeRotl

	// OpcodeRotr rotates the given integer value to the right: `v = Rotr x, y`.
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

	// OpcodeClz counts the number of leading zeros: `v = clz x`.
	OpcodeClz

	// OpcodeCtz counts the number of trailing zeros: `v = ctz x`.
	OpcodeCtz

	// OpcodePopcnt counts the number of 1-bits: `v = popcnt x`.
	OpcodePopcnt

	// OpcodeFcmp compares two floating point values: `v = fcmp Cond, x, y`.
	OpcodeFcmp

	// OpcodeFadd performs a floating point addition: / `v = Fadd x, y`.
	OpcodeFadd

	// OpcodeFsub performs a floating point subtraction: `v = Fsub x, y`.
	OpcodeFsub

	// OpcodeFmul performs a floating point multiplication: `v = Fmul x, y`.
	OpcodeFmul

	// OpcodeFdiv performs a floating point division: `v = Fdiv x, y`.
	OpcodeFdiv

	// OpcodeSqrt takes the square root of the given floating point value: `v = sqrt x`.
	OpcodeSqrt

	// OpcodeFneg negates the given floating point value: `v = Fneg x`.
	OpcodeFneg

	// OpcodeFabs takes the absolute value of the given floating point value: `v = fabs x`.
	OpcodeFabs

	// OpcodeFcopysign ...
	// `v = fcopysign x, y`.
	OpcodeFcopysign

	// OpcodeFmin takes the minimum of two floating point values: `v = fmin x, y`.
	OpcodeFmin

	// OpcodeFmax takes the maximum of two floating point values: `v = fmax x, y`.
	OpcodeFmax

	// OpcodeCeil takes the ceiling of the given floating point value: `v = ceil x`.
	OpcodeCeil

	// OpcodeFloor takes the floor of the given floating point value: `v = floor x`.
	OpcodeFloor

	// OpcodeTrunc takes the truncation of the given floating point value: `v = trunc x`.
	OpcodeTrunc

	// OpcodeNearest takes the nearest integer of the given floating point value: `v = nearest x`.
	OpcodeNearest

	// OpcodeBitcast is a bitcast operation: `v = bitcast MemFlags, x`.
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

	// OpcodeFpromote promotes the given floating point value: `v = Fpromote x`.
	OpcodeFpromote

	// OpcodeFdemote demotes the given float point value: `v = Fdemote x`.
	OpcodeFdemote

	// OpcodeFvdemote ...
	// `v = fvdemote x`.
	OpcodeFvdemote

	// OpcodeFcvtToUint ...
	// `v = fcvt_to_uint x`.
	OpcodeFcvtToUint

	// OpcodeFcvtToSint converts a floating point value to a signed integer: `v = FcvtToSint x`.
	OpcodeFcvtToSint

	// OpcodeFcvtToUintSat converts a floating point value to an unsigned integer: `v = FcvtToUintSat x`.
	OpcodeFcvtToUintSat

	// OpcodeFcvtToSintSat ...
	// `v = fcvt_to_sint_sat x`.
	OpcodeFcvtToSintSat

	// OpcodeFcvtFromUint converts an unsigned integer to a floating point value: `v = FcvtFromUint x`.
	OpcodeFcvtFromUint

	// OpcodeFcvtFromSint converts a signed integer to a floating point value: `v = FcvtFromSint x`.
	OpcodeFcvtFromSint

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
	returnTypesFnV128                    = func(b *builder, instr *Instruction) (t1 Type, ts []Type) { return TypeV128, nil }
)

// sideEffect provides the info to determine if an instruction has side effects which
// is used to determine if it can be optimized out, interchanged with others, etc.
type sideEffect byte

const (
	sideEffectUnknown sideEffect = iota
	// sideEffectStrict represents an instruction with side effects, and should be always alive plus cannot be reordered.
	sideEffectStrict
	// sideEffectTraps represents an instruction that can trap, and should be always alive but can be reordered within the group.
	sideEffectTraps
	// sideEffectNone represents an instruction without side effects, and can be eliminated if the result is not used, plus can be reordered within the group.
	sideEffectNone
)

// instructionSideEffects provides the info to determine if an instruction has side effects.
// Instructions with side effects must not be eliminated regardless whether the result is used or not.
var instructionSideEffects = [opcodeEnd]sideEffect{
	OpcodeUndefined:          sideEffectStrict,
	OpcodeJump:               sideEffectStrict,
	OpcodeIconst:             sideEffectNone,
	OpcodeCall:               sideEffectStrict,
	OpcodeCallIndirect:       sideEffectStrict,
	OpcodeIadd:               sideEffectNone,
	OpcodeImul:               sideEffectNone,
	OpcodeIsub:               sideEffectNone,
	OpcodeIcmp:               sideEffectNone,
	OpcodeBand:               sideEffectNone,
	OpcodeBor:                sideEffectNone,
	OpcodeBxor:               sideEffectNone,
	OpcodeRotl:               sideEffectNone,
	OpcodeRotr:               sideEffectNone,
	OpcodeFcmp:               sideEffectNone,
	OpcodeFadd:               sideEffectNone,
	OpcodeClz:                sideEffectNone,
	OpcodeCtz:                sideEffectNone,
	OpcodePopcnt:             sideEffectNone,
	OpcodeLoad:               sideEffectNone,
	OpcodeUload8:             sideEffectNone,
	OpcodeUload16:            sideEffectNone,
	OpcodeUload32:            sideEffectNone,
	OpcodeSload8:             sideEffectNone,
	OpcodeSload16:            sideEffectNone,
	OpcodeSload32:            sideEffectNone,
	OpcodeSExtend:            sideEffectNone,
	OpcodeUExtend:            sideEffectNone,
	OpcodeFsub:               sideEffectNone,
	OpcodeF32const:           sideEffectNone,
	OpcodeF64const:           sideEffectNone,
	OpcodeIshl:               sideEffectNone,
	OpcodeSshr:               sideEffectNone,
	OpcodeUshr:               sideEffectNone,
	OpcodeStore:              sideEffectStrict,
	OpcodeIstore8:            sideEffectStrict,
	OpcodeIstore16:           sideEffectStrict,
	OpcodeIstore32:           sideEffectStrict,
	OpcodeExitWithCode:       sideEffectStrict,
	OpcodeExitIfTrueWithCode: sideEffectStrict,
	OpcodeReturn:             sideEffectStrict,
	OpcodeBrz:                sideEffectStrict,
	OpcodeBrnz:               sideEffectStrict,
	OpcodeBrTable:            sideEffectStrict,
	OpcodeFdiv:               sideEffectNone,
	OpcodeFmul:               sideEffectNone,
	OpcodeFmax:               sideEffectNone,
	OpcodeSelect:             sideEffectNone,
	OpcodeFmin:               sideEffectNone,
	OpcodeFneg:               sideEffectNone,
	OpcodeFcvtToSint:         sideEffectTraps,
	OpcodeFcvtToUint:         sideEffectTraps,
	OpcodeFcvtFromSint:       sideEffectNone,
	OpcodeFcvtFromUint:       sideEffectNone,
	OpcodeFcvtToSintSat:      sideEffectNone,
	OpcodeFcvtToUintSat:      sideEffectNone,
	OpcodeFdemote:            sideEffectNone,
	OpcodeFpromote:           sideEffectNone,
	OpcodeBitcast:            sideEffectNone,
	OpcodeIreduce:            sideEffectNone,
	OpcodeSqrt:               sideEffectNone,
	OpcodeCeil:               sideEffectNone,
	OpcodeFloor:              sideEffectNone,
	OpcodeTrunc:              sideEffectNone,
	OpcodeNearest:            sideEffectNone,
	OpcodeSdiv:               sideEffectTraps,
	OpcodeSrem:               sideEffectTraps,
	OpcodeUdiv:               sideEffectTraps,
	OpcodeUrem:               sideEffectTraps,
	OpcodeFabs:               sideEffectNone,
	OpcodeFcopysign:          sideEffectNone,
	OpcodeVconst:             sideEffectNone,
	OpcodeVbor:               sideEffectNone,
	OpcodeVbxor:              sideEffectNone,
	OpcodeVband:              sideEffectNone,
	OpcodeVbandnot:           sideEffectNone,
	OpcodeVbnot:              sideEffectNone,
	OpcodeVbitselect:         sideEffectStrict,
	OpcodeVanyTrue:           sideEffectNone,
	OpcodeVallTrue:           sideEffectNone,
	OpcodeVhighBits:          sideEffectNone,
	OpcodeVIadd:              sideEffectNone,
	OpcodeVSaddSat:           sideEffectNone,
	OpcodeVUaddSat:           sideEffectNone,
	OpcodeVIsub:              sideEffectNone,
	OpcodeVSsubSat:           sideEffectNone,
	OpcodeVUsubSat:           sideEffectNone,
	OpcodeVImin:              sideEffectNone,
	OpcodeVUmin:              sideEffectNone,
	OpcodeVImax:              sideEffectNone,
	OpcodeVUmax:              sideEffectNone,
	OpcodeVAvgRound:          sideEffectNone,
	OpcodeVImul:              sideEffectNone,
	OpcodeVIabs:              sideEffectNone,
	OpcodeVIneg:              sideEffectNone,
	OpcodeVIpopcnt:           sideEffectNone,
}

// sideEffect returns true if this instruction has side effects.
func (i *Instruction) sideEffect() sideEffect {
	if e := instructionSideEffects[i.opcode]; e == sideEffectUnknown {
		panic("BUG: side effect info not registered for " + i.opcode.String())
	} else {
		return e
	}
}

// instructionReturnTypes provides the function to determine the return types of an instruction.
var instructionReturnTypes = [opcodeEnd]returnTypesFn{
	OpcodeVbor:       returnTypesFnV128,
	OpcodeVbxor:      returnTypesFnV128,
	OpcodeVband:      returnTypesFnV128,
	OpcodeVbnot:      returnTypesFnV128,
	OpcodeVbandnot:   returnTypesFnV128,
	OpcodeVbitselect: returnTypesFnV128,
	OpcodeVanyTrue:   returnTypesFnV128,
	OpcodeVallTrue:   returnTypesFnV128,
	OpcodeVhighBits:  returnTypesFnV128,
	OpcodeVIadd:      returnTypesFnV128,
	OpcodeVSaddSat:   returnTypesFnV128,
	OpcodeVUaddSat:   returnTypesFnV128,
	OpcodeVIsub:      returnTypesFnV128,
	OpcodeVSsubSat:   returnTypesFnV128,
	OpcodeVUsubSat:   returnTypesFnV128,
	OpcodeVImin:      returnTypesFnV128,
	OpcodeVUmin:      returnTypesFnV128,
	OpcodeVImax:      returnTypesFnV128,
	OpcodeVUmax:      returnTypesFnV128,
	OpcodeVImul:      returnTypesFnV128,
	OpcodeVAvgRound:  returnTypesFnV128,
	OpcodeVIabs:      returnTypesFnV128,
	OpcodeVIneg:      returnTypesFnV128,
	OpcodeVIpopcnt:   returnTypesFnV128,
	OpcodeBand:       returnTypesFnSingle,
	OpcodeFcopysign:  returnTypesFnSingle,
	OpcodeBitcast:    returnTypesFnSingle,
	OpcodeBor:        returnTypesFnSingle,
	OpcodeBxor:       returnTypesFnSingle,
	OpcodeRotl:       returnTypesFnSingle,
	OpcodeRotr:       returnTypesFnSingle,
	OpcodeIshl:       returnTypesFnSingle,
	OpcodeSshr:       returnTypesFnSingle,
	OpcodeSdiv:       returnTypesFnSingle,
	OpcodeSrem:       returnTypesFnSingle,
	OpcodeUdiv:       returnTypesFnSingle,
	OpcodeUrem:       returnTypesFnSingle,
	OpcodeUshr:       returnTypesFnSingle,
	OpcodeJump:       returnTypesFnNoReturns,
	OpcodeUndefined:  returnTypesFnNoReturns,
	OpcodeIconst:     returnTypesFnSingle,
	OpcodeSelect:     returnTypesFnSingle,
	OpcodeSExtend:    returnTypesFnSingle,
	OpcodeUExtend:    returnTypesFnSingle,
	OpcodeIreduce:    returnTypesFnSingle,
	OpcodeFabs:       returnTypesFnSingle,
	OpcodeSqrt:       returnTypesFnSingle,
	OpcodeCeil:       returnTypesFnSingle,
	OpcodeFloor:      returnTypesFnSingle,
	OpcodeTrunc:      returnTypesFnSingle,
	OpcodeNearest:    returnTypesFnSingle,
	OpcodeCallIndirect: func(b *builder, instr *Instruction) (t1 Type, ts []Type) {
		sigID := SignatureID(instr.u1)
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
		sigID := SignatureID(instr.u2)
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
	OpcodePopcnt:             returnTypesFnSingle,
	OpcodeStore:              returnTypesFnNoReturns,
	OpcodeIstore8:            returnTypesFnNoReturns,
	OpcodeIstore16:           returnTypesFnNoReturns,
	OpcodeIstore32:           returnTypesFnNoReturns,
	OpcodeExitWithCode:       returnTypesFnNoReturns,
	OpcodeExitIfTrueWithCode: returnTypesFnNoReturns,
	OpcodeReturn:             returnTypesFnNoReturns,
	OpcodeBrz:                returnTypesFnNoReturns,
	OpcodeBrnz:               returnTypesFnNoReturns,
	OpcodeBrTable:            returnTypesFnNoReturns,
	OpcodeUload8:             returnTypesFnSingle,
	OpcodeUload16:            returnTypesFnSingle,
	OpcodeUload32:            returnTypesFnSingle,
	OpcodeSload8:             returnTypesFnSingle,
	OpcodeSload16:            returnTypesFnSingle,
	OpcodeSload32:            returnTypesFnSingle,
	OpcodeFcvtToSint:         returnTypesFnSingle,
	OpcodeFcvtToUint:         returnTypesFnSingle,
	OpcodeFcvtFromSint:       returnTypesFnSingle,
	OpcodeFcvtFromUint:       returnTypesFnSingle,
	OpcodeFcvtToSintSat:      returnTypesFnSingle,
	OpcodeFcvtToUintSat:      returnTypesFnSingle,
	OpcodeFneg:               returnTypesFnSingle,
	OpcodeFdemote:            returnTypesFnF32,
	OpcodeFpromote:           returnTypesFnF64,
	OpcodeVconst:             returnTypesFnV128,
}

// AsLoad initializes this instruction as a store instruction with OpcodeLoad.
func (i *Instruction) AsLoad(ptr Value, offset uint32, typ Type) *Instruction {
	i.opcode = OpcodeLoad
	i.v = ptr
	i.u1 = uint64(offset)
	i.typ = typ
	return i
}

// AsExtLoad initializes this instruction as a store instruction with OpcodeLoad.
func (i *Instruction) AsExtLoad(op Opcode, ptr Value, offset uint32, dst64bit bool) {
	i.opcode = op
	i.v = ptr
	i.u1 = uint64(offset)
	if dst64bit {
		i.typ = TypeI64
	} else {
		i.typ = TypeI32
	}
}

// AsSimdLoad initializes this instruction as a load instruction with OpcodeLoad 128 bit.
func (i *Instruction) AsSimdLoad(op Opcode, ptr Value, offset uint32) {
	i.opcode = op
	i.v = ptr
	i.u1 = uint64(offset)
	i.typ = TypeV128
}

// LoadData returns the operands for a load instruction.
func (i *Instruction) LoadData() (ptr Value, offset uint32, typ Type) {
	return i.v, uint32(i.u1), i.typ
}

// AsStore initializes this instruction as a store instruction with OpcodeStore.
func (i *Instruction) AsStore(storeOp Opcode, value, ptr Value, offset uint32) *Instruction {
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
	i.u1 = uint64(offset) | dstSize<<32
	return i
}

// StoreData returns the operands for a store instruction.
func (i *Instruction) StoreData() (value, ptr Value, offset uint32, storeSizeInBits byte) {
	return i.v, i.v2, uint32(i.u1), byte(i.u1 >> 32)
}

// AsIconst64 initializes this instruction as a 64-bit integer constant instruction with OpcodeIconst.
func (i *Instruction) AsIconst64(v uint64) *Instruction {
	i.opcode = OpcodeIconst
	i.typ = TypeI64
	i.u1 = v
	return i
}

// AsIconst32 initializes this instruction as a 32-bit integer constant instruction with OpcodeIconst.
func (i *Instruction) AsIconst32(v uint32) *Instruction {
	i.opcode = OpcodeIconst
	i.typ = TypeI32
	i.u1 = uint64(v)
	return i
}

// AsIadd initializes this instruction as an integer addition instruction with OpcodeIadd.
func (i *Instruction) AsIadd(x, y Value) *Instruction {
	i.opcode = OpcodeIadd
	i.v = x
	i.v2 = y
	i.typ = x.Type()
	return i
}

// AsVIadd initializes this instruction as an integer addition instruction with OpcodeVIadd on a vector.
func (i *Instruction) AsVIadd(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVIadd
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVSaddSat initializes this instruction as a vector addition with saturation instruction with OpcodeVSaddSat on a vector.
func (i *Instruction) AsVSaddSat(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVSaddSat
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVUaddSat initializes this instruction as a vector addition with saturation instruction with OpcodeVUaddSat on a vector.
func (i *Instruction) AsVUaddSat(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVUaddSat
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVIsub initializes this instruction as an integer subtraction instruction with OpcodeVIsub on a vector.
func (i *Instruction) AsVIsub(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVIsub
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVSsubSat initializes this instruction as a vector addition with saturation instruction with OpcodeVSsubSat on a vector.
func (i *Instruction) AsVSsubSat(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVSsubSat
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVUsubSat initializes this instruction as a vector addition with saturation instruction with OpcodeVUsubSat on a vector.
func (i *Instruction) AsVUsubSat(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVUsubSat
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVImin initializes this instruction as a signed integer min instruction with OpcodeVImin on a vector.
func (i *Instruction) AsVImin(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVImin
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVUmin initializes this instruction as an unsigned integer min instruction with OpcodeVUmin on a vector.
func (i *Instruction) AsVUmin(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVUmin
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVImax initializes this instruction as a signed integer max instruction with OpcodeVImax on a vector.
func (i *Instruction) AsVImax(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVImax
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVUmax initializes this instruction as an unsigned integer max instruction with OpcodeVUmax on a vector.
func (i *Instruction) AsVUmax(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVUmax
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVAvgRound initializes this instruction as an unsigned integer avg instruction, truncating to zero with OpcodeVAvgRound on a vector.
func (i *Instruction) AsVAvgRound(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVAvgRound
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVImul initializes this instruction as an integer subtraction multiplication with OpcodeVImul on a vector.
func (i *Instruction) AsVImul(x, y Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVImul
	i.v = x
	i.v2 = y
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVIabs initializes this instruction as a vector absolute value with OpcodeVIabs.
func (i *Instruction) AsVIabs(x Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVIabs
	i.v = x
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVIneg initializes this instruction as a vector negation with OpcodeVIneg.
func (i *Instruction) AsVIneg(x Value, lane VecLane) *Instruction {
	i.opcode = OpcodeVIneg
	i.v = x
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsVIpopcnt initializes this instruction as a Population Count instruction with OpcodeVIpopcnt on a vector.
func (i *Instruction) AsVIpopcnt(x Value, lane VecLane) *Instruction {
	if lane != VecLaneI8x16 {
		panic("Unsupported lane type " + lane.String())
	}
	i.opcode = OpcodeVIpopcnt
	i.v = x
	i.u1 = uint64(lane)
	i.typ = TypeV128
	return i
}

// AsImul initializes this instruction as an integer addition instruction with OpcodeImul.
func (i *Instruction) AsImul(x, y Value) {
	i.opcode = OpcodeImul
	i.v = x
	i.v2 = y
	i.typ = x.Type()
}

func (i *Instruction) Insert(b Builder) *Instruction {
	b.InsertInstruction(i)
	return i
}

// AsIsub initializes this instruction as an integer subtraction instruction with OpcodeIsub.
func (i *Instruction) AsIsub(x, y Value) *Instruction {
	i.opcode = OpcodeIsub
	i.v = x
	i.v2 = y
	i.typ = x.Type()
	return i
}

// AsIcmp initializes this instruction as an integer comparison instruction with OpcodeIcmp.
func (i *Instruction) AsIcmp(x, y Value, c IntegerCmpCond) *Instruction {
	i.opcode = OpcodeIcmp
	i.v = x
	i.v2 = y
	i.u1 = uint64(c)
	i.typ = TypeI32
	return i
}

// AsFcmp initializes this instruction as an integer comparison instruction with OpcodeFcmp.
func (i *Instruction) AsFcmp(x, y Value, c FloatCmpCond) {
	i.opcode = OpcodeFcmp
	i.v = x
	i.v2 = y
	i.u1 = uint64(c)
	i.typ = TypeI32
}

// AsSDiv initializes this instruction as an integer bitwise and instruction with OpcodeSdiv.
func (i *Instruction) AsSDiv(x, y, ctx Value) *Instruction {
	i.opcode = OpcodeSdiv
	i.v = x
	i.v2 = y
	i.v3 = ctx
	i.typ = x.Type()
	return i
}

// AsUDiv initializes this instruction as an integer bitwise and instruction with OpcodeUdiv.
func (i *Instruction) AsUDiv(x, y, ctx Value) *Instruction {
	i.opcode = OpcodeUdiv
	i.v = x
	i.v2 = y
	i.v3 = ctx
	i.typ = x.Type()
	return i
}

// AsSRem initializes this instruction as an integer bitwise and instruction with OpcodeSrem.
func (i *Instruction) AsSRem(x, y, ctx Value) *Instruction {
	i.opcode = OpcodeSrem
	i.v = x
	i.v2 = y
	i.v3 = ctx
	i.typ = x.Type()
	return i
}

// AsURem initializes this instruction as an integer bitwise and instruction with OpcodeUrem.
func (i *Instruction) AsURem(x, y, ctx Value) *Instruction {
	i.opcode = OpcodeUrem
	i.v = x
	i.v2 = y
	i.v3 = ctx
	i.typ = x.Type()
	return i
}

// AsBand initializes this instruction as an integer bitwise and instruction with OpcodeBand.
func (i *Instruction) AsBand(x, amount Value) {
	i.opcode = OpcodeBand
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsBor initializes this instruction as an integer bitwise or instruction with OpcodeBor.
func (i *Instruction) AsBor(x, amount Value) {
	i.opcode = OpcodeBor
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsBxor initializes this instruction as an integer bitwise xor instruction with OpcodeBxor.
func (i *Instruction) AsBxor(x, amount Value) {
	i.opcode = OpcodeBxor
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsIshl initializes this instruction as an integer shift left instruction with OpcodeIshl.
func (i *Instruction) AsIshl(x, amount Value) *Instruction {
	i.opcode = OpcodeIshl
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
	return i
}

// AsUshr initializes this instruction as an integer unsigned shift right (logical shift right) instruction with OpcodeUshr.
func (i *Instruction) AsUshr(x, amount Value) *Instruction {
	i.opcode = OpcodeUshr
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
	return i
}

// AsSshr initializes this instruction as an integer signed shift right (arithmetic shift right) instruction with OpcodeSshr.
func (i *Instruction) AsSshr(x, amount Value) {
	i.opcode = OpcodeSshr
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsRotl initializes this instruction as a word rotate left instruction with OpcodeRotl.
func (i *Instruction) AsRotl(x, amount Value) {
	i.opcode = OpcodeRotl
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// AsRotr initializes this instruction as a word rotate right instruction with OpcodeRotr.
func (i *Instruction) AsRotr(x, amount Value) {
	i.opcode = OpcodeRotr
	i.v = x
	i.v2 = amount
	i.typ = x.Type()
}

// IcmpData returns the operands and comparison condition of this integer comparison instruction.
func (i *Instruction) IcmpData() (x, y Value, c IntegerCmpCond) {
	return i.v, i.v2, IntegerCmpCond(i.u1)
}

// FcmpData returns the operands and comparison condition of this floating-point comparison instruction.
func (i *Instruction) FcmpData() (x, y Value, c FloatCmpCond) {
	return i.v, i.v2, FloatCmpCond(i.u1)
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
func (i *Instruction) AsF32const(f float32) *Instruction {
	i.opcode = OpcodeF32const
	i.typ = TypeF64
	i.u1 = uint64(math.Float32bits(f))
	return i
}

// AsF64const initializes this instruction as a 64-bit floating-point constant instruction with OpcodeF64const.
func (i *Instruction) AsF64const(f float64) *Instruction {
	i.opcode = OpcodeF64const
	i.typ = TypeF64
	i.u1 = math.Float64bits(f)
	return i
}

// AsVconst initializes this instruction as a vector constant instruction with OpcodeVconst.
func (i *Instruction) AsVconst(lo, hi uint64) *Instruction {
	i.opcode = OpcodeVconst
	i.typ = TypeV128
	i.u1 = lo
	i.u2 = hi
	return i
}

// AsVbnot initializes this instruction as a vector negation instruction with OpcodeVbnot.
func (i *Instruction) AsVbnot(v Value) *Instruction {
	i.opcode = OpcodeVbnot
	i.typ = TypeV128
	i.v = v
	return i
}

// AsVband initializes this instruction as an and vector instruction with OpcodeVband.
func (i *Instruction) AsVband(x, y Value) *Instruction {
	i.opcode = OpcodeVband
	i.typ = TypeV128
	i.v = x
	i.v2 = y
	return i
}

// AsVbor initializes this instruction as an or vector instruction with OpcodeVbor.
func (i *Instruction) AsVbor(x, y Value) *Instruction {
	i.opcode = OpcodeVbor
	i.typ = TypeV128
	i.v = x
	i.v2 = y
	return i
}

// AsVbxor initializes this instruction as a xor vector instruction with OpcodeVbxor.
func (i *Instruction) AsVbxor(x, y Value) *Instruction {
	i.opcode = OpcodeVbxor
	i.typ = TypeV128
	i.v = x
	i.v2 = y
	return i
}

// AsVbandnot initializes this instruction as an and-not vector instruction with OpcodeVbandnot.
func (i *Instruction) AsVbandnot(x, y Value) *Instruction {
	i.opcode = OpcodeVbandnot
	i.typ = TypeV128
	i.v = x
	i.v2 = y
	return i
}

// AsVbitselect initializes this instruction as a bit select vector instruction with OpcodeVbitselect.
func (i *Instruction) AsVbitselect(c, x, y Value) *Instruction {
	i.opcode = OpcodeVbitselect
	i.typ = TypeV128
	i.v = c
	i.v2 = x
	i.v3 = y
	return i
}

// VconstData returns the operands of this vector constant instruction.
func (i *Instruction) VconstData() (lo, hi uint64) {
	return i.u1, i.u2
}

// AsReturn initializes this instruction as a return instruction with OpcodeReturn.
func (i *Instruction) AsReturn(vs []Value) *Instruction {
	i.opcode = OpcodeReturn
	i.vs = vs
	return i
}

// AsIreduce initializes this instruction as a reduction instruction with OpcodeIreduce.
func (i *Instruction) AsIreduce(v Value, dstType Type) *Instruction {
	i.opcode = OpcodeIreduce
	i.v = v
	i.typ = dstType
	return i
}

// ReturnVals returns the return values of OpcodeReturn.
func (i *Instruction) ReturnVals() []Value {
	return i.vs
}

// AsExitWithCode initializes this instruction as a trap instruction with OpcodeExitWithCode.
func (i *Instruction) AsExitWithCode(ctx Value, code wazevoapi.ExitCode) {
	i.opcode = OpcodeExitWithCode
	i.v = ctx
	i.u1 = uint64(code)
}

// AsExitIfTrueWithCode initializes this instruction as a trap instruction with OpcodeExitIfTrueWithCode.
func (i *Instruction) AsExitIfTrueWithCode(ctx, c Value, code wazevoapi.ExitCode) {
	i.opcode = OpcodeExitIfTrueWithCode
	i.v = ctx
	i.v2 = c
	i.u1 = uint64(code)
}

// ExitWithCodeData returns the context and exit code of OpcodeExitWithCode.
func (i *Instruction) ExitWithCodeData() (ctx Value, code wazevoapi.ExitCode) {
	return i.v, wazevoapi.ExitCode(i.u1)
}

// ExitIfTrueWithCodeData returns the context and exit code of OpcodeExitWithCode.
func (i *Instruction) ExitIfTrueWithCodeData() (ctx, c Value, code wazevoapi.ExitCode) {
	return i.v, i.v2, wazevoapi.ExitCode(i.u1)
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

// BrTableData returns the branch table data for this instruction necessary for backends.
func (i *Instruction) BrTableData() (index Value, targets []BasicBlock) {
	if i.opcode != OpcodeBrTable {
		panic("BUG: BrTableData only available for OpcodeBrTable")
	}
	index = i.v
	targets = i.targets
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
	return i.opcode == OpcodeJump && i.u1 != 0
}

// AsFallthroughJump marks this instruction as a fallthrough jump.
func (i *Instruction) AsFallthroughJump() {
	if i.opcode != OpcodeJump {
		panic("BUG: AsFallthroughJump only available for OpcodeJump")
	}
	i.u1 = 1
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

// AsBrTable initializes this instruction as a branch-table instruction with OpcodeBrTable.
func (i *Instruction) AsBrTable(index Value, targets []BasicBlock) {
	i.opcode = OpcodeBrTable
	i.v = index
	i.targets = targets
}

// AsCall initializes this instruction as a call instruction with OpcodeCall.
func (i *Instruction) AsCall(ref FuncRef, sig *Signature, args []Value) {
	i.opcode = OpcodeCall
	i.u1 = uint64(ref)
	i.vs = args
	i.u2 = uint64(sig.ID)
	sig.used = true
}

// CallData returns the call data for this instruction necessary for backends.
func (i *Instruction) CallData() (ref FuncRef, sigID SignatureID, args []Value) {
	if i.opcode != OpcodeCall {
		panic("BUG: CallData only available for OpcodeCall")
	}
	ref = FuncRef(i.u1)
	sigID = SignatureID(i.u2)
	args = i.vs
	return
}

// AsCallIndirect initializes this instruction as a call-indirect instruction with OpcodeCallIndirect.
func (i *Instruction) AsCallIndirect(funcPtr Value, sig *Signature, args []Value) *Instruction {
	i.opcode = OpcodeCallIndirect
	i.typ = TypeF64
	i.vs = args
	i.v = funcPtr
	i.u1 = uint64(sig.ID)
	sig.used = true
	return i
}

// CallIndirectData returns the call indirect data for this instruction necessary for backends.
func (i *Instruction) CallIndirectData() (funcPtr Value, sigID SignatureID, args []Value) {
	if i.opcode != OpcodeCallIndirect {
		panic("BUG: CallIndirectData only available for OpcodeCallIndirect")
	}
	funcPtr = i.v
	sigID = SignatureID(i.u1)
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

// AsPopcnt initializes this instruction as a Population Count instruction with OpcodePopcnt.
func (i *Instruction) AsPopcnt(x Value) {
	i.opcode = OpcodePopcnt
	i.v = x
	i.typ = x.Type()
}

// AsFneg initializes this instruction as an instruction with OpcodeFneg.
func (i *Instruction) AsFneg(x Value) *Instruction {
	i.opcode = OpcodeFneg
	i.v = x
	i.typ = x.Type()
	return i
}

// AsSqrt initializes this instruction as an instruction with OpcodeSqrt.
func (i *Instruction) AsSqrt(x Value) *Instruction {
	i.opcode = OpcodeSqrt
	i.v = x
	i.typ = x.Type()
	return i
}

// AsFabs initializes this instruction as an instruction with OpcodeFabs.
func (i *Instruction) AsFabs(x Value) *Instruction {
	i.opcode = OpcodeFabs
	i.v = x
	i.typ = x.Type()
	return i
}

// AsFcopysign initializes this instruction as an instruction with OpcodeFcopysign.
func (i *Instruction) AsFcopysign(x, y Value) *Instruction {
	i.opcode = OpcodeFcopysign
	i.v = x
	i.v2 = y
	i.typ = x.Type()
	return i
}

// AsCeil initializes this instruction as an instruction with OpcodeCeil.
func (i *Instruction) AsCeil(x Value) *Instruction {
	i.opcode = OpcodeCeil
	i.v = x
	i.typ = x.Type()
	return i
}

// AsFloor initializes this instruction as an instruction with OpcodeFloor.
func (i *Instruction) AsFloor(x Value) *Instruction {
	i.opcode = OpcodeFloor
	i.v = x
	i.typ = x.Type()
	return i
}

// AsTrunc initializes this instruction as an instruction with OpcodeTrunc.
func (i *Instruction) AsTrunc(x Value) *Instruction {
	i.opcode = OpcodeTrunc
	i.v = x
	i.typ = x.Type()
	return i
}

// AsNearest initializes this instruction as an instruction with OpcodeNearest.
func (i *Instruction) AsNearest(x Value) *Instruction {
	i.opcode = OpcodeNearest
	i.v = x
	i.typ = x.Type()
	return i
}

// AsBitcast initializes this instruction as an instruction with OpcodeBitcast.
func (i *Instruction) AsBitcast(x Value, dstType Type) *Instruction {
	i.opcode = OpcodeBitcast
	i.v = x
	i.typ = dstType
	return i
}

// BitcastData returns the operands for a bitcast instruction.
func (i *Instruction) BitcastData() (x Value, dstType Type) {
	return i.v, i.typ
}

// AsFdemote initializes this instruction as an instruction with OpcodeFdemote.
func (i *Instruction) AsFdemote(x Value) {
	i.opcode = OpcodeFdemote
	i.v = x
	i.typ = TypeF32
}

// AsFpromote initializes this instruction as an instruction with OpcodeFpromote.
func (i *Instruction) AsFpromote(x Value) {
	i.opcode = OpcodeFpromote
	i.v = x
	i.typ = TypeF64
}

// AsFcvtFromInt initializes this instruction as an instruction with either OpcodeFcvtFromUint or OpcodeFcvtFromSint
func (i *Instruction) AsFcvtFromInt(x Value, signed bool, dst64bit bool) *Instruction {
	if signed {
		i.opcode = OpcodeFcvtFromSint
	} else {
		i.opcode = OpcodeFcvtFromUint
	}
	i.v = x
	if dst64bit {
		i.typ = TypeF64
	} else {
		i.typ = TypeF32
	}
	return i
}

// AsFcvtToInt initializes this instruction as an instruction with either OpcodeFcvtToUint or OpcodeFcvtToSint
func (i *Instruction) AsFcvtToInt(x, ctx Value, signed bool, dst64bit bool, sat bool) *Instruction {
	switch {
	case signed && !sat:
		i.opcode = OpcodeFcvtToSint
	case !signed && !sat:
		i.opcode = OpcodeFcvtToUint
	case signed && sat:
		i.opcode = OpcodeFcvtToSintSat
	case !signed && sat:
		i.opcode = OpcodeFcvtToUintSat
	}
	i.v = x
	i.v2 = ctx
	if dst64bit {
		i.typ = TypeI64
	} else {
		i.typ = TypeI32
	}
	return i
}

// AsSExtend initializes this instruction as a sign extension instruction with OpcodeSExtend.
func (i *Instruction) AsSExtend(v Value, from, to byte) {
	i.opcode = OpcodeSExtend
	i.v = v
	i.u1 = uint64(from)<<8 | uint64(to)
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
	i.u1 = uint64(from)<<8 | uint64(to)
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
	from = byte(i.u1 >> 8)
	to = byte(i.u1)
	signed = i.opcode == OpcodeSExtend
	return
}

// AsSelect initializes this instruction as an unsigned extension instruction with OpcodeSelect.
func (i *Instruction) AsSelect(c, x, y Value) *Instruction {
	i.opcode = OpcodeSelect
	i.v = c
	i.v2 = x
	i.v3 = y
	i.typ = x.Type()
	return i
}

// SelectData returns the select data for this instruction necessary for backends.
func (i *Instruction) SelectData() (c, x, y Value) {
	c = i.v
	x = i.v2
	y = i.v3
	return
}

// ExtendFromToBits returns the from and to bit size for the extension instruction.
func (i *Instruction) ExtendFromToBits() (from, to byte) {
	from = byte(i.u1 >> 8)
	to = byte(i.u1)
	return
}

// Format returns a string representation of this instruction with the given builder.
// For debugging purposes only.
func (i *Instruction) Format(b Builder) string {
	var instSuffix string
	switch i.opcode {
	case OpcodeExitWithCode:
		instSuffix = fmt.Sprintf(" %s, %s", i.v.Format(b), wazevoapi.ExitCode(i.u1))
	case OpcodeExitIfTrueWithCode:
		instSuffix = fmt.Sprintf(" %s, %s, %s", i.v2.Format(b), i.v.Format(b), wazevoapi.ExitCode(i.u1))
	case OpcodeIadd, OpcodeIsub, OpcodeImul, OpcodeFadd, OpcodeFsub, OpcodeFmin, OpcodeFmax, OpcodeFdiv, OpcodeFmul:
		instSuffix = fmt.Sprintf(" %s, %s", i.v.Format(b), i.v2.Format(b))
	case OpcodeIcmp:
		instSuffix = fmt.Sprintf(" %s, %s, %s", IntegerCmpCond(i.u1), i.v.Format(b), i.v2.Format(b))
	case OpcodeFcmp:
		instSuffix = fmt.Sprintf(" %s, %s, %s", FloatCmpCond(i.u1), i.v.Format(b), i.v2.Format(b))
	case OpcodeSExtend, OpcodeUExtend:
		instSuffix = fmt.Sprintf(" %s, %d->%d", i.v.Format(b), i.u1>>8, i.u1&0xff)
	case OpcodeCall, OpcodeCallIndirect:
		vs := make([]string, len(i.vs))
		for idx := range vs {
			vs[idx] = i.vs[idx].Format(b)
		}
		if i.opcode == OpcodeCallIndirect {
			instSuffix = fmt.Sprintf(" %s:%s, %s", i.v.Format(b), SignatureID(i.u1), strings.Join(vs, ", "))
		} else {
			instSuffix = fmt.Sprintf(" %s:%s, %s", FuncRef(i.u1), SignatureID(i.u2), strings.Join(vs, ", "))
		}
	case OpcodeStore, OpcodeIstore8, OpcodeIstore16, OpcodeIstore32:
		instSuffix = fmt.Sprintf(" %s, %s, %#x", i.v.Format(b), i.v2.Format(b), int32(i.u1))
	case OpcodeLoad:
		instSuffix = fmt.Sprintf(" %s, %#x", i.v.Format(b), int32(i.u1))
	case OpcodeUload8, OpcodeUload16, OpcodeUload32, OpcodeSload8, OpcodeSload16, OpcodeSload32:
		instSuffix = fmt.Sprintf(" %s, %#x", i.v.Format(b), int32(i.u1))
	case OpcodeSelect, OpcodeVbitselect:
		instSuffix = fmt.Sprintf(" %s, %s, %s", i.v.Format(b), i.v2.Format(b), i.v3.Format(b))
	case OpcodeIconst:
		switch i.typ {
		case TypeI32:
			instSuffix = fmt.Sprintf("_32 %#x", uint32(i.u1))
		case TypeI64:
			instSuffix = fmt.Sprintf("_64 %#x", i.u1)
		}
	case OpcodeVconst:
		instSuffix = fmt.Sprintf(" %016x %016x", i.u1, i.u2)
	case OpcodeF32const:
		instSuffix = fmt.Sprintf(" %f", math.Float32frombits(uint32(i.u1)))
	case OpcodeF64const:
		instSuffix = fmt.Sprintf(" %f", math.Float64frombits(i.u1))
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
	case OpcodeBrTable:
		// `BrTable index, [label1, label2, ... labelN]`
		instSuffix = fmt.Sprintf(" %s", i.v.Format(b))
		instSuffix += ", ["
		for i, target := range i.targets {
			blk := target.(*basicBlock)
			if i == 0 {
				instSuffix += blk.Name()
			} else {
				instSuffix += ", " + blk.Name()
			}
		}
		instSuffix += "]"
	case OpcodeBand, OpcodeBor, OpcodeBxor, OpcodeRotr, OpcodeRotl, OpcodeIshl, OpcodeSshr, OpcodeUshr,
		OpcodeSdiv, OpcodeUdiv, OpcodeFcopysign, OpcodeSrem, OpcodeUrem,
		OpcodeVbnot, OpcodeVbxor, OpcodeVbor, OpcodeVband, OpcodeVbandnot:
		instSuffix = fmt.Sprintf(" %s, %s", i.v.Format(b), i.v2.Format(b))
	case OpcodeUndefined:
	case OpcodeClz, OpcodeCtz, OpcodePopcnt, OpcodeFneg, OpcodeFcvtToSint, OpcodeFcvtToUint, OpcodeFcvtFromSint,
		OpcodeFcvtFromUint, OpcodeFcvtToSintSat, OpcodeFcvtToUintSat, OpcodeFdemote, OpcodeFpromote, OpcodeIreduce, OpcodeBitcast, OpcodeSqrt, OpcodeFabs,
		OpcodeCeil, OpcodeFloor, OpcodeTrunc, OpcodeNearest:
		instSuffix = " " + i.v.Format(b)
	case OpcodeVIadd, OpcodeVSaddSat, OpcodeVUaddSat, OpcodeVIsub, OpcodeVSsubSat, OpcodeVUsubSat,
		OpcodeVImin, OpcodeVUmin, OpcodeVImax, OpcodeVUmax, OpcodeVImul:
		instSuffix = fmt.Sprintf(".%s %s, %s", VecLane(i.u1), i.v.Format(b), i.v2.Format(b))
	case OpcodeVIabs, OpcodeVIneg, OpcodeVIpopcnt, OpcodeVhighBits, OpcodeVallTrue, OpcodeVanyTrue:
		instSuffix = fmt.Sprintf(".%s %s", VecLane(i.u1), i.v.Format(b))
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
		panic("BUG: " + i.opcode.String())
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
		ret = i.u1
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
	case OpcodeCtz:
		return "Ctz"
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
	case OpcodeFneg:
		return "Fneg"
	case OpcodeFabs:
		return "Fabs"
	case OpcodeFcopysign:
		return "Fcopysign"
	case OpcodeFmin:
		return "Fmin"
	case OpcodeFmax:
		return "Fmax"
	case OpcodeCeil:
		return "Ceil"
	case OpcodeFloor:
		return "Floor"
	case OpcodeTrunc:
		return "Trunc"
	case OpcodeNearest:
		return "Nearest"
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
	case OpcodeVbor:
		return "Vbor"
	case OpcodeVbxor:
		return "Vbxor"
	case OpcodeVband:
		return "Vband"
	case OpcodeVbandnot:
		return "Vbandnot"
	case OpcodeVbnot:
		return "Vbnot"
	case OpcodeVbitselect:
		return "Vbitselect"
	case OpcodeVIadd:
		return "VIadd"
	case OpcodeVSaddSat:
		return "VSaddSat"
	case OpcodeVUaddSat:
		return "VUaddSat"
	case OpcodeVSsubSat:
		return "VSsubSat"
	case OpcodeVUsubSat:
		return "VUsubSat"
	case OpcodeVIsub:
		return "VIsub"
	case OpcodeVImin:
		return "VImin"
	case OpcodeVUmin:
		return "VUmin"
	case OpcodeVImax:
		return "VImax"
	case OpcodeVUmax:
		return "VUmax"
	case OpcodeVImul:
		return "VImul"
	case OpcodeVIabs:
		return "VIabs"
	case OpcodeVIneg:
		return "VIneg"
	case OpcodeVIpopcnt:
		return "VIpopcnt"
	}
	panic(fmt.Sprintf("unknown opcode %d", o))
}
