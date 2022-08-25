package compiler

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

var runtime = vs.NewWazeroCompilerRuntime

func TestAllocation(t *testing.T) {
	vs.RunTestAllocation(t, runtime)
}

func BenchmarkAllocation(b *testing.B) {
	vs.RunBenchmarkAllocation(b, runtime)
}

func TestFactorial(t *testing.T) {
	vs.RunTestFactorial(t, runtime)
}

func BenchmarkFactorial(b *testing.B) {
	vs.RunBenchmarkFactorial(b, runtime)
}

func TestHostCall(t *testing.T) {
	vs.RunTestHostCall(t, runtime)
}

func BenchmarkHostCall(b *testing.B) {
	vs.RunBenchmarkHostCall(b, runtime)
}

func TestMemory(t *testing.T) {
	vs.RunTestMemory(t, runtime)
}

func BenchmarkMemory(b *testing.B) {
	vs.RunBenchmarkMemory(b, runtime)
}
