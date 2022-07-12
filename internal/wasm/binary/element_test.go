package binary

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func Test_ensureElementKindFuncRef(t *testing.T) {
	require.NoError(t, ensureElementKindFuncRef(bytes.NewReader([]byte{0x0})))
	require.Error(t, ensureElementKindFuncRef(bytes.NewReader([]byte{0x1})))
}

func Test_decodeElementInitValueVector(t *testing.T) {
	tests := []struct {
		in  []byte
		exp []*wasm.Index
	}{
		{
			in:  []byte{0},
			exp: []*wasm.Index{},
		},
		{
			in:  []byte{5, 1, 2, 3, 4, 5},
			exp: []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := decodeElementInitValueVector(bytes.NewReader(tc.in))
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func Test_decodeElementConstExprVector(t *testing.T) {
	tests := []struct {
		in       []byte
		refType  wasm.RefType
		exp      []*wasm.Index
		features wasm.Features
	}{
		{
			in:       []byte{0},
			exp:      []*wasm.Index{},
			refType:  wasm.RefTypeFuncref,
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				2, // Two indexes.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc, 100, wasm.OpcodeEnd,
			},
			exp:      []*wasm.Index{nil, uint32Ptr(100)},
			refType:  wasm.RefTypeFuncref,
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				3, // Three indexes.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
			},
			exp:      []*wasm.Index{nil, uint32Ptr(165675008), nil},
			refType:  wasm.RefTypeFuncref,
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				2, // Two indexes.
				wasm.OpcodeRefNull, wasm.RefTypeExternref, wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeExternref, wasm.OpcodeEnd,
			},
			exp:      []*wasm.Index{nil, nil},
			refType:  wasm.RefTypeExternref,
			features: wasm.FeatureBulkMemoryOperations,
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := decodeElementConstExprVector(bytes.NewReader(tc.in), tc.refType, tc.features)
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func Test_decodeElementConstExprVector_errors(t *testing.T) {
	tests := []struct {
		name     string
		in       []byte
		refType  wasm.RefType
		expErr   string
		features wasm.Features
	}{
		{
			name:   "eof",
			expErr: "failed to get the size of constexpr vector: EOF",
		},
		{
			name:   "feature",
			in:     []byte{1, wasm.OpcodeRefNull, wasm.RefTypeExternref, wasm.OpcodeEnd},
			expErr: "ref.null is not supported as feature \"bulk-memory-operations\" is disabled",
		},
		{
			name:     "type mismatch - ref.null",
			in:       []byte{1, wasm.OpcodeRefNull, wasm.RefTypeExternref, wasm.OpcodeEnd},
			refType:  wasm.RefTypeFuncref,
			features: wasm.Features20220419,
			expErr:   "element type mismatch: want funcref, but constexpr has externref",
		},
		{
			name:     "type mismatch - ref.null",
			in:       []byte{1, wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd},
			refType:  wasm.RefTypeExternref,
			features: wasm.Features20220419,
			expErr:   "element type mismatch: want externref, but constexpr has funcref",
		},
		{
			name:     "invalid ref type",
			in:       []byte{1, wasm.OpcodeRefNull, 0xff, wasm.OpcodeEnd},
			refType:  wasm.RefTypeExternref,
			features: wasm.Features20220419,
			expErr:   "invalid type for ref.null: 0xff",
		},
		{
			name:     "type mismatch - ref.fuc",
			in:       []byte{1, wasm.OpcodeRefFunc, 0, wasm.OpcodeEnd},
			refType:  wasm.RefTypeExternref,
			features: wasm.Features20220419,
			expErr:   "element type mismatch: want externref, but constexpr has funcref",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeElementConstExprVector(bytes.NewReader(tc.in), tc.refType, tc.features)
			require.EqualError(t, err, tc.expErr)
		})
	}
}

func TestDecodeElementSegment(t *testing.T) {
	tests := []struct {
		name     string
		in       []byte
		exp      *wasm.ElementSegment
		expErr   string
		features wasm.Features
	}{
		{
			name: "legacy",
			in: []byte{
				0, // Prefix (which is previously the table index fixed to zero)
				// Offset const expr.
				wasm.OpcodeI32Const, 1, wasm.OpcodeEnd,
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{1}},
				Init:       []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "legacy multi byte const expr data",
			in: []byte{
				0, // Prefix (which is previously the table index fixed to zero)
				// Offset const expr.
				wasm.OpcodeI32Const, 0x80, 0, wasm.OpcodeEnd,
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0x80, 0}},
				Init:       []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{

			name: "passive value vector",
			in: []byte{
				1, // Prefix.
				0, // Elem kind must be fixed to zero.
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			exp: &wasm.ElementSegment{
				Init: []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
				Mode: wasm.ElementModePassive,
				Type: wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "active with table index encoded.",
			in: []byte{
				2, // Prefix.
				0,
				// Offset const expr.
				wasm.OpcodeI32Const, 0x80, 0, wasm.OpcodeEnd,
				0, // Elem kind must be fixed to zero.
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0x80, 0}},
				Init:       []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{

			name: "active with non zero table index encoded.",
			in: []byte{
				2, // Prefix.
				10,
				// Offset const expr.
				wasm.OpcodeI32Const, 0x80, 0, wasm.OpcodeEnd,
				0, // Elem kind must be fixed to zero.
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0x80, 0}},
				Init:       []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
				TableIndex: 10,
			},
			features: wasm.FeatureBulkMemoryOperations | wasm.FeatureReferenceTypes,
		},
		{

			name: "active with non zero table index encoded but reference-types disabled",
			in: []byte{
				2, // Prefix.
				10,
				// Offset const expr.
				wasm.OpcodeI32Const, 0x80, 0, wasm.OpcodeEnd,
				0, // Elem kind must be fixed to zero.
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			expErr:   `table index must be zero but was 10: feature "reference-types" is disabled`,
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "declarative",
			in: []byte{
				3, // Prefix.
				0, // Elem kind must be fixed to zero.
				// Init vector.
				5, 1, 2, 3, 4, 5,
			},
			exp: &wasm.ElementSegment{
				Init: []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4), uint32Ptr(5)},
				Mode: wasm.ElementModeDeclarative,
				Type: wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "active const expr vector",
			in: []byte{
				4, // Prefix.
				// Offset expr.
				wasm.OpcodeI32Const, 0x80, 1, wasm.OpcodeEnd,
				// Init const expr vector.
				3, // number of const expr.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0x80, 1}},
				Init:       []*wasm.Index{nil, uint32Ptr(165675008), nil},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "passive const expr vector - funcref",
			in: []byte{
				5, // Prefix.
				wasm.RefTypeFuncref,
				// Init const expr vector.
				3, // number of const expr.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
			},
			exp: &wasm.ElementSegment{
				Init: []*wasm.Index{nil, uint32Ptr(165675008), nil},
				Mode: wasm.ElementModePassive,
				Type: wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "passive const expr vector - unknown ref type",
			in: []byte{
				5, // Prefix.
				0xff,
			},
			expErr:   `ref type must be funcref or externref for element as of WebAssembly 2.0`,
			features: wasm.FeatureBulkMemoryOperations | wasm.FeatureReferenceTypes,
		},
		{
			name: "active with table index and const expr vector",
			in: []byte{
				6, // Prefix.
				0,
				// Offset expr.
				wasm.OpcodeI32Const, 0x80, 1, wasm.OpcodeEnd,
				wasm.RefTypeFuncref,
				// Init const expr vector.
				3, // number of const expr.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0x80, 1}},
				Init:       []*wasm.Index{nil, uint32Ptr(165675008), nil},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "active with non zero table index and const expr vector",
			in: []byte{
				6, // Prefix.
				10,
				// Offset expr.
				wasm.OpcodeI32Const, 0x80, 1, wasm.OpcodeEnd,
				wasm.RefTypeFuncref,
				// Init const expr vector.
				3, // number of const expr.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
			},
			exp: &wasm.ElementSegment{
				OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0x80, 1}},
				Init:       []*wasm.Index{nil, uint32Ptr(165675008), nil},
				Mode:       wasm.ElementModeActive,
				Type:       wasm.RefTypeFuncref,
				TableIndex: 10,
			},
			features: wasm.FeatureBulkMemoryOperations | wasm.FeatureReferenceTypes,
		},
		{
			name: "active with non zero table index and const expr vector but feature disabled",
			in: []byte{
				6, // Prefix.
				10,
				// Offset expr.
				wasm.OpcodeI32Const, 0x80, 1, wasm.OpcodeEnd,
				wasm.RefTypeFuncref,
				// Init const expr vector.
				3, // number of const expr.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
			},
			expErr:   `table index must be zero but was 10: feature "reference-types" is disabled`,
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			name: "declarative const expr vector",
			in: []byte{
				7, // Prefix.
				wasm.RefTypeFuncref,
				// Init const expr vector.
				2, // number of const expr.
				wasm.OpcodeRefNull, wasm.RefTypeFuncref, wasm.OpcodeEnd,
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
			},
			exp: &wasm.ElementSegment{
				Init: []*wasm.Index{nil, uint32Ptr(165675008)},
				Mode: wasm.ElementModeDeclarative,
				Type: wasm.RefTypeFuncref,
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			actual, err := decodeElementSegment(bytes.NewReader(tc.in), tc.features)
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, actual, tc.exp)
			}
		})
	}
}

func TestDecodeElementSegment_errors(t *testing.T) {
	_, err := decodeElementSegment(bytes.NewReader([]byte{1}), wasm.FeatureMultiValue)
	require.EqualError(t, err, `non-zero prefix for element segment is invalid as feature "bulk-memory-operations" is disabled`)
}
