package bench

import (
	"math"
	"testing"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func BenchmarkMemory(b *testing.B) {
	var mem = &wasm.MemoryInstance{Buffer: make([]byte, math.MaxUint16), Min: 1}
	if !mem.WriteByte(10, 16) {
		b.Fail()
	}

	b.Run("ReadByte", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if v, ok := mem.ReadByte(10); !ok || v != 16 {
				b.Fail()
			}
		}
	})

	b.Run("ReadUint32Le", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if v, ok := mem.ReadUint32Le(10); !ok || v != 16 {
				b.Fail()
			}
		}
	})

	b.Run("WriteByte", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !mem.WriteByte(10, 16) {
				b.Fail()
			}
		}
	})

	b.Run("WriteUint32Le", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !mem.WriteUint32Le(10, 16) {
				b.Fail()
			}
		}
	})
}
