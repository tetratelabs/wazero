package internalwasm

import (
	"fmt"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// HostExports implements wasm.HostExports
type HostExports struct {
	NameToFunctionInstance map[string]*FunctionInstance
}

// Function implements wasm.HostExports Function
func (g *HostExports) Function(name string) publicwasm.HostFunction {
	return g.NameToFunctionInstance[name]
}

func (s *Store) getOrCreateHostModuleInstance(moduleName string) *ModuleInstance {
	if m, ok := s.moduleInstances[moduleName]; ok {
		return m
	}

	m := &ModuleInstance{Name: moduleName, Exports: make(map[string]*ExportInstance, 0)}
	s.moduleInstances[moduleName] = m
	return m
}

// ExportHostFunctions is defined internally for use in WASI tests and to keep the code size in the root directory small.
func (s *Store) ExportHostFunctions(moduleName string, nameToGoFunc map[string]interface{}) (publicwasm.HostExports, error) {
	if err := s.requireModuleUnused(moduleName); err != nil {
		return nil, err
	}

	exportCount := len(nameToGoFunc)

	m := s.getOrCreateHostModuleInstance(moduleName)
	ret := HostExports{NameToFunctionInstance: make(map[string]*FunctionInstance, exportCount)}
	for name, goFunc := range nameToGoFunc {
		if hf, err := NewGoFunc(name, goFunc); err != nil {
			return nil, err
		} else if function, err := s.exportHostFunction(m, hf); err != nil {
			return nil, err
		} else {
			ret.NameToFunctionInstance[name] = function
		}
	}
	return &ret, nil
}

// exportHostFunction exports a function so that it can be imported under the given module and name. If a function already
// exists for this module and name it is ignored rather than overwritten.
//
// Note: The wasm.Memory of the fn will be from the importing module.
func (s *Store) exportHostFunction(m *ModuleInstance, hf *GoFunc) (*FunctionInstance, error) {
	typeInstance, err := s.getTypeInstance(hf.functionType)
	if err != nil {
		return nil, err
	}

	f := &FunctionInstance{
		Name:           fmt.Sprintf("%s.%s", m.Name, hf.wasmFunctionName),
		HostFunction:   hf.goFunc,
		FunctionKind:   hf.functionKind,
		FunctionType:   typeInstance,
		ModuleInstance: m,
	}

	s.addFunctionInstances(f)

	if err = s.engine.Compile(f); err != nil {
		return nil, fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}

	if err = m.addExport(hf.wasmFunctionName, &ExportInstance{Type: ExternTypeFunc, Function: f}); err != nil {
		return nil, err
	}

	m.Functions = append(m.Functions, f)
	return f, nil
}

func (s *Store) ExportHostGlobal(m *ModuleInstance, name string, value uint64, valueType ValueType, mutable bool) error {
	g := &GlobalInstance{
		Val:  value,
		Type: &GlobalType{Mutable: mutable, ValType: valueType},
	}

	m.Globals = append(m.Globals, g)
	s.addGlobalInstances(g)

	return m.addExport(name, &ExportInstance{Type: ExternTypeGlobal, Global: g})
}

func (s *Store) ExportHostTableInstance(m *ModuleInstance, name string, min uint32, max *uint32) error {
	t := newTableInstance(min, max)

	// TODO: check if the module already has memory, and if so, returns error.
	m.TableInstance = t
	s.addTableInstance(t)

	return m.addExport(name, &ExportInstance{Type: ExternTypeTable, Table: t})
}

func (s *Store) ExportHostMemoryInstance(m *ModuleInstance, name string, min uint32, max *uint32) error {
	memory := &MemoryInstance{
		Buffer: make([]byte, MemoryPagesToBytesNum(min)),
		Min:    min,
		Max:    max,
	}

	// TODO: check if the module already has memory, and if so, returns error.
	m.MemoryInstance = memory
	s.addMemoryInstance(memory)

	return m.addExport(name, &ExportInstance{Type: ExternTypeMemory, Memory: memory})
}
