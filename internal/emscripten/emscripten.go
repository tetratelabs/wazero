package emscripten

import (
	"context"
	"strconv"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const FunctionNotifyMemoryGrowth = "emscripten_notify_memory_growth"

var NotifyMemoryGrowth = &wasm.HostFunc{
	ExportName: FunctionNotifyMemoryGrowth,
	Name:       FunctionNotifyMemoryGrowth,
	ParamTypes: []wasm.ValueType{wasm.ValueTypeI32},
	ParamNames: []string{"memory_index"},
	Code:       wasm.Code{GoFunc: api.GoModuleFunc(func(context.Context, api.Module, []uint64) {})},
}

// InvokePrefix is the naming convention of Emscripten dynamic functions.
//
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
// and invoke it with the remaining parameters.
const InvokePrefix = "invoke_"

func NewInvokeFunc(importName string, params, results []api.ValueType) *wasm.HostFunc {
	// The type we invoke is the same type as the import except without the
	// index parameter.
	fn := &InvokeFunc{&wasm.FunctionType{Results: results}}
	if len(params) > 1 {
		fn.FunctionType.Params = params[1:]
	}

	// Now, make friendly parameter names.
	paramNames := make([]string, len(params))
	paramNames[0] = "index"
	for i := 1; i < len(paramNames); i++ {
		paramNames[i] = "a" + strconv.Itoa(i)
	}
	return &wasm.HostFunc{
		ExportName:  importName,
		ParamTypes:  params,
		ParamNames:  paramNames,
		ResultTypes: results,
		Code:        wasm.Code{GoFunc: fn},
	}
}

type InvokeFunc struct {
	*wasm.FunctionType
}

// Call implements api.GoModuleFunction by special casing dynamic calls needed
// for emscripten `invoke_` functions such as `invoke_ii` or `invoke_v`.
func (v *InvokeFunc) Call(ctx context.Context, mod api.Module, stack []uint64) {
	m := mod.(*wasm.ModuleInstance)

	// Lookup the type of the function we are calling indirectly.
	typeID, err := m.GetFunctionTypeID(v.FunctionType)
	if err != nil {
		panic(err)
	}

	tableOffset := wasm.Index(stack[0]) // position in the module's only table.
	params := stack[1:]                 // parameters to the dynamic function being called

	// Lookup the table index we will call.
	t := m.Tables[0] // Note: Emscripten doesn't use multiple tables
	idx, err := m.Engine.LookupFunction(t, typeID, tableOffset)
	if err != nil {
		panic(err)
	}

	ret, err := m.Engine.NewFunction(idx).Call(ctx, params...)
	if err != nil {
		panic(err)
	}
	// if there are any results, copy them back to the stack
	copy(stack, ret)
}
