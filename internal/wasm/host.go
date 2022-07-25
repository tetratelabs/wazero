package wasm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tetratelabs/wazero/internal/wasmdebug"
)

// HostFunc is a function with an inlined type, used for NewHostModule.
// Any corresponding FunctionType will be reused or added to the Module.
type HostFunc struct {
	// ExportNames is equivalent to  the same method on api.FunctionDefinition.
	ExportNames []string

	// Name is equivalent to  the same method on api.FunctionDefinition.
	Name string

	// ParamTypes is equivalent to  the same method on api.FunctionDefinition.
	ParamTypes []ValueType

	// ParamNames is equivalent to  the same method on api.FunctionDefinition.
	ParamNames []string

	// ResultTypes is equivalent to  the same method on api.FunctionDefinition.
	ResultTypes []ValueType

	// Code is the equivalent function in the SectionIDCode.
	Code *Code
}

// NewGoFunc returns a HostFunc for the given parameters or panics.
func NewGoFunc(exportName string, name string, paramNames []string, fn interface{}) *HostFunc {
	return (&HostFunc{
		ExportNames: []string{exportName},
		Name:        name,
		ParamNames:  paramNames,
	}).MustGoFunc(fn)
}

// MustGoFunc calls WithGoFunc or panics on error.
func (f *HostFunc) MustGoFunc(fn interface{}) *HostFunc {
	if ret, err := f.WithGoFunc(fn); err != nil {
		panic(err)
	} else {
		return ret
	}
}

// WithGoFunc returns a copy of the function, replacing its Code.GoFunc.
func (f *HostFunc) WithGoFunc(fn interface{}) (*HostFunc, error) {
	ret := *f
	var err error
	ret.ParamTypes, ret.ResultTypes, ret.Code, err = parseGoFunc(fn)
	return &ret, err
}

// WithWasm returns a copy of the function, replacing its Code.Body.
func (f *HostFunc) WithWasm(body []byte) *HostFunc {
	ret := *f
	ret.Code = &Code{IsHostFunction: true, Body: body}
	if f.Code != nil {
		ret.Code.LocalTypes = f.Code.LocalTypes
	}
	return &ret
}

// NewHostModule is defined internally for use in WASI tests and to keep the code size in the root directory small.
func NewHostModule(
	moduleName string,
	nameToGoFunc map[string]interface{},
	funcToNames map[string][]string,
	nameToMemory map[string]*Memory,
	nameToGlobal map[string]*Global,
	enabledFeatures Features,
) (m *Module, err error) {
	if moduleName != "" {
		m = &Module{NameSection: &NameSection{ModuleName: moduleName}}
	} else {
		m = &Module{}
	}

	funcCount := uint32(len(nameToGoFunc))
	memoryCount := uint32(len(nameToMemory))
	globalCount := uint32(len(nameToGlobal))
	exportCount := funcCount + memoryCount + globalCount
	if exportCount > 0 {
		m.ExportSection = make([]*Export, 0, exportCount)
	}

	// Check name collision as exports cannot collide on names, regardless of type.
	for name := range nameToGoFunc {
		// manually generate the error message as we don't have debug names yet.
		if _, ok := nameToMemory[name]; ok {
			return nil, fmt.Errorf("func[%s.%s] exports the same name as a memory", moduleName, name)
		}
		if _, ok := nameToGlobal[name]; ok {
			return nil, fmt.Errorf("func[%s.%s] exports the same name as a global", moduleName, name)
		}
	}
	for name := range nameToMemory {
		if _, ok := nameToGlobal[name]; ok {
			return nil, fmt.Errorf("memory[%s] exports the same name as a global", name)
		}
	}

	if funcCount > 0 {
		if err = addFuncs(m, nameToGoFunc, funcToNames, enabledFeatures); err != nil {
			return
		}
	}

	if memoryCount > 0 {
		if err = addMemory(m, nameToMemory); err != nil {
			return
		}
	}

	// TODO: we can use enabledFeatures to fail early on things like mutable globals (once supported)
	if globalCount > 0 {
		if err = addGlobals(m, nameToGlobal); err != nil {
			return
		}
	}

	// Assigns the ModuleID by calculating sha256 on inputs as host modules do not have `wasm` to hash.
	m.AssignModuleID([]byte(fmt.Sprintf("%s:%v:%v:%v:%v",
		moduleName, nameToGoFunc, nameToMemory, nameToGlobal, enabledFeatures)))
	m.BuildFunctionDefinitions()
	return
}

func addFuncs(
	m *Module,
	nameToGoFunc map[string]interface{},
	funcToNames map[string][]string,
	enabledFeatures Features,
) (err error) {
	if m.NameSection == nil {
		m.NameSection = &NameSection{}
	}
	moduleName := m.NameSection.ModuleName
	nameToFunc := make(map[string]*HostFunc, len(nameToGoFunc))
	sortedExportNames := make([]string, len(nameToFunc))
	for k := range nameToGoFunc {
		sortedExportNames = append(sortedExportNames, k)
	}

	// Sort names for consistent iteration
	sort.Strings(sortedExportNames)

	funcNames := make([]string, len(nameToFunc))
	for _, k := range sortedExportNames {
		v := nameToGoFunc[k]
		if hf, ok := v.(*HostFunc); ok {
			nameToFunc[hf.Name] = hf
			funcNames = append(funcNames, hf.Name)
		} else {
			params, results, code, ftErr := parseGoFunc(v)
			if ftErr != nil {
				return fmt.Errorf("func[%s.%s] %w", moduleName, k, ftErr)
			}
			hf = &HostFunc{
				ExportNames: []string{k},
				Name:        k,
				ParamTypes:  params,
				ResultTypes: results,
				Code:        code,
			}
			if names := funcToNames[k]; names != nil {
				namesLen := len(names)
				if namesLen > 1 && namesLen-1 != len(params) {
					return fmt.Errorf("func[%s.%s] has %d params, but %d param names", moduleName, k, namesLen-1, len(params))
				}
				hf.Name = names[0]
				hf.ParamNames = names[1:]
			}
			nameToFunc[k] = hf
			funcNames = append(funcNames, k)
		}
	}

	funcCount := uint32(len(nameToFunc))
	m.NameSection.FunctionNames = make([]*NameAssoc, 0, funcCount)
	m.FunctionSection = make([]Index, 0, funcCount)
	m.CodeSection = make([]*Code, 0, funcCount)
	m.FunctionDefinitionSection = make([]*FunctionDefinition, 0, funcCount)

	idx := Index(0)
	for _, name := range funcNames {
		hf := nameToFunc[name]
		debugName := wasmdebug.FuncName(moduleName, name, idx)
		typeIdx, typeErr := m.maybeAddType(hf.ParamTypes, hf.ResultTypes, enabledFeatures)
		if typeErr != nil {
			return fmt.Errorf("func[%s] %v", debugName, typeErr)
		}
		m.FunctionSection = append(m.FunctionSection, typeIdx)
		m.CodeSection = append(m.CodeSection, hf.Code)
		for _, export := range hf.ExportNames {
			m.ExportSection = append(m.ExportSection, &Export{Type: ExternTypeFunc, Name: export, Index: idx})
		}
		m.NameSection.FunctionNames = append(m.NameSection.FunctionNames, &NameAssoc{Index: idx, Name: hf.Name})
		if len(hf.ParamNames) > 0 {
			localNames := &NameMapAssoc{Index: idx}
			for i, n := range hf.ParamNames {
				localNames.NameMap = append(localNames.NameMap, &NameAssoc{Index: Index(i), Name: n})
			}
			m.NameSection.LocalNames = append(m.NameSection.LocalNames, localNames)
		}
		idx++
	}
	return nil
}

func addMemory(m *Module, nameToMemory map[string]*Memory) error {
	memoryCount := uint32(len(nameToMemory))

	// Only one memory can be defined or imported
	if memoryCount > 1 {
		memoryNames := make([]string, 0, memoryCount)
		for k := range nameToMemory {
			memoryNames = append(memoryNames, k)
		}
		sort.Strings(memoryNames) // For consistent error messages
		return fmt.Errorf("only one memory is allowed, but configured: %s", strings.Join(memoryNames, ", "))
	}

	// Find the memory name to export.
	var name string
	for k, v := range nameToMemory {
		name = k
		if v.Min > v.Max {
			return fmt.Errorf("memory[%s] min %d pages (%s) > max %d pages (%s)", name, v.Min, PagesToUnitOfBytes(v.Min), v.Max, PagesToUnitOfBytes(v.Max))
		}
		m.MemorySection = v
	}

	m.ExportSection = append(m.ExportSection, &Export{Type: ExternTypeMemory, Name: name, Index: 0})
	return nil
}

func addGlobals(m *Module, globals map[string]*Global) error {
	globalCount := len(globals)
	m.GlobalSection = make([]*Global, 0, globalCount)

	globalNames := make([]string, 0, globalCount)
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames) // For consistent iteration order

	for i, name := range globalNames {
		m.GlobalSection = append(m.GlobalSection, globals[name])
		m.ExportSection = append(m.ExportSection, &Export{Type: ExternTypeGlobal, Name: name, Index: Index(i)})
	}
	return nil
}

func (m *Module) maybeAddType(params, results []ValueType, enabledFeatures Features) (Index, error) {
	if len(results) > 1 {
		// Guard >1.0 feature multi-value
		if err := enabledFeatures.Require(FeatureMultiValue); err != nil {
			return 0, fmt.Errorf("multiple result types invalid as %v", err)
		}
	}
	for i, t := range m.TypeSection {
		if t.EqualsSignature(params, results) {
			return Index(i), nil
		}
	}

	result := m.SectionElementCount(SectionIDType)
	toAdd := &FunctionType{Params: params, Results: results}
	m.TypeSection = append(m.TypeSection, toAdd)
	return result, nil
}
