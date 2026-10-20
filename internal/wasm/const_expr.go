package wasm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero/internal/leb128"
)

type ConstantExpression struct {
	Data []byte
}

func evaluateConstExpr(e *ConstantExpression, globalResolver func(globalIndex Index) (ValueType, uint64, uint64, error), funcRefResolver func(funcIndex Index) (Reference, error)) ([]uint64, ValueType, error) {
	return evaluateConstExprWithModule(e, globalResolver, funcRefResolver, nil, nil)
}

// evaluateConstExprWithModule is the GC-aware evaluator.
// mod provides the TypeSection for GC opcode type lookups (nil-safe).
// mi provides runtime allocation (TypeIDs, GCRegister); nil means
// validation mode — GC opcodes are type-checked but produce a 0
// placeholder uint64.
func evaluateConstExprWithModule(
	e *ConstantExpression,
	globalResolver func(globalIndex Index) (ValueType, uint64, uint64, error),
	funcRefResolver func(funcIndex Index) (Reference, error),
	mod *Module,
	mi *ModuleInstance,
) ([]uint64, ValueType, error) {
	var stack []uint64
	var typeStack []ValueType
	var pc uint64
	data := e.Data
	for {
		if pc >= uint64(len(data)) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		opCode := data[pc]
		pc++
		switch opCode {
		case OpcodeI32Const:
			v, n, err := leb128.LoadInt32(data[pc:])
			if err != nil {
				return nil, 0, fmt.Errorf("read i32: %w", err)
			}
			pc += n
			stack = append(stack, uint64(uint32(v)))
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI64Const:
			v, n, err := leb128.LoadInt64(data[pc:])
			if err != nil {
				return nil, 0, fmt.Errorf("read i64: %w", err)
			}
			pc += n
			stack = append(stack, uint64(v))
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeF32Const:
			if len(data[pc:]) < 4 {
				return nil, 0, io.ErrUnexpectedEOF
			}
			v := binary.LittleEndian.Uint32(data[pc:])
			pc += 4
			stack = append(stack, uint64(v))
			typeStack = append(typeStack, ValueTypeF32)
		case OpcodeF64Const:
			if len(data[pc:]) < 8 {
				return nil, 0, io.ErrUnexpectedEOF
			}
			v := binary.LittleEndian.Uint64(data[pc:])
			pc += 8
			stack = append(stack, uint64(v))
			typeStack = append(typeStack, ValueTypeF64)
		case OpcodeGlobalGet:
			v, n, err := leb128.LoadUint32(data[pc:])
			if err != nil {
				return nil, 0, fmt.Errorf("read index of global: %w", err)
			}
			pc += n
			typ, lo, hi, err := globalResolver(Index(v))
			if err != nil {
				return nil, 0, err
			}
			switch typ {
			case ValueTypeV128:
				stack = append(stack, lo, hi)
			default:
				stack = append(stack, lo)
			}
			typeStack = append(typeStack, typ)
		case OpcodeRefNull:
			// Heap type is s33 LEB. Positive: concrete type index → push
			// nullable (ref null $idx). Negative: abstract heap-type byte.
			if pc >= uint64(len(data)) {
				return nil, 0, fmt.Errorf("read reference type for ref.null: %w", io.ErrShortBuffer)
			}
			ht, n, err := leb128.DecodeInt33AsInt64(bytes.NewReader(data[pc:]))
			if err != nil {
				return nil, 0, fmt.Errorf("read ref.null heap type: %w", err)
			}
			pc += n
			var valType ValueType
			if ht >= 0 {
				valType = ValueTypeConcreteRef(uint32(ht), true)
			} else {
				valType = ValueType(byte(ht & 0x7F))
				switch valType {
				case ValueTypeFuncref, ValueTypeExternref, ValueTypeExnref,
					ValueTypeAnyref, ValueTypeEqref, ValueTypeI31ref,
					ValueTypeStructref, ValueTypeArrayref, ValueTypeNullref,
					ValueTypeNoFuncref, ValueTypeNoExternref, ValueTypeNoExnref:
				default:
					return nil, 0, fmt.Errorf("invalid type for ref.null: %d", ht)
				}
			}
			stack = append(stack, 0)
			typeStack = append(typeStack, valType)
		case OpcodeRefFunc:
			v, n, err := leb128.LoadUint32(data[pc:])
			if err != nil {
				return nil, 0, fmt.Errorf("read i32: %w", err)
			}
			pc += n
			ref, err := funcRefResolver(Index(v))
			if err != nil {
				return nil, 0, err
			}
			stack = append(stack, uint64(ref))
			var t ValueType
			if mod != nil {
				if typeIdx, ok := mod.typeIndexOfFunction(Index(v)); ok {
					t = ValueTypeConcreteRef(typeIdx, false)
				} else {
					t = ValueTypeFuncref
				}
			} else {
				t = ValueTypeFuncref
			}
			typeStack = append(typeStack, t)
		case OpcodeVecPrefix:
			if data[pc] != OpcodeVecV128Const {
				return nil, 0, fmt.Errorf("invalid vector opcode for const expression: %#x", data[pc-1])
			}
			pc++
			if len(data[pc:]) < 16 {
				return nil, 0, fmt.Errorf("%s needs 16 bytes but was %d bytes", OpcodeVecV128ConstName, len(data[pc:]))
			}
			lo := binary.LittleEndian.Uint64(data[pc:])
			pc += 8
			hi := binary.LittleEndian.Uint64(data[pc:])
			pc += 8
			stack = append(stack, lo, hi)
			typeStack = append(typeStack, ValueTypeV128)
		case OpcodeI32Add:
			if len(typeStack) < 2 {
				return nil, 0, errors.New("stack underflow on i32.add")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI32 || v2 != ValueTypeI32 {
				return nil, 0, fmt.Errorf("type mismatch on i32.add: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, uint64(uint32(a)+uint32(b)))
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI32Sub:
			if len(typeStack) < 2 {
				return nil, 0, errors.New("stack underflow on i32.sub")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI32 || v2 != ValueTypeI32 {
				return nil, 0, fmt.Errorf("type mismatch on i32.sub: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, uint64(uint32(a)-uint32(b)))
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI32Mul:
			if len(typeStack) < 2 {
				return nil, 0, errors.New("stack underflow on i32.mul")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI32 || v2 != ValueTypeI32 {
				return nil, 0, fmt.Errorf("type mismatch on i32.mul: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, uint64(uint32(a)*uint32(b)))
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI64Add:
			if len(typeStack) < 2 {
				return nil, 0, errors.New("stack underflow on i64.add")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI64 || v2 != ValueTypeI64 {
				return nil, 0, fmt.Errorf("type mismatch on i64.add: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, a+b)
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeI64Sub:
			if len(typeStack) < 2 {
				return nil, 0, errors.New("stack underflow on i64.sub")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI64 || v2 != ValueTypeI64 {
				return nil, 0, fmt.Errorf("type mismatch on i64.sub: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, a-b)
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeI64Mul:
			if len(typeStack) < 2 {
				return nil, 0, errors.New("stack underflow on i64.mul")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI64 || v2 != ValueTypeI64 {
				return nil, 0, fmt.Errorf("type mismatch on i64.mul: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, a*b)
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeEnd:
			if len(typeStack) != 1 {
				return nil, 0, errors.New("stack has more than one value at end of constant expression")
			}
			return stack, typeStack[0], nil
		case OpcodeGCPrefix:
			sub, n, err := leb128.LoadUint32(data[pc:])
			if err != nil {
				return nil, 0, fmt.Errorf("read GC sub-opcode: %w", err)
			}
			pc += n
			switch sub {
			case OpcodeGCRefI31:
				if len(typeStack) < 1 || typeStack[len(typeStack)-1] != ValueTypeI32 {
					return nil, 0, errors.New("ref.i31 requires i32 on stack")
				}
				v := stack[len(stack)-1]
				stack[len(stack)-1] = PackI31(uint32(v))
				typeStack[len(typeStack)-1] = ValueTypeI31ref.AsNonNullable()
			case OpcodeGCStructNew, OpcodeGCStructNewDefault:
				typeIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, fmt.Errorf("read struct.new type index: %w", err)
				}
				pc += m
				var fields []any
				if sub == OpcodeGCStructNew && mi != nil {
					schema := mod.TypeSection[typeIdx]
					fields = make([]any, len(schema.Fields))
					for i := len(schema.Fields) - 1; i >= 0; i-- {
						raw := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						typeStack = typeStack[:len(typeStack)-1]
						fields[i] = encodeFieldValueForConst(schema.Fields[i], raw)
					}
				} else if sub == OpcodeGCStructNew {
					fc := 0
					if mod != nil && int(typeIdx) < len(mod.TypeSection) {
						fc = len(mod.TypeSection[typeIdx].Fields)
					}
					if fc > len(stack) {
						return nil, 0, errors.New("struct.new: stack underflow")
					}
					stack = stack[:len(stack)-fc]
					typeStack = typeStack[:len(typeStack)-fc]
				} else if mi != nil {
					schema := mod.TypeSection[typeIdx]
					fields = make([]any, len(schema.Fields))
					for i := range schema.Fields {
						fields[i] = DefaultFieldValue(schema.Fields[i])
					}
				}
				if mi != nil {
					s := NewWasmStructWith(mi.TypeIDs[typeIdx], fields)
					stack = append(stack, mi.GCRegister(s))
				} else {
					stack = append(stack, 0)
				}
				typeStack = append(typeStack, ValueTypeConcreteRef(typeIdx, false))
			case OpcodeGCArrayNew, OpcodeGCArrayNewDefault:
				typeIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, fmt.Errorf("read array.new type index: %w", err)
				}
				pc += m
				var elems []any
				if sub == OpcodeGCArrayNew {
					if len(stack) < 2 {
						return nil, 0, errors.New("array.new: stack underflow")
					}
					length := uint32(stack[len(stack)-1])
					rawElem := stack[len(stack)-2]
					stack = stack[:len(stack)-2]
					typeStack = typeStack[:len(typeStack)-2]
					if mi != nil {
						schema := mod.TypeSection[typeIdx].ArrayField
						stored := encodeFieldValueForConst(schema, rawElem)
						elems = make([]any, length)
						for i := range elems {
							elems[i] = stored
						}
					}
				} else { // ArrayNewDefault
					if len(stack) < 1 {
						return nil, 0, errors.New("array.new_default: stack underflow")
					}
					length := uint32(stack[len(stack)-1])
					stack = stack[:len(stack)-1]
					typeStack = typeStack[:len(typeStack)-1]
					if mi != nil {
						schema := mod.TypeSection[typeIdx].ArrayField
						def := DefaultFieldValue(schema)
						elems = make([]any, length)
						for i := range elems {
							elems[i] = def
						}
					}
				}
				if mi != nil {
					a := NewWasmArrayWith(mi.TypeIDs[typeIdx], elems)
					stack = append(stack, mi.GCRegister(a))
				} else {
					stack = append(stack, 0)
				}
				typeStack = append(typeStack, ValueTypeConcreteRef(typeIdx, false))
			case OpcodeGCArrayNewFixed:
				typeIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, fmt.Errorf("read array.new_fixed type index: %w", err)
				}
				pc += m
				count, n2, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, fmt.Errorf("read array.new_fixed count: %w", err)
				}
				pc += n2
				if uint32(len(stack)) < count {
					return nil, 0, errors.New("array.new_fixed: stack underflow")
				}
				var elems []any
				if mi != nil {
					schema := mod.TypeSection[typeIdx].ArrayField
					elems = make([]any, count)
					for i := int(count) - 1; i >= 0; i-- {
						raw := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						typeStack = typeStack[:len(typeStack)-1]
						elems[i] = encodeFieldValueForConst(schema, raw)
					}
				} else {
					stack = stack[:uint32(len(stack))-count]
					typeStack = typeStack[:uint32(len(typeStack))-count]
				}
				if mi != nil {
					a := NewWasmArrayWith(mi.TypeIDs[typeIdx], elems)
					stack = append(stack, mi.GCRegister(a))
				} else {
					stack = append(stack, 0)
				}
				typeStack = append(typeStack, ValueTypeConcreteRef(typeIdx, false))
			case OpcodeGCAnyConvertExtern:
				if len(typeStack) < 1 {
					return nil, 0, errors.New("any.convert_extern: stack underflow")
				}
				v := stack[len(stack)-1]
				if v != 0 {
					stack[len(stack)-1] = PackExternAsAny(v)
				}
				typeStack[len(typeStack)-1] = ValueTypeAnyref
			case OpcodeGCExternConvertAny:
				if len(typeStack) < 1 {
					return nil, 0, errors.New("extern.convert_any: stack underflow")
				}
				v := stack[len(stack)-1]
				if v != 0 && IsTaggedExternAsAny(v) {
					stack[len(stack)-1] = UnpackExternAsAny(v)
				}
				typeStack[len(typeStack)-1] = ValueTypeExternref
			default:
				return nil, 0, fmt.Errorf("GC sub-opcode %#x is not yet supported in const expressions", sub)
			}
		default:
			return nil, 0, fmt.Errorf("invalid opcode for const expression: 0x%x", opCode)
		}
	}
}

func evaluateConstExprInModuleInstance(e *ConstantExpression, m *ModuleInstance) []uint64 {
	v, _, _ := evaluateConstExprWithModule(
		e,
		func(globalIndex Index) (ValueType, uint64, uint64, error) {
			g := m.Globals[globalIndex]
			return g.Type.ValType, g.Val, g.ValHi, nil
		},
		func(funcIndex Index) (Reference, error) {
			return m.Engine.FunctionInstanceReference(funcIndex), nil
		},
		m.Source,
		m,
	)
	return v
}

func NewConstantExpressionFromOpcode(
	opcode byte, opData []byte,
) ConstantExpression {
	data := make([]byte, 0, 3+len(opData)) // 2 for opcode and optional vec prefix, 1 for end
	if opcode == OpcodeVecV128Const {
		data = append(data, OpcodeVecPrefix)
	}
	data = append(data, opcode)
	data = append(data, opData...)
	data = append(data, OpcodeEnd)
	return ConstantExpression{Data: data}
}

func NewConstantExpressionFromI32(val int32) ConstantExpression {
	return NewConstantExpressionFromOpcode(OpcodeI32Const, leb128.EncodeInt32(val))
}

func NewConstantExpressionFromI64(val int64) ConstantExpression {
	return NewConstantExpressionFromOpcode(OpcodeI64Const, leb128.EncodeInt64(val))
}

// encodeFieldValueForConst converts an operand-stack uint64 to the
// Go-typed value stored in WasmStruct.Fields / WasmArray.Elements,
// using the supplied FieldType to interpret packed vs numeric storage.
func encodeFieldValueForConst(f FieldType, raw uint64) any {
	switch f.Kind() {
	case ValueTypeI8.Kind():
		return NarrowI8(int32(uint32(raw)))
	case ValueTypeI16.Kind():
		return NarrowI16(int32(uint32(raw)))
	}
	switch f.AsImmutable() {
	case ValueTypeI32:
		return int32(uint32(raw))
	case ValueTypeI64:
		return int64(raw)
	case ValueTypeF32:
		return math.Float32frombits(uint32(raw))
	case ValueTypeF64:
		return math.Float64frombits(raw)
	}
	if f.IsRef() {
		if raw == 0 {
			return nil
		}
		if IsGCRef(raw) {
			if IsGCStructRef(raw) {
				return (*WasmStruct)(UntagGCPointer(raw))
			}
			return (*WasmArray)(UntagGCPointer(raw))
		}
	}
	return raw
}
