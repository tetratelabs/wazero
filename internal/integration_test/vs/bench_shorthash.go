package vs

import (
	_ "embed"
	"testing"
)

var (
	// shorthashWasm is a wasi binary which runs the same wasm function inside
	// a loop. See https://github.com/tetratelabs/wazero/issues/947
	//
	// Taken from https://github.com/jedisct1/webassembly-benchmarks/tree/master/2022-12/wasm
	//go:embed testdata/shorthash.wasm
	shorthashWasm   []byte
	shorthashConfig *RuntimeConfig
)

func init() {
	shorthashConfig = &RuntimeConfig{
		ModuleName: "shorthash",
		ModuleWasm: shorthashWasm,
		NeedsWASI:  true, // runs as a _start function
	}
}

func RunTestShorthash(t *testing.T, runtime func() Runtime) {
	// not testCall as there are no exported functions except _start
	testInstantiate(t, runtime, shorthashConfig)
}

func RunBenchmarkShorthash(b *testing.B, runtime func() Runtime) {
	benchmark(b, runtime, shorthashConfig, nil)
}
