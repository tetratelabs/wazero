package sysfs

import (
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func BenchmarkFsFileUtimesNs(b *testing.B) {
	f, errno := OpenOSFile(path.Join(b.TempDir(), "file"), sys.O_CREAT, 0)
	if errno != 0 {
		b.Fatal(errno)
	}
	defer f.Close()

	atim := int64(123*time.Second + 4*time.Microsecond)
	mtim := atim

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if errno := f.Utimens(atim, mtim); errno != 0 {
			b.Fatal(errno)
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
		{name: "embed.api.FS Read", fs: embedFS, pread: false},
		{name: "embed.api.FS Pread", fs: embedFS, pread: true},
	}

	buf := make([]byte, 3)

	for _, bc := range benches {
		bc := bc

		b.Run(bc.name, func(b *testing.B) {
			name := "wazero.txt"
			f, errno := OpenFSFile(bc.fs, name, sys.O_RDONLY, 0)
			if errno != 0 {
				b.Fatal(errno)
			}
			defer f.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				var n int
				var errno sys.Errno

				// Reset the read position back to the beginning of the file.
				if _, errno = f.Seek(0, io.SeekStart); errno != 0 {
					b.Fatal(errno)
				}

				b.StartTimer()
				if bc.pread {
					n, errno = f.Pread(buf, 3)
				} else {
					n, errno = f.Read(buf)
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
