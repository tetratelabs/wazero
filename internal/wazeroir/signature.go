package wazeroir

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// signature represents how a Wasm opcode
// manipulates the value stacks in terms of value types.
type signature struct {
	in, out []UnsignedType
}

var (
	signature_None_None    = &signature{}
	signature_Unknown_None = &signature{
		in: []UnsignedType{UnsignedTypeUnknown},
	}
	signature_None_I32 = &signature{
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_None_I64 = &signature{
		out: []UnsignedType{UnsignedTypeI64},
	}
	signature_None_F32 = &signature{
		out: []UnsignedType{UnsignedTypeF32},
	}
	signature_None_F64 = &signature{
		out: []UnsignedType{UnsignedTypeF64},
	}
	signature_I32_None = &signature{
		in: []UnsignedType{UnsignedTypeI32},
	}
	signature_I32_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeI32},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_I32_I64 = &signature{
		in:  []UnsignedType{UnsignedTypeI32},
		out: []UnsignedType{UnsignedTypeI64},
	}
	signature_I64_I64 = &signature{
		in:  []UnsignedType{UnsignedTypeI64},
		out: []UnsignedType{UnsignedTypeI64},
	}
	signature_I32_F32 = &signature{
		in:  []UnsignedType{UnsignedTypeI32},
		out: []UnsignedType{UnsignedTypeF32},
	}
	signature_I32_F64 = &signature{
		in:  []UnsignedType{UnsignedTypeI32},
		out: []UnsignedType{UnsignedTypeF64},
	}
	signature_I64_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeI64},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_I64_F32 = &signature{
		in:  []UnsignedType{UnsignedTypeI64},
		out: []UnsignedType{UnsignedTypeF32},
	}
	signature_I64_F64 = &signature{
		in:  []UnsignedType{UnsignedTypeI64},
		out: []UnsignedType{UnsignedTypeF64},
	}
	signature_F32_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeF32},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_F32_I64 = &signature{
		in:  []UnsignedType{UnsignedTypeF32},
		out: []UnsignedType{UnsignedTypeI64},
	}
	signature_F32_F64 = &signature{
		in:  []UnsignedType{UnsignedTypeF32},
		out: []UnsignedType{UnsignedTypeF64},
	}
	signature_F32_F32 = &signature{
		in:  []UnsignedType{UnsignedTypeF32},
		out: []UnsignedType{UnsignedTypeF32},
	}
	signature_F64_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeF64},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_F64_F32 = &signature{
		in:  []UnsignedType{UnsignedTypeF64},
		out: []UnsignedType{UnsignedTypeF32},
	}
	signature_F64_I64 = &signature{
		in:  []UnsignedType{UnsignedTypeF64},
		out: []UnsignedType{UnsignedTypeI64},
	}
	signature_F64_F64 = &signature{
		in:  []UnsignedType{UnsignedTypeF64},
		out: []UnsignedType{UnsignedTypeF64},
	}
	signature_I32I32_None = &signature{
		in: []UnsignedType{UnsignedTypeI32, UnsignedTypeI32},
	}
	signature_I32I32_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeI32, UnsignedTypeI32},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_I32I64_None = &signature{
		in: []UnsignedType{UnsignedTypeI32, UnsignedTypeI64},
	}
	signature_I32F32_None = &signature{
		in: []UnsignedType{UnsignedTypeI32, UnsignedTypeF32},
	}
	signature_I32F64_None = &signature{
		in: []UnsignedType{UnsignedTypeI32, UnsignedTypeF64},
	}
	signature_I64I64_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeI64, UnsignedTypeI64},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_I64I64_I64 = &signature{
		in:  []UnsignedType{UnsignedTypeI64, UnsignedTypeI64},
		out: []UnsignedType{UnsignedTypeI64},
	}
	signature_F32F32_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeF32, UnsignedTypeF32},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_F32F32_F32 = &signature{
		in:  []UnsignedType{UnsignedTypeF32, UnsignedTypeF32},
		out: []UnsignedType{UnsignedTypeF32},
	}
	signature_F64F64_I32 = &signature{
		in:  []UnsignedType{UnsignedTypeF64, UnsignedTypeF64},
		out: []UnsignedType{UnsignedTypeI32},
	}
	signature_F64F64_F64 = &signature{
		in:  []UnsignedType{UnsignedTypeF64, UnsignedTypeF64},
		out: []UnsignedType{UnsignedTypeF64},
	}
	signature_UnknownUnknownI32_Unknown = &signature{
		in:  []UnsignedType{UnsignedTypeUnknown, UnsignedTypeUnknown, UnsignedTypeI32},
		out: []UnsignedType{UnsignedTypeUnknown},
	}
)

// wasmOpcodeSignature returns the signature of given Wasm opcode.
// Note that some of opcodes' signature vary depending on
// the function instance (for example, local types).
// "index" parameter is not used by most of opcodes.
// The returned signature is used for stack validation when lowering Wasm's opcodes to wazeroir.
func wasmOpcodeSignature(f *wasm.FunctionInstance, op wasm.Opcode, index uint32) (*signature, error) {
	switch op {
	case wasm.OpcodeUnreachable, wasm.OpcodeNop, wasm.OpcodeBlock, wasm.OpcodeLoop:
		return signature_None_None, nil
	case wasm.OpcodeIf:
		return signature_I32_None, nil
	case wasm.OpcodeElse, wasm.OpcodeEnd, wasm.OpcodeBr:
		return signature_None_None, nil
	case wasm.OpcodeBrIf, wasm.OpcodeBrTable:
		return signature_I32_None, nil
	case wasm.OpcodeReturn:
		return signature_None_None, nil
	case wasm.OpcodeCall:
		return funcTypeToSignature(f.Module.Functions[index].Type), nil
	case wasm.OpcodeCallIndirect:
		ret := funcTypeToSignature(f.Module.Types[index].Type)
		ret.in = append(ret.in, UnsignedTypeI32)
		return ret, nil
	case wasm.OpcodeDrop:
		return signature_Unknown_None, nil
	case wasm.OpcodeSelect:
		return signature_UnknownUnknownI32_Unknown, nil
	case wasm.OpcodeLocalGet:
		inputLen := uint32(len(f.Type.Params))
		if l := uint32(len(f.LocalTypes)) + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t UnsignedType
		if index < inputLen {
			t = wasmValueTypeToUnsignedType(f.Type.Params[index])
		} else {
			t = wasmValueTypeToUnsignedType(f.LocalTypes[index-inputLen])
		}
		return &signature{out: []UnsignedType{t}}, nil
	case wasm.OpcodeLocalSet:
		inputLen := uint32(len(f.Type.Params))
		if l := uint32(len(f.LocalTypes)) + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t UnsignedType
		if index < inputLen {
			t = wasmValueTypeToUnsignedType(f.Type.Params[index])
		} else {
			t = wasmValueTypeToUnsignedType(f.LocalTypes[index-inputLen])
		}
		return &signature{in: []UnsignedType{t}}, nil
	case wasm.OpcodeLocalTee:
		inputLen := uint32(len(f.Type.Params))
		if l := uint32(len(f.LocalTypes)) + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t UnsignedType
		if index < inputLen {
			t = wasmValueTypeToUnsignedType(f.Type.Params[index])
		} else {
			t = wasmValueTypeToUnsignedType(f.LocalTypes[index-inputLen])
		}
		return &signature{in: []UnsignedType{t}, out: []UnsignedType{t}}, nil
	case wasm.OpcodeGlobalGet:
		if len(f.Module.Globals) <= int(index) {
			return nil, fmt.Errorf("invalid global index for global.get %d >= %d", index, len(f.Module.Globals))
		}
		return &signature{
			out: []UnsignedType{wasmValueTypeToUnsignedType(f.Module.Globals[index].Type.ValType)},
		}, nil
	case wasm.OpcodeGlobalSet:
		if len(f.Module.Globals) <= int(index) {
			return nil, fmt.Errorf("invalid global index for global.get %d >= %d", index, len(f.Module.Globals))
		}
		return &signature{
			in: []UnsignedType{wasmValueTypeToUnsignedType(f.Module.Globals[index].Type.ValType)},
		}, nil
	case wasm.OpcodeI32Load:
		return signature_I32_I32, nil
	case wasm.OpcodeI64Load:
		return signature_I32_I64, nil
	case wasm.OpcodeF32Load:
		return signature_I32_F32, nil
	case wasm.OpcodeF64Load:
		return signature_I32_F64, nil
	case wasm.OpcodeI32Load8S, wasm.OpcodeI32Load8U, wasm.OpcodeI32Load16S, wasm.OpcodeI32Load16U:
		return signature_I32_I32, nil
	case wasm.OpcodeI64Load8S, wasm.OpcodeI64Load8U, wasm.OpcodeI64Load16S, wasm.OpcodeI64Load16U,
		wasm.OpcodeI64Load32S, wasm.OpcodeI64Load32U:
		return signature_I32_I64, nil
	case wasm.OpcodeI32Store:
		return signature_I32I32_None, nil
	case wasm.OpcodeI64Store:
		return signature_I32I64_None, nil
	case wasm.OpcodeF32Store:
		return signature_I32F32_None, nil
	case wasm.OpcodeF64Store:
		return signature_I32F64_None, nil
	case wasm.OpcodeI32Store8:
		return signature_I32I32_None, nil
	case wasm.OpcodeI32Store16:
		return signature_I32I32_None, nil
	case wasm.OpcodeI64Store8:
		return signature_I32I64_None, nil
	case wasm.OpcodeI64Store16:
		return signature_I32I64_None, nil
	case wasm.OpcodeI64Store32:
		return signature_I32I64_None, nil
	case wasm.OpcodeMemorySize:
		return signature_None_I32, nil
	case wasm.OpcodeMemoryGrow:
		return signature_I32_I32, nil
	case wasm.OpcodeI32Const:
		return signature_None_I32, nil
	case wasm.OpcodeI64Const:
		return signature_None_I64, nil
	case wasm.OpcodeF32Const:
		return signature_None_F32, nil
	case wasm.OpcodeF64Const:
		return signature_None_F64, nil
	case wasm.OpcodeI32Eqz:
		return signature_I32_I32, nil
	case wasm.OpcodeI32Eq, wasm.OpcodeI32Ne, wasm.OpcodeI32LtS,
		wasm.OpcodeI32LtU, wasm.OpcodeI32GtS, wasm.OpcodeI32GtU,
		wasm.OpcodeI32LeS, wasm.OpcodeI32LeU, wasm.OpcodeI32GeS,
		wasm.OpcodeI32GeU:
		return signature_I32I32_I32, nil
	case wasm.OpcodeI64Eqz:
		return signature_I64_I32, nil
	case wasm.OpcodeI64Eq, wasm.OpcodeI64Ne, wasm.OpcodeI64LtS,
		wasm.OpcodeI64LtU, wasm.OpcodeI64GtS, wasm.OpcodeI64GtU,
		wasm.OpcodeI64LeS, wasm.OpcodeI64LeU, wasm.OpcodeI64GeS,
		wasm.OpcodeI64GeU:
		return signature_I64I64_I32, nil
	case wasm.OpcodeF32Eq, wasm.OpcodeF32Ne, wasm.OpcodeF32Lt,
		wasm.OpcodeF32Gt, wasm.OpcodeF32Le, wasm.OpcodeF32Ge:
		return signature_F32F32_I32, nil
	case wasm.OpcodeF64Eq, wasm.OpcodeF64Ne, wasm.OpcodeF64Lt,
		wasm.OpcodeF64Gt, wasm.OpcodeF64Le, wasm.OpcodeF64Ge:
		return signature_F64F64_I32, nil
	case wasm.OpcodeI32Clz, wasm.OpcodeI32Ctz, wasm.OpcodeI32Popcnt:
		return signature_I32_I32, nil
	case wasm.OpcodeI32Add, wasm.OpcodeI32Sub, wasm.OpcodeI32Mul,
		wasm.OpcodeI32DivS, wasm.OpcodeI32DivU, wasm.OpcodeI32RemS,
		wasm.OpcodeI32RemU, wasm.OpcodeI32And, wasm.OpcodeI32Or,
		wasm.OpcodeI32Xor, wasm.OpcodeI32Shl, wasm.OpcodeI32ShrS,
		wasm.OpcodeI32ShrU, wasm.OpcodeI32Rotl, wasm.OpcodeI32Rotr:
		return signature_I32I32_I32, nil
	case wasm.OpcodeI64Clz, wasm.OpcodeI64Ctz, wasm.OpcodeI64Popcnt:
		return signature_I64_I64, nil
	case wasm.OpcodeI64Add, wasm.OpcodeI64Sub, wasm.OpcodeI64Mul,
		wasm.OpcodeI64DivS, wasm.OpcodeI64DivU, wasm.OpcodeI64RemS,
		wasm.OpcodeI64RemU, wasm.OpcodeI64And, wasm.OpcodeI64Or,
		wasm.OpcodeI64Xor, wasm.OpcodeI64Shl, wasm.OpcodeI64ShrS,
		wasm.OpcodeI64ShrU, wasm.OpcodeI64Rotl, wasm.OpcodeI64Rotr:
		return signature_I64I64_I64, nil
	case wasm.OpcodeF32Abs, wasm.OpcodeF32Neg, wasm.OpcodeF32Ceil,
		wasm.OpcodeF32Floor, wasm.OpcodeF32Trunc, wasm.OpcodeF32Nearest,
		wasm.OpcodeF32Sqrt:
		return signature_F32_F32, nil
	case wasm.OpcodeF32Add, wasm.OpcodeF32Sub, wasm.OpcodeF32Mul,
		wasm.OpcodeF32Div, wasm.OpcodeF32Min, wasm.OpcodeF32Max,
		wasm.OpcodeF32Copysign:
		return signature_F32F32_F32, nil
	case wasm.OpcodeF64Abs, wasm.OpcodeF64Neg, wasm.OpcodeF64Ceil,
		wasm.OpcodeF64Floor, wasm.OpcodeF64Trunc, wasm.OpcodeF64Nearest,
		wasm.OpcodeF64Sqrt:
		return signature_F64_F64, nil
	case wasm.OpcodeF64Add, wasm.OpcodeF64Sub, wasm.OpcodeF64Mul,
		wasm.OpcodeF64Div, wasm.OpcodeF64Min, wasm.OpcodeF64Max,
		wasm.OpcodeF64Copysign:
		return signature_F64F64_F64, nil
	case wasm.OpcodeI32WrapI64:
		return signature_I64_I32, nil
	case wasm.OpcodeI32TruncF32S, wasm.OpcodeI32TruncF32U:
		return signature_F32_I32, nil
	case wasm.OpcodeI32TruncF64S, wasm.OpcodeI32TruncF64U:
		return signature_F64_I32, nil
	case wasm.OpcodeI64ExtendI32S, wasm.OpcodeI64ExtendI32U:
		return signature_I32_I64, nil
	case wasm.OpcodeI64TruncF32S, wasm.OpcodeI64TruncF32U:
		return signature_F32_I64, nil
	case wasm.OpcodeI64TruncF64S, wasm.OpcodeI64TruncF64U:
		return signature_F64_I64, nil
	case wasm.OpcodeF32ConvertI32s, wasm.OpcodeF32ConvertI32U:
		return signature_I32_F32, nil
	case wasm.OpcodeF32ConvertI64S, wasm.OpcodeF32ConvertI64U:
		return signature_I64_F32, nil
	case wasm.OpcodeF32DemoteF64:
		return signature_F64_F32, nil
	case wasm.OpcodeF64ConvertI32S, wasm.OpcodeF64ConvertI32U:
		return signature_I32_F64, nil
	case wasm.OpcodeF64ConvertI64S, wasm.OpcodeF64ConvertI64U:
		return signature_I64_F64, nil
	case wasm.OpcodeF64PromoteF32:
		return signature_F32_F64, nil
	case wasm.OpcodeI32ReinterpretF32:
		return signature_F32_I32, nil
	case wasm.OpcodeI64ReinterpretF64:
		return signature_F64_I64, nil
	case wasm.OpcodeF32ReinterpretI32:
		return signature_I32_F32, nil
	case wasm.OpcodeF64ReinterpretI64:
		return signature_I64_F64, nil
	case wasm.OpcodeI32Extend8S, wasm.OpcodeI32Extend16S:
		return signature_I32_I32, nil
	case wasm.OpcodeI64Extend8S, wasm.OpcodeI64Extend16S, wasm.OpcodeI64Extend32S:
		return signature_I64_I64, nil
	default:
		return nil, fmt.Errorf("unsupported instruction in wazeroir: 0x%x", op)
	}
}

func funcTypeToSignature(tps *wasm.FunctionType) *signature {
	ret := &signature{}
	for _, vt := range tps.Params {
		ret.in = append(ret.in, wasmValueTypeToUnsignedType(vt))
	}
	for _, vt := range tps.Results {
		ret.out = append(ret.out, wasmValueTypeToUnsignedType(vt))
	}
	return ret
}

func wasmValueTypeToUnsignedType(vt wasm.ValueType) UnsignedType {
	switch vt {
	case wasm.ValueTypeI32:
		return UnsignedTypeI32
	case wasm.ValueTypeI64:
		return UnsignedTypeI64
	case wasm.ValueTypeF32:
		return UnsignedTypeF32
	case wasm.ValueTypeF64:
		return UnsignedTypeF64
	}
	panic("unreachable")
}
