package table

import (
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// LookupFunction tries to get an api.Function from the table instance specified by `tableIndex` and `tableOffset` in the
// given api.Module. The user of this function must be well aware of the structure of the given api.Module,
// and the offset and table index must be valid. If this fails to find it, e.g. table is not found,
// table offset is out of range, violates the expected types, this panics according to the same semantics as
// call_indirect instruction: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/instructions.html#xref-syntax-instructions-syntax-instr-control-mathsf-call-indirect-x-y
//
//   - `module` is a module instance to look up the function.
//   - `tableIndex` is the index of the table instance in the module.
//   - `tableOffset` is the offset of the lookup target in the table.
//   - `expectedParamTypes` and `expectedResultTypes` are used to check the type of the function found in the table.
//
// Note: the returned api.Function is always valid, i.e. not nil, if this returns without panic.
func LookupFunction(
	module api.Module, tableIndex uint32, tableOffset uint32,
	expectedParamTypes, expectedResultTypes []api.ValueType,
) api.Function {
	m := module.(*wasm.ModuleInstance)
	typ := &wasm.FunctionType{Params: expectedParamTypes, Results: expectedResultTypes}
	typ.CacheNumInUint64()
	typeID := m.GetFunctionTypeID(typ)
	if int(tableIndex) >= len(m.Tables) {
		panic("table index out of range")
	}
	table := m.Tables[tableIndex]
	return m.LookupFunction(table, typeID, tableOffset)
}
