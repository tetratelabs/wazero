//go:build amd64
// +build amd64

// Wasmtime cannot be used non-amd64 platform.
package bench

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
)

// BenchmarkMakeWasmFunc tracks the time spent calling internalwasm.MakeWasmFunc
func BenchmarkMakeWasmFunc(b *testing.B) {
	benches := []struct {
		name   string
		engine func() *wazero.Engine
	}{
		{name: "Interpreter", engine: wazero.NewEngineInterpreter},
		{name: "JIT", engine: wazero.NewEngineJIT},
	}

	for _, bb := range benches {
		bc := bb

		mod, err := wazero.DecodeModuleBinary(facWasm)
		if err != nil {
			b.Fatal(err)
		}

		store := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: bc.engine()})

		exports, err := wazero.InstantiateModule(store, mod)
		if err != nil {
			b.Fatal(err)
		}

		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {

				var fn func(uint64) uint64
				if err := wazero.MakeWasmFunc(exports, "fac-iter", &fn); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
