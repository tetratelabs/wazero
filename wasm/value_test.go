package wasm

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadValueTypes(t *testing.T) {
	for i, c := range []struct {
		bytes []byte
		num   uint32
		exp   []ValueType
	}{
		{
			bytes: []byte{0x7e}, num: 1, exp: []ValueType{ValueTypeI64},
		},
		{
			bytes: []byte{0x7f, 0x7e}, num: 2, exp: []ValueType{ValueTypeI32, ValueTypeI64},
		},
		{
			bytes: []byte{0x7f, 0x7e, 0x7d}, num: 2, exp: []ValueType{ValueTypeI32, ValueTypeI64},
		},
		{
			bytes: []byte{0x7f, 0x7e, 0x7d, 0x7c}, num: 4,
			exp: []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := readValueTypes(bytes.NewBuffer(c.bytes), c.num)
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestReadNameValue(t *testing.T) {
	exp := "abcdefghij"
	buf := []byte{0x0a}
	buf = append(buf, exp...)
	actual, err := readNameValue(bytes.NewBuffer(buf))
	require.NoError(t, err)
	assert.Equal(t, exp, actual)
}

func TestHasSameValues(t *testing.T) {
	for _, c := range []struct {
		a, b []ValueType
		exp  bool
	}{
		{a: []ValueType{}, exp: true},
		{a: []ValueType{}, b: []ValueType{}, exp: true},
		{a: []ValueType{ValueTypeF64}, exp: false},
		{a: []ValueType{ValueTypeF64}, b: []ValueType{ValueTypeF64}, exp: true},
	} {
		assert.Equal(t, c.exp, hasSameSignature(c.a, c.b))
	}
}
