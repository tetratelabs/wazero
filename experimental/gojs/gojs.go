// Package gojs allows you to run wasm binaries compiled by Go when `GOOS=js`
// and `GOARCH=wasm`.
//
// # Usage
//
// When `GOOS=js` and `GOARCH=wasm`, Go's compiler targets WebAssembly 1.0
// Binary format (%.wasm).
//
// Ex.
//
//	GOOS=js GOARCH=wasm go build -o cat.wasm .
//
// After compiling `cat.wasm` with wazero.Runtime's `CompileModule`, run it
// like below:
//
//	err = gojs.Run(ctx, r, compiled, config)
//	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
//		log.Panicln(err)
//	} else if !ok {
//		log.Panicln(err)
//	}
//
// Under the scenes, the compiled Wasm calls host functions that support the
// runtime.GOOS. This is similar to what is implemented in wasm_exec.js. See
// https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js
//
// # Experimental
//
// Go defines js "EXPERIMENTAL... exempt from the Go compatibility promise."
// Accordingly, wazero cannot guarantee this will work from release to release,
// or that usage will be relatively free of bugs. Due to this and the
// relatively high implementation overhead, most will choose TinyGo instead.
package gojs

import (
	"context"
	"net/http"

	"github.com/tetratelabs/wazero"
	. "github.com/tetratelabs/wazero/internal/gojs"
)

// WithRoundTripper sets the http.RoundTripper used to Run Wasm.
//
// For example, if the code compiled via `GOARCH=wasm GOOS=js` uses
// http.RoundTripper, you can avoid failures by assigning an implementation
// like so:
//
//	ctx = gojs.WithRoundTripper(ctx, http.DefaultTransport)
//	err = gojs.Run(ctx, r, compiled, config)
func WithRoundTripper(ctx context.Context, rt http.RoundTripper) context.Context {
	return context.WithValue(ctx, RoundTripperKey{}, rt)
}

// Run instantiates a new module and calls "run" with the given config.
//
// # Parameters
//
//   - ctx: context to use when instantiating the module and calling "run".
//   - r: runtime to instantiate both the host and guest (compiled) module into.
//   - compiled: guest binary compiled with `GOARCH=wasm GOOS=js`
//   - config: the configuration such as args, env or filesystem to use.
//
// # Example
//
// After compiling your Wasm binary with wazero.Runtime's `CompileModule`, run
// it like below:
//
//	err = gojs.Run(ctx, r, compiled, config)
//	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
//		log.Panicln(err)
//	} else if !ok {
//		log.Panicln(err)
//	}
//
// Note: Both the host and guest module are closed after being run.
func Run(ctx context.Context, r wazero.Runtime, compiled wazero.CompiledModule, config wazero.ModuleConfig) error {
	// Instantiate the imports needed by go-compiled wasm.
	js, err := moduleBuilder(r).Instantiate(ctx, r)
	if err != nil {
		return err
	}
	defer js.Close(ctx)

	// Instantiate the module compiled by go, noting it has no init function.
	mod, err := r.InstantiateModule(ctx, compiled, config)
	if err != nil {
		return err
	}
	defer mod.Close(ctx)

	// Extract the args and env from the module config and write it to memory.
	ctx = WithState(ctx)
	argc, argv, err := WriteArgsAndEnviron(ctx, mod)
	if err != nil {
		return err
	}
	// Invoke the run function.
	_, err = mod.ExportedFunction("run").Call(ctx, uint64(argc), uint64(argv))
	return err
}

// moduleBuilder returns a new wazero.ModuleBuilder
func moduleBuilder(r wazero.Runtime) wazero.ModuleBuilder {
	return r.NewModuleBuilder("go").
		ExportFunction(GetRandomData.Name(), GetRandomData).
		ExportFunction(Nanotime1.Name(), Nanotime1).
		ExportFunction(WasmExit.Name(), WasmExit).
		ExportFunction(CopyBytesToJS.Name(), CopyBytesToJS).
		ExportFunction(ValueCall.Name(), ValueCall).
		ExportFunction(ValueGet.Name(), ValueGet).
		ExportFunction(ValueIndex.Name(), ValueIndex).
		ExportFunction(ValueLength.Name(), ValueLength).
		ExportFunction(ValueNew.Name(), ValueNew).
		ExportFunction(ValueSet.Name(), ValueSet).
		ExportFunction(WasmWrite.Name(), WasmWrite).
		ExportFunction(ResetMemoryDataView.Name, ResetMemoryDataView).
		ExportFunction(Walltime.Name(), Walltime).
		ExportFunction(ScheduleTimeoutEvent.Name, ScheduleTimeoutEvent).
		ExportFunction(ClearTimeoutEvent.Name, ClearTimeoutEvent).
		ExportFunction(FinalizeRef.Name(), FinalizeRef).
		ExportFunction(StringVal.Name(), StringVal).
		ExportFunction(ValueDelete.Name, ValueDelete).
		ExportFunction(ValueSetIndex.Name, ValueSetIndex).
		ExportFunction(ValueInvoke.Name, ValueInvoke).
		ExportFunction(ValuePrepareString.Name(), ValuePrepareString).
		ExportFunction(ValueInstanceOf.Name, ValueInstanceOf).
		ExportFunction(ValueLoadString.Name(), ValueLoadString).
		ExportFunction(CopyBytesToGo.Name(), CopyBytesToGo).
		ExportFunction(Debug.Name, Debug)
}
