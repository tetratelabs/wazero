package wasm

import "github.com/tetratelabs/wazero/api"

// ExportedFunctions returns the implementations of api.ExportedFunction.
func (m *Module) ExportedFunctions() (ret []api.ExportedFunction) {
	for _, exp := range m.ExportSection {
		if exp.Type == ExternTypeFunc {
			tp := m.TypeOfFunction(exp.Index)
			ret = append(ret, &exportedFunction{
				exportedName: exp.Name,
				params:       tp.Params,
				results:      tp.Results,
			})
		}
	}
	return
}

// exportedFunction implements api.ExportedFunction
type exportedFunction struct {
	// exportedName is the name of export entry of this function,
	// which might differ from the one in the name custom section etc.
	exportedName    string
	params, results []ValueType
}

// Name implements api.ExportedFunction Name.
func (e *exportedFunction) Name() string {
	return e.exportedName
}

// ParamTypes implements api.ExportedFunction ParamTypes.
func (e *exportedFunction) ParamTypes() []ValueType {
	return e.params
}

// ResultTypes implements api.ExportedFunction ResultTypes.
func (e *exportedFunction) ResultTypes() []ValueType {
	return e.results
}
