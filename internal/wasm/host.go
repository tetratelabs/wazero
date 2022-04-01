package wasm

import (
	"fmt"
	"reflect"
	"sort"
)

// NewHostModule is defined internally for use in WASI tests and to keep the code size in the root directory small.
func NewHostModule(moduleName string, nameToGoFunc map[string]interface{}) (*Module, error) {
	hostFunctionCount := uint32(len(nameToGoFunc))
	if hostFunctionCount == 0 {
		if moduleName != "" {
			return &Module{NameSection: &NameSection{ModuleName: moduleName}}, nil
		} else {
			return &Module{}, nil
		}
	}

	m := &Module{
		NameSection:         &NameSection{ModuleName: moduleName, FunctionNames: make([]*NameAssoc, 0, hostFunctionCount)},
		HostFunctionSection: make([]*reflect.Value, 0, hostFunctionCount),
		ExportSection:       make(map[string]*Export, hostFunctionCount),
	}

	// Ensure insertion order is consistent
	names := make([]string, 0, hostFunctionCount)
	for k := range nameToGoFunc {
		names = append(names, k)
	}
	sort.Strings(names)

	for idx := Index(0); idx < hostFunctionCount; idx++ {
		name := names[idx]
		fn := reflect.ValueOf(nameToGoFunc[name])
		_, functionType, _, err := getFunctionType(&fn, false)
		if err != nil {
			return nil, fmt.Errorf("func[%s] %w", name, err)
		}

		m.FunctionSection = append(m.FunctionSection, m.maybeAddType(functionType))
		m.HostFunctionSection = append(m.HostFunctionSection, &fn)
		m.ExportSection[name] = &Export{Type: ExternTypeFunc, Name: name, Index: idx}
		m.NameSection.FunctionNames = append(m.NameSection.FunctionNames, &NameAssoc{Index: idx, Name: name})
	}
	return m, nil
}

func (m *Module) maybeAddType(ft *FunctionType) Index {
	for i, t := range m.TypeSection {
		if t.EqualsSignature(ft.Params, ft.Results) {
			return Index(i)
		}
	}

	result := m.SectionElementCount(SectionIDType)
	m.TypeSection = append(m.TypeSection, ft)
	return result
}

func (m *Module) validateHostFunctions() error {
	functionCount := m.SectionElementCount(SectionIDFunction)
	hostFunctionCount := m.SectionElementCount(SectionIDHostFunction)
	if functionCount == 0 && hostFunctionCount == 0 {
		return nil
	}

	typeCount := m.SectionElementCount(SectionIDType)
	if hostFunctionCount != functionCount {
		return fmt.Errorf("host function count (%d) != function count (%d)", hostFunctionCount, functionCount)
	}

	for idx, typeIndex := range m.FunctionSection {
		_, ft, _, err := getFunctionType(m.HostFunctionSection[idx], false)
		if err != nil {
			return fmt.Errorf("%s is not a valid go func: %w", m.funcDesc(SectionIDHostFunction, Index(idx)), err)
		}

		if typeIndex >= typeCount {
			return fmt.Errorf("%s type section index out of range: %d", m.funcDesc(SectionIDHostFunction, Index(idx)), typeIndex)
		}

		t := m.TypeSection[typeIndex]
		if !t.EqualsSignature(ft.Params, ft.Results) {
			return fmt.Errorf("%s signature doesn't match type section: %s != %s", m.funcDesc(SectionIDHostFunction, Index(idx)), ft, t)
		}
	}
	return nil
}

func (m *Module) buildHostFunctionInstances() (functions []*FunctionInstance) {
	var functionNames = m.NameSection.FunctionNames
	for idx, typeIndex := range m.FunctionSection {
		fn := m.HostFunctionSection[idx]
		f := &FunctionInstance{
			Name:   functionNames[idx].Name,
			Kind:   kind(fn.Type()),
			Type:   m.TypeSection[typeIndex],
			GoFunc: fn,
			Index:  Index(idx),
		}
		functions = append(functions, f)
	}
	return
}
