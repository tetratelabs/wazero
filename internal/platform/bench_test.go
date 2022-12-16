package platform

import (
	"os"
	"testing"
)

func Benchmark_IsTerminal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsTerminal(os.Stdout.Fd())
	}
}
