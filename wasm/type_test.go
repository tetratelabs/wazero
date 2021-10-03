package wasm

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFunctionType(t *testing.T) {
	t.Run("ng", func(t *testing.T) {
		buf := []byte{0x00}
		_, err := readFunctionType(bytes.NewBuffer(buf))
		assert.True(t, errors.Is(err, ErrInvalidByte))
		t.Log(err)
	})

	for i, c := range []struct {
		bytes []byte
		exp   *FunctionType
	}{
		{
			bytes: []byte{0x60, 0x0, 0x0},
			exp: &FunctionType{
				InputTypes:  []ValueType{},
				ReturnTypes: []ValueType{},
			},
		},
		{
			bytes: []byte{0x60, 0x2, 0x7f, 0x7e, 0x0},
			exp: &FunctionType{
				InputTypes:  []ValueType{ValueTypeI32, ValueTypeI64},
				ReturnTypes: []ValueType{},
			},
		},
		{
			bytes: []byte{0x60, 0x1, 0x7e, 0x2, 0x7f, 0x7e},
			exp: &FunctionType{
				InputTypes:  []ValueType{ValueTypeI64},
				ReturnTypes: []ValueType{ValueTypeI32, ValueTypeI64},
			},
		},
		{
			bytes: []byte{0x60, 0x0, 0x2, 0x7f, 0x7e},
			exp: &FunctionType{
				InputTypes:  []ValueType{},
				ReturnTypes: []ValueType{ValueTypeI32, ValueTypeI64},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readFunctionType(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestReadLimitsType(t *testing.T) {
	for i, c := range []struct {
		bytes []byte
		exp   *LimitsType
	}{
		{bytes: []byte{0x00, 0xa}, exp: &LimitsType{Min: 10}},
		{bytes: []byte{0x01, 0xa, 0xa}, exp: &LimitsType{Min: 10, Max: uint32Ptr(10)}},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readLimitsType(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func uint32Ptr(in uint32) *uint32 {
	return &in
}

func TestReadTableType(t *testing.T) {
	t.Run("ng", func(t *testing.T) {
		buf := []byte{0x00}
		_, err := readTableType(bytes.NewBuffer(buf))
		require.True(t, errors.Is(err, ErrInvalidByte))
		t.Log(err)
	})

	for i, c := range []struct {
		bytes []byte
		exp   *TableType
	}{
		{
			bytes: []byte{0x70, 0x00, 0xa},
			exp: &TableType{
				ElemType: 0x70,
				Limit:    &LimitsType{Min: 10},
			},
		},
		{
			bytes: []byte{0x70, 0x01, 0x01, 0xa},
			exp: &TableType{
				ElemType: 0x70,
				Limit:    &LimitsType{Min: 1, Max: uint32Ptr(10)},
			},
		},
	} {
		c := c
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readTableType(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestReadMemoryType(t *testing.T) {
	for i, c := range []struct {
		bytes []byte
		exp   *MemoryType
	}{
		{bytes: []byte{0x00, 0xa}, exp: &MemoryType{Min: 10}},
		{bytes: []byte{0x01, 0xa, 0xa}, exp: &MemoryType{Min: 10, Max: uint32Ptr(10)}},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readMemoryType(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestReadGlobalType(t *testing.T) {
	t.Run("ng", func(t *testing.T) {
		buf := []byte{0x7e, 0x3}
		_, err := readGlobalType(bytes.NewBuffer(buf))
		require.True(t, errors.Is(err, ErrInvalidByte))
		t.Log(err)
	})

	for i, c := range []struct {
		bytes []byte
		exp   *GlobalType
	}{
		{bytes: []byte{0x7e, 0x00}, exp: &GlobalType{Value: ValueTypeI64, Mutable: false}},
		{bytes: []byte{0x7e, 0x01}, exp: &GlobalType{Value: ValueTypeI64, Mutable: true}},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readGlobalType(bytes.NewBuffer(c.bytes))
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}
