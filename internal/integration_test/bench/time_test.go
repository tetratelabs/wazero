package bench

import (
	"testing"
	"time"
	_ "unsafe" // for go:linkname
)

// This benchmark shows one possibility of implementing the platform clock
// more efficiently. By linking with CGO, code can avoid calling clocks twice
// to implement sys.Walltime using time.Now. It can also drop unnecessary setup
// of base time and conversions to implement sys.Nanotime using time.Since.
//
// This doesn't show an even more efficient option, which is to implement time
// in assembly, as that would be distracting and add build constraints.

//go:noescape
//go:linkname walltime runtime.walltime
func walltime() (int64, int32)

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

func BenchmarkTime(b *testing.B) {
	b.Run("time.Now", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = time.Now()
		}
	})
	b.Run("runtime.walltime", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = walltime()
		}
	})

	base := time.Now()
	b.Run("time.Since", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = time.Since(base)
		}
	})
	b.Run("runtime.nanotime", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = nanotime()
		}
	})
}
