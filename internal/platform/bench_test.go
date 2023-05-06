package platform

import (
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"
	"testing"
)

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
		if err := UtimensFile(f, times); err != 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkFsFileRead(b *testing.B) {
	dirFS := os.DirFS("testdata")
	embedFS, err := fs.Sub(testdata, "testdata")
	if err != nil {
		b.Fatal(err)
	}

	benches := []struct {
		name  string
		fs    fs.FS
		pread bool
	}{
		{name: "os.DirFS Read", fs: dirFS, pread: false},
		{name: "os.DirFS Pread", fs: dirFS, pread: true},
		{name: "embed.FS Read", fs: embedFS, pread: false},
		{name: "embed.FS Pread", fs: embedFS, pread: true},
	}

	buf := make([]byte, 3)

	for _, bc := range benches {
		bc := bc

		b.Run(bc.name, func(b *testing.B) {
			name := "wazero.txt"
			f, err := bc.fs.Open(name)
			if err != nil {
				b.Fatal(err)
			}
			defer f.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Reset the read position back to the beginning of the file.
				if _, err = f.(io.Seeker).Seek(0, io.SeekStart); err != nil {
					b.Fatal(err)
				}

				fs := NewFsFile(name, syscall.O_RDONLY, f)
				var n int
				var errno syscall.Errno

				b.StartTimer()
				if bc.pread {
					n, errno = fs.Pread(buf, 3)
				} else {
					n, errno = fs.Read(buf)
				}
				b.StopTimer()

				if errno != 0 {
					b.Fatal(errno)
				} else if bufLen := len(buf); n < bufLen {
					b.Fatalf("nread %d less than capacity %d", n, bufLen)
				}
			}
		})
	}
}
