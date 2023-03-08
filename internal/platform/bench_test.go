package platform

import (
	"os"
	"path"
	"syscall"
	"testing"
)

func Benchmark_IsTerminal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsTerminal(os.Stdout.Fd())
	}
}

func Benchmark_UtimensFile(b *testing.B) {
	tmpDir := b.TempDir()
	f, err := os.Create(path.Join(tmpDir, "file"))
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	times := &[2]syscall.Timespec{
		{Sec: 123, Nsec: 4 * 1e3},
		{Sec: 123, Nsec: 4 * 1e3},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := UtimensFile(f, times); err != nil {
			b.Fatal(err)
		}
	}
}
