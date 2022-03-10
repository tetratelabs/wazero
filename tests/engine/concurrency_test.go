package adhoc

import (
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestJITConcurrency(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAdhocTestsUnderHighConcurrency(t, wazero.NewRuntimeConfigJIT)
	// TODO: Add concurrent instantiation, invocation and release on a single store test case in https://github.com/tetratelabs/wazero/issues/293

}

func TestInterpreterConcurrency(t *testing.T) {
	runAdhocTestsUnderHighConcurrency(t, wazero.NewRuntimeConfigInterpreter)
	// TODO: Add concurrent instantiation, invocation and release on a single store test case in https://github.com/tetratelabs/wazero/issues/293
}

func runAdhocTestsUnderHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("huge stack", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testHugeStack)
	})
	t.Run("fibonacci", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testFibonacci)
	})
	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testUnreachable)
	})
	t.Run("memory", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testMemory)
	})
	t.Run("recursive entry", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testRecursiveEntry)
	})
	t.Run("imported-and-exported func", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testImportedAndExportedFunc)
	})
}

// runAdhocTestUnderHighConcurrency runs a test case in adhoc_test.go with multiple goroutines.
func runAdhocTestUnderHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig,
	adhocTest func(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig)) {
	const goroutinesPerCase = 1000
	var wg sync.WaitGroup
	wg.Add(goroutinesPerCase)
	for i := 0; i < goroutinesPerCase; i++ {
		go func() {
			defer wg.Done()
			adhocTest(t, newRuntimeConfig)
		}()
	}
	wg.Wait()
}
