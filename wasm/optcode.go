package wasm

// Opcode is a control instruction. See https://www.w3.org/TR/wasm-core-1/#control-instructions
type Opcode = byte

const (
	// OpcodeUnreachable causes an unconditional trap.
	OpcodeUnreachable Opcode = 0x00
	// OpcodeNop does nothing
	OpcodeNop Opcode = 0x01
	// OpcodeBlock brackets a sequence of instructions. A branch instruction on an if label breaks out to after its
	// OpcodeEnd.
	OpcodeBlock Opcode = 0x02
	// OpcodeLoop brackets a sequence of instructions. A branch instruction on a loop label will jump back to the
	// beginning of its block.
	OpcodeLoop Opcode = 0x03
	// OpcodeIf brackets a sequence of instructions. When the top of the stack evaluates to 1, the block is executed.
	// Zero jumps to the optional OpcodeElse. A branch instruction on an if label breaks out to after its OpcodeEnd.
	OpcodeIf Opcode = 0x04
	// OpcodeElse brackets a sequence of instructions enclosed by an OpcodeIf. A branch instruction on a then label
	// breaks out to after the OpcodeEnd on the enclosing OpcodeIf.
	OpcodeElse Opcode = 0x05
	// OpcodeEnd terminates a control instruction OpcodeBlock, OpcodeLoop or OpcodeIf.
	OpcodeEnd          Opcode = 0x0b
	OpcodeBr           Opcode = 0x0c
	OpcodeBrIf         Opcode = 0x0d
	OpcodeBrTable      Opcode = 0x0e
	OpcodeReturn       Opcode = 0x0f
	OpcodeCall         Opcode = 0x10
	OpcodeCallIndirect Opcode = 0x11

	// parametric instructions

	OpcodeDrop   Opcode = 0x1a
	OpcodeSelect Opcode = 0x1b

	// variable instructions

	OpcodeLocalGet  Opcode = 0x20
	OpcodeLocalSet  Opcode = 0x21
	OpcodeLocalTee  Opcode = 0x22
	OpcodeGlobalGet Opcode = 0x23
	OpcodeGlobalSet Opcode = 0x24

	// memory instructions

	OpcodeI32Load    Opcode = 0x28
	OpcodeI64Load    Opcode = 0x29
	OpcodeF32Load    Opcode = 0x2a
	OpcodeF64Load    Opcode = 0x2b
	OpcodeI32Load8s  Opcode = 0x2c
	OpcodeI32Load8u  Opcode = 0x2d
	OpcodeI32Load16s Opcode = 0x2e
	OpcodeI32Load16u Opcode = 0x2f
	OpcodeI64Load8s  Opcode = 0x30
	OpcodeI64Load8u  Opcode = 0x31
	OpcodeI64Load16s Opcode = 0x32
	OpcodeI64Load16u Opcode = 0x33
	OpcodeI64Load32s Opcode = 0x34
	OpcodeI64Load32u Opcode = 0x35
	OpcodeI32Store   Opcode = 0x36
	OpcodeI64Store   Opcode = 0x37
	OpcodeF32Store   Opcode = 0x38
	OpcodeF64Store   Opcode = 0x39
	OpcodeI32Store8  Opcode = 0x3a
	OpcodeI32Store16 Opcode = 0x3b
	OpcodeI64Store8  Opcode = 0x3c
	OpcodeI64Store16 Opcode = 0x3d
	OpcodeI64Store32 Opcode = 0x3e
	OpcodeMemorySize Opcode = 0x3f
	OpcodeMemoryGrow Opcode = 0x40

	// const instructions

	OpcodeI32Const Opcode = 0x41
	OpcodeI64Const Opcode = 0x42
	OpcodeF32Const Opcode = 0x43
	OpcodeF64Const Opcode = 0x44

	// numeric instructions

	OpcodeI32eqz Opcode = 0x45
	OpcodeI32eq  Opcode = 0x46
	OpcodeI32ne  Opcode = 0x47
	OpcodeI32lts Opcode = 0x48
	OpcodeI32ltu Opcode = 0x49
	OpcodeI32gts Opcode = 0x4a
	OpcodeI32gtu Opcode = 0x4b
	OpcodeI32les Opcode = 0x4c
	OpcodeI32leu Opcode = 0x4d
	OpcodeI32ges Opcode = 0x4e
	OpcodeI32geu Opcode = 0x4f

	OpcodeI64eqz Opcode = 0x50
	OpcodeI64eq  Opcode = 0x51
	OpcodeI64ne  Opcode = 0x52
	OpcodeI64lts Opcode = 0x53
	OpcodeI64ltu Opcode = 0x54
	OpcodeI64gts Opcode = 0x55
	OpcodeI64gtu Opcode = 0x56
	OpcodeI64les Opcode = 0x57
	OpcodeI64leu Opcode = 0x58
	OpcodeI64ges Opcode = 0x59
	OpcodeI64geu Opcode = 0x5a

	OpcodeF32eq Opcode = 0x5b
	OpcodeF32ne Opcode = 0x5c
	OpcodeF32lt Opcode = 0x5d
	OpcodeF32gt Opcode = 0x5e
	OpcodeF32le Opcode = 0x5f
	OpcodeF32ge Opcode = 0x60

	OpcodeF64eq Opcode = 0x61
	OpcodeF64ne Opcode = 0x62
	OpcodeF64lt Opcode = 0x63
	OpcodeF64gt Opcode = 0x64
	OpcodeF64le Opcode = 0x65
	OpcodeF64ge Opcode = 0x66

	OpcodeI32clz    Opcode = 0x67
	OpcodeI32ctz    Opcode = 0x68
	OpcodeI32popcnt Opcode = 0x69
	OpcodeI32add    Opcode = 0x6a
	OpcodeI32sub    Opcode = 0x6b
	OpcodeI32mul    Opcode = 0x6c
	OpcodeI32divs   Opcode = 0x6d
	OpcodeI32divu   Opcode = 0x6e
	OpcodeI32rems   Opcode = 0x6f
	OpcodeI32remu   Opcode = 0x70
	OpcodeI32and    Opcode = 0x71
	OpcodeI32or     Opcode = 0x72
	OpcodeI32xor    Opcode = 0x73
	OpcodeI32shl    Opcode = 0x74
	OpcodeI32shrs   Opcode = 0x75
	OpcodeI32shru   Opcode = 0x76
	OpcodeI32rotl   Opcode = 0x77
	OpcodeI32rotr   Opcode = 0x78

	OpcodeI64clz    Opcode = 0x79
	OpcodeI64ctz    Opcode = 0x7a
	OpcodeI64popcnt Opcode = 0x7b
	OpcodeI64add    Opcode = 0x7c
	OpcodeI64sub    Opcode = 0x7d
	OpcodeI64mul    Opcode = 0x7e
	OpcodeI64divs   Opcode = 0x7f
	OpcodeI64divu   Opcode = 0x80
	OpcodeI64rems   Opcode = 0x81
	OpcodeI64remu   Opcode = 0x82
	OpcodeI64and    Opcode = 0x83
	OpcodeI64or     Opcode = 0x84
	OpcodeI64xor    Opcode = 0x85
	OpcodeI64shl    Opcode = 0x86
	OpcodeI64shrs   Opcode = 0x87
	OpcodeI64shru   Opcode = 0x88
	OpcodeI64rotl   Opcode = 0x89
	OpcodeI64rotr   Opcode = 0x8a

	OpcodeF32abs      Opcode = 0x8b
	OpcodeF32neg      Opcode = 0x8c
	OpcodeF32ceil     Opcode = 0x8d
	OpcodeF32floor    Opcode = 0x8e
	OpcodeF32trunc    Opcode = 0x8f
	OpcodeF32nearest  Opcode = 0x90
	OpcodeF32sqrt     Opcode = 0x91
	OpcodeF32add      Opcode = 0x92
	OpcodeF32sub      Opcode = 0x93
	OpcodeF32mul      Opcode = 0x94
	OpcodeF32div      Opcode = 0x95
	OpcodeF32min      Opcode = 0x96
	OpcodeF32max      Opcode = 0x97
	OpcodeF32copysign Opcode = 0x98

	OpcodeF64abs      Opcode = 0x99
	OpcodeF64neg      Opcode = 0x9a
	OpcodeF64ceil     Opcode = 0x9b
	OpcodeF64floor    Opcode = 0x9c
	OpcodeF64trunc    Opcode = 0x9d
	OpcodeF64nearest  Opcode = 0x9e
	OpcodeF64sqrt     Opcode = 0x9f
	OpcodeF64add      Opcode = 0xa0
	OpcodeF64sub      Opcode = 0xa1
	OpcodeF64mul      Opcode = 0xa2
	OpcodeF64div      Opcode = 0xa3
	OpcodeF64min      Opcode = 0xa4
	OpcodeF64max      Opcode = 0xa5
	OpcodeF64copysign Opcode = 0xa6

	OpcodeI32wrapI64   Opcode = 0xa7
	OpcodeI32truncf32s Opcode = 0xa8
	OpcodeI32truncf32u Opcode = 0xa9
	OpcodeI32truncf64s Opcode = 0xaa
	OpcodeI32truncf64u Opcode = 0xab

	OpcodeI64Extendi32s Opcode = 0xac
	OpcodeI64Extendi32u Opcode = 0xad
	OpcodeI64TruncF32s  Opcode = 0xae
	OpcodeI64TruncF32u  Opcode = 0xaf
	OpcodeI64Truncf64s  Opcode = 0xb0
	OpcodeI64Truncf64u  Opcode = 0xb1

	OpcodeF32Converti32s Opcode = 0xb2
	OpcodeF32Converti32u Opcode = 0xb3
	OpcodeF32Converti64s Opcode = 0xb4
	OpcodeF32Converti64u Opcode = 0xb5
	OpcodeF32Demotef64   Opcode = 0xb6

	OpcodeF64Converti32s Opcode = 0xb7
	OpcodeF64Converti32u Opcode = 0xb8
	OpcodeF64Converti64s Opcode = 0xb9
	OpcodeF64Converti64u Opcode = 0xba
	OpcodeF64Promotef32  Opcode = 0xbb

	OpcodeI32Reinterpretf32 Opcode = 0xbc
	OpcodeI64Reinterpretf64 Opcode = 0xbd
	OpcodeF32Reinterpreti32 Opcode = 0xbe
	OpcodeF64Reinterpreti64 Opcode = 0xbf
)

// opcodeNames is index-coordinated with Opcode
var opcodeNames = [256]string{
	0x00: "Unreachable",
	0x01: "Nop",
	0x02: "Block",
	0x03: "Loop",
	0x04: "If",
	0x05: "Else",
	0x0b: "End",
	0x0c: "Br",
	0x0d: "BrIf",
	0x0e: "BrTable",
	0x0f: "Return",
	0x10: "Call",
	0x11: "CallIndirect",
	0x1a: "Drop",
	0x1b: "Select",
	0x20: "LocalGet",
	0x21: "LocalSet",
	0x22: "LocalTee",
	0x23: "GlobalGet",
	0x24: "GlobalSet",
	0x28: "I32Load",
	0x29: "I64Load",
	0x2a: "F32Load",
	0x2b: "F64Load",
	0x2c: "I32Load8s",
	0x2d: "I32Load8u",
	0x2e: "I32Load16s",
	0x2f: "I32Load16u",
	0x30: "I64Load8s",
	0x31: "I64Load8u",
	0x32: "I64Load16s",
	0x33: "I64Load16u",
	0x34: "I64Load32s",
	0x35: "I64Load32u",
	0x36: "I32Store",
	0x37: "I64Store",
	0x38: "F32Store",
	0x39: "F64Store",
	0x3a: "I32Store8 ",
	0x3b: "I32Store16",
	0x3c: "I64Store8 ",
	0x3d: "I64Store16",
	0x3e: "I64Store32",
	0x3f: "MemorySize",
	0x40: "MemoryGrow",
	0x41: "I32Const",
	0x42: "I64Const",
	0x43: "F32Const",
	0x44: "F64Const",
	0x45: "I32eqz",
	0x46: "I32eq",
	0x47: "I32ne",
	0x48: "I32lts",
	0x49: "I32ltu",
	0x4a: "I32gts",
	0x4b: "I32gtu",
	0x4c: "I32les",
	0x4d: "I32leu",
	0x4e: "I32ges",
	0x4f: "I32geu",
	0x50: "I64eqz",
	0x51: "I64eq",
	0x52: "I64ne",
	0x53: "I64lts",
	0x54: "I64ltu",
	0x55: "I64gts",
	0x56: "I64gtu",
	0x57: "I64les",
	0x58: "I64leu",
	0x59: "I64ges",
	0x5a: "I64geu",
	0x5b: "F32eq",
	0x5c: "F32ne",
	0x5d: "F32lt",
	0x5e: "F32gt",
	0x5f: "F32le",
	0x60: "F32ge",
	0x61: "F64eq",
	0x62: "F64ne",
	0x63: "F64lt",
	0x64: "F64gt",
	0x65: "F64le",
	0x66: "F64ge",
	0x67: "I32clz",
	0x68: "I32ctz",
	0x69: "I32popcnt",
	0x6a: "I32add",
	0x6b: "I32sub",
	0x6c: "I32mul",
	0x6d: "I32divs",
	0x6e: "I32divu",
	0x6f: "I32rems",
	0x70: "I32remu",
	0x71: "I32and",
	0x72: "I32or",
	0x73: "I32xor",
	0x74: "I32shl",
	0x75: "I32shrs",
	0x76: "I32shru",
	0x77: "I32rotl",
	0x78: "I32rotr",
	0x79: "I64clz",
	0x7a: "I64ctz",
	0x7b: "I64popcnt",
	0x7c: "I64add",
	0x7d: "I64sub",
	0x7e: "I64mul",
	0x7f: "I64divs",
	0x80: "I64divu",
	0x81: "I64rems",
	0x82: "I64remu",
	0x83: "I64and",
	0x84: "I64or",
	0x85: "I64xor",
	0x86: "I64shl",
	0x87: "I64shrs",
	0x88: "I64shru",
	0x89: "I64rotl",
	0x8a: "I64rotr",
	0x8b: "F32abs",
	0x8c: "F32neg",
	0x8d: "F32ceil",
	0x8e: "F32floor",
	0x8f: "F32trunc",
	0x90: "F32nearest",
	0x91: "F32sqrt",
	0x92: "F32add",
	0x93: "F32sub",
	0x94: "F32mul",
	0x95: "F32div",
	0x96: "F32min",
	0x97: "F32max",
	0x98: "F32copysign",
	0x99: "F64abs",
	0x9a: "F64neg",
	0x9b: "F64ceil",
	0x9c: "F64floor",
	0x9d: "F64trunc",
	0x9e: "F64nearest",
	0x9f: "F64sqrt",
	0xa0: "F64add",
	0xa1: "F64sub",
	0xa2: "F64mul",
	0xa3: "F64div",
	0xa4: "F64min",
	0xa5: "F64max",
	0xa6: "F64copysign",
	0xa7: "I32wrapI64",
	0xa8: "I32truncf32s",
	0xa9: "I32truncf32u",
	0xaa: "I32truncf64s",
	0xab: "I32truncf64u",
	0xac: "I64Extendi32s",
	0xad: "I64Extendi32u",
	0xae: "I64TruncF32s ",
	0xaf: "I64TruncF32u ",
	0xb0: "I64Truncf64s ",
	0xb1: "I64Truncf64u ",
	0xb2: "F32Converti32s",
	0xb3: "F32Converti32u",
	0xb4: "F32Converti64s",
	0xb5: "F32Converti64u",
	0xb6: "F32Demotef64  ",
	0xb7: "F64Converti32s",
	0xb8: "F64Converti32u",
	0xb9: "F64Converti64s",
	0xba: "F64Converti64u",
	0xbb: "F64Promotef32 ",
	0xbc: "I32reinterpretf32",
	0xbd: "I64reinterpretf64",
	0xbe: "F32reinterpreti32",
	0xbf: "F64reinterpreti64",
}

// OpcodeName returns the string name of this opcode.
func OpcodeName(oc Opcode) string {
	return opcodeNames[oc]
}
