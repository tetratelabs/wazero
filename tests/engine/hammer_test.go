package adhoc

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasm"
)

var hammers = map[string]func(t *testing.T, r wazero.Runtime){
	// Tests here are similar to what's described in /RATIONALE.md, but deviate as they involve blocking functions.
	"close importing module while in use": closeImportingModuleWhileInUse,
	"close imported module while in use":  closeImportedModuleWhileInUse,
}

func TestEngineJIT_hammer(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAllTests(t, hammers, wazero.NewRuntimeConfigJIT())
}

func TestEngineInterpreter_hammer(t *testing.T) {
	runAllTests(t, hammers, wazero.NewRuntimeConfigInterpreter())
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

func closeImportingModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	args := []uint64{1, 123}
	exp := args[0] + args[1]

	P := 8               // P+1 == max count of goroutines/in-flight function calls
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(P / 2)) // Ensure goroutines have to switch cores.

	running := make(chan bool)
	unblocked := make(chan bool)
	b := &blocker{running: running, unblocked: unblocked}

	imported, err := r.NewModuleBuilder("host").ExportFunction("block", b.fn).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	importing, err := r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	// Add channel that tracks P goroutines.
	done := make(chan int)
	for p := 0; p < P; p++ {
		go func() { // Launch goroutine 'p'
			defer completeGoroutine(t, done)

			// As this is a blocking function, we can only run 1 function per goroutine.
			requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
		}()
	}

	// Wait until all functions are in-flight.
	for i := 0; i < P; i++ {
		<-running
	}

	// Close the module that exported blockAfterAdd (noting calls to this are in-flight).
	require.NoError(t, importing.Close())

	// Prove a module can be redefined even with in-flight calls.
	importing, err = r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	// If unloading worked properly, a new function call should route to the newly instantiated module.
	go func() {
		defer completeGoroutine(t, done)

		requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
	}()
	<-running // Wait for the above function to be in-flight
	P++

	// Unblock the functions to ensure they don't err on the return path of a closed module.
	for i := 0; i < P; i++ {
		unblocked <- true
		<-done
	}
}

func closeImportedModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	args := []uint64{1, 123}
	exp := args[0] + args[1]

	P := 8               // P+1 == max count of goroutines/in-flight function calls
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(P / 2)) // Ensure goroutines have to switch cores.

	running := make(chan bool)
	unblocked := make(chan bool)
	b := &blocker{running: running, unblocked: unblocked}

	imported, err := r.NewModuleBuilder("host").ExportFunction("block", b.fn).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	importing, err := r.InstantiateModuleFromSource(blockAfterAddSource)
	require.NoError(t, err)
	defer importing.Close()

	// Add channel that tracks P goroutines.
	done := make(chan int)
	for p := 0; p < P; p++ {
		go func() { // Launch goroutine 'p'
			defer completeGoroutine(t, done)

			// As this is a blocking function, we can only run 1 function per goroutine.
			requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
		}()
	}

	// Wait until all functions are in-flight.
	for i := 0; i < P; i++ {
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

	// If unloading worked properly, a new function call should route to the newly instantiated module.
	go func() {
		defer completeGoroutine(t, done)

		requireFunctionCall(t, importing.ExportedFunction("block_after_add"), args, exp)
	}()
	<-running // Wait for the above function to be in-flight
	P++

	// Unblock the functions to ensure they don't err on the return path of a closed module.
	for i := 0; i < P; i++ {
		unblocked <- true
		<-done
	}
}

func requireFunctionCall(t *testing.T, fn wasm.Function, args []uint64, exp uint64) {
	res, err := fn.Call(nil, args...)
	// We don't expect an error because there's currently no functionality to detect or fail on a closed module.
	require.NoError(t, err)
	require.Equal(t, exp, res[0])
}

func completeGoroutine(t *testing.T, c chan int) {
	// Ensure each require.XX failure is visible on hammer test fail.
	if recovered := recover(); recovered != nil {
		t.Error(recovered.(string))
	}
	c <- 1
}
