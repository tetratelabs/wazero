package binary

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func ensureElementKindFuncRef(r *bytes.Reader) error {
	elemKind, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read element prefix: %w", err)
	}
	if elemKind != 0x0 { // ElemKind is fixed to 0x0 now: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/binary/modules.html#element-section
		return fmt.Errorf("element kind must be zero but was 0x%x", elemKind)
	}
	return nil
}

func decodeElementInitValueVector(r *bytes.Reader) ([]*wasm.Index, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	vec := make([]*wasm.Index, vs)
	for i := range vec {
		u32, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read function index: %w", err)
		}
		vec[i] = &u32
	}
	return vec, nil
}

func decodeElementConstExprVector(r *bytes.Reader, enabledFeatures wasm.Features) ([]*wasm.Index, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}
	vec := make([]*wasm.Index, vs)
	for i := range vec {
		expr, err := decodeConstantExpression(r, enabledFeatures)
		if err != nil {
			return nil, err
		}
		switch expr.Opcode {
		case wasm.OpcodeRefFunc:
			v, _, _ := leb128.DecodeUint32(bytes.NewReader(expr.Data))
			vec[i] = &v
		case wasm.OpcodeRefNull:
			// vec[i] is already nil, so nothing to do.
		default:
			return nil, fmt.Errorf("const expr must be either ref.null or ref.func but was %s", wasm.InstructionName(expr.Opcode))
		}
	}
	return vec, nil
}

func decodeElementRefType(r *bytes.Reader) (ret wasm.RefType, err error) {
	ret, err = r.ReadByte()
	if err != nil {
		err = fmt.Errorf("read element ref type: %w", err)
		return
	}
	if ret != wasm.RefTypeFuncref {
		// TODO: this will be relaxed to accept externref when we implement the reference-types proposal.
		err = errors.New("ref type must be funcref for element")
	}
	return
}

func decodeElementSegment(r *bytes.Reader, enabledFeatures wasm.Features) (*wasm.ElementSegment, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read element prefix: %w", err)
	}

	if prefix != 0 {
		if err := enabledFeatures.Require(wasm.FeatureBulkMemoryOperations); err != nil {
			return nil, fmt.Errorf("non-zero prefix for element segment is invalid as %w", err)
		}
	}

	// Encoding depends on the prefix and described at https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/binary/modules.html#element-section
	switch prefix {
	case 0:
		// Legacy prefix which is WebAssembly 1.0 compatible.
		expr, err := decodeConstantExpression(r, enabledFeatures)
		if err != nil {
			return nil, fmt.Errorf("read expr for offset: %w", err)
		}

		init, err := decodeElementInitValueVector(r)
		if err != nil {
			return nil, err
		}

		return &wasm.ElementSegment{
			OffsetExpr: expr,
			Init:       init,
			Type:       wasm.RefTypeFuncref,
			Mode:       wasm.ElementModeActive,
		}, nil
	case 1:
		// Prefix 1 requires funcref.
		if err = ensureElementKindFuncRef(r); err != nil {
			return nil, err
		}

		init, err := decodeElementInitValueVector(r)
		if err != nil {
			return nil, err
		}
		return &wasm.ElementSegment{
			Init: init,
			Type: wasm.RefTypeFuncref,
			Mode: wasm.ElementModePassive,
		}, nil
	case 2:
		tableIndex, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("get size of vector: %w", err)
		}

		if tableIndex != 0 {
			// TODO: this will be relaxed after reference type proposal impl.
			return nil, fmt.Errorf("table index must be zero but was %d", tableIndex)
		}

		expr, err := decodeConstantExpression(r, enabledFeatures)
		if err != nil {
			return nil, fmt.Errorf("read expr for offset: %w", err)
		}

		// Prefix 2 requires funcref.
		if err = ensureElementKindFuncRef(r); err != nil {
			return nil, err
		}

		init, err := decodeElementInitValueVector(r)
		if err != nil {
			return nil, err
		}
		return &wasm.ElementSegment{
			OffsetExpr: expr,
			Init:       init,
			Type:       wasm.RefTypeFuncref,
			Mode:       wasm.ElementModeActive,
		}, nil
	case 3:
		// Prefix 3 requires funcref.
		if err = ensureElementKindFuncRef(r); err != nil {
			return nil, err
		}
		init, err := decodeElementInitValueVector(r)
		if err != nil {
			return nil, err
		}
		return &wasm.ElementSegment{
			Init: init,
			Type: wasm.RefTypeFuncref,
			Mode: wasm.ElementModeDeclarative,
		}, nil
	case 4:
		expr, err := decodeConstantExpression(r, enabledFeatures)
		if err != nil {
			return nil, fmt.Errorf("read expr for offset: %w", err)
		}

		init, err := decodeElementConstExprVector(r, enabledFeatures)
		if err != nil {
			return nil, err
		}

		return &wasm.ElementSegment{
			OffsetExpr: expr,
			Init:       init,
			Type:       wasm.RefTypeFuncref,
			Mode:       wasm.ElementModeActive,
		}, nil
	case 5:
		refType, err := decodeElementRefType(r)
		if err != nil {
			return nil, err
		}
		init, err := decodeElementConstExprVector(r, enabledFeatures)
		if err != nil {
			return nil, err
		}
		return &wasm.ElementSegment{
			Init: init,
			Type: refType,
			Mode: wasm.ElementModePassive,
		}, nil
	case 6:
		tableIndex, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("get size of vector: %w", err)
		}

		if tableIndex != 0 {
			// TODO: this will be relaxed after reference type proposal impl.
			return nil, fmt.Errorf("table index must be zero but was %d", tableIndex)
		}
		expr, err := decodeConstantExpression(r, enabledFeatures)
		if err != nil {
			return nil, fmt.Errorf("read expr for offset: %w", err)
		}

		refType, err := decodeElementRefType(r)
		if err != nil {
			return nil, err
		}

		init, err := decodeElementConstExprVector(r, enabledFeatures)
		if err != nil {
			return nil, err
		}

		return &wasm.ElementSegment{
			OffsetExpr: expr,
			Init:       init,
			Type:       refType,
			Mode:       wasm.ElementModeActive,
		}, nil
	case 7:
		refType, err := decodeElementRefType(r)
		if err != nil {
			return nil, err
		}
		init, err := decodeElementConstExprVector(r, enabledFeatures)
		if err != nil {
			return nil, err
		}
		return &wasm.ElementSegment{
			Init: init,
			Type: refType,
			Mode: wasm.ElementModeDeclarative,
		}, nil
	default:
		return nil, fmt.Errorf("invalid element segment prefix: 0x%x", prefix)
	}
}

// encodeCode returns the wasm.ElementSegment encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#element-section%E2%91%A0
func encodeElement(e *wasm.ElementSegment) (ret []byte) {
	if e.Mode == wasm.ElementModeActive {
		// As of WebAssembly 2.0, multiple tables are not supported.
		ret = append(ret, leb128.EncodeInt32(0)...)
		ret = append(ret, encodeConstantExpression(e.OffsetExpr)...)
		ret = append(ret, leb128.EncodeUint32(uint32(len(e.Init)))...)
		for _, idx := range e.Init {
			ret = append(ret, leb128.EncodeInt32(int32(*idx))...)
		}
	} else {
		panic("TODO: support encoding for non-active elements in bulk-memory-operations proposal")
	}
	return
}
