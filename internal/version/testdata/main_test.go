package main

import (
	"sync"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
)

// TestGetWazeroVersion ensures that GetWazeroVersion returns the version of wazero in the go.mod in the
// downstream wazero users.
func TestGetWazeroVersion(t *testing.T) {
	// This matches the one in the "replace" statement in the go.mod.
	const exp = "v0.0.0-20220818123113-1948909ec0b1"
	const callers = 100

	start := make(chan struct{})
	results := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- version.GetWazeroVersion()
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		require.Equal(t, exp, result)
	}
}
