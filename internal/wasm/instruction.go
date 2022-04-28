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
	OpcodeEnd Opcode = 0x0b

	// OpcodeBr is a stack-polymorphic opcode that performs an unconditional branch. How the stack is modified depends
	// on whether the "br" is enclosed by a loop, and if FeatureMultiValue is enabled.
	//
	// Here are the rules in pseudocode about how the stack is modified based on the "br" operand L (label):
	//	if L is loop: append(L.originalStackWithoutInputs, N-values popped from the stack) where N == L.inputs
	//	else: append(L.originalStackWithoutInputs, N-values popped from the stack) where N == L.results
	//
	// In WebAssembly 1.0 (20191205), N can be zero or one. When FeatureMultiValue is enabled, N can be more than one,
	// depending on the type use of the label L.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefsyntax-instr-controlmathsfbrl
	OpcodeBr Opcode = 0x0c
	// ^^ TODO: Add a diagram to help explain br l means that branch into AFTER l for non-loop labels

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

	// OpcodeRefNull pushes a null reference value whose type is specified by immediate to this opcode.
	// This is defined in the reference-types proposal, but necessary for FeatureBulkMemoryOperations as well.
	//
	// Currently only supported in the constant expression in element segments.
	OpcodeRefNull = 0xd0
	// OpcodeRefIsNull pops a reference value, and pushes 1 if it is null, 0 otherwise.
	// This is defined in the reference-types proposal, but necessary for FeatureBulkMemoryOperations as well.
	//
	// Currently not supported.
	OpcodeRefIsNull = 0xd1
	// OpcodeRefFunc pushes a funcref value whose index equals the immediate to this opcode.
	// This is defined in the reference-types proposal, but necessary for FeatureBulkMemoryOperations as well.
	//
	// Currently only supported in the constant expression in element segments.
	OpcodeRefFunc = 0xd2

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

	// OpcodeMiscPrefix is the prefix of various multi-byte opcodes.
	// Introduced in FeatureNonTrappingFloatToIntConversion, but used in other
	// features, such as FeatureBulkMemoryOperations.
	OpcodeMiscPrefix Opcode = 0xfc
)

// OpcodeMisc represents opcodes of the miscellaneous operations.
// Such an operations has multi-byte encoding which is prefixed by OpcodeMiscPrefix.
type OpcodeMisc = byte

const (
	// Below are toggled with FeatureNonTrappingFloatToIntConversion.
	// https://github.com/WebAssembly/spec/blob/ce4b6c4d47eb06098cc7ab2e81f24748da822f20/proposals/nontrapping-float-to-int-conversion/Overview.md

	OpcodeMiscI32TruncSatF32S OpcodeMisc = 0x00
	OpcodeMiscI32TruncSatF32U OpcodeMisc = 0x01
	OpcodeMiscI32TruncSatF64S OpcodeMisc = 0x02
	OpcodeMiscI32TruncSatF64U OpcodeMisc = 0x03
	OpcodeMiscI64TruncSatF32S OpcodeMisc = 0x04
	OpcodeMiscI64TruncSatF32U OpcodeMisc = 0x05
	OpcodeMiscI64TruncSatF64S OpcodeMisc = 0x06
	OpcodeMiscI64TruncSatF64U OpcodeMisc = 0x07

	// Below are toggled with FeatureBulkMemoryOperations.
	// Opcodes are those new in document/core/appendix/index-instructions.rst (the commit that merged the feature).
	// See https://github.com/WebAssembly/spec/commit/7fa2f20a6df4cf1c114582c8cb60f5bfcdbf1be1
	// See https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/appendix/changes.html#bulk-memory-and-table-instructions

	OpcodeMiscMemoryInit OpcodeMisc = 0x08
	OpcodeMiscDataDrop   OpcodeMisc = 0x09
	OpcodeMiscMemoryCopy OpcodeMisc = 0x0a
	OpcodeMiscMemoryFill OpcodeMisc = 0x0b
	OpcodeMiscTableInit  OpcodeMisc = 0x0c
	OpcodeMiscElemDrop   OpcodeMisc = 0x0d
	OpcodeMiscTableCopy  OpcodeMisc = 0x0e
	OpcodeMiscTableGrow  OpcodeMisc = 0x0f
	OpcodeMiscTableSize  OpcodeMisc = 0x10
	OpcodeMiscTableFill  OpcodeMisc = 0x11
)

const (
	OpcodeUnreachableName       = "unreachable"
	OpcodeNopName               = "nop"
	OpcodeBlockName             = "block"
	OpcodeLoopName              = "loop"
	OpcodeIfName                = "if"
	OpcodeElseName              = "else"
	OpcodeEndName               = "end"
	OpcodeBrName                = "br"
	OpcodeBrIfName              = "br_if"
	OpcodeBrTableName           = "br_table"
	OpcodeReturnName            = "return"
	OpcodeCallName              = "call"
	OpcodeCallIndirectName      = "call_indirect"
	OpcodeDropName              = "drop"
	OpcodeSelectName            = "select"
	OpcodeLocalGetName          = "local.get"
	OpcodeLocalSetName          = "local.set"
	OpcodeLocalTeeName          = "local.tee"
	OpcodeGlobalGetName         = "global.get"
	OpcodeGlobalSetName         = "global.set"
	OpcodeI32LoadName           = "i32.load"
	OpcodeI64LoadName           = "i64.load"
	OpcodeF32LoadName           = "f32.load"
	OpcodeF64LoadName           = "f64.load"
	OpcodeI32Load8SName         = "i32.load8_s"
	OpcodeI32Load8UName         = "i32.load8_u"
	OpcodeI32Load16SName        = "i32.load16_s"
	OpcodeI32Load16UName        = "i32.load16_u"
	OpcodeI64Load8SName         = "i64.load8_s"
	OpcodeI64Load8UName         = "i64.load8_u"
	OpcodeI64Load16SName        = "i64.load16_s"
	OpcodeI64Load16UName        = "i64.load16_u"
	OpcodeI64Load32SName        = "i64.load32_s"
	OpcodeI64Load32UName        = "i64.load32_u"
	OpcodeI32StoreName          = "i32.store"
	OpcodeI64StoreName          = "i64.store"
	OpcodeF32StoreName          = "f32.store"
	OpcodeF64StoreName          = "f64.store"
	OpcodeI32Store8Name         = "i32.store8"
	OpcodeI32Store16Name        = "i32.store16"
	OpcodeI64Store8Name         = "i64.store8"
	OpcodeI64Store16Name        = "i64.store16"
	OpcodeI64Store32Name        = "i64.store32"
	OpcodeMemorySizeName        = "memory.size"
	OpcodeMemoryGrowName        = "memory.grow"
	OpcodeI32ConstName          = "i32.const"
	OpcodeI64ConstName          = "i64.const"
	OpcodeF32ConstName          = "f32.const"
	OpcodeF64ConstName          = "f64.const"
	OpcodeI32EqzName            = "i32.eqz"
	OpcodeI32EqName             = "i32.eq"
	OpcodeI32NeName             = "i32.ne"
	OpcodeI32LtSName            = "i32.lt_s"
	OpcodeI32LtUName            = "i32.lt_u"
	OpcodeI32GtSName            = "i32.gt_s"
	OpcodeI32GtUName            = "i32.gt_u"
	OpcodeI32LeSName            = "i32.le_s"
	OpcodeI32LeUName            = "i32.le_u"
	OpcodeI32GeSName            = "i32.ge_s"
	OpcodeI32GeUName            = "i32.ge_u"
	OpcodeI64EqzName            = "i64.eqz"
	OpcodeI64EqName             = "i64.eq"
	OpcodeI64NeName             = "i64.ne"
	OpcodeI64LtSName            = "i64.lt_s"
	OpcodeI64LtUName            = "i64.lt_u"
	OpcodeI64GtSName            = "i64.gt_s"
	OpcodeI64GtUName            = "i64.gt_u"
	OpcodeI64LeSName            = "i64.le_s"
	OpcodeI64LeUName            = "i64.le_u"
	OpcodeI64GeSName            = "i64.ge_s"
	OpcodeI64GeUName            = "i64.ge_u"
	OpcodeF32EqName             = "f32.eq"
	OpcodeF32NeName             = "f32.ne"
	OpcodeF32LtName             = "f32.lt"
	OpcodeF32GtName             = "f32.gt"
	OpcodeF32LeName             = "f32.le"
	OpcodeF32GeName             = "f32.ge"
	OpcodeF64EqName             = "f64.eq"
	OpcodeF64NeName             = "f64.ne"
	OpcodeF64LtName             = "f64.lt"
	OpcodeF64GtName             = "f64.gt"
	OpcodeF64LeName             = "f64.le"
	OpcodeF64GeName             = "f64.ge"
	OpcodeI32ClzName            = "i32.clz"
	OpcodeI32CtzName            = "i32.ctz"
	OpcodeI32PopcntName         = "i32.popcnt"
	OpcodeI32AddName            = "i32.add"
	OpcodeI32SubName            = "i32.sub"
	OpcodeI32MulName            = "i32.mul"
	OpcodeI32DivSName           = "i32.div_s"
	OpcodeI32DivUName           = "i32.div_u"
	OpcodeI32RemSName           = "i32.rem_s"
	OpcodeI32RemUName           = "i32.rem_u"
	OpcodeI32AndName            = "i32.and"
	OpcodeI32OrName             = "i32.or"
	OpcodeI32XorName            = "i32.xor"
	OpcodeI32ShlName            = "i32.shl"
	OpcodeI32ShrSName           = "i32.shr_s"
	OpcodeI32ShrUName           = "i32.shr_u"
	OpcodeI32RotlName           = "i32.rotl"
	OpcodeI32RotrName           = "i32.rotr"
	OpcodeI64ClzName            = "i64.clz"
	OpcodeI64CtzName            = "i64.ctz"
	OpcodeI64PopcntName         = "i64.popcnt"
	OpcodeI64AddName            = "i64.add"
	OpcodeI64SubName            = "i64.sub"
	OpcodeI64MulName            = "i64.mul"
	OpcodeI64DivSName           = "i64.div_s"
	OpcodeI64DivUName           = "i64.div_u"
	OpcodeI64RemSName           = "i64.rem_s"
	OpcodeI64RemUName           = "i64.rem_u"
	OpcodeI64AndName            = "i64.and"
	OpcodeI64OrName             = "i64.or"
	OpcodeI64XorName            = "i64.xor"
	OpcodeI64ShlName            = "i64.shl"
	OpcodeI64ShrSName           = "i64.shr_s"
	OpcodeI64ShrUName           = "i64.shr_u"
	OpcodeI64RotlName           = "i64.rotl"
	OpcodeI64RotrName           = "i64.rotr"
	OpcodeF32AbsName            = "f32.abs"
	OpcodeF32NegName            = "f32.neg"
	OpcodeF32CeilName           = "f32.ceil"
	OpcodeF32FloorName          = "f32.floor"
	OpcodeF32TruncName          = "f32.trunc"
	OpcodeF32NearestName        = "f32.nearest"
	OpcodeF32SqrtName           = "f32.sqrt"
	OpcodeF32AddName            = "f32.add"
	OpcodeF32SubName            = "f32.sub"
	OpcodeF32MulName            = "f32.mul"
	OpcodeF32DivName            = "f32.div"
	OpcodeF32MinName            = "f32.min"
	OpcodeF32MaxName            = "f32.max"
	OpcodeF32CopysignName       = "f32.copysign"
	OpcodeF64AbsName            = "f64.abs"
	OpcodeF64NegName            = "f64.neg"
	OpcodeF64CeilName           = "f64.ceil"
	OpcodeF64FloorName          = "f64.floor"
	OpcodeF64TruncName          = "f64.trunc"
	OpcodeF64NearestName        = "f64.nearest"
	OpcodeF64SqrtName           = "f64.sqrt"
	OpcodeF64AddName            = "f64.add"
	OpcodeF64SubName            = "f64.sub"
	OpcodeF64MulName            = "f64.mul"
	OpcodeF64DivName            = "f64.div"
	OpcodeF64MinName            = "f64.min"
	OpcodeF64MaxName            = "f64.max"
	OpcodeF64CopysignName       = "f64.copysign"
	OpcodeI32WrapI64Name        = "i32.wrap_i64"
	OpcodeI32TruncF32SName      = "i32.trunc_f32_s"
	OpcodeI32TruncF32UName      = "i32.trunc_f32_u"
	OpcodeI32TruncF64SName      = "i32.trunc_f64_s"
	OpcodeI32TruncF64UName      = "i32.trunc_f64_u"
	OpcodeI64ExtendI32SName     = "i64.extend_i32_s"
	OpcodeI64ExtendI32UName     = "i64.extend_i32_u"
	OpcodeI64TruncF32SName      = "i64.trunc_f32_s"
	OpcodeI64TruncF32UName      = "i64.trunc_f32_u"
	OpcodeI64TruncF64SName      = "i64.trunc_f64_s"
	OpcodeI64TruncF64UName      = "i64.trunc_f64_u"
	OpcodeF32ConvertI32sName    = "f32.convert_i32_s"
	OpcodeF32ConvertI32UName    = "f32.convert_i32_u"
	OpcodeF32ConvertI64SName    = "f32.convert_i64_s"
	OpcodeF32ConvertI64UName    = "f32.convert_i64u"
	OpcodeF32DemoteF64Name      = "f32.demote_f64"
	OpcodeF64ConvertI32SName    = "f64.convert_i32_s"
	OpcodeF64ConvertI32UName    = "f64.convert_i32_u"
	OpcodeF64ConvertI64SName    = "f64.convert_i64_s"
	OpcodeF64ConvertI64UName    = "f64.convert_i64_u"
	OpcodeF64PromoteF32Name     = "f64.promote_f32"
	OpcodeI32ReinterpretF32Name = "i32.reinterpret_f32"
	OpcodeI64ReinterpretF64Name = "i64.reinterpret_f64"
	OpcodeF32ReinterpretI32Name = "f32.reinterpret_i32"
	OpcodeF64ReinterpretI64Name = "f64.reinterpret_i64"

	OpcodeRefNullName   = "ref.null"
	OpcodeRefIsNullName = "ref.is_null"
	OpcodeRefFuncName   = "ref.func"

	// Below are toggled with FeatureSignExtensionOps

	OpcodeI32Extend8SName  = "i32.extend8_s"
	OpcodeI32Extend16SName = "i32.extend16_s"
	OpcodeI64Extend8SName  = "i64.extend8_s"
	OpcodeI64Extend16SName = "i64.extend16_s"
	OpcodeI64Extend32SName = "i64.extend32_s"

	OpcodeMiscPrefixName = "misc_prefix"
)

var instructionNames = [256]string{
	OpcodeUnreachable:       OpcodeUnreachableName,
	OpcodeNop:               OpcodeNopName,
	OpcodeBlock:             OpcodeBlockName,
	OpcodeLoop:              OpcodeLoopName,
	OpcodeIf:                OpcodeIfName,
	OpcodeElse:              OpcodeElseName,
	OpcodeEnd:               OpcodeEndName,
	OpcodeBr:                OpcodeBrName,
	OpcodeBrIf:              OpcodeBrIfName,
	OpcodeBrTable:           OpcodeBrTableName,
	OpcodeReturn:            OpcodeReturnName,
	OpcodeCall:              OpcodeCallName,
	OpcodeCallIndirect:      OpcodeCallIndirectName,
	OpcodeDrop:              OpcodeDropName,
	OpcodeSelect:            OpcodeSelectName,
	OpcodeLocalGet:          OpcodeLocalGetName,
	OpcodeLocalSet:          OpcodeLocalSetName,
	OpcodeLocalTee:          OpcodeLocalTeeName,
	OpcodeGlobalGet:         OpcodeGlobalGetName,
	OpcodeGlobalSet:         OpcodeGlobalSetName,
	OpcodeI32Load:           OpcodeI32LoadName,
	OpcodeI64Load:           OpcodeI64LoadName,
	OpcodeF32Load:           OpcodeF32LoadName,
	OpcodeF64Load:           OpcodeF64LoadName,
	OpcodeI32Load8S:         OpcodeI32Load8SName,
	OpcodeI32Load8U:         OpcodeI32Load8UName,
	OpcodeI32Load16S:        OpcodeI32Load16SName,
	OpcodeI32Load16U:        OpcodeI32Load16UName,
	OpcodeI64Load8S:         OpcodeI64Load8SName,
	OpcodeI64Load8U:         OpcodeI64Load8UName,
	OpcodeI64Load16S:        OpcodeI64Load16SName,
	OpcodeI64Load16U:        OpcodeI64Load16UName,
	OpcodeI64Load32S:        OpcodeI64Load32SName,
	OpcodeI64Load32U:        OpcodeI64Load32UName,
	OpcodeI32Store:          OpcodeI32StoreName,
	OpcodeI64Store:          OpcodeI64StoreName,
	OpcodeF32Store:          OpcodeF32StoreName,
	OpcodeF64Store:          OpcodeF64StoreName,
	OpcodeI32Store8:         OpcodeI32Store8Name,
	OpcodeI32Store16:        OpcodeI32Store16Name,
	OpcodeI64Store8:         OpcodeI64Store8Name,
	OpcodeI64Store16:        OpcodeI64Store16Name,
	OpcodeI64Store32:        OpcodeI64Store32Name,
	OpcodeMemorySize:        OpcodeMemorySizeName,
	OpcodeMemoryGrow:        OpcodeMemoryGrowName,
	OpcodeI32Const:          OpcodeI32ConstName,
	OpcodeI64Const:          OpcodeI64ConstName,
	OpcodeF32Const:          OpcodeF32ConstName,
	OpcodeF64Const:          OpcodeF64ConstName,
	OpcodeI32Eqz:            OpcodeI32EqzName,
	OpcodeI32Eq:             OpcodeI32EqName,
	OpcodeI32Ne:             OpcodeI32NeName,
	OpcodeI32LtS:            OpcodeI32LtSName,
	OpcodeI32LtU:            OpcodeI32LtUName,
	OpcodeI32GtS:            OpcodeI32GtSName,
	OpcodeI32GtU:            OpcodeI32GtUName,
	OpcodeI32LeS:            OpcodeI32LeSName,
	OpcodeI32LeU:            OpcodeI32LeUName,
	OpcodeI32GeS:            OpcodeI32GeSName,
	OpcodeI32GeU:            OpcodeI32GeUName,
	OpcodeI64Eqz:            OpcodeI64EqzName,
	OpcodeI64Eq:             OpcodeI64EqName,
	OpcodeI64Ne:             OpcodeI64NeName,
	OpcodeI64LtS:            OpcodeI64LtSName,
	OpcodeI64LtU:            OpcodeI64LtUName,
	OpcodeI64GtS:            OpcodeI64GtSName,
	OpcodeI64GtU:            OpcodeI64GtUName,
	OpcodeI64LeS:            OpcodeI64LeSName,
	OpcodeI64LeU:            OpcodeI64LeUName,
	OpcodeI64GeS:            OpcodeI64GeSName,
	OpcodeI64GeU:            OpcodeI64GeUName,
	OpcodeF32Eq:             OpcodeF32EqName,
	OpcodeF32Ne:             OpcodeF32NeName,
	OpcodeF32Lt:             OpcodeF32LtName,
	OpcodeF32Gt:             OpcodeF32GtName,
	OpcodeF32Le:             OpcodeF32LeName,
	OpcodeF32Ge:             OpcodeF32GeName,
	OpcodeF64Eq:             OpcodeF64EqName,
	OpcodeF64Ne:             OpcodeF64NeName,
	OpcodeF64Lt:             OpcodeF64LtName,
	OpcodeF64Gt:             OpcodeF64GtName,
	OpcodeF64Le:             OpcodeF64LeName,
	OpcodeF64Ge:             OpcodeF64GeName,
	OpcodeI32Clz:            OpcodeI32ClzName,
	OpcodeI32Ctz:            OpcodeI32CtzName,
	OpcodeI32Popcnt:         OpcodeI32PopcntName,
	OpcodeI32Add:            OpcodeI32AddName,
	OpcodeI32Sub:            OpcodeI32SubName,
	OpcodeI32Mul:            OpcodeI32MulName,
	OpcodeI32DivS:           OpcodeI32DivSName,
	OpcodeI32DivU:           OpcodeI32DivUName,
	OpcodeI32RemS:           OpcodeI32RemSName,
	OpcodeI32RemU:           OpcodeI32RemUName,
	OpcodeI32And:            OpcodeI32AndName,
	OpcodeI32Or:             OpcodeI32OrName,
	OpcodeI32Xor:            OpcodeI32XorName,
	OpcodeI32Shl:            OpcodeI32ShlName,
	OpcodeI32ShrS:           OpcodeI32ShrSName,
	OpcodeI32ShrU:           OpcodeI32ShrUName,
	OpcodeI32Rotl:           OpcodeI32RotlName,
	OpcodeI32Rotr:           OpcodeI32RotrName,
	OpcodeI64Clz:            OpcodeI64ClzName,
	OpcodeI64Ctz:            OpcodeI64CtzName,
	OpcodeI64Popcnt:         OpcodeI64PopcntName,
	OpcodeI64Add:            OpcodeI64AddName,
	OpcodeI64Sub:            OpcodeI64SubName,
	OpcodeI64Mul:            OpcodeI64MulName,
	OpcodeI64DivS:           OpcodeI64DivSName,
	OpcodeI64DivU:           OpcodeI64DivUName,
	OpcodeI64RemS:           OpcodeI64RemSName,
	OpcodeI64RemU:           OpcodeI64RemUName,
	OpcodeI64And:            OpcodeI64AndName,
	OpcodeI64Or:             OpcodeI64OrName,
	OpcodeI64Xor:            OpcodeI64XorName,
	OpcodeI64Shl:            OpcodeI64ShlName,
	OpcodeI64ShrS:           OpcodeI64ShrSName,
	OpcodeI64ShrU:           OpcodeI64ShrUName,
	OpcodeI64Rotl:           OpcodeI64RotlName,
	OpcodeI64Rotr:           OpcodeI64RotrName,
	OpcodeF32Abs:            OpcodeF32AbsName,
	OpcodeF32Neg:            OpcodeF32NegName,
	OpcodeF32Ceil:           OpcodeF32CeilName,
	OpcodeF32Floor:          OpcodeF32FloorName,
	OpcodeF32Trunc:          OpcodeF32TruncName,
	OpcodeF32Nearest:        OpcodeF32NearestName,
	OpcodeF32Sqrt:           OpcodeF32SqrtName,
	OpcodeF32Add:            OpcodeF32AddName,
	OpcodeF32Sub:            OpcodeF32SubName,
	OpcodeF32Mul:            OpcodeF32MulName,
	OpcodeF32Div:            OpcodeF32DivName,
	OpcodeF32Min:            OpcodeF32MinName,
	OpcodeF32Max:            OpcodeF32MaxName,
	OpcodeF32Copysign:       OpcodeF32CopysignName,
	OpcodeF64Abs:            OpcodeF64AbsName,
	OpcodeF64Neg:            OpcodeF64NegName,
	OpcodeF64Ceil:           OpcodeF64CeilName,
	OpcodeF64Floor:          OpcodeF64FloorName,
	OpcodeF64Trunc:          OpcodeF64TruncName,
	OpcodeF64Nearest:        OpcodeF64NearestName,
	OpcodeF64Sqrt:           OpcodeF64SqrtName,
	OpcodeF64Add:            OpcodeF64AddName,
	OpcodeF64Sub:            OpcodeF64SubName,
	OpcodeF64Mul:            OpcodeF64MulName,
	OpcodeF64Div:            OpcodeF64DivName,
	OpcodeF64Min:            OpcodeF64MinName,
	OpcodeF64Max:            OpcodeF64MaxName,
	OpcodeF64Copysign:       OpcodeF64CopysignName,
	OpcodeI32WrapI64:        OpcodeI32WrapI64Name,
	OpcodeI32TruncF32S:      OpcodeI32TruncF32SName,
	OpcodeI32TruncF32U:      OpcodeI32TruncF32UName,
	OpcodeI32TruncF64S:      OpcodeI32TruncF64SName,
	OpcodeI32TruncF64U:      OpcodeI32TruncF64UName,
	OpcodeI64ExtendI32S:     OpcodeI64ExtendI32SName,
	OpcodeI64ExtendI32U:     OpcodeI64ExtendI32UName,
	OpcodeI64TruncF32S:      OpcodeI64TruncF32SName,
	OpcodeI64TruncF32U:      OpcodeI64TruncF32UName,
	OpcodeI64TruncF64S:      OpcodeI64TruncF64SName,
	OpcodeI64TruncF64U:      OpcodeI64TruncF64UName,
	OpcodeF32ConvertI32s:    OpcodeF32ConvertI32sName,
	OpcodeF32ConvertI32U:    OpcodeF32ConvertI32UName,
	OpcodeF32ConvertI64S:    OpcodeF32ConvertI64SName,
	OpcodeF32ConvertI64U:    OpcodeF32ConvertI64UName,
	OpcodeF32DemoteF64:      OpcodeF32DemoteF64Name,
	OpcodeF64ConvertI32S:    OpcodeF64ConvertI32SName,
	OpcodeF64ConvertI32U:    OpcodeF64ConvertI32UName,
	OpcodeF64ConvertI64S:    OpcodeF64ConvertI64SName,
	OpcodeF64ConvertI64U:    OpcodeF64ConvertI64UName,
	OpcodeF64PromoteF32:     OpcodeF64PromoteF32Name,
	OpcodeI32ReinterpretF32: OpcodeI32ReinterpretF32Name,
	OpcodeI64ReinterpretF64: OpcodeI64ReinterpretF64Name,
	OpcodeF32ReinterpretI32: OpcodeF32ReinterpretI32Name,
	OpcodeF64ReinterpretI64: OpcodeF64ReinterpretI64Name,

	OpcodeRefNull:   OpcodeRefNullName,
	OpcodeRefIsNull: OpcodeRefIsNullName,
	OpcodeRefFunc:   OpcodeRefFuncName,

	// Below are toggled with FeatureSignExtensionOps
	OpcodeI32Extend8S:  OpcodeI32Extend8SName,
	OpcodeI32Extend16S: OpcodeI32Extend16SName,
	OpcodeI64Extend8S:  OpcodeI64Extend8SName,
	OpcodeI64Extend16S: OpcodeI64Extend16SName,
	OpcodeI64Extend32S: OpcodeI64Extend32SName,

	OpcodeMiscPrefix: OpcodeMiscPrefixName,
}

// InstructionName returns the instruction corresponding to this binary Opcode.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#a7-index-of-instructions
func InstructionName(oc Opcode) string {
	return instructionNames[oc]
}

const (
	OpcodeI32TruncSatF32SName = "i32.trunc_sat_f32_s"
	OpcodeI32TruncSatF32UName = "i32.trunc_sat_f32_u"
	OpcodeI32TruncSatF64SName = "i32.trunc_sat_f64_s"
	OpcodeI32TruncSatF64UName = "i32.trunc_sat_f64_u"
	OpcodeI64TruncSatF32SName = "i64.trunc_sat_f32_s"
	OpcodeI64TruncSatF32UName = "i64.trunc_sat_f32_u"
	OpcodeI64TruncSatF64SName = "i64.trunc_sat_f64_s"
	OpcodeI64TruncSatF64UName = "i64.trunc_sat_f64_u"

	OpcodeMemoryInitName = "memory.init"
	OpcodeDataDropName   = "data.drop"
	OpcodeMemoryCopyName = "memory.copy"
	OpcodeMemoryFillName = "memory.fill"
	OpcodeTableInitName  = "table.init"
	OpcodeElemDropName   = "elem.drop"
	OpcodeTableCopyName  = "table.copy"
	OpcodeTableGrowName  = "table.grow"
	OpcodeTableSizeName  = "table.size"
	OpcodeTableFillName  = "table.fill"
)

var miscInstructionNames = [256]string{
	OpcodeMiscI32TruncSatF32S: OpcodeI32TruncSatF32SName,
	OpcodeMiscI32TruncSatF32U: OpcodeI32TruncSatF32UName,
	OpcodeMiscI32TruncSatF64S: OpcodeI32TruncSatF64SName,
	OpcodeMiscI32TruncSatF64U: OpcodeI32TruncSatF64UName,
	OpcodeMiscI64TruncSatF32S: OpcodeI64TruncSatF32SName,
	OpcodeMiscI64TruncSatF32U: OpcodeI64TruncSatF32UName,
	OpcodeMiscI64TruncSatF64S: OpcodeI64TruncSatF64SName,
	OpcodeMiscI64TruncSatF64U: OpcodeI64TruncSatF64UName,

	OpcodeMiscMemoryInit: OpcodeMemoryInitName,
	OpcodeMiscDataDrop:   OpcodeDataDropName,
	OpcodeMiscMemoryCopy: OpcodeMemoryCopyName,
	OpcodeMiscMemoryFill: OpcodeMemoryFillName,
	OpcodeMiscTableInit:  OpcodeTableInitName,
	OpcodeMiscElemDrop:   OpcodeElemDropName,
	OpcodeMiscTableCopy:  OpcodeTableCopyName,
	OpcodeMiscTableGrow:  OpcodeTableGrowName,
	OpcodeMiscTableSize:  OpcodeTableSizeName,
	OpcodeMiscTableFill:  OpcodeTableFillName,
}

// MiscInstructionName returns the instruction corresponding to this miscellaneous Opcode.
func MiscInstructionName(oc OpcodeMisc) string {
	return miscInstructionNames[oc]
}
