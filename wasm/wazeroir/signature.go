package wazeroir

import (
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// signature represents how a Wasm opcode
// manipulates the value stacks in terms of value types.
type signature struct {
	in, out []SignLessType
}

var (
	signature_None_None    = &signature{}
	signature_Unknown_None = &signature{
		in: []SignLessType{SignLessTypeUnknown},
	}
	signature_None_I32 = &signature{
		out: []SignLessType{SignLessTypeI32},
	}
	signature_None_I64 = &signature{
		out: []SignLessType{SignLessTypeI64},
	}
	signature_None_F32 = &signature{
		out: []SignLessType{SignLessTypeF32},
	}
	signature_None_F64 = &signature{
		out: []SignLessType{SignLessTypeF64},
	}
	signature_I32_None = &signature{
		in: []SignLessType{SignLessTypeI32},
	}
	signature_I32_I32 = &signature{
		in:  []SignLessType{SignLessTypeI32},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_I32_I64 = &signature{
		in:  []SignLessType{SignLessTypeI32},
		out: []SignLessType{SignLessTypeI64},
	}
	signature_I64_I64 = &signature{
		in:  []SignLessType{SignLessTypeI64},
		out: []SignLessType{SignLessTypeI64},
	}
	signature_I32_F32 = &signature{
		in:  []SignLessType{SignLessTypeI32},
		out: []SignLessType{SignLessTypeF32},
	}
	signature_I32_F64 = &signature{
		in:  []SignLessType{SignLessTypeI32},
		out: []SignLessType{SignLessTypeF64},
	}
	signature_I64_I32 = &signature{
		in:  []SignLessType{SignLessTypeI64},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_I64_F32 = &signature{
		in:  []SignLessType{SignLessTypeI64},
		out: []SignLessType{SignLessTypeF32},
	}
	signature_I64_F64 = &signature{
		in:  []SignLessType{SignLessTypeI64},
		out: []SignLessType{SignLessTypeF64},
	}
	signature_F32_I32 = &signature{
		in:  []SignLessType{SignLessTypeF32},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_F32_I64 = &signature{
		in:  []SignLessType{SignLessTypeF32},
		out: []SignLessType{SignLessTypeI64},
	}
	signature_F32_F64 = &signature{
		in:  []SignLessType{SignLessTypeF32},
		out: []SignLessType{SignLessTypeF64},
	}
	signature_F32_F32 = &signature{
		in:  []SignLessType{SignLessTypeF32},
		out: []SignLessType{SignLessTypeF32},
	}
	signature_F64_I32 = &signature{
		in:  []SignLessType{SignLessTypeF64},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_F64_F32 = &signature{
		in:  []SignLessType{SignLessTypeF64},
		out: []SignLessType{SignLessTypeF32},
	}
	signature_F64_I64 = &signature{
		in:  []SignLessType{SignLessTypeF64},
		out: []SignLessType{SignLessTypeI64},
	}
	signature_F64_F64 = &signature{
		in:  []SignLessType{SignLessTypeF64},
		out: []SignLessType{SignLessTypeF64},
	}
	signature_I32I32_None = &signature{
		in: []SignLessType{SignLessTypeI32, SignLessTypeI32},
	}
	signature_I32I32_I32 = &signature{
		in:  []SignLessType{SignLessTypeI32, SignLessTypeI32},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_I32I64_None = &signature{
		in: []SignLessType{SignLessTypeI32, SignLessTypeI64},
	}
	signature_I32F32_None = &signature{
		in: []SignLessType{SignLessTypeI32, SignLessTypeF32},
	}
	signature_I32F64_None = &signature{
		in: []SignLessType{SignLessTypeI32, SignLessTypeF64},
	}
	signature_I64I64_I32 = &signature{
		in:  []SignLessType{SignLessTypeI64, SignLessTypeI64},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_I64I64_I64 = &signature{
		in:  []SignLessType{SignLessTypeI64, SignLessTypeI64},
		out: []SignLessType{SignLessTypeI64},
	}
	signature_F32F32_I32 = &signature{
		in:  []SignLessType{SignLessTypeF32, SignLessTypeF32},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_F32F32_F32 = &signature{
		in:  []SignLessType{SignLessTypeF32, SignLessTypeF32},
		out: []SignLessType{SignLessTypeF32},
	}
	signature_F64F64_I32 = &signature{
		in:  []SignLessType{SignLessTypeF64, SignLessTypeF64},
		out: []SignLessType{SignLessTypeI32},
	}
	signature_F64F64_F64 = &signature{
		in:  []SignLessType{SignLessTypeF64, SignLessTypeF64},
		out: []SignLessType{SignLessTypeF64},
	}
	signature_UnknownUnkownI32_Unknown = &signature{
		in:  []SignLessType{SignLessTypeUnknown, SignLessTypeUnknown, SignLessTypeI32},
		out: []SignLessType{SignLessTypeUnknown},
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
		return funcTypeToSignature(f.ModuleInstance.Functions[index].Signature), nil
	case wasm.OpcodeCallIndirect:
		ret := funcTypeToSignature(f.ModuleInstance.Types[index])
		ret.in = append(ret.in, SignLessTypeI32)
		return ret, nil
	case wasm.OpcodeDrop:
		return signature_Unknown_None, nil
	case wasm.OpcodeSelect:
		return signature_UnknownUnkownI32_Unknown, nil
	case wasm.OpcodeLocalGet:
		inputLen := uint32(len(f.Signature.InputTypes))
		if l := f.NumLocals + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t SignLessType
		if index < inputLen {
			t = WasmValueTypeToSignless(f.Signature.InputTypes[index])
		} else {
			t = WasmValueTypeToSignless(f.LocalTypes[index-inputLen])
		}
		return &signature{out: []SignLessType{t}}, nil
	case wasm.OpcodeLocalSet:
		inputLen := uint32(len(f.Signature.InputTypes))
		if l := f.NumLocals + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t SignLessType
		if index < inputLen {
			t = WasmValueTypeToSignless(f.Signature.InputTypes[index])
		} else {
			t = WasmValueTypeToSignless(f.LocalTypes[index-inputLen])
		}
		return &signature{in: []SignLessType{t}}, nil
	case wasm.OpcodeLocalTee:
		inputLen := uint32(len(f.Signature.InputTypes))
		if l := f.NumLocals + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t SignLessType
		if index < inputLen {
			t = WasmValueTypeToSignless(f.Signature.InputTypes[index])
		} else {
			t = WasmValueTypeToSignless(f.LocalTypes[index-inputLen])
		}
		return &signature{in: []SignLessType{t}, out: []SignLessType{t}}, nil
	case wasm.OpcodeGlobalGet:
		if len(f.ModuleInstance.Globals) <= int(index) {
			return nil, fmt.Errorf("invalid global index for global.get %d >= %d", index, len(f.ModuleInstance.Globals))
		}
		return &signature{
			out: []SignLessType{WasmValueTypeToSignless(f.ModuleInstance.Globals[index].Type.ValType)},
		}, nil
	case wasm.OpcodeGlobalSet:
		if len(f.ModuleInstance.Globals) <= int(index) {
			return nil, fmt.Errorf("invalid global index for global.get %d >= %d", index, len(f.ModuleInstance.Globals))
		}
		return &signature{
			in: []SignLessType{WasmValueTypeToSignless(f.ModuleInstance.Globals[index].Type.ValType)},
		}, nil
	case wasm.OpcodeI32Load:
		return signature_I32_I32, nil
	case wasm.OpcodeI64Load:
		return signature_I32_I64, nil
	case wasm.OpcodeF32Load:
		return signature_I32_F32, nil
	case wasm.OpcodeF64Load:
		return signature_I32_F64, nil
	case wasm.OpcodeI32Load8s, wasm.OpcodeI32Load8u, wasm.OpcodeI32Load16s, wasm.OpcodeI32Load16u:
		return signature_I32_I32, nil
	case wasm.OpcodeI64Load8s, wasm.OpcodeI64Load8u, wasm.OpcodeI64Load16s, wasm.OpcodeI64Load16u,
		wasm.OpcodeI64Load32s, wasm.OpcodeI64Load32u:
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
	case wasm.OpcodeI32eqz:
		return signature_I32_I32, nil
	case wasm.OpcodeI32eq, wasm.OpcodeI32ne, wasm.OpcodeI32lts,
		wasm.OpcodeI32ltu, wasm.OpcodeI32gts, wasm.OpcodeI32gtu,
		wasm.OpcodeI32les, wasm.OpcodeI32leu, wasm.OpcodeI32ges,
		wasm.OpcodeI32geu:
		return signature_I32I32_I32, nil
	case wasm.OpcodeI64eqz:
		return signature_I64_I32, nil
	case wasm.OpcodeI64eq, wasm.OpcodeI64ne, wasm.OpcodeI64lts,
		wasm.OpcodeI64ltu, wasm.OpcodeI64gts, wasm.OpcodeI64gtu,
		wasm.OpcodeI64les, wasm.OpcodeI64leu, wasm.OpcodeI64ges,
		wasm.OpcodeI64geu:
		return signature_I64I64_I32, nil
	case wasm.OpcodeF32eq, wasm.OpcodeF32ne, wasm.OpcodeF32lt,
		wasm.OpcodeF32gt, wasm.OpcodeF32le, wasm.OpcodeF32ge:
		return signature_F32F32_I32, nil
	case wasm.OpcodeF64eq, wasm.OpcodeF64ne, wasm.OpcodeF64lt,
		wasm.OpcodeF64gt, wasm.OpcodeF64le, wasm.OpcodeF64ge:
		return signature_F64F64_I32, nil
	case wasm.OpcodeI32clz, wasm.OpcodeI32ctz, wasm.OpcodeI32popcnt:
		return signature_I32_I32, nil
	case wasm.OpcodeI32add, wasm.OpcodeI32sub, wasm.OpcodeI32mul,
		wasm.OpcodeI32divs, wasm.OpcodeI32divu, wasm.OpcodeI32rems,
		wasm.OpcodeI32remu, wasm.OpcodeI32and, wasm.OpcodeI32or,
		wasm.OpcodeI32xor, wasm.OpcodeI32shl, wasm.OpcodeI32shrs,
		wasm.OpcodeI32shru, wasm.OpcodeI32rotl, wasm.OpcodeI32rotr:
		return signature_I32I32_I32, nil
	case wasm.OpcodeI64clz, wasm.OpcodeI64ctz, wasm.OpcodeI64popcnt:
		return signature_I64_I64, nil
	case wasm.OpcodeI64add, wasm.OpcodeI64sub, wasm.OpcodeI64mul,
		wasm.OpcodeI64divs, wasm.OpcodeI64divu, wasm.OpcodeI64rems,
		wasm.OpcodeI64remu, wasm.OpcodeI64and, wasm.OpcodeI64or,
		wasm.OpcodeI64xor, wasm.OpcodeI64shl, wasm.OpcodeI64shrs,
		wasm.OpcodeI64shru, wasm.OpcodeI64rotl, wasm.OpcodeI64rotr:
		return signature_I64I64_I64, nil
	case wasm.OpcodeF32abs, wasm.OpcodeF32neg, wasm.OpcodeF32ceil,
		wasm.OpcodeF32floor, wasm.OpcodeF32trunc, wasm.OpcodeF32nearest,
		wasm.OpcodeF32sqrt:
		return signature_F32_F32, nil
	case wasm.OpcodeF32add, wasm.OpcodeF32sub, wasm.OpcodeF32mul,
		wasm.OpcodeF32div, wasm.OpcodeF32min, wasm.OpcodeF32max,
		wasm.OpcodeF32copysign:
		return signature_F32F32_F32, nil
	case wasm.OpcodeF64abs, wasm.OpcodeF64neg, wasm.OpcodeF64ceil,
		wasm.OpcodeF64floor, wasm.OpcodeF64trunc, wasm.OpcodeF64nearest,
		wasm.OpcodeF64sqrt:
		return signature_F64_F64, nil
	case wasm.OpcodeF64add, wasm.OpcodeF64sub, wasm.OpcodeF64mul,
		wasm.OpcodeF64div, wasm.OpcodeF64min, wasm.OpcodeF64max,
		wasm.OpcodeF64copysign:
		return signature_F64F64_F64, nil
	case wasm.OpcodeI32wrapI64:
		return signature_I64_I32, nil
	case wasm.OpcodeI32truncf32s, wasm.OpcodeI32truncf32u:
		return signature_F32_I32, nil
	case wasm.OpcodeI32truncf64s, wasm.OpcodeI32truncf64u:
		return signature_F64_I32, nil
	case wasm.OpcodeI64Extendi32s, wasm.OpcodeI64Extendi32u:
		return signature_I32_I64, nil
	case wasm.OpcodeI64TruncF32s, wasm.OpcodeI64TruncF32u:
		return signature_F32_I64, nil
	case wasm.OpcodeI64Truncf64s, wasm.OpcodeI64Truncf64u:
		return signature_F64_I64, nil
	case wasm.OpcodeF32Converti32s, wasm.OpcodeF32Converti32u:
		return signature_I32_F32, nil
	case wasm.OpcodeF32Converti64s, wasm.OpcodeF32Converti64u:
		return signature_I64_F32, nil
	case wasm.OpcodeF32Demotef64:
		return signature_F64_F32, nil
	case wasm.OpcodeF64Converti32s, wasm.OpcodeF64Converti32u:
		return signature_I32_F64, nil
	case wasm.OpcodeF64Converti64s, wasm.OpcodeF64Converti64u:
		return signature_I64_F64, nil
	case wasm.OpcodeF64Promotef32:
		return signature_F32_F64, nil
	case wasm.OpcodeI32Reinterpretf32:
		return signature_F32_I32, nil
	case wasm.OpcodeI64Reinterpretf64:
		return signature_F64_I64, nil
	case wasm.OpcodeF32Reinterpreti32:
		return signature_I32_F32, nil
	case wasm.OpcodeF64Reinterpreti64:
		return signature_I64_F64, nil
	default:
		return nil, fmt.Errorf("unsupported instruction in wazeroir: 0x%x", op)
	}
}

func funcTypeToSignature(tps *wasm.FunctionType) *signature {
	ret := &signature{}
	for _, vt := range tps.InputTypes {
		ret.in = append(ret.in, WasmValueTypeToSignless(vt))
	}
	for _, vt := range tps.ReturnTypes {
		ret.out = append(ret.out, WasmValueTypeToSignless(vt))
	}
	return ret
}

func WasmValueTypeToSignless(vt wasm.ValueType) SignLessType {
	switch vt {
	case wasm.ValueTypeI32:
		return SignLessTypeI32
	case wasm.ValueTypeI64:
		return SignLessTypeI64
	case wasm.ValueTypeF32:
		return SignLessTypeF32
	case wasm.ValueTypeF64:
		return SignLessTypeF64
	}
	panic("unreachable")
}
