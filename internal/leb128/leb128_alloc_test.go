package leb128

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestLeb128NoAlloc ensures no allocation required in the leb128 package.
func TestLeb128NoAlloc(t *testing.T) {
	t.Run("LoadUint32", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkLoadUint32)
		require.Zero(t, result.AllocsPerOp())
	})
	t.Run("LoadUint64", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkLoadUint64)
		require.Zero(t, result.AllocsPerOp())
	})
	t.Run("LoadInt32", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkLoadInt32)
		require.Zero(t, result.AllocsPerOp())
	})
	t.Run("LoadInt64", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkLoadInt64)
		require.Zero(t, result.AllocsPerOp())
	})
	t.Run("DecodeUint32", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkDecodeUint32)
		require.Zero(t, result.AllocsPerOp())
	})
	t.Run("DecodeInt32", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkDecodeInt32)
		require.Zero(t, result.AllocsPerOp())
	})
	t.Run("DecodeInt64", func(t *testing.T) {
		result := testing.Benchmark(BenchmarkDecodeInt64)
		require.Zero(t, result.AllocsPerOp())
	})
}

func BenchmarkLoadUint32(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := LoadUint32([]byte{0x80, 0x80, 0x80, 0x4f})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadUint64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := LoadUint64([]byte{0x80, 0x80, 0x80, 0x4f})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadInt32(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := LoadInt32([]byte{0x80, 0x80, 0x80, 0x4f})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadInt64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := LoadInt64([]byte{0x80, 0x80, 0x80, 0x4f})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeUint32(b *testing.B) {
	b.ReportAllocs()
	data := []byte{0x80, 0x80, 0x80, 0x4f}
	r := bytes.NewReader(data)
	for i := 0; i < b.N; i++ {
		_, _, err := DecodeUint32(r)
		if err != nil {
			b.Fatal(err)
		}
		r.Reset(data)
	}
}

func BenchmarkDecodeInt32(b *testing.B) {
	b.ReportAllocs()
	data := []byte{0x80, 0x80, 0x80, 0x4f}
	r := bytes.NewReader(data)
	for i := 0; i < b.N; i++ {
		_, _, err := DecodeInt32(r)
		if err != nil {
			b.Fatal(err)
		}
		r.Reset(data)
	}
}

func BenchmarkDecodeInt64(b *testing.B) {
	b.ReportAllocs()
	data := []byte{0x80, 0x80, 0x80, 0x4f}
	r := bytes.NewReader(data)
	for i := 0; i < b.N; i++ {
		_, _, err := DecodeInt64(r)
		if err != nil {
			b.Fatal(err)
		}
		r.Reset(data)
	}
}
