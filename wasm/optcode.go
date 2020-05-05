package wasm

type OptCode byte

const (
	// control instruction
	OptCodeUnreachable  OptCode = 0x00
	OptCodeNop          OptCode = 0x01
	OptCodeBlock        OptCode = 0x02
	OptCodeLoop         OptCode = 0x03
	OptCodeIf           OptCode = 0x04
	OptCodeElse         OptCode = 0x05
	OptCodeEnd          OptCode = 0x0b
	OptCodeBr           OptCode = 0x0c
	OptCodeBrIf         OptCode = 0x0d
	OptCodeBrTable      OptCode = 0x0e
	OptCodeReturn       OptCode = 0x0f
	OptCodeCall         OptCode = 0x10
	OptCodeCallIndirect OptCode = 0x11

	// parametric instruction
	OptCodeDrop   OptCode = 0x1a
	OptCodeSelect OptCode = 0x1b

	// variable instruction
	OptCodeLocalGet  OptCode = 0x20
	OptCodeLocalSet  OptCode = 0x21
	OptCodeLocalTee  OptCode = 0x22
	OptCodeGlobalGet OptCode = 0x23
	OptCodeGlobalSet OptCode = 0x24

	// memory instruction
	OptCodeI32Load    OptCode = 0x28
	OptCodeI64Load    OptCode = 0x29
	OptCodeF32Load    OptCode = 0x2a
	OptCodeF64Load    OptCode = 0x2b
	OptCodeI32Load8s  OptCode = 0x2c
	OptCodeI32Load8u  OptCode = 0x2d
	OptCodeI32Load16s OptCode = 0x2e
	OptCodeI32Load16u OptCode = 0x2f
	OptCodeI64Load8s  OptCode = 0x30
	OptCodeI64Load8u  OptCode = 0x31
	OptCodeI64Load16s OptCode = 0x32
	OptCodeI64Load16u OptCode = 0x33
	OptCodeI64Load32s OptCode = 0x34
	OptCodeI64Load32u OptCode = 0x35
	OptCodeI32Store   OptCode = 0x36
	OptCodeI64Store   OptCode = 0x37
	OptCodeF32Store   OptCode = 0x38
	OptCodeF64Store   OptCode = 0x39
	OptCodeI32Store8  OptCode = 0x3a
	OptCodeI32Store16 OptCode = 0x3b
	OptCodeI64Store8  OptCode = 0x3c
	OptCodeI64Store16 OptCode = 0x3d
	OptCodeI64Store32 OptCode = 0x3e
	OptCodeMemorySize OptCode = 0x3f
	OptCodeMemoryGrow OptCode = 0x40

	// numeric instruction
	OptCodeI32Const OptCode = 0x41
	OptCodeI64Const OptCode = 0x42
	OptCodeF32Const OptCode = 0x43
	OptCodeF64Const OptCode = 0x44

	OptCodeI32eqz OptCode = 0x45
	OptCodeI32eq  OptCode = 0x46
	OptCodeI32ne  OptCode = 0x47
	OptCodeI32lts OptCode = 0x48
	OptCodeI32ltu OptCode = 0x49
	OptCodeI32gts OptCode = 0x4a
	OptCodeI32gtu OptCode = 0x4b
	OptCodeI32les OptCode = 0x4c
	OptCodeI32leu OptCode = 0x4d
	OptCodeI32ges OptCode = 0x4e
	OptCodeI32geu OptCode = 0x4f

	OptCodeI64eqz OptCode = 0x50
	OptCodeI64eq  OptCode = 0x51
	OptCodeI64ne  OptCode = 0x52
	OptCodeI64lts OptCode = 0x53
	OptCodeI64ltu OptCode = 0x54
	OptCodeI64gts OptCode = 0x55
	OptCodeI64gtu OptCode = 0x56
	OptCodeI64les OptCode = 0x57
	OptCodeI64leu OptCode = 0x58
	OptCodeI64ges OptCode = 0x59
	OptCodeI64geu OptCode = 0x5a

	OptCodeF32eq OptCode = 0x5b
	OptCodeF32ne OptCode = 0x5c
	OptCodeF32lt OptCode = 0x5d
	OptCodeF32gt OptCode = 0x5e
	OptCodeF32le OptCode = 0x5f
	OptCodeF32ge OptCode = 0x60

	OptCodeF64eq OptCode = 0x61
	OptCodeF64ne OptCode = 0x62
	OptCodeF64lt OptCode = 0x63
	OptCodeF64gt OptCode = 0x64
	OptCodeF64le OptCode = 0x65
	OptCodeF64ge OptCode = 0x66

	OptCodeI32clz    OptCode = 0x67
	OptCodeI32ctz    OptCode = 0x68
	OptCodeI32popcnt OptCode = 0x69
	OptCodeI32add    OptCode = 0x6a
	OptCodeI32sub    OptCode = 0x6b
	OptCodeI32mul    OptCode = 0x6c
	OptCodeI32divs   OptCode = 0x6d
	OptCodeI32divu   OptCode = 0x6e
	OptCodeI32rems   OptCode = 0x6f
	OptCodeI32remu   OptCode = 0x70
	OptCodeI32and    OptCode = 0x71
	OptCodeI32or     OptCode = 0x72
	OptCodeI32xor    OptCode = 0x73
	OptCodeI32shl    OptCode = 0x74
	OptCodeI32shrs   OptCode = 0x75
	OptCodeI32shru   OptCode = 0x76
	OptCodeI32rotl   OptCode = 0x77
	OptCodeI32rotr   OptCode = 0x78

	OptCodeI64clz    OptCode = 0x79
	OptCodeI64ctz    OptCode = 0x7a
	OptCodeI64popcnt OptCode = 0x7b
	OptCodeI64add    OptCode = 0x7c
	OptCodeI64sub    OptCode = 0x7d
	OptCodeI64mul    OptCode = 0x7e
	OptCodeI64divs   OptCode = 0x7f
	OptCodeI64divu   OptCode = 0x80
	OptCodeI64rems   OptCode = 0x81
	OptCodeI64remu   OptCode = 0x82
	OptCodeI64and    OptCode = 0x83
	OptCodeI64or     OptCode = 0x84
	OptCodeI64xor    OptCode = 0x85
	OptCodeI64shl    OptCode = 0x86
	OptCodeI64shrs   OptCode = 0x87
	OptCodeI64shru   OptCode = 0x88
	OptCodeI64rotl   OptCode = 0x89
	OptCodeI64rotr   OptCode = 0x8a

	OptCodeF32abs      OptCode = 0x8b
	OptCodeF32neg      OptCode = 0x8c
	OptCodeF32ceil     OptCode = 0x8d
	OptCodeF32floor    OptCode = 0x8e
	OptCodeF32trunc    OptCode = 0x8f
	OptCodeF32nearest  OptCode = 0x90
	OptCodeF32sqrt     OptCode = 0x91
	OptCodeF32add      OptCode = 0x92
	OptCodeF32sub      OptCode = 0x93
	OptCodeF32mul      OptCode = 0x94
	OptCodeF32div      OptCode = 0x95
	OptCodeF32min      OptCode = 0x96
	OptCodeF32max      OptCode = 0x97
	OptCodeF32copysign OptCode = 0x98

	OptCodeF64abs      OptCode = 0x99
	OptCodeF64neg      OptCode = 0x9a
	OptCodeF64ceil     OptCode = 0x9b
	OptCodeF64floor    OptCode = 0x9c
	OptCodeF64trunc    OptCode = 0x9d
	OptCodeF64nearest  OptCode = 0x9e
	OptCodeF64sqrt     OptCode = 0x9f
	OptCodeF64add      OptCode = 0xa0
	OptCodeF64sub      OptCode = 0xa1
	OptCodeF64mul      OptCode = 0xa2
	OptCodeF64div      OptCode = 0xa3
	OptCodeF64min      OptCode = 0xa4
	OptCodeF64max      OptCode = 0xa5
	OptCodeF64copysign OptCode = 0xa6

	OptCodeI32wrapI64   OptCode = 0xa7
	OptCodeI32truncf32s OptCode = 0xa8
	OptCodeI32truncf32u OptCode = 0xa9
	OptCodeI32truncf64s OptCode = 0xaa
	OptCodeI32truncf64u OptCode = 0xab

	OptCodeI64Extendi32s OptCode = 0xac
	OptCodeI64Extendi32u OptCode = 0xad
	OptCodeI64TruncF32s  OptCode = 0xae
	OptCodeI64TruncF32u  OptCode = 0xaf
	OptCodeI64Truncf64s  OptCode = 0xb0
	OptCodeI64Truncf64u  OptCode = 0xb1

	OptCodeF32Converti32s OptCode = 0xb2
	OptCodeF32Converti32u OptCode = 0xb3
	OptCodeF32Converti64s OptCode = 0xb4
	OptCodeF32Converti64u OptCode = 0xb5
	OptCodeF32Demotef64   OptCode = 0xb6

	OptCodeF64Converti32s OptCode = 0xb7
	OptCodeF64Converti32u OptCode = 0xb8
	OptCodeF64Converti64s OptCode = 0xb9
	OptCodeF64Converti64u OptCode = 0xba
	OptCodeF64Promotef32  OptCode = 0xbb

	OptCodeI32reinterpretf32 OptCode = 0xbc
	OptCodeI64reinterpretf64 OptCode = 0xbd
	OptCodeF32reinterpreti32 OptCode = 0xbe
	OptCodeF64reinterpreti64 OptCode = 0xbf
)
