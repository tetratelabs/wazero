// Package gojs allows you to run wasm binaries compiled by Go when
// `GOARCH=wasm GOOS=js`. See https://wazero.io/languages/go/ for more.
//
// # Experimental
//
// Go defines js "EXPERIMENTAL... exempt from the Go compatibility promise."
// Accordingly, wazero cannot guarantee this will work from release to release,
// or that usage will be relatively free of bugs. Moreover, `GOOS=wasi` will
// happen, and once that's available in two releases wazero will remove this
// package.
//
// Due to these concerns and the relatively high implementation overhead, most
// will choose TinyGo instead of gojs.
package gojs

import (
	"context"
	"net/http"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/internal/gojs"
	internalconfig "github.com/tetratelabs/wazero/internal/gojs/config"
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
// into the runtime.
//
// # Notes
//
//   - Failure cases are documented on wazero.Runtime InstantiateModule.
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewHostModuleBuilder("go")
	NewFunctionExporter().ExportFunctions(builder)
	return builder.Instantiate(ctx)
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

// Config extends wazero.ModuleConfig with GOOS=js specific extensions.
// Use NewConfig to create an instance.
type Config interface {
	// WithOSWorkdir sets the initial working directory used to Run Wasm to
	// the value of os.Getwd instead of the default of root "/".
	//
	// Here's an example that overrides this to the current directory:
	//
	//	err = gojs.Run(ctx, r, compiled, gojs.NewConfig(moduleConfig).
	//			WithOSWorkdir())
	//
	// Note: To use this feature requires mounting the real root directory via
	// wazero.FSConfig `WithDirMount`. On windows, this root must be the same drive
	// as the value of os.Getwd. For example, it would be an error to mount `C:\`
	// as the guest path "", while the current directory is inside `D:\`.
	WithOSWorkdir() Config

	// WithOSUser allows the guest to see the current user's uid, gid, euid and
	// groups, instead of zero for each value.
	//
	// Here's an example that uses the real user's IDs:
	//
	//	err = gojs.Run(ctx, r, compiled, gojs.NewConfig(moduleConfig).
	//			WithOSUser())
	//
	// Note: This has no effect on windows.
	WithOSUser() Config

	// WithRoundTripper sets the http.RoundTripper used to Run Wasm.
	//
	// For example, if the code compiled via `GOARCH=wasm GOOS=js` uses
	// http.RoundTripper, you can avoid failures by assigning an implementation
	// like so:
	//
	//	err = gojs.Run(ctx, r, compiled, gojs.NewConfig(moduleConfig).
	//			WithRoundTripper(ctx, http.DefaultTransport))
	WithRoundTripper(http.RoundTripper) Config
}

// NewConfig returns a Config that can be used for configuring module instantiation.
func NewConfig(moduleConfig wazero.ModuleConfig) Config {
	return &cfg{moduleConfig: moduleConfig, internal: internalconfig.NewConfig()}
}

type cfg struct {
	moduleConfig wazero.ModuleConfig
	internal     *internalconfig.Config
}

func (c *cfg) clone() *cfg {
	return &cfg{moduleConfig: c.moduleConfig, internal: c.internal.Clone()}
}

// WithOSWorkdir implements Config.WithOSWorkdir
func (c *cfg) WithOSWorkdir() Config {
	ret := c.clone()
	ret.internal.OsWorkdir = true
	return ret
}

// WithOSUser implements Config.WithOSUser
func (c *cfg) WithOSUser() Config {
	ret := c.clone()
	ret.internal.OsUser = true
	return ret
}

// WithRoundTripper implements Config.WithRoundTripper
func (c *cfg) WithRoundTripper(rt http.RoundTripper) Config {
	ret := c.clone()
	ret.internal.Rt = rt
	return ret
}

// Run instantiates a new module and calls "run" with the given config.
//
// # Parameters
//
//   - ctx: context to use when instantiating the module and calling "run".
//   - r: runtime to instantiate both the host and guest (compiled) module in.
//   - compiled: guest binary compiled with `GOARCH=wasm GOOS=js`
//   - config: the Config to use including wazero.ModuleConfig or extensions of
//     it.
//
// # Example
//
// After compiling your Wasm binary with wazero.Runtime's `CompileModule`, run
// it like below:
//
//	// Instantiate host functions needed by gojs
//	gojs.MustInstantiate(ctx, r)
//
//	// Assign any configuration relevant for your compiled wasm.
//	config := gojs.NewConfig(wazero.NewConfig())
//
//	// Run your wasm, notably handing any ExitError
//	err = gojs.Run(ctx, r, compiled, config)
//	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
//		log.Panicln(err)
//	} else if !ok {
//		log.Panicln(err)
//	}
//
// # Notes
//
//   - Wasm generated by `GOARCH=wasm GOOS=js` is very slow to compile: Use
//     wazero.RuntimeConfig with wazero.CompilationCache when re-running the
//     same binary.
//   - The guest module is closed after being run.
func Run(ctx context.Context, r wazero.Runtime, compiled wazero.CompiledModule, moduleConfig Config) error {
	c := moduleConfig.(*cfg)
	_, err := RunAndReturnState(ctx, r, compiled, c.moduleConfig, c.internal)
	return err
}
