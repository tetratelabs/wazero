// Package time benchmarks shows ways to implementing the platform clock more
// efficiently. As long as CGO is available, all platforms can use
// `runtime.nanotime` to more efficiently implement sys.Nanotime vs using
// time.Since or x/sys.
//
// While results would be more impressive, this doesn't show how to use
// `runtime.walltime` to avoid the double-performance vs using time.Now. The
// corresponding function only exists in darwin, so prevents this benchmark
// from running on other platforms.
package time

import (
	"testing"
	"time"
	_ "unsafe" // for go:linkname

	"golang.org/x/sys/unix"
)

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

func BenchmarkClock(b *testing.B) {
	b.Run("time.Now", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = time.Now()
		}
	})
	b.Run("ClockGettime(CLOCK_REALTIME)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var now unix.Timespec
			if err := unix.ClockGettime(unix.CLOCK_REALTIME, &now); err != nil {
				b.Fatal(err)
			}
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
	b.Run("ClockGettime(CLOCK_MONOTONIC)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var tick unix.Timespec
			if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &tick); err != nil {
				b.Fatal(err)
			}
		}
	})
}
