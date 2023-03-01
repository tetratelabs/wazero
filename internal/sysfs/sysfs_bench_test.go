package sysfs

import (
	"io"
	"io/fs"
	"os"
	"testing"
)

func BenchmarkReaderAtOffset(b *testing.B) {
	dirFS := os.DirFS("testdata")
	embedFS, err := fs.Sub(testdata, "testdata")
	if err != nil {
		b.Fatal(err)
	}

	benches := []struct {
		name string
		fs   fs.FS
		ra   bool
	}{
		{name: "os.DirFS io.File", fs: dirFS, ra: false},
		{name: "os.DirFS readerAtOffset", fs: dirFS, ra: true},
		{name: "embed.FS embed.file", fs: embedFS, ra: false},
		{name: "embed.FS seekToOffsetReader", fs: embedFS, ra: true},
	}

	buf := make([]byte, 3)

	for _, bc := range benches {
		bc := bc

		b.Run(bc.name, func(b *testing.B) {
			f, err := bc.fs.Open("wazero.txt")
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

				// Get the reader we are benchmarking
				var r io.Reader = f
				if bc.ra {
					r = ReaderAtOffset(f, 3)
				}

				b.StartTimer()
				if _, err := r.Read(buf); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
			}
		})
	}
}
