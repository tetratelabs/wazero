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
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

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
// runtime default namespace.
//
// # Notes
//
//   - Failure cases are documented on wazero.Namespace InstantiateModule.
//   - Closing the wazero.Runtime has the same effect as closing the result.
//   - To add more functions to the "env" module, use FunctionExporter.
//   - To instantiate into another wazero.Namespace, use FunctionExporter.
func Instantiate(ctx context.Context, r wazero.Runtime) (api.Closer, error) {
	builder := r.NewHostModuleBuilder("env")
	NewFunctionExporter().ExportFunctions(builder)
	return builder.Instantiate(ctx, r)
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
	exporter.ExportHostFunc(notifyMemoryGrowth)
	exporter.ExportHostFunc(invokeI)
	exporter.ExportHostFunc(invokeIi)
	exporter.ExportHostFunc(invokeIii)
	exporter.ExportHostFunc(invokeIiii)
	exporter.ExportHostFunc(invokeIiiii)
	exporter.ExportHostFunc(invokeV)
	exporter.ExportHostFunc(invokeVi)
	exporter.ExportHostFunc(invokeVii)
	exporter.ExportHostFunc(invokeViii)
	exporter.ExportHostFunc(invokeViiii)
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

// All `invoke_` functions have an initial "index" parameter of
// api.ValueTypeI32. This is the index of the desired funcref in the only table
// in the module. The type of the funcref is via naming convention. The first
// character after `invoke_` decides the result type: 'v' for no result or 'i'
// for api.ValueTypeI32. Any count of 'i' following that are api.ValueTypeI32
// parameters.
//
// For example, the function `invoke_iiiii` signature has five parameters, but
// also five i's. The five 'i's mean there are four parameters
//
//	(import "env" "invoke_iiiii" (func $invoke_iiiii
//		(param i32 i32 i32 i32 i32) (result i32))))
//
// So, the above function if invoked `invoke_iiiii(1234, 1, 2, 3, 4)` would
// look up the funcref at table index 1234, which has a type i32i32i3232_i32
// and invoke it with the remaining parameters,
const (
	i32 = wasm.ValueTypeI32

	functionInvokeI     = "invoke_i"
	functionInvokeIi    = "invoke_ii"
	functionInvokeIii   = "invoke_iii"
	functionInvokeIiii  = "invoke_iiii"
	functionInvokeIiiii = "invoke_iiiii"

	functionInvokeV     = "invoke_v"
	functionInvokeVi    = "invoke_vi"
	functionInvokeVii   = "invoke_vii"
	functionInvokeViii  = "invoke_viii"
	functionInvokeViiii = "invoke_viiii"
)

var invokeI = &wasm.HostFunc{
	ExportNames: []string{functionInvokeI},
	Name:        functionInvokeI,
	ParamTypes:  []api.ValueType{i32},
	ParamNames:  []string{"index"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeIFn),
	},
}

func invokeIFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "v_i32", wasm.Index(params[0]), nil)
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeIi = &wasm.HostFunc{
	ExportNames: []string{functionInvokeIi},
	Name:        functionInvokeIi,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"index", "a1"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeIiFn),
	},
}

func invokeIiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32_i32", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeIii = &wasm.HostFunc{
	ExportNames: []string{functionInvokeIii},
	Name:        functionInvokeIii,
	ParamTypes:  []api.ValueType{i32, i32, i32},
	ParamNames:  []string{"index", "a1", "a2"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeIiiFn),
	},
}

func invokeIiiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32i32_i32", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeIiii = &wasm.HostFunc{
	ExportNames: []string{functionInvokeIiii},
	Name:        functionInvokeIiii,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32},
	ParamNames:  []string{"index", "a1", "a2", "a3"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeIiiiFn),
	},
}

func invokeIiiiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32i32i32_i32", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeIiiii = &wasm.HostFunc{
	ExportNames: []string{functionInvokeIiiii},
	Name:        functionInvokeIiiii,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32, i32},
	ParamNames:  []string{"index", "a1", "a2", "a3", "a4"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeIiiiiFn),
	},
}

func invokeIiiiiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32i32i32i32_i32", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeV = &wasm.HostFunc{
	ExportNames: []string{functionInvokeV},
	Name:        functionInvokeV,
	ParamTypes:  []api.ValueType{i32},
	ParamNames:  []string{"index"},
	ResultTypes: []api.ValueType{},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeVFn),
	},
}

func invokeVFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "v_v", wasm.Index(params[0]), nil)
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeVi = &wasm.HostFunc{
	ExportNames: []string{functionInvokeVi},
	Name:        functionInvokeVi,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"index", "a1"},
	ResultTypes: []api.ValueType{},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeViFn),
	},
}

func invokeViFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32_v", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeVii = &wasm.HostFunc{
	ExportNames: []string{functionInvokeVii},
	Name:        functionInvokeVii,
	ParamTypes:  []api.ValueType{i32, i32, i32},
	ParamNames:  []string{"index", "a1", "a2"},
	ResultTypes: []api.ValueType{},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeViiFn),
	},
}

func invokeViiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32i32_v", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeViii = &wasm.HostFunc{
	ExportNames: []string{functionInvokeViii},
	Name:        functionInvokeViii,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32},
	ParamNames:  []string{"index", "a1", "a2", "a3"},
	ResultTypes: []api.ValueType{},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeViiiFn),
	},
}

func invokeViiiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32i32i32_v", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

var invokeViiii = &wasm.HostFunc{
	ExportNames: []string{functionInvokeViiii},
	Name:        functionInvokeViiii,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32, i32},
	ParamNames:  []string{"index", "a1", "a2", "a3", "a4"},
	ResultTypes: []api.ValueType{},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         api.GoModuleFunc(invokeViiiiFn),
	},
}

func invokeViiiiFn(ctx context.Context, mod api.Module, params []uint64) []uint64 {
	ret, err := callDynamic(ctx, mod.(*wasm.CallContext), "i32i32i32i32_v", wasm.Index(params[0]), params[1:])
	if err != nil {
		panic(err)
	}
	return ret
}

// callDynamic special cases dynamic calls needed for emscripten `invoke_`
// functions such as `invoke_ii` or `invoke_v`.
//
// # Parameters
//
//   - ctx: the propagated go context.
//   - callCtx: the incoming context of the `invoke_` function.
//   - typeName: used to look up the function type. ex "i32i32_i32" or "v_i32"
//   - tableOffset: position in the module's only table
//   - params: parameters to the funcref
func callDynamic(ctx context.Context, callCtx *wasm.CallContext, typeName string, tableOffset wasm.Index, params []uint64) (results []uint64, err error) {
	m := callCtx.Module()
	typeId, ok := m.TypeIDIndex[typeName]
	if !ok {
		return nil, wasmruntime.ErrRuntimeIndirectCallTypeMismatch
	}

	t := m.Tables[0] // Emscripten doesn't use multiple tables
	idx, err := m.Engine.LookupFunction(t, typeId, tableOffset)
	if err != nil {
		return nil, err
	}
	return callCtx.Function(idx).Call(ctx, params...)
}
