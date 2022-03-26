package adhoc

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasm"
)

func TestJITConcurrency(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAdhocTestsUnderHighConcurrency(t, wazero.NewRuntimeConfigJIT)
	singleModuleHighConcurrency(t, wazero.NewRuntimeConfigJIT)
}

func TestInterpreterConcurrency(t *testing.T) {
	runAdhocTestsUnderHighConcurrency(t, wazero.NewRuntimeConfigInterpreter)
	singleModuleHighConcurrency(t, wazero.NewRuntimeConfigInterpreter)
}

func runAdhocTestsUnderHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("huge stack", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testHugeStack)
	})
	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testUnreachable)
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

func singleModuleHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("close importing module while in use", func(t *testing.T) {
		closeImportingModuleWhileInUse(t, newRuntimeConfig)

	})
	t.Run("close imported module while in use", func(t *testing.T) {
		closeImportedModuleWhileInUse(t, newRuntimeConfig)
	})
}

type blocker struct {
	running   chan bool
	unblocked chan bool
	// closed should panic if fn is called when the value is 1.
	//
	// Note: Exclusively reading and updating this with atomics guarantees cross-goroutine observations.
	// See /RATIONALE.md
	closed uint32
}

func (b *blocker) close() {
	atomic.StoreUint32(&b.closed, 1)
}

// fn sends into the running channel, then blocks until it can receive from unblocked one.
func (b *blocker) fn(input uint32) uint32 {
	if atomic.LoadUint32(&b.closed) == 1 {
		panic(errors.New("closed"))
	}
	b.running <- true // Signal the goroutine is running
	<-b.unblocked     // Await until unblocked
	return input
}

var blockAfterAddSource = []byte(`(module
	(import "host" "block" (func $block (param i32) (result i32)))
	(func $block_after_add (param i32) (param i32) (result i32)
		local.get 0
		local.get 1
		i32.add
		call $block
	)
	(export "block_after_add" (func $block_after_add))
)`)

func closeImportingModuleWhileInUse(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	args := []uint64{1, 123}
	exp := args[0] + args[1]
	const goroutines = 1000

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	running := make(chan bool)
	unblocked := make(chan bool)
	b := &blocker{running: running, unblocked: unblocked}

	imported, err := r.NewModuleBuilder("host").ExportFunction("block", b.fn).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	importing, err := r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
		}()
	}

	// Wait until all goroutines are running.
	for i := 0; i < goroutines; i++ {
		<-running
	}
	// Close the module that exported blockAfterAdd (noting calls to this are in-flight).
	require.NoError(t, importing.Close())

	// Prove a module can be redefined even with in-flight calls.
	importing, err = r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	// If unloading worked properly, a new function call should route to the newly instantiated module.
	wg.Add(1)
	go func() {
		defer wg.Done()
		requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
	}()
	<-running // Wait for the above function to be in-flight

	// Unblock the other goroutines to ensure they don't err on the return path of a closed module.
	for i := 0; i < goroutines+1; i++ {
		unblocked <- true
	}
	wg.Wait() // Wait for all goroutines to finish.
}

func closeImportedModuleWhileInUse(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	args := []uint64{1, 123}
	exp := args[0] + args[1]
	const goroutines = 1000

	r := wazero.NewRuntimeWithConfig(newRuntimeConfig())

	running := make(chan bool)
	unblocked := make(chan bool)
	b := &blocker{running: running, unblocked: unblocked}

	imported, err := r.NewModuleBuilder("host").ExportFunction("block", b.fn).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	importing, err := r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
		}()
	}

	// Wait until all goroutines are running.
	for i := 0; i < goroutines; i++ {
		<-running
	}
	// Close the module that exported the host function (noting calls to this are in-flight).
	require.NoError(t, imported.Close())
	// Close the underlying host function, which causes future calls to it to fail.
	b.close()
	require.Panics(t, func() {
		b.fn(1) // validate it would fail if accidentally called
	})
	// Close the importing module
	require.NoError(t, importing.Close())

	// Prove a host module can be redefined even with in-flight calls.
	b1 := &blocker{running: running, unblocked: unblocked} // New instance, so not yet closed!
	imported, err = r.NewModuleBuilder("host").ExportFunction("block", b1.fn).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	// Redefine the importing module, which should link to the redefined host module.
	importing, err = r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	// If unloading worked properly, a new function call should route to the newly instantiated host module.
	wg.Add(1)
	go func() {
		defer wg.Done()
		requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
	}()
	<-running // Wait for the above function to be in-flight

	// Unblock the other goroutines to ensure they don't err on the return path of a closed module.
	for i := 0; i < goroutines+1; i++ {
		unblocked <- true
	}
	wg.Wait() // Wait for all goroutines to finish.
}

func requireFunctionCall(t *testing.T, fn wasm.Function, args []uint64, exp uint64) {
	res, err := fn.Call(nil, args...)
	// We don't expect an error because there's currently no functionality to detect or fail on a closed module.
	require.NoError(t, err)
	require.Equal(t, exp, res[0])
}
