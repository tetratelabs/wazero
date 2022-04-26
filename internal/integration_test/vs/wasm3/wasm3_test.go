//go:build amd64 && cgo && !windows

package wasm3

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

var runtime = newWasm3Runtime

func TestAllocation(t *testing.T) {
	t.Skip("https://github.com/birros/go-wasm3/issues/5")
	vs.RunTestAllocation(t, runtime)
}

func BenchmarkAllocation(b *testing.B) {
	b.Skip("https://github.com/birros/go-wasm3/issues/5")
	vs.RunBenchmarkAllocation(b, runtime)
}

func TestBenchmarkAllocation_Call_JITFastest(t *testing.T) {
	t.Skip("https://github.com/birros/go-wasm3/issues/5")
	vs.RunTestBenchmarkAllocation_Call_JITFastest(t, runtime())
}

func TestFactorial(t *testing.T) {
	vs.RunTestFactorial(t, runtime)
}

func BenchmarkFactorial(b *testing.B) {
	vs.RunBenchmarkFactorial(b, runtime)
}

func TestBenchmarkFactorial_Call_JITFastest(t *testing.T) {
	vs.RunTestBenchmarkFactorial_Call_JITFastest(t, runtime())
}
