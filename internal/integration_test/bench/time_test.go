package bench

import (
	"testing"
	"time"
	_ "unsafe" // for go:linkname
)

// This benchmark shows one possibility of implementing the platform clock
// more efficiently. As long as CGO is available, all platforms can use
// `runtime.nanotime` to more efficiently implement sys.Nanotime vs using
// time.Since.
//
// While results would be more impressive, this doesn't show how to use
// `runtime.walltime` to avoid the double-performance vs using time.Now. The
// corresponding function only exists in darwin, so prevents this benchmark
// from running on other platforms.

// TODO: implement with x/sys

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

func BenchmarkTime(b *testing.B) {
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
