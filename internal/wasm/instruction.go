package wasm

// Opcode is the binary Opcode of an instruction. See also InstructionName
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
	OpcodeI32Load8S  Opcode = 0x2c
	OpcodeI32Load8U  Opcode = 0x2d
	OpcodeI32Load16S Opcode = 0x2e
	OpcodeI32Load16U Opcode = 0x2f
	OpcodeI64Load8S  Opcode = 0x30
	OpcodeI64Load8U  Opcode = 0x31
	OpcodeI64Load16S Opcode = 0x32
	OpcodeI64Load16U Opcode = 0x33
	OpcodeI64Load32S Opcode = 0x34
	OpcodeI64Load32U Opcode = 0x35
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

	OpcodeI32Eqz Opcode = 0x45
	OpcodeI32Eq  Opcode = 0x46
	OpcodeI32Ne  Opcode = 0x47
	OpcodeI32LtS Opcode = 0x48
	OpcodeI32LtU Opcode = 0x49
	OpcodeI32GtS Opcode = 0x4a
	OpcodeI32GtU Opcode = 0x4b
	OpcodeI32LeS Opcode = 0x4c
	OpcodeI32LeU Opcode = 0x4d
	OpcodeI32GeS Opcode = 0x4e
	OpcodeI32GeU Opcode = 0x4f

	OpcodeI64Eqz Opcode = 0x50
	OpcodeI64Eq  Opcode = 0x51
	OpcodeI64Ne  Opcode = 0x52
	OpcodeI64LtS Opcode = 0x53
	OpcodeI64LtU Opcode = 0x54
	OpcodeI64GtS Opcode = 0x55
	OpcodeI64GtU Opcode = 0x56
	OpcodeI64LeS Opcode = 0x57
	OpcodeI64LeU Opcode = 0x58
	OpcodeI64GeS Opcode = 0x59
	OpcodeI64GeU Opcode = 0x5a

	OpcodeF32Eq Opcode = 0x5b
	OpcodeF32Ne Opcode = 0x5c
	OpcodeF32Lt Opcode = 0x5d
	OpcodeF32Gt Opcode = 0x5e
	OpcodeF32Le Opcode = 0x5f
	OpcodeF32Ge Opcode = 0x60

	OpcodeF64Eq Opcode = 0x61
	OpcodeF64Ne Opcode = 0x62
	OpcodeF64Lt Opcode = 0x63
	OpcodeF64Gt Opcode = 0x64
	OpcodeF64Le Opcode = 0x65
	OpcodeF64Ge Opcode = 0x66

	OpcodeI32Clz    Opcode = 0x67
	OpcodeI32Ctz    Opcode = 0x68
	OpcodeI32Popcnt Opcode = 0x69
	OpcodeI32Add    Opcode = 0x6a
	OpcodeI32Sub    Opcode = 0x6b
	OpcodeI32Mul    Opcode = 0x6c
	OpcodeI32DivS   Opcode = 0x6d
	OpcodeI32DivU   Opcode = 0x6e
	OpcodeI32RemS   Opcode = 0x6f
	OpcodeI32RemU   Opcode = 0x70
	OpcodeI32And    Opcode = 0x71
	OpcodeI32Or     Opcode = 0x72
	OpcodeI32Xor    Opcode = 0x73
	OpcodeI32Shl    Opcode = 0x74
	OpcodeI32ShrS   Opcode = 0x75
	OpcodeI32ShrU   Opcode = 0x76
	OpcodeI32Rotl   Opcode = 0x77
	OpcodeI32Rotr   Opcode = 0x78

	OpcodeI64Clz    Opcode = 0x79
	OpcodeI64Ctz    Opcode = 0x7a
	OpcodeI64Popcnt Opcode = 0x7b
	OpcodeI64Add    Opcode = 0x7c
	OpcodeI64Sub    Opcode = 0x7d
	OpcodeI64Mul    Opcode = 0x7e
	OpcodeI64DivS   Opcode = 0x7f
	OpcodeI64DivU   Opcode = 0x80
	OpcodeI64RemS   Opcode = 0x81
	OpcodeI64RemU   Opcode = 0x82
	OpcodeI64And    Opcode = 0x83
	OpcodeI64Or     Opcode = 0x84
	OpcodeI64Xor    Opcode = 0x85
	OpcodeI64Shl    Opcode = 0x86
	OpcodeI64ShrS   Opcode = 0x87
	OpcodeI64ShrU   Opcode = 0x88
	OpcodeI64Rotl   Opcode = 0x89
	OpcodeI64Rotr   Opcode = 0x8a

	OpcodeF32Abs      Opcode = 0x8b
	OpcodeF32Neg      Opcode = 0x8c
	OpcodeF32Ceil     Opcode = 0x8d
	OpcodeF32Floor    Opcode = 0x8e
	OpcodeF32Trunc    Opcode = 0x8f
	OpcodeF32Nearest  Opcode = 0x90
	OpcodeF32Sqrt     Opcode = 0x91
	OpcodeF32Add      Opcode = 0x92
	OpcodeF32Sub      Opcode = 0x93
	OpcodeF32Mul      Opcode = 0x94
	OpcodeF32Div      Opcode = 0x95
	OpcodeF32Min      Opcode = 0x96
	OpcodeF32Max      Opcode = 0x97
	OpcodeF32Copysign Opcode = 0x98

	OpcodeF64Abs      Opcode = 0x99
	OpcodeF64Neg      Opcode = 0x9a
	OpcodeF64Ceil     Opcode = 0x9b
	OpcodeF64Floor    Opcode = 0x9c
	OpcodeF64Trunc    Opcode = 0x9d
	OpcodeF64Nearest  Opcode = 0x9e
	OpcodeF64Sqrt     Opcode = 0x9f
	OpcodeF64Add      Opcode = 0xa0
	OpcodeF64Sub      Opcode = 0xa1
	OpcodeF64Mul      Opcode = 0xa2
	OpcodeF64Div      Opcode = 0xa3
	OpcodeF64Min      Opcode = 0xa4
	OpcodeF64Max      Opcode = 0xa5
	OpcodeF64Copysign Opcode = 0xa6

	OpcodeI32WrapI64   Opcode = 0xa7
	OpcodeI32TruncF32S Opcode = 0xa8
	OpcodeI32TruncF32U Opcode = 0xa9
	OpcodeI32TruncF64S Opcode = 0xaa
	OpcodeI32TruncF64U Opcode = 0xab

	OpcodeI64ExtendI32S Opcode = 0xac
	OpcodeI64ExtendI32U Opcode = 0xad
	OpcodeI64TruncF32S  Opcode = 0xae
	OpcodeI64TruncF32U  Opcode = 0xaf
	OpcodeI64TruncF64S  Opcode = 0xb0
	OpcodeI64TruncF64U  Opcode = 0xb1

	OpcodeF32ConvertI32s Opcode = 0xb2
	OpcodeF32ConvertI32U Opcode = 0xb3
	OpcodeF32ConvertI64S Opcode = 0xb4
	OpcodeF32ConvertI64U Opcode = 0xb5
	OpcodeF32DemoteF64   Opcode = 0xb6

	OpcodeF64ConvertI32S Opcode = 0xb7
	OpcodeF64ConvertI32U Opcode = 0xb8
	OpcodeF64ConvertI64S Opcode = 0xb9
	OpcodeF64ConvertI64U Opcode = 0xba
	OpcodeF64PromoteF32  Opcode = 0xbb

	OpcodeI32ReinterpretF32 Opcode = 0xbc
	OpcodeI64ReinterpretF64 Opcode = 0xbd
	OpcodeF32ReinterpretI32 Opcode = 0xbe
	OpcodeF64ReinterpretI64 Opcode = 0xbf

	// Below are toggled with FeatureSignExtensionOps

	// OpcodeI32Extend8S extends a signed 8-bit integer to a 32-bit integer.
	// Note: This is dependent on the flag FeatureSignExtensionOps
	OpcodeI32Extend8S Opcode = 0xc0

	// OpcodeI32Extend16S extends a signed 16-bit integer to a 32-bit integer.
	// Note: This is dependent on the flag FeatureSignExtensionOps
	OpcodeI32Extend16S Opcode = 0xc1

	// OpcodeI64Extend8S extends a signed 8-bit integer to a 64-bit integer.
	// Note: This is dependent on the flag FeatureSignExtensionOps
	OpcodeI64Extend8S Opcode = 0xc2

	// OpcodeI64Extend16S extends a signed 16-bit integer to a 64-bit integer.
	// Note: This is dependent on the flag FeatureSignExtensionOps
	OpcodeI64Extend16S Opcode = 0xc3

	// OpcodeI64Extend32S extends a signed 32-bit integer to a 64-bit integer.
	// Note: This is dependent on the flag FeatureSignExtensionOps
	OpcodeI64Extend32S Opcode = 0xc4

	LastOpcode = OpcodeI64Extend32S
)

var instructionNames = [256]string{
	OpcodeUnreachable:       "unreachable",
	OpcodeNop:               "nop",
	OpcodeBlock:             "block",
	OpcodeLoop:              "loop",
	OpcodeIf:                "if",
	OpcodeElse:              "else",
	OpcodeEnd:               "end",
	OpcodeBr:                "br",
	OpcodeBrIf:              "br_if",
	OpcodeBrTable:           "br_table",
	OpcodeReturn:            "return",
	OpcodeCall:              "call",
	OpcodeCallIndirect:      "call_indirect",
	OpcodeDrop:              "drop",
	OpcodeSelect:            "select",
	OpcodeLocalGet:          "local.get",
	OpcodeLocalSet:          "local.set",
	OpcodeLocalTee:          "local.tee",
	OpcodeGlobalGet:         "global.get",
	OpcodeGlobalSet:         "global.set",
	OpcodeI32Load:           "i32.load",
	OpcodeI64Load:           "i64.load",
	OpcodeF32Load:           "f32.load",
	OpcodeF64Load:           "f64.load",
	OpcodeI32Load8S:         "i32.load8_s",
	OpcodeI32Load8U:         "i32.load8_u",
	OpcodeI32Load16S:        "i32.load16_s",
	OpcodeI32Load16U:        "i32.load16_u",
	OpcodeI64Load8S:         "i64.load8_s",
	OpcodeI64Load8U:         "i64.load8_u",
	OpcodeI64Load16S:        "i64.load16_s",
	OpcodeI64Load16U:        "i64.load16_u",
	OpcodeI64Load32S:        "i64.load32_s",
	OpcodeI64Load32U:        "i64.load32_u",
	OpcodeI32Store:          "i32.store",
	OpcodeI64Store:          "i64.store",
	OpcodeF32Store:          "f32.store",
	OpcodeF64Store:          "f64.store",
	OpcodeI32Store8:         "i32.store8",
	OpcodeI32Store16:        "i32.store16",
	OpcodeI64Store8:         "i64.store8",
	OpcodeI64Store16:        "i64.store16",
	OpcodeI64Store32:        "i64.store32",
	OpcodeMemorySize:        "memory.size",
	OpcodeMemoryGrow:        "memory.grow",
	OpcodeI32Const:          "i32.const",
	OpcodeI64Const:          "i64.const",
	OpcodeF32Const:          "f32.const",
	OpcodeF64Const:          "f64.const",
	OpcodeI32Eqz:            "i32.eqz",
	OpcodeI32Eq:             "i32.eq",
	OpcodeI32Ne:             "i32.ne",
	OpcodeI32LtS:            "i32.lt_s",
	OpcodeI32LtU:            "i32.lt_u",
	OpcodeI32GtS:            "i32.gt_s",
	OpcodeI32GtU:            "i32.gt_u",
	OpcodeI32LeS:            "i32.le_s",
	OpcodeI32LeU:            "i32.le_u",
	OpcodeI32GeS:            "i32.ge_s",
	OpcodeI32GeU:            "i32.ge_u",
	OpcodeI64Eqz:            "i64.eqz",
	OpcodeI64Eq:             "i64.eq",
	OpcodeI64Ne:             "i64.ne",
	OpcodeI64LtS:            "i64.lt_s",
	OpcodeI64LtU:            "i64.lt_u",
	OpcodeI64GtS:            "i64.gt_s",
	OpcodeI64GtU:            "i64.gt_u",
	OpcodeI64LeS:            "i64.le_s",
	OpcodeI64LeU:            "i64.le_u",
	OpcodeI64GeS:            "i64.ge_s",
	OpcodeI64GeU:            "i64.ge_u",
	OpcodeF32Eq:             "f32.eq",
	OpcodeF32Ne:             "f32.ne",
	OpcodeF32Lt:             "f32.lt",
	OpcodeF32Gt:             "f32.gt",
	OpcodeF32Le:             "f32.le",
	OpcodeF32Ge:             "f32.ge",
	OpcodeF64Eq:             "f64.eq",
	OpcodeF64Ne:             "f64.ne",
	OpcodeF64Lt:             "f64.lt",
	OpcodeF64Gt:             "f64.gt",
	OpcodeF64Le:             "f64.le",
	OpcodeF64Ge:             "f64.ge",
	OpcodeI32Clz:            "i32.clz",
	OpcodeI32Ctz:            "i32.ctz",
	OpcodeI32Popcnt:         "i32.popcnt",
	OpcodeI32Add:            "i32.add",
	OpcodeI32Sub:            "i32.sub",
	OpcodeI32Mul:            "i32.mul",
	OpcodeI32DivS:           "i32.div_s",
	OpcodeI32DivU:           "i32.div_u",
	OpcodeI32RemS:           "i32.rem_s",
	OpcodeI32RemU:           "i32.rem_u",
	OpcodeI32And:            "i32.and",
	OpcodeI32Or:             "i32.or",
	OpcodeI32Xor:            "i32.xor",
	OpcodeI32Shl:            "i32.shl",
	OpcodeI32ShrS:           "i32.shr_s",
	OpcodeI32ShrU:           "i32.shr_u",
	OpcodeI32Rotl:           "i32.rotl",
	OpcodeI32Rotr:           "i32.rotr",
	OpcodeI64Clz:            "i64.clz",
	OpcodeI64Ctz:            "i64.ctz",
	OpcodeI64Popcnt:         "i64.popcnt",
	OpcodeI64Add:            "i64.add",
	OpcodeI64Sub:            "i64.sub",
	OpcodeI64Mul:            "i64.mul",
	OpcodeI64DivS:           "i64.div_s",
	OpcodeI64DivU:           "i64.div_u",
	OpcodeI64RemS:           "i64.rem_s",
	OpcodeI64RemU:           "i64.rem_u",
	OpcodeI64And:            "i64.and",
	OpcodeI64Or:             "i64.or",
	OpcodeI64Xor:            "i64.xor",
	OpcodeI64Shl:            "i64.shl",
	OpcodeI64ShrS:           "i64.shr_s",
	OpcodeI64ShrU:           "i64.shr_u",
	OpcodeI64Rotl:           "i64.rotl",
	OpcodeI64Rotr:           "i64.rotr",
	OpcodeF32Abs:            "f32.abs",
	OpcodeF32Neg:            "f32.neg",
	OpcodeF32Ceil:           "f32.ceil",
	OpcodeF32Floor:          "f32.floor",
	OpcodeF32Trunc:          "f32.trunc",
	OpcodeF32Nearest:        "f32.nearest",
	OpcodeF32Sqrt:           "f32.sqrt",
	OpcodeF32Add:            "f32.add",
	OpcodeF32Sub:            "f32.sub",
	OpcodeF32Mul:            "f32.mul",
	OpcodeF32Div:            "f32.div",
	OpcodeF32Min:            "f32.min",
	OpcodeF32Max:            "f32.max",
	OpcodeF32Copysign:       "f32.copysign",
	OpcodeF64Abs:            "f64.abs",
	OpcodeF64Neg:            "f64.neg",
	OpcodeF64Ceil:           "f64.ceil",
	OpcodeF64Floor:          "f64.floor",
	OpcodeF64Trunc:          "f64.trunc",
	OpcodeF64Nearest:        "f64.nearest",
	OpcodeF64Sqrt:           "f64.sqrt",
	OpcodeF64Add:            "f64.add",
	OpcodeF64Sub:            "f64.sub",
	OpcodeF64Mul:            "f64.mul",
	OpcodeF64Div:            "f64.div",
	OpcodeF64Min:            "f64.min",
	OpcodeF64Max:            "f64.max",
	OpcodeF64Copysign:       "f64.copysign",
	OpcodeI32WrapI64:        "i32.wrap_i64",
	OpcodeI32TruncF32S:      "i32.trunc_f32_s",
	OpcodeI32TruncF32U:      "i32.trunc_f32_u",
	OpcodeI32TruncF64S:      "i32.trunc_f64_s",
	OpcodeI32TruncF64U:      "i32.trunc_f64_u",
	OpcodeI64ExtendI32S:     "i64.extend_i32_s",
	OpcodeI64ExtendI32U:     "i64.extend_i32_u",
	OpcodeI64TruncF32S:      "i64.trunc_f32_s",
	OpcodeI64TruncF32U:      "i64.trunc_f32_u",
	OpcodeI64TruncF64S:      "i64.trunc_f64_s",
	OpcodeI64TruncF64U:      "i64.trunc_f64_u",
	OpcodeF32ConvertI32s:    "f32.convert_i32_s",
	OpcodeF32ConvertI32U:    "f32.convert_i32_u",
	OpcodeF32ConvertI64S:    "f32.convert_i64_s",
	OpcodeF32ConvertI64U:    "f32.convert_i64u",
	OpcodeF32DemoteF64:      "f32.demote_f64",
	OpcodeF64ConvertI32S:    "f64.convert_i32_s",
	OpcodeF64ConvertI32U:    "f64.convert_i32_u",
	OpcodeF64ConvertI64S:    "f64.convert_i64_s",
	OpcodeF64ConvertI64U:    "f64.convert_i64_u",
	OpcodeF64PromoteF32:     "f64.promote_f32",
	OpcodeI32ReinterpretF32: "i32.reinterpret_f32",
	OpcodeI64ReinterpretF64: "i64.reinterpret_f64",
	OpcodeF32ReinterpretI32: "f32.reinterpret_i32",
	OpcodeF64ReinterpretI64: "f64.reinterpret_i64",

	// Below are toggled with FeatureSignExtensionOps
	OpcodeI32Extend8S:  "i32.extend8_s",
	OpcodeI32Extend16S: "i32.extend16_s",
	OpcodeI64Extend8S:  "i64.extend8_s",
	OpcodeI64Extend16S: "i64.extend16_s",
	OpcodeI64Extend32S: "i64.extend32_s",
}

// InstructionName returns the instruction corresponding to this binary Opcode.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#a7-index-of-instructions
func InstructionName(oc Opcode) string {
	return instructionNames[oc]
}
