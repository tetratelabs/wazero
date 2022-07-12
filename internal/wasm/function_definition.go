package wasm

import (
	"github.com/tetratelabs/wazero/api"
)

// ImportedFunctions returns the definitions of each imported function.
func (m *Module) ImportedFunctions() (ret []api.FunctionDefinition) {
	for _, d := range m.functionDefinitions {
		if d.importDesc != nil {
			ret = append(ret, d)
		}
	}
	return
}

// ExportedFunctions returns the definitions of each exported function.
func (m *Module) ExportedFunctions() (ret []api.FunctionDefinition) {
	for _, d := range m.functionDefinitions {
		if d.exportNames != nil {
			ret = append(ret, d)
		}
	}
	return
}

// BuildFunctionDefinitions generates function metadata that can be parsed from
// the module. This must be called after all validation.
//
// Note: This is exported for tests who don't use wazero.Runtime or
// NewHostModule to compile the module.
func (m *Module) BuildFunctionDefinitions() *Module {
	if len(m.FunctionSection) == 0 {
		return m
	}

	var moduleName string
	var functionNames NameMap
	var localNames IndirectNameMap
	if m.NameSection != nil {
		moduleName = m.NameSection.ModuleName
		functionNames = m.NameSection.FunctionNames
		localNames = m.NameSection.LocalNames
	}

	importCount := m.ImportFuncCount()
	m.functionDefinitions = make([]*functionDefinition, 0, importCount+uint32(len(m.FunctionSection)))

	importFuncIdx := Index(0)
	for _, i := range m.ImportSection {
		if i.Type != ExternTypeFunc {
			continue
		}

		m.functionDefinitions = append(m.functionDefinitions, &functionDefinition{
			importDesc: &[2]string{i.Module, i.Name},
			index:      importFuncIdx,
			funcType:   m.TypeSection[i.DescFunc],
		})
		importFuncIdx++
	}

	for codeIndex, typeIndex := range m.FunctionSection {
		m.functionDefinitions = append(m.functionDefinitions, &functionDefinition{
			index:    Index(codeIndex) + importCount,
			funcType: m.TypeSection[typeIndex],
		})
	}

	n, nLen := 0, len(functionNames)
	for _, d := range m.functionDefinitions {
		// The function name section begins with imports, but can be sparse.
		// This keeps track of how far in the name section we've searched.
		funcIdx := d.index
		var funcName string
		for ; n < nLen; n++ {
			next := functionNames[n]
			if next.Index > funcIdx {
				break // we have function names, but starting at a later index.
			} else if next.Index == funcIdx {
				funcName = next.Name
				break
			}
		}

		d.moduleName = moduleName
		d.name = funcName
		d.paramNames = paramNames(localNames, funcIdx, len(d.funcType.Params))

		for _, e := range m.ExportSection {
			if e.Type == ExternTypeFunc && e.Index == funcIdx {
				d.exportNames = append(d.exportNames, e.Name)
			}
		}
	}
	return m
}

// functionDefinition implements api.FunctionDefinition
type functionDefinition struct {
	moduleName  string
	index       Index
	name        string
	funcType    *FunctionType
	importDesc  *[2]string
	exportNames []string
	paramNames  []string
}

// ModuleName implements the same method as documented on api.FunctionDefinition.
func (f *functionDefinition) ModuleName() string {
	return f.moduleName
}

// Index implements the same method as documented on api.FunctionDefinition.
func (f *functionDefinition) Index() uint32 {
	return f.index
}

// Name implements the same method as documented on api.FunctionDefinition.
func (f *functionDefinition) Name() string {
	return f.name
}

// Import implements the same method as documented on api.FunctionDefinition.
func (f *functionDefinition) Import() (moduleName, name string, isImport bool) {
	if importDesc := f.importDesc; importDesc != nil {
		return importDesc[0], importDesc[1], true
	}
	return "", "", false
}

// ExportNames implements the same method as documented on api.FunctionDefinition.
func (f *functionDefinition) ExportNames() []string {
	return f.exportNames
}

// ParamNames implements the same method as documented on api.FunctionDefinition.
func (f *functionDefinition) ParamNames() []string {
	return f.paramNames
}

// ParamTypes implements api.FunctionDefinition ParamTypes.
func (f *functionDefinition) ParamTypes() []ValueType {
	return f.funcType.Params
}

// ResultTypes implements api.FunctionDefinition ResultTypes.
func (f *functionDefinition) ResultTypes() []ValueType {
	return f.funcType.Results
}
