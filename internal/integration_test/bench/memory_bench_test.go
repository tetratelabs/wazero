package bench

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func BenchmarkMemory(b *testing.B) {
	var mem = &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize), Min: 1}
	if !mem.WriteByte(testCtx, 10, 16) {
		b.Fail()
	}

	b.Run("ReadByte", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if v, ok := mem.ReadByte(testCtx, 10); !ok || v != 16 {
				b.Fail()
			}
		}
	})

	b.Run("ReadUint32Le", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if v, ok := mem.ReadUint32Le(testCtx, 10); !ok || v != 16 {
				b.Fail()
			}
		}
	})

	b.Run("WriteByte", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !mem.WriteByte(testCtx, 10, 16) {
				b.Fail()
			}
		}
	})

	b.Run("WriteUint32Le", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !mem.WriteUint32Le(testCtx, 10, 16) {
				b.Fail()
			}
		}
	})
}
