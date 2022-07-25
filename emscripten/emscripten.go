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
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Instantiate instantiates the "env" module used by Emscripten into the
// runtime default namespace.
//
// # Notes
//
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
//   - To instantiate into another wazero.Namespace, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewModuleBuilder("env")
	NewFunctionExporter().ExportFunctions(builder)
	return builder.Instantiate(ctx, r)
}

// FunctionExporter configures the functions in the "env" module used by
// Emscripten.
type FunctionExporter interface {
	// ExportFunctions builds functions to export with a wazero.ModuleBuilder
	// named "env".
	ExportFunctions(builder wazero.ModuleBuilder)
}

// NewFunctionExporter returns a FunctionExporter object with trace disabled.
func NewFunctionExporter() FunctionExporter {
	return &functionExporter{}
}

type functionExporter struct{}

// ExportFunctions implements FunctionExporter.ExportFunctions
func (e *functionExporter) ExportFunctions(builder wazero.ModuleBuilder) {
	builder.ExportFunction(notifyMemoryGrowth.Name, notifyMemoryGrowth)
}

// emscriptenNotifyMemoryGrowth is called when wasm is compiled with
// `-s ALLOW_MEMORY_GROWTH` and a "memory.grow" instruction succeeded.
// The memoryIndex parameter will be zero until "multi-memory" is implemented.
//
// Note: This implementation is a no-op and can be overridden by users manually
// by redefining the same function. wazero will likely implement a generic
// memory growth hook obviating this as well.
//
// Here's the import in a user's module that ends up using this, in WebAssembly
// 1.0 (MVP) Text Format:
//
//	(import "env" "emscripten_notify_memory_growth"
//	  (func $emscripten_notify_memory_growth (param $memory_index i32)))
//
// See https://github.com/emscripten-core/emscripten/blob/3.1.16/system/lib/standalone/standalone.c#L118
// and https://emscripten.org/docs/api_reference/emscripten.h.html#abi-functions
const functionNotifyMemoryGrowth = "emscripten_notify_memory_growth"

var notifyMemoryGrowth = &wasm.HostFunc{
	ExportNames: []string{functionNotifyMemoryGrowth},
	Name:        functionNotifyMemoryGrowth,
	ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
	ParamNames:  []string{"memory_index"},
	Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeEnd}},
}
