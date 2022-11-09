package leb128

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestEncode_DecodeInt32(t *testing.T) {
	for _, c := range []struct {
		input    int32
		expected []byte
	}{
		{input: -165675008, expected: []byte{0x80, 0x80, 0x80, 0xb1, 0x7f}},
		{input: -624485, expected: []byte{0x9b, 0xf1, 0x59}},
		{input: -16256, expected: []byte{0x80, 0x81, 0x7f}},
		{input: -4, expected: []byte{0x7c}},
		{input: -1, expected: []byte{0x7f}},
		{input: 0, expected: []byte{0x00}},
		{input: 1, expected: []byte{0x01}},
		{input: 4, expected: []byte{0x04}},
		{input: 16256, expected: []byte{0x80, 0xff, 0x0}},
		{input: 624485, expected: []byte{0xe5, 0x8e, 0x26}},
		{input: 165675008, expected: []byte{0x80, 0x80, 0x80, 0xcf, 0x0}},
		{input: int32(math.MaxInt32), expected: []byte{0xff, 0xff, 0xff, 0xff, 0x7}},
	} {
		require.Equal(t, c.expected, EncodeInt32(c.input))
		decoded, _, err := LoadInt32(c.expected)
		require.NoError(t, err)
		require.Equal(t, c.input, decoded)
	}
}

func TestEncode_DecodeInt64(t *testing.T) {
	for _, c := range []struct {
		input    int64
		expected []byte
	}{
		{input: -math.MaxInt32, expected: []byte{0x81, 0x80, 0x80, 0x80, 0x78}},
		{input: -165675008, expected: []byte{0x80, 0x80, 0x80, 0xb1, 0x7f}},
		{input: -624485, expected: []byte{0x9b, 0xf1, 0x59}},
		{input: -16256, expected: []byte{0x80, 0x81, 0x7f}},
		{input: -4, expected: []byte{0x7c}},
		{input: -1, expected: []byte{0x7f}},
		{input: 0, expected: []byte{0x00}},
		{input: 1, expected: []byte{0x01}},
		{input: 4, expected: []byte{0x04}},
		{input: 16256, expected: []byte{0x80, 0xff, 0x0}},
		{input: 624485, expected: []byte{0xe5, 0x8e, 0x26}},
		{input: 165675008, expected: []byte{0x80, 0x80, 0x80, 0xcf, 0x0}},
		{input: math.MaxInt32, expected: []byte{0xff, 0xff, 0xff, 0xff, 0x7}},
		{input: math.MaxInt64, expected: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}},
	} {
		require.Equal(t, c.expected, EncodeInt64(c.input))
		decoded, _, err := LoadInt64(c.expected)
		require.NoError(t, err)
		require.Equal(t, c.input, decoded)
	}
}

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
		{input: uint32(math.MaxUint32), expected: []byte{0xff, 0xff, 0xff, 0xff, 0xf}},
	} {
		require.Equal(t, c.expected, EncodeUint32(c.input))
	}
}

func TestEncodeUint64(t *testing.T) {
	for _, c := range []struct {
		input    uint64
		expected []byte
	}{
		{input: 0, expected: []byte{0x00}},
		{input: 1, expected: []byte{0x01}},
		{input: 4, expected: []byte{0x04}},
		{input: 16256, expected: []byte{0x80, 0x7f}},
		{input: 624485, expected: []byte{0xe5, 0x8e, 0x26}},
		{input: 165675008, expected: []byte{0x80, 0x80, 0x80, 0x4f}},
		{input: math.MaxUint32, expected: []byte{0xff, 0xff, 0xff, 0xff, 0xf}},
		{input: math.MaxUint64, expected: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1}},
	} {
		require.Equal(t, c.expected, EncodeUint64(c.input))
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
		{bytes: []byte{0x80, 0}, exp: 0},
		{bytes: []byte{0x80, 0x7f}, exp: 16256},
		{bytes: []byte{0xe5, 0x8e, 0x26}, exp: 624485},
		{bytes: []byte{0x80, 0x80, 0x80, 0x4f}, exp: 165675008},
		{bytes: []byte{0xff, 0xff, 0xff, 0xff, 0xf}, exp: math.MaxUint32},
		{bytes: []byte{0x83, 0x80, 0x80, 0x80, 0x80, 0x00}, expErr: true},
		{bytes: []byte{0x82, 0x80, 0x80, 0x80, 0x70}, expErr: true},
		{bytes: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, expErr: true},
	} {
		actual, num, err := LoadUint32(c.bytes)
		if c.expErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, c.exp, actual)
			require.Equal(t, uint64(len(c.bytes)), num)
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
		{bytes: []byte{0xff, 0xff, 0xff, 0xff, 0xf}, exp: math.MaxUint32},
		{bytes: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1}, exp: math.MaxUint64},
		{bytes: []byte{0x89, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x71}, expErr: true},
	} {
		actual, num, err := LoadUint64(c.bytes)
		if c.expErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, c.exp, actual)
			require.Equal(t, uint64(len(c.bytes)), num)
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
		actual, num, err := LoadInt32(c.bytes)
		if c.expErr {
			require.Error(t, err, fmt.Sprintf("%d-th got value %d", i, actual))
		} else {
			require.NoError(t, err, i)
			require.Equal(t, c.exp, actual, i)
			require.Equal(t, uint64(len(c.bytes)), num, i)
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
		require.Equal(t, c.exp, actual)
		require.Equal(t, uint64(len(c.bytes)), num)
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
		{
			bytes: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7f},
			exp:   -9223372036854775808,
		},
	} {
		actual, num, err := LoadInt64(c.bytes)
		require.NoError(t, err)
		require.Equal(t, c.exp, actual)
		require.Equal(t, uint64(len(c.bytes)), num)
	}
}
