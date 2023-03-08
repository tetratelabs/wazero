package platform

import (
	"os"
	"path"
	"testing"
)

func Benchmark_IsTerminal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsTerminal(os.Stdout.Fd())
	}
}

func Benchmark_UtimesNano(b *testing.B) {
	tmpDir := b.TempDir()
	f, err := os.Create(path.Join(tmpDir, "file"))
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := UtimesNanoFile(f, 1, 1); err != nil {
			b.Fatal(err)
		}
	}
}
