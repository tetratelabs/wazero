package wasm

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadImportDesc(t *testing.T) {
	t.Run("ng", func(t *testing.T) {
		buf := []byte{0x04}
		_, err := readImportDesc(bytes.NewBuffer(buf))
		require.True(t, errors.Is(err, ErrInvalidByte))
		t.Log(err)
	})

	for i, c := range []struct {
		bytes []byte
		exp   *ImportDesc
	}{
		{
			bytes: []byte{0x00, 0x0a},
			exp: &ImportDesc{
				Kind:         0,
				TypeIndexPtr: uint32Ptr(10),
			},
		},
		{
			bytes: []byte{0x01, 0x70, 0x0, 0x0a},
			exp: &ImportDesc{
				Kind: 1,
				TableTypePtr: &TableType{
					ElemType: 0x70,
					Limit:    &LimitsType{Min: 10},
				},
			},
		},
		{
			bytes: []byte{0x02, 0x0, 0x0a},
			exp: &ImportDesc{
				Kind:       2,
				MemTypePtr: &MemoryType{Min: 10},
			},
		},
		{
			bytes: []byte{0x03, 0x7e, 0x01},
			exp: &ImportDesc{
				Kind:          3,
				GlobalTypePtr: &GlobalType{ValType: ValueTypeI64, Mutable: true},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readImportDesc(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})

	}
}

func TestReadImportSegment(t *testing.T) {
	exp := &ImportSegment{
		Module: "abc",
		Name:   "ABC",
		Desc:   &ImportDesc{Kind: 0, TypeIndexPtr: uint32Ptr(10)},
	}

	buf := []byte{byte(len(exp.Module))}
	buf = append(buf, exp.Module...)
	buf = append(buf, byte(len(exp.Name)))
	buf = append(buf, exp.Name...)
	buf = append(buf, 0x00, 0x0a)

	actual, err := readImportSegment(bytes.NewBuffer(buf))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}

func TestReadGlobalSegment(t *testing.T) {
	exp := &GlobalSegment{
		Type: &GlobalType{ValType: ValueTypeI64, Mutable: false},
		Init: &ConstantExpression{
			OptCode: OptCodeI64Const,
			Data:    []byte{0x01},
		},
	}

	buf := []byte{0x7e, 0x00, 0x42, 0x01, 0x0b}
	actual, err := readGlobalSegment(bytes.NewBuffer(buf))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}

func TestReadExportDesc(t *testing.T) {
	t.Run("ng", func(t *testing.T) {
		buf := []byte{0x04}
		_, err := readExportDesc(bytes.NewBuffer(buf))
		require.True(t, errors.Is(err, ErrInvalidByte))
		t.Log(err)
	})

	for i, c := range []struct {
		bytes []byte
		exp   *ExportDesc
	}{
		{
			bytes: []byte{0x00, 0x0a},
			exp:   &ExportDesc{Kind: 0, Index: 10},
		},
		{
			bytes: []byte{0x01, 0x05},
			exp:   &ExportDesc{Kind: 1, Index: 5},
		},
		{
			bytes: []byte{0x02, 0x01},
			exp:   &ExportDesc{Kind: 2, Index: 1},
		},
		{
			bytes: []byte{0x03, 0x0b},
			exp:   &ExportDesc{Kind: 3, Index: 11},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readExportDesc(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})

	}
}

func TestReadExportSegment(t *testing.T) {
	exp := &ExportSegment{
		Name: "ABC",
		Desc: &ExportDesc{Kind: 0, Index: 10},
	}

	buf := []byte{byte(len(exp.Name))}
	buf = append(buf, exp.Name...)
	buf = append(buf, 0x00, 0x0a)

	actual, err := readExportSegment(bytes.NewBuffer(buf))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}

func TestReadElementSegment(t *testing.T) {
	for i, c := range []struct {
		bytes []byte
		exp   *ElementSegment
	}{
		{
			bytes: []byte{0xa, 0x41, 0x1, 0x0b, 0x02, 0x05, 0x07},
			exp: &ElementSegment{
				TableIndex: 10,
				OffsetExpr: &ConstantExpression{
					OptCode: OptCodeI32Const,
					Data:    []byte{0x01},
				},
				Init: []uint32{5, 7},
			},
		},
		{
			bytes: []byte{0x3, 0x41, 0x04, 0x0b, 0x01, 0x0a},
			exp: &ElementSegment{
				TableIndex: 3,
				OffsetExpr: &ConstantExpression{
					OptCode: OptCodeI32Const,
					Data:    []byte{0x04},
				},
				Init: []uint32{10},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readElementSegment(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestReadCodeSegment(t *testing.T) {
	buf := []byte{0x9, 0x1, 0x1, 0x1, 0x1, 0x1, 0x12, 0x3, 0x01, 0x0b}
	exp := &CodeSegment{
		NumLocals: 0x01,
		Body:      []byte{0x1, 0x1, 0x12, 0x3, 0x01},
	}
	actual, err := readCodeSegment(bytes.NewBuffer(buf))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}

func TestDataSegment(t *testing.T) {
	for i, c := range []struct {
		bytes []byte
		exp   *DataSegment
	}{
		{
			bytes: []byte{0x0, 0x41, 0x1, 0x0b, 0x02, 0x05, 0x07},
			exp: &DataSegment{
				OffsetExpression: &ConstantExpression{
					OptCode: OptCodeI32Const,
					Data:    []byte{0x01},
				},
				Init: []byte{5, 7},
			},
		},
		{
			bytes: []byte{0x0, 0x41, 0x04, 0x0b, 0x01, 0x0a},
			exp: &DataSegment{
				OffsetExpression: &ConstantExpression{
					OptCode: OptCodeI32Const,
					Data:    []byte{0x04},
				},
				Init: []byte{0x0a},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readDataSegment(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}
