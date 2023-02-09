package wazero_test

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/tetratelabs/wazero"
)

// infiniteLoopWasm exports a function named "infinite_loop" that never exits.
//
//go:embed internal/integration_test/engine/testdata/infinite_loop.wasm
var infiniteLoopWasm []byte

// ExampleRuntimeConfig_WithEnsureTermination_context_timeout demonstrates how to ensure the termination
// of infinite loop function with context.Context created by context.WithTimeout powered by
// RuntimeConfig.WithEnsureTermination configuration.
func ExampleRuntimeConfig_WithEnsureTermination_context_timeout() {
	ctx := context.Background()

	r := wazero.NewRuntimeWithConfig(context.Background(),
		// Enables the WithEnsureTermination option.
		wazero.NewRuntimeConfig().WithEnsureTermination(true))

	compiledModule, err := r.CompileModule(context.Background(), infiniteLoopWasm)
	if err != nil {
		log.Panicln(err)
	}

	moduleInstance, err := r.InstantiateModule(context.Background(), compiledModule,
		wazero.NewModuleConfig().WithName("malicious_wasm"))
	if err != nil {
		log.Panicln(err)
	}

	infiniteLoop := moduleInstance.ExportedFunction("infinite_loop")

	// Create the context.Context to be passed to the invocation of infinite_loop.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Invoke the infinite loop with the timeout context.
	_, err = infiniteLoop.Call(ctx)

	// Timeout is correctly handled and causes the termination of infinite loop.
	fmt.Println(err)

	// Output:
	//	module "malicious_wasm" closed with context deadline exceeded
}

// ExampleRuntimeConfig_WithEnsureTermination_context_timeout demonstrates how to ensure the termination
// of infinite loop function with context.Context created by context.WithCancel powered by
// RuntimeConfig.WithEnsureTermination configuration.
func ExampleRuntimeConfig_WithEnsureTermination_context_cancel() {
	ctx := context.Background()

	r := wazero.NewRuntimeWithConfig(context.Background(),
		// Enables the WithEnsureTermination option.
		wazero.NewRuntimeConfig().WithEnsureTermination(true))

	compiledModule, err := r.CompileModule(context.Background(), infiniteLoopWasm)
	if err != nil {
		log.Panicln(err)
	}

	moduleInstance, err := r.InstantiateModule(context.Background(), compiledModule,
		wazero.NewModuleConfig().WithName("malicious_wasm"))
	if err != nil {
		log.Panicln(err)
	}

	infiniteLoop := moduleInstance.ExportedFunction("infinite_loop")

	// Create the context.Context to be passed to the invocation of infinite_loop.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// After 2 seconds, cancel the invocation of infinite loop.
		time.Sleep(2 * time.Second)
		cancel()
	}()

	// Invoke the infinite loop with the timeout context.
	_, err = infiniteLoop.Call(ctx)

	// context Cancelation is correctly handled and causes the termination of infinite loop.
	fmt.Println(err)

	// Output:
	//	module "malicious_wasm" closed with context canceled
}
