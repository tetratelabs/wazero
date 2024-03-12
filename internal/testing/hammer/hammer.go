package hammer

import (
	"runtime"
	"sync"
	"testing"
)

// Hammer invokes a test concurrently in P goroutines N times per goroutine.
//
// Here's an example:
//
//	P := 8               // max count of goroutines
//	N := 1000            // work per goroutine
//	if testing.Short() { // Adjust down if `-test.short`
//		P = 4
//		N = 100
//	}
//
//	hammer.NewHammer(t, P, N).Run(func(name string) {
//		// Do test using name if something needs to be unique.
//	}, nil)
//
//	if t.Failed() {
//		return // At least one test failed, so return now.
//	}
//
// See /RATIONALE.md
type Hammer interface {
	// Run invokes a concurrency test, as described in /RATIONALE.md.
	//
	// * test is concurrently run in P goroutines, each looping N times.
	//   * name is unique within the hammer.
	// * onRunning is any function to run after all goroutines are running, but before test executes.
	//
	// On completion, return early if there's a failure like this:
	//	if t.Failed() {
	//		return
	//	}
	Run(test func(p, n int), onRunning func())
}

// NewHammer returns a Hammer initialized to indicated count of goroutines (P) and iterations per goroutine (N).
// As discussed in /RATIONALE.md, optimize for Hammer.Run completing in .1 second on a modern laptop.
func NewHammer(t *testing.T, P, N int) Hammer {
	return &hammer{t: t, P: P, N: N}
}

// hammer implements Hammer
type hammer struct {
	// t is the calling test
	t *testing.T
	// P is the max count of goroutines
	P int
	// N is the work per goroutine
	N int
}

// Run implements Hammer.Run
func (h *hammer) Run(test func(p, n int), onRunning func()) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(h.P / 2)) // Ensure goroutines have to switch cores.

	// running track
	running := make(chan int)
	// unblock needs to happen atomically, so we need to use a WaitGroup
	var unblocked sync.WaitGroup
	finished := make(chan int)

	unblocked.Add(h.P) // P goroutines will be unblocked by the current goroutine.
	for p := 0; p < h.P; p++ {
		p := p // pin p, so it is stable inside the goroutine.

		go func() { // Launch goroutine 'p'
			defer func() { // Ensure each require.XX failure is visible on hammer test fail.
				if recovered := recover(); recovered != nil {
					// Has been seen to be string, runtime.errorString, and it may be others. Let
					// printing take care of conversion in a generic way.
					h.t.Error(recovered)
				}
				finished <- 1
			}()
			running <- 1

			unblocked.Wait()           // Wait to be unblocked
			for n := 0; n < h.N; n++ { // Invoke one test
				test(p, n)
			}
		}()
	}

	// Block until P goroutines are running.
	for i := 0; i < h.P; i++ {
		<-running
	}

	if onRunning != nil {
		onRunning()
	}

	// Release all goroutines at the same time.
	unblocked.Add(-h.P)

	// Block until P goroutines finish.
	for i := 0; i < h.P; i++ {
		<-finished
	}
}
