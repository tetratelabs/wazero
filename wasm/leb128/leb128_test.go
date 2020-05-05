package leb128

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeUint32(t *testing.T) {
	for _, c := range []struct {
		bytes []byte
		exp   uint32
	}{
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0x80, 0x7f}, exp: 16256},
		{bytes: []byte{0xe5, 0x8e, 0x26}, exp: 624485},
		{bytes: []byte{0x80, 0x80, 0x80, 0x4f}, exp: 165675008},
		{bytes: []byte{0x89, 0x80, 0x80, 0x80, 0x01}, exp: 268435465},
	} {
		actual, num, err := DecodeUint32(bytes.NewReader(c.bytes))
		require.NoError(t, err)
		assert.Equal(t, c.exp, actual)
		assert.Equal(t, uint64(len(c.bytes)), num)
	}
}

func TestDecodeUint64(t *testing.T) {
	for _, c := range []struct {
		bytes []byte
		exp   uint64
	}{
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0x80, 0x7f}, exp: 16256},
		{bytes: []byte{0xe5, 0x8e, 0x26}, exp: 624485},
		{bytes: []byte{0x80, 0x80, 0x80, 0x4f}, exp: 165675008},
		{bytes: []byte{0x89, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}, exp: 9223372036854775817},
	} {
		actual, num, err := DecodeUint64(bytes.NewReader(c.bytes))
		require.NoError(t, err)
		assert.Equal(t, c.exp, actual)
		assert.Equal(t, uint64(len(c.bytes)), num)
	}
}

func TestDecodeInt32(t *testing.T) {
	for _, c := range []struct {
		bytes []byte
		exp   int32
	}{
		{bytes: []byte{0x00}, exp: 0},
		{bytes: []byte{0x04}, exp: 4},
		{bytes: []byte{0xFF, 0x00}, exp: 127},
		{bytes: []byte{0x81, 0x01}, exp: 129},
		{bytes: []byte{0x7f}, exp: -1},
		{bytes: []byte{0x81, 0x7f}, exp: -127},
		{bytes: []byte{0xFF, 0x7e}, exp: -129},
	} {
		actual, num, err := DecodeInt32(bytes.NewReader(c.bytes))
		require.NoError(t, err)
		assert.Equal(t, c.exp, actual)
		assert.Equal(t, uint64(len(c.bytes)), num)
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
