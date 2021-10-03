package wasm

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModule_executeConstExpression(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, expr := range []*ConstantExpression{
			{OptCode: 0xa},
			{OptCode: OptCodeGlobalGet, Data: []byte{0x2}},
		} {
			m := &Module{IndexSpace: new(ModuleIndexSpace)}
			_, err := m.executeConstExpression(expr)
			assert.Error(t, err)
			t.Log(err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		for _, c := range []struct {
			m    Module
			expr *ConstantExpression
			val  interface{}
		}{
			{
				expr: &ConstantExpression{
					OptCode: OptCodeI64Const,
					Data:    []byte{0x5},
				},
				val: uint64(5),
			},
			{
				expr: &ConstantExpression{
					OptCode: OptCodeI32Const,
					Data:    []byte{0x5},
				},
				val: uint32(5),
			},
			{
				expr: &ConstantExpression{
					OptCode: OptCodeF32Const,
					Data:    []byte{0x40, 0xe1, 0x47, 0x40},
				},
				val: float32(3.1231232),
			},
			{
				expr: &ConstantExpression{
					OptCode: OptCodeF64Const,
					Data:    []byte{0x5e, 0xc4, 0xd8, 0xf9, 0x27, 0xfc, 0x08, 0x40},
				},
				val: 3.1231231231,
			},
		} {

			actual, err := c.m.executeConstExpression(c.expr)
			require.NoError(t, err)
			assert.Equal(t, c.val, actual)
		}
	})
}

func TestReadConstantExpression(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, b := range [][]byte{
			{}, {0xaa}, {0x41, 0x1}, {0x41, 0x1, 0x41},
		} {
			_, err := readConstantExpression(bytes.NewBuffer(b))
			assert.Error(t, err)
			t.Log(err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		for _, c := range []struct {
			bytes []byte
			exp   *ConstantExpression
		}{
			{
				bytes: []byte{0x42, 0x01, 0x0b},
				exp:   &ConstantExpression{OptCode: OptCodeI64Const, Data: []byte{0x01}},
			},
			{
				bytes: []byte{0x43, 0x40, 0xe1, 0x47, 0x40, 0x0b},
				exp:   &ConstantExpression{OptCode: OptCodeF32Const, Data: []byte{0x40, 0xe1, 0x47, 0x40}},
			},
			{
				bytes: []byte{0x23, 0x01, 0x0b},
				exp:   &ConstantExpression{OptCode: OptCodeGlobalGet, Data: []byte{0x01}},
			},
		} {
			actual, err := readConstantExpression(bytes.NewBuffer(c.bytes))
			assert.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		}
	})
}

func TestReadFloat32(t *testing.T) {
	var exp float32 = 3.1231231231
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, math.Float32bits(exp))
	actual, err := readFloat32(bytes.NewBuffer(bs))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}

func TestReadFloat64(t *testing.T) {
	exp := 3.1231231231
	bs := make([]byte, 8)
	binary.LittleEndian.PutUint64(bs, math.Float64bits(exp))

	actual, err := readFloat64(bytes.NewBuffer(bs))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}
