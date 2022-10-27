package wasm

import (
	"errors"
	"fmt"
	"sort"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
)

type ProxyFuncExporter interface {
	ExportProxyFunc(*ProxyFunc)
}

// ProxyFunc is a function defined both in wasm and go. This is used to
// optimize the Go signature or obviate calls based on what can be done
// mechanically in wasm.
type ProxyFunc struct {
	// Proxy must be a wasm func
	Proxy *HostFunc
	// Proxied should be a go func.
	Proxied *HostFunc

	// CallBodyPos is the position in Code.Body of the caller to replace the
	// real funcIdx of the proxied.
	CallBodyPos int
}

func (p *ProxyFunc) Name() string {
	return p.Proxied.Name
}

type HostFuncExporter interface {
	ExportHostFunc(*HostFunc)
}

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

// MustGoReflectFunc calls WithGoReflectFunc or panics on error.
func (f *HostFunc) MustGoReflectFunc(fn interface{}) *HostFunc {
	if ret, err := f.WithGoReflectFunc(fn); err != nil {
		panic(err)
	} else {
		return ret
	}
}

// WithGoFunc returns a copy of the function, replacing its Code.GoFunc.
func (f *HostFunc) WithGoFunc(fn api.GoFunc) *HostFunc {
	ret := *f
	ret.Code = &Code{IsHostFunction: true, GoFunc: fn}
	return &ret
}

// WithGoModuleFunc returns a copy of the function, replacing its Code.GoFunc.
func (f *HostFunc) WithGoModuleFunc(fn api.GoModuleFunc) *HostFunc {
	ret := *f
	ret.Code = &Code{IsHostFunction: true, GoFunc: fn}
	return &ret
}

// WithGoReflectFunc returns a copy of the function, replacing its Code.GoFunc.
func (f *HostFunc) WithGoReflectFunc(fn interface{}) (*HostFunc, error) {
	ret := *f
	var err error
	ret.ParamTypes, ret.ResultTypes, ret.Code, err = parseGoReflectFunc(fn)
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
	enabledFeatures api.CoreFeatures,
) (m *Module, err error) {
	if moduleName != "" {
		m = &Module{NameSection: &NameSection{ModuleName: moduleName}}
	} else {
		m = &Module{}
	}

	if exportCount := uint32(len(nameToGoFunc)); exportCount > 0 {
		m.ExportSection = make([]*Export, 0, exportCount)
		if err = addFuncs(m, nameToGoFunc, funcToNames, enabledFeatures); err != nil {
			return
		}
	}

	// Assigns the ModuleID by calculating sha256 on inputs as host modules do not have `wasm` to hash.
	m.AssignModuleID([]byte(fmt.Sprintf("%s:%v:%v", moduleName, nameToGoFunc, enabledFeatures)))
	m.BuildFunctionDefinitions()
	return
}

// maxProxiedFuncIdx is the maximum index where leb128 encoding matches the bit
// of an unsigned literal byte. Using this simplifies host function index
// substitution.
//
// Note: this is 127, not 255 because when the MSB is set, leb128 encoding
// doesn't match the literal byte.
const maxProxiedFuncIdx = 127

func addFuncs(
	m *Module,
	nameToGoFunc map[string]interface{},
	funcToNames map[string][]string,
	enabledFeatures api.CoreFeatures,
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
		} else if pf, ok := v.(*ProxyFunc); ok {
			// First, add the proxied function which also gives us the real
			// position in the function index namespace, We will need this
			// later. We've kept code simpler by limiting the max index to
			// what is encodable in a single byte. This is ok as we don't have
			// any current use cases for hundreds of proxy functions.
			proxiedIdx := len(funcNames)
			if proxiedIdx > maxProxiedFuncIdx {
				return errors.New("TODO: proxied funcidx larger than one byte")
			}
			nameToFunc[pf.Proxied.Name] = pf.Proxied
			funcNames = append(funcNames, pf.Proxied.Name)

			// Now that we have the real index of the proxied function,
			// substitute that for the zero placeholder in the proxy's code
			// body. This placeholder is at index CallBodyPos in the slice.
			proxyBody := make([]byte, len(pf.Proxy.Code.Body))
			copy(proxyBody, pf.Proxy.Code.Body)
			proxyBody[pf.CallBodyPos] = byte(proxiedIdx)
			proxy := pf.Proxy.WithWasm(proxyBody)

			nameToFunc[proxy.Name] = proxy
			funcNames = append(funcNames, proxy.Name)
		} else { // reflection
			params, results, code, ftErr := parseGoReflectFunc(v)
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

func (m *Module) maybeAddType(params, results []ValueType, enabledFeatures api.CoreFeatures) (Index, error) {
	if len(results) > 1 {
		// Guard >1.0 feature multi-value
		if err := enabledFeatures.RequireEnabled(api.CoreFeatureMultiValue); err != nil {
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
