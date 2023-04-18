// Package emscripten contains Go-defined special functions imported by
// Emscripten under the module name "env".
//
// Emscripten has many imports which are triggered on build flags. Use
// FunctionExporter, instead of Instantiate, to define more "env" functions.
//
// # Relationship to WASI
//
// Emscripten typically requires wasi_snapshot_preview1 to implement exit.
//
// See wasi_snapshot_preview1.Instantiate and
// https://github.com/emscripten-core/emscripten/wiki/WebAssembly-Standalone
package emscripten

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	internal "github.com/tetratelabs/wazero/internal/emscripten"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const i32 = wasm.ValueTypeI32

// MustInstantiate calls Instantiate or panics on error.
//
// This is a simpler function for those who know the module "env" is not
// already instantiated, and don't need to unload it.
func MustInstantiate(ctx context.Context, r wazero.Runtime) {
	if _, err := Instantiate(ctx, r); err != nil {
		panic(err)
	}
}

// Instantiate instantiates the "env" module used by Emscripten into the
// runtime.
//
// # Notes
//
//   - Failure cases are documented on wazero.Runtime InstantiateModule.
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewHostModuleBuilder("env")
	NewFunctionExporter().ExportFunctions(builder)
	return builder.Instantiate(ctx)
}

// FunctionExporter configures the functions in the "env" module used by
// Emscripten.
type FunctionExporter interface {
	// ExportFunctions builds functions to export with a wazero.HostModuleBuilder
	// named "env".
	ExportFunctions(wazero.HostModuleBuilder)
}

// NewFunctionExporter returns a FunctionExporter object with trace disabled.
func NewFunctionExporter() FunctionExporter {
	return &functionExporter{}
}

type functionExporter struct{}

// ExportFunctions implements FunctionExporter.ExportFunctions
func (functionExporter) ExportFunctions(builder wazero.HostModuleBuilder) {
	exporter := builder.(wasm.HostFuncExporter)
	exporter.ExportHostFunc(internal.NotifyMemoryGrowth)

	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_i", []api.ValueType{i32}, []api.ValueType{i32}))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_ii", []api.ValueType{i32, i32}, []api.ValueType{i32}))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_iii", []api.ValueType{i32, i32, i32}, []api.ValueType{i32}))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_iiii", []api.ValueType{i32, i32, i32, i32}, []api.ValueType{i32}))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_iiiii", []api.ValueType{i32, i32, i32, i32, i32}, []api.ValueType{i32}))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_v", []api.ValueType{i32}, nil))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_vi", []api.ValueType{i32, i32}, nil))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_vii", []api.ValueType{i32, i32, i32}, nil))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_viii", []api.ValueType{i32, i32, i32, i32}, nil))
	exporter.ExportHostFunc(internal.NewInvokeFunc("invoke_viiii", []api.ValueType{i32, i32, i32, i32, i32}, nil))
}
