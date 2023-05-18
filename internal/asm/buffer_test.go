package asm_test

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestCodeSegmentZeroValue(t *testing.T) {
	withCodeSegment(t, func(code *asm.CodeSegment) {
		require.Equal(t, uintptr(0), code.Addr())
		require.Equal(t, uintptr(0), code.Size())
		require.Equal(t, 0, code.Len())
		require.Equal(t, ([]byte)(nil), code.Bytes())

		buf := code.Next()
		require.Equal(t, 0, buf.Cap())
		require.Equal(t, 0, buf.Len())
		require.Equal(t, ([]byte)(nil), buf.Bytes())
	})
}

func TestCodeSegmentMapUnmap(t *testing.T) {
	withCodeSegment(t, func(code *asm.CodeSegment) {
		const size = 4096
		require.NoError(t, code.Map(size))
		require.NotEqual(t, uintptr(0), code.Addr())
		require.Equal(t, uintptr(size), code.Size())
		require.Equal(t, size, code.Len())
		require.NotEqual(t, ([]byte)(nil), code.Bytes())

		for i := 0; i < 3; i++ {
			require.NoError(t, code.Unmap())
			require.Equal(t, uintptr(0), code.Addr())
			require.Equal(t, uintptr(0), code.Size())
			require.Equal(t, 0, code.Len())
			require.Equal(t, ([]byte)(nil), code.Bytes())
		}
	})
}

func TestBufferAppendByte(t *testing.T) {
	withBuffer(t, func(buf asm.Buffer) {
		data := []byte("Hello World!")

		for i, c := range data {
			buf.AppendByte(c)
			require.NotEqual(t, 0, buf.Cap())
			require.Equal(t, i+1, buf.Len())
			require.Equal(t, data[:i+1], buf.Bytes())
		}
	})
}

func TestBufferAppendBytes(t *testing.T) {
	withBuffer(t, func(buf asm.Buffer) {
		buf.AppendBytes([]byte("Hello World!"))
		require.NotEqual(t, 0, buf.Cap())
		require.Equal(t, 12, buf.Len())
		require.Equal(t, []byte("Hello World!"), buf.Bytes())
	})
}

func TestBufferAppendUint32(t *testing.T) {
	withBuffer(t, func(buf asm.Buffer) {
		values := []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		bytes := unsafe.Slice(*(**byte)(unsafe.Pointer(&values)), 4*len(values))

		for i, v := range values {
			buf.AppendUint32(v)
			require.NotEqual(t, 0, buf.Cap())
			require.Equal(t, 4*(i+1), buf.Len())
			require.Equal(t, bytes[:4*(i+1)], buf.Bytes())
		}
	})
}

func TestBufferReset(t *testing.T) {
	withBuffer(t, func(buf asm.Buffer) {
		buf.AppendBytes([]byte("Hello World!"))
		require.NotEqual(t, 0, buf.Cap())
		require.Equal(t, 12, buf.Len())
		require.Equal(t, []byte("Hello World!"), buf.Bytes())

		buf.Reset()
		require.Equal(t, 0, buf.Len())
		require.Equal(t, []byte{}, buf.Bytes())
	})
}

func TestBufferTruncate(t *testing.T) {
	withBuffer(t, func(buf asm.Buffer) {
		buf.AppendBytes([]byte("Hello World!"))
		require.NotEqual(t, 0, buf.Cap())
		require.Equal(t, 12, buf.Len())
		require.Equal(t, []byte("Hello World!"), buf.Bytes())

		buf.Truncate(5)
		require.Equal(t, 5, buf.Len())
		require.Equal(t, []byte("Hello"), buf.Bytes())
	})
}

func withCodeSegment(t *testing.T, f func(*asm.CodeSegment)) {
	code := asm.NewCodeSegment(nil)
	defer func() { require.NoError(t, code.Unmap()) }()
	f(code)
}

func withBuffer(t *testing.T, f func(asm.Buffer)) {
	withCodeSegment(t, func(code *asm.CodeSegment) {
		// Repeat the test multiple times to ensure that Next works as expected.
		for i := 0; i < 10; i++ {
			f(code.Next())
		}
	})
}
