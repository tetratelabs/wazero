package wazeroir

import (
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// signature represents how a Wasm optcode
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

// wasmOptcodeSignature returns the signature of given Wasm optcode.
// Note that some of optcodes' signature vary depending on
// the function instance (for example, local types).
// "index" parameter is not used by most of optcodes.
// The returned signature is used for stack validation when lowering Wasm's optcodes to wazeroir.
func wasmOptcodeSignature(f *wasm.FunctionInstance, op wasm.OptCode, index uint32) (*signature, error) {
	switch op {
	case wasm.OptCodeUnreachable, wasm.OptCodeNop, wasm.OptCodeBlock, wasm.OptCodeLoop:
		return signature_None_None, nil
	case wasm.OptCodeIf:
		return signature_I32_None, nil
	case wasm.OptCodeElse, wasm.OptCodeEnd, wasm.OptCodeBr:
		return signature_None_None, nil
	case wasm.OptCodeBrIf, wasm.OptCodeBrTable:
		return signature_I32_None, nil
	case wasm.OptCodeReturn:
		return signature_None_None, nil
	case wasm.OptCodeCall:
		return funcTypeToSignature(f.ModuleInstance.Functions[index].Signature), nil
	case wasm.OptCodeCallIndirect:
		ret := funcTypeToSignature(f.ModuleInstance.Types[index])
		ret.in = append(ret.in, SignLessTypeI32)
		return ret, nil
	case wasm.OptCodeDrop:
		return signature_Unknown_None, nil
	case wasm.OptCodeSelect:
		return signature_UnknownUnkownI32_Unknown, nil
	case wasm.OptCodeLocalGet:
		inputLen := uint32(len(f.Signature.InputTypes))
		if l := f.NumLocals + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t SignLessType
		if index < inputLen {
			t = wasmValueTypeToSignless(f.Signature.InputTypes[index])
		} else {
			t = wasmValueTypeToSignless(f.LocalTypes[index-inputLen])
		}
		return &signature{out: []SignLessType{t}}, nil
	case wasm.OptCodeLocalSet:
		inputLen := uint32(len(f.Signature.InputTypes))
		if l := f.NumLocals + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t SignLessType
		if index < inputLen {
			t = wasmValueTypeToSignless(f.Signature.InputTypes[index])
		} else {
			t = wasmValueTypeToSignless(f.LocalTypes[index-inputLen])
		}
		return &signature{in: []SignLessType{t}}, nil
	case wasm.OptCodeLocalTee:
		inputLen := uint32(len(f.Signature.InputTypes))
		if l := f.NumLocals + inputLen; index >= l {
			return nil, fmt.Errorf("invalid local index for local.get %d >= %d", index, l)
		}
		var t SignLessType
		if index < inputLen {
			t = wasmValueTypeToSignless(f.Signature.InputTypes[index])
		} else {
			t = wasmValueTypeToSignless(f.LocalTypes[index-inputLen])
		}
		return &signature{in: []SignLessType{t}, out: []SignLessType{t}}, nil
	case wasm.OptCodeGlobalGet:
		if len(f.ModuleInstance.Globals) <= int(index) {
			return nil, fmt.Errorf("invalid global index for global.get %d >= %d", index, len(f.ModuleInstance.Globals))
		}
		return &signature{
			out: []SignLessType{wasmValueTypeToSignless(f.ModuleInstance.Globals[index].Type.ValType)},
		}, nil
	case wasm.OptCodeGlobalSet:
		if len(f.ModuleInstance.Globals) <= int(index) {
			return nil, fmt.Errorf("invalid global index for global.get %d >= %d", index, len(f.ModuleInstance.Globals))
		}
		return &signature{
			in: []SignLessType{wasmValueTypeToSignless(f.ModuleInstance.Globals[index].Type.ValType)},
		}, nil
	case wasm.OptCodeI32Load:
		return signature_I32_I32, nil
	case wasm.OptCodeI64Load:
		return signature_I32_I64, nil
	case wasm.OptCodeF32Load:
		return signature_I32_F32, nil
	case wasm.OptCodeF64Load:
		return signature_I32_F64, nil
	case wasm.OptCodeI32Load8s, wasm.OptCodeI32Load8u, wasm.OptCodeI32Load16s, wasm.OptCodeI32Load16u:
		return signature_I32_I32, nil
	case wasm.OptCodeI64Load8s, wasm.OptCodeI64Load8u, wasm.OptCodeI64Load16s, wasm.OptCodeI64Load16u,
		wasm.OptCodeI64Load32s, wasm.OptCodeI64Load32u:
		return signature_I32_I64, nil
	case wasm.OptCodeI32Store:
		return signature_I32I32_None, nil
	case wasm.OptCodeI64Store:
		return signature_I32I64_None, nil
	case wasm.OptCodeF32Store:
		return signature_I32F32_None, nil
	case wasm.OptCodeF64Store:
		return signature_I32F64_None, nil
	case wasm.OptCodeI32Store8:
		return signature_I32I32_None, nil
	case wasm.OptCodeI32Store16:
		return signature_I32I32_None, nil
	case wasm.OptCodeI64Store8:
		return signature_I32I64_None, nil
	case wasm.OptCodeI64Store16:
		return signature_I32I64_None, nil
	case wasm.OptCodeI64Store32:
		return signature_I32I64_None, nil
	case wasm.OptCodeMemorySize:
		return signature_None_I32, nil
	case wasm.OptCodeMemoryGrow:
		return signature_I32_I32, nil
	case wasm.OptCodeI32Const:
		return signature_None_I32, nil
	case wasm.OptCodeI64Const:
		return signature_None_I64, nil
	case wasm.OptCodeF32Const:
		return signature_None_F32, nil
	case wasm.OptCodeF64Const:
		return signature_None_F64, nil
	case wasm.OptCodeI32eqz:
		return signature_I32_I32, nil
	case wasm.OptCodeI32eq, wasm.OptCodeI32ne, wasm.OptCodeI32lts,
		wasm.OptCodeI32ltu, wasm.OptCodeI32gts, wasm.OptCodeI32gtu,
		wasm.OptCodeI32les, wasm.OptCodeI32leu, wasm.OptCodeI32ges,
		wasm.OptCodeI32geu:
		return signature_I32I32_I32, nil
	case wasm.OptCodeI64eqz:
		return signature_I64_I32, nil
	case wasm.OptCodeI64eq, wasm.OptCodeI64ne, wasm.OptCodeI64lts,
		wasm.OptCodeI64ltu, wasm.OptCodeI64gts, wasm.OptCodeI64gtu,
		wasm.OptCodeI64les, wasm.OptCodeI64leu, wasm.OptCodeI64ges,
		wasm.OptCodeI64geu:
		return signature_I64I64_I32, nil
	case wasm.OptCodeF32eq, wasm.OptCodeF32ne, wasm.OptCodeF32lt,
		wasm.OptCodeF32gt, wasm.OptCodeF32le, wasm.OptCodeF32ge:
		return signature_F32F32_I32, nil
	case wasm.OptCodeF64eq, wasm.OptCodeF64ne, wasm.OptCodeF64lt,
		wasm.OptCodeF64gt, wasm.OptCodeF64le, wasm.OptCodeF64ge:
		return signature_F64F64_I32, nil
	case wasm.OptCodeI32clz, wasm.OptCodeI32ctz, wasm.OptCodeI32popcnt:
		return signature_I32_I32, nil
	case wasm.OptCodeI32add, wasm.OptCodeI32sub, wasm.OptCodeI32mul,
		wasm.OptCodeI32divs, wasm.OptCodeI32divu, wasm.OptCodeI32rems,
		wasm.OptCodeI32remu, wasm.OptCodeI32and, wasm.OptCodeI32or,
		wasm.OptCodeI32xor, wasm.OptCodeI32shl, wasm.OptCodeI32shrs,
		wasm.OptCodeI32shru, wasm.OptCodeI32rotl, wasm.OptCodeI32rotr:
		return signature_I32I32_I32, nil
	case wasm.OptCodeI64clz, wasm.OptCodeI64ctz, wasm.OptCodeI64popcnt:
		return signature_I64_I64, nil
	case wasm.OptCodeI64add, wasm.OptCodeI64sub, wasm.OptCodeI64mul,
		wasm.OptCodeI64divs, wasm.OptCodeI64divu, wasm.OptCodeI64rems,
		wasm.OptCodeI64remu, wasm.OptCodeI64and, wasm.OptCodeI64or,
		wasm.OptCodeI64xor, wasm.OptCodeI64shl, wasm.OptCodeI64shrs,
		wasm.OptCodeI64shru, wasm.OptCodeI64rotl, wasm.OptCodeI64rotr:
		return signature_I64I64_I64, nil
	case wasm.OptCodeF32abs, wasm.OptCodeF32neg, wasm.OptCodeF32ceil,
		wasm.OptCodeF32floor, wasm.OptCodeF32trunc, wasm.OptCodeF32nearest,
		wasm.OptCodeF32sqrt:
		return signature_F32_F32, nil
	case wasm.OptCodeF32add, wasm.OptCodeF32sub, wasm.OptCodeF32mul,
		wasm.OptCodeF32div, wasm.OptCodeF32min, wasm.OptCodeF32max,
		wasm.OptCodeF32copysign:
		return signature_F32F32_F32, nil
	case wasm.OptCodeF64abs, wasm.OptCodeF64neg, wasm.OptCodeF64ceil,
		wasm.OptCodeF64floor, wasm.OptCodeF64trunc, wasm.OptCodeF64nearest,
		wasm.OptCodeF64sqrt:
		return signature_F64_F64, nil
	case wasm.OptCodeF64add, wasm.OptCodeF64sub, wasm.OptCodeF64mul,
		wasm.OptCodeF64div, wasm.OptCodeF64min, wasm.OptCodeF64max,
		wasm.OptCodeF64copysign:
		return signature_F64F64_F64, nil
	case wasm.OptCodeI32wrapI64:
		return signature_I64_I32, nil
	case wasm.OptCodeI32truncf32s, wasm.OptCodeI32truncf32u:
		return signature_F32_I32, nil
	case wasm.OptCodeI32truncf64s, wasm.OptCodeI32truncf64u:
		return signature_F64_I32, nil
	case wasm.OptCodeI64Extendi32s, wasm.OptCodeI64Extendi32u:
		return signature_I32_I64, nil
	case wasm.OptCodeI64TruncF32s, wasm.OptCodeI64TruncF32u:
		return signature_F32_I64, nil
	case wasm.OptCodeI64Truncf64s, wasm.OptCodeI64Truncf64u:
		return signature_F64_I64, nil
	case wasm.OptCodeF32Converti32s, wasm.OptCodeF32Converti32u:
		return signature_I32_F32, nil
	case wasm.OptCodeF32Converti64s, wasm.OptCodeF32Converti64u:
		return signature_I64_F32, nil
	case wasm.OptCodeF32Demotef64:
		return signature_F64_F32, nil
	case wasm.OptCodeF64Converti32s, wasm.OptCodeF64Converti32u:
		return signature_I32_F64, nil
	case wasm.OptCodeF64Converti64s, wasm.OptCodeF64Converti64u:
		return signature_I64_F64, nil
	case wasm.OptCodeF64Promotef32:
		return signature_F32_F64, nil
	case wasm.OptCodeI32reinterpretf32:
		return signature_F32_I32, nil
	case wasm.OptCodeI64reinterpretf64:
		return signature_F64_I64, nil
	case wasm.OptCodeF32reinterpreti32:
		return signature_I32_F32, nil
	case wasm.OptCodeF64reinterpreti64:
		return signature_I64_F64, nil
	default:
		return nil, fmt.Errorf("unsupported instruction in wazeroir: 0x%x", op)
	}
}

func funcTypeToSignature(tps *wasm.FunctionType) *signature {
	ret := &signature{}
	for _, vt := range tps.InputTypes {
		ret.in = append(ret.in, wasmValueTypeToSignless(vt))
	}
	for _, vt := range tps.ReturnTypes {
		ret.out = append(ret.out, wasmValueTypeToSignless(vt))
	}
	return ret
}

func wasmValueTypeToSignless(vt wasm.ValueType) SignLessType {
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
