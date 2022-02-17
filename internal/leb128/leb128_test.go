package leb128

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeUint32(t *testing.T) {
	for _, c := range []struct {
		input    uint32
		expected []byte
	}{
		{input: 0, expected: []byte{0x00}},
		{input: 1, expected: []byte{0x01}},
		{input: 4, expected: []byte{0x04}},
		{input: 16256, expected: []byte{0x80, 0x7f}},
		{input: 624485, expected: []byte{0xe5, 0x8e, 0x26}},
		{input: 165675008, expected: []byte{0x80, 0x80, 0x80, 0x4f}},
		{input: 0xffffffff, expected: []byte{0xff, 0xff, 0xff, 0xff, 0xf}},
	} {
		require.Equal(t, c.expected, EncodeUint32(c.input))
	}
}

func TestDecodeUint32(t *testing.T) {
	for _, c := range []struct {
		bytes  []byte
		exp    uint32
		expErr bool
	}{
		{bytes: []byte{0xff, 0xff, 0xff, 0xff, 0xf}, exp: 0xffffffff},
		{bytes: []byte{0x00}, exp: 0},
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0x01}, exp: 1},
		{bytes: []byte{0x80, 0x7f}, exp: 16256},
		{bytes: []byte{0xe5, 0x8e, 0x26}, exp: 624485},
		{bytes: []byte{0x80, 0x80, 0x80, 0x4f}, exp: 165675008},
		{bytes: []byte{0x83, 0x80, 0x80, 0x80, 0x80, 0x00}, expErr: true},
		{bytes: []byte{0x82, 0x80, 0x80, 0x80, 0x70}, expErr: true},
		{bytes: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, expErr: true},
	} {
		actual, num, err := DecodeUint32(bytes.NewReader(c.bytes))
		if c.expErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
			assert.Equal(t, uint64(len(c.bytes)), num)
		}
	}
}

func TestDecodeUint64(t *testing.T) {
	for _, c := range []struct {
		bytes  []byte
		exp    uint64
		expErr bool
	}{
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0x80, 0x7f}, exp: 16256},
		{bytes: []byte{0xe5, 0x8e, 0x26}, exp: 624485},
		{bytes: []byte{0x80, 0x80, 0x80, 0x4f}, exp: 165675008},
		{bytes: []byte{0x89, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x71}, expErr: true},
	} {
		actual, num, err := DecodeUint64(bytes.NewReader(c.bytes))
		if c.expErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
			assert.Equal(t, uint64(len(c.bytes)), num)
		}
	}
}

func TestDecodeInt32(t *testing.T) {
	for i, c := range []struct {
		bytes  []byte
		exp    int32
		expErr bool
	}{
		{bytes: []byte{0x13}, exp: 19},
		{bytes: []byte{0x00}, exp: 0},
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0xFF, 0x00}, exp: 127},
		{bytes: []byte{0x81, 0x01}, exp: 129},
		{bytes: []byte{0x7f}, exp: -1},
		{bytes: []byte{0x81, 0x7f}, exp: -127},
		{bytes: []byte{0xFF, 0x7e}, exp: -129},
		{bytes: []byte{0xff, 0xff, 0xff, 0xff, 0x0f}, expErr: true},
		{bytes: []byte{0xff, 0xff, 0xff, 0xff, 0x4f}, expErr: true},
		{bytes: []byte{0x80, 0x80, 0x80, 0x80, 0x70}, expErr: true},
	} {
		actual, num, err := DecodeInt32(bytes.NewReader(c.bytes))
		if c.expErr {
			assert.Error(t, err, fmt.Sprintf("%d-th got value %d", i, actual))
		} else {
			assert.NoError(t, err, i)
			assert.Equal(t, c.exp, actual, i)
			assert.Equal(t, uint64(len(c.bytes)), num, i)
		}
	}
}

func TestDecodeInt33AsInt64(t *testing.T) {
	for _, c := range []struct {
		bytes []byte
		exp   int64
	}{
		{bytes: []byte{0x00}, exp: 0},
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0x40}, exp: -64},
		{bytes: []byte{0x7f}, exp: -1},
		{bytes: []byte{0x7e}, exp: -2},
		{bytes: []byte{0x7d}, exp: -3},
		{bytes: []byte{0x7c}, exp: -4},
		{bytes: []byte{0xFF, 0x00}, exp: 127},
		{bytes: []byte{0x81, 0x01}, exp: 129},
		{bytes: []byte{0x7f}, exp: -1},
		{bytes: []byte{0x81, 0x7f}, exp: -127},
		{bytes: []byte{0xFF, 0x7e}, exp: -129},
	} {
		actual, num, err := DecodeInt33AsInt64(bytes.NewReader(c.bytes))
		require.NoError(t, err)
		assert.Equal(t, c.exp, actual)
		assert.Equal(t, uint64(len(c.bytes)), num)
	}
}

func TestDecodeInt64(t *testing.T) {
	for _, c := range []struct {
		bytes []byte
		exp   int64
	}{
		{bytes: []byte{0x00}, exp: 0},
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0xFF, 0x00}, exp: 127},
		{bytes: []byte{0x81, 0x01}, exp: 129},
		{bytes: []byte{0x7f}, exp: -1},
		{bytes: []byte{0x81, 0x7f}, exp: -127},
		{bytes: []byte{0xFF, 0x7e}, exp: -129},
		{bytes: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7f},
			exp: -9223372036854775808},
	} {
		actual, num, err := DecodeInt64(bytes.NewReader(c.bytes))
		require.NoError(t, err)
		assert.Equal(t, c.exp, actual)
		assert.Equal(t, uint64(len(c.bytes)), num)
	}
}
