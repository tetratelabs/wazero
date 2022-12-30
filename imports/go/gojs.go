// Package gojs allows you to run wasm binaries compiled by Go when
// `GOARCH=wasm GOOS=js`. See https://wazero.io/languages/go/ for more.
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
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/internal/gojs"
	. "github.com/tetratelabs/wazero/internal/gojs/run"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// MustInstantiate calls Instantiate or panics on error.
//
// This is a simpler function for those who know the module "go" is not
// already instantiated, and don't need to unload it.
func MustInstantiate(ctx context.Context, r wazero.Runtime) {
	if _, err := Instantiate(ctx, r); err != nil {
		panic(err)
	}
}

// Instantiate instantiates the "go" module, used by `GOARCH=wasm GOOS=js`,
// into the runtime default namespace.
//
// # Notes
//
//   - Failure cases are documented on wazero.Namespace InstantiateModule.
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
//   - To instantiate into another wazero.Namespace, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewHostModuleBuilder("go")
	NewFunctionExporter().ExportFunctions(builder)
	return builder.Instantiate(ctx, r)
}

// FunctionExporter configures the functions in the "go" module used by
// `GOARCH=wasm GOOS=js`.
type FunctionExporter interface {
	// ExportFunctions builds functions to export with a
	// wazero.HostModuleBuilder named "go".
	ExportFunctions(wazero.HostModuleBuilder)
}

// NewFunctionExporter returns a FunctionExporter object.
func NewFunctionExporter() FunctionExporter {
	return &functionExporter{}
}

type functionExporter struct{}

// ExportFunctions implements FunctionExporter.ExportFunctions
func (e *functionExporter) ExportFunctions(builder wazero.HostModuleBuilder) {
	hfExporter := builder.(wasm.HostFuncExporter)

	hfExporter.ExportHostFunc(GetRandomData)
	hfExporter.ExportHostFunc(Nanotime1)
	hfExporter.ExportHostFunc(WasmExit)
	hfExporter.ExportHostFunc(CopyBytesToJS)
	hfExporter.ExportHostFunc(ValueCall)
	hfExporter.ExportHostFunc(ValueGet)
	hfExporter.ExportHostFunc(ValueIndex)
	hfExporter.ExportHostFunc(ValueLength)
	hfExporter.ExportHostFunc(ValueNew)
	hfExporter.ExportHostFunc(ValueSet)
	hfExporter.ExportHostFunc(WasmWrite)
	hfExporter.ExportHostFunc(ResetMemoryDataView)
	hfExporter.ExportHostFunc(Walltime)
	hfExporter.ExportHostFunc(ScheduleTimeoutEvent)
	hfExporter.ExportHostFunc(ClearTimeoutEvent)
	hfExporter.ExportHostFunc(FinalizeRef)
	hfExporter.ExportHostFunc(StringVal)
	hfExporter.ExportHostFunc(ValueDelete)
	hfExporter.ExportHostFunc(ValueSetIndex)
	hfExporter.ExportHostFunc(ValueInvoke)
	hfExporter.ExportHostFunc(ValuePrepareString)
	hfExporter.ExportHostFunc(ValueInstanceOf)
	hfExporter.ExportHostFunc(ValueLoadString)
	hfExporter.ExportHostFunc(CopyBytesToGo)
	hfExporter.ExportHostFunc(Debug)
}

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
//   - r: runtime to instantiate both the host and guest (compiled) module in.
//   - compiled: guest binary compiled with `GOARCH=wasm GOOS=js`
//   - config: the configuration such as args, env or filesystem to use.
//
// # Example
//
// After compiling your Wasm binary with wazero.Runtime's `CompileModule`, run
// it like below:
//
//	// Use compilation cache to reduce performance penalty of multiple runs.
//	ctx = experimental.WithCompilationCacheDirName(ctx, ".build")
//
//	// Instantiate the host functions used for each call.
//	gojs.MustInstantiate(r)
//
//	// Execute the "run" function, which corresponds to "main" in stars/main.go.
//	err = gojs.Run(ctx, r, compiled, config)
//	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
//		log.Panicln(err)
//	} else if !ok {
//		log.Panicln(err)
//	}
//
// # Notes
//
//   - Use wazero.RuntimeConfig `WithWasmCore2` to avoid needing to pick >1.0
//     features set by `GOWASM` or used internally by Run.
//   - Wasm generated by `GOARCH=wasm GOOS=js` is very slow to compile.
//     Use experimental.WithCompilationCacheDirName to improve performance.
//   - The guest module is closed after being run.
func Run(ctx context.Context, ns wazero.Namespace, compiled wazero.CompiledModule, config wazero.ModuleConfig) error {
	_, err := RunAndReturnState(ctx, ns, compiled, config)
	return err
}
