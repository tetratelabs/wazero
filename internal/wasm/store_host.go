package internalwasm

import (
	"fmt"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// hostExports implements wasm.HostExports
type hostExports struct {
	NameToFunctionInstance map[string]*FunctionInstance
}

// Function implements wasm.HostExports Function
func (g *hostExports) Function(name string) publicwasm.HostFunction {
	return g.NameToFunctionInstance[name]
}

// ExportHostFunctions is defined internally for use in WASI tests and to keep the code size in the root directory small.
func (s *Store) ExportHostModule(moduleName string, nametToGoFunc map[string]interface{}) (publicwasm.HostExports, error) {
	if err := s.requireModuleUnused(moduleName); err != nil {
		return nil, err
	}

	m := &ModuleInstance{Name: moduleName, Exports: make(map[string]*ExportInstance)}
	s.moduleInstances[moduleName] = m

	if err := s.exportHostFunctions(m, nametToGoFunc); err != nil {
		return nil, s.ReleaseModuleInstance(m)
	}

	// TODO: Allow globals, table and memory per https://github.com/tetratelabs/wazero/issues/279

	ret := &hostExports{NameToFunctionInstance: make(map[string]*FunctionInstance, len(nametToGoFunc))}
	for name := range nametToGoFunc {
		ret.NameToFunctionInstance[name] = m.Exports[name].Function
	}

	s.hostExports[moduleName] = ret
	return ret, nil
}

// exportHostFunctions is defined internally for use in WASI tests and to keep the code size in the root directory small.
func (s *Store) exportHostFunctions(m *ModuleInstance, nameToGoFunc map[string]interface{}) error {
	for name, goFunc := range nameToGoFunc {
		hf, err := NewGoFunc(name, goFunc)
		if err != nil {
			return err
		}
		if err = s.exportHostFunction(m, hf); err != nil {
			return err
		}
	}
	return nil
}

// exportHostFunction exports a function so that it can be imported under the given module and name. If a function already
// exists for this module and name it is ignored rather than overwritten.
//
// Note: The wasm.Memory of the fn will be from the importing module.
func (s *Store) exportHostFunction(m *ModuleInstance, hf *GoFunc) error {
	typeInstance, err := s.getTypeInstance(hf.functionType)
	if err != nil {
		return err
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
		return fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}

	if err = m.addExport(hf.wasmFunctionName, &ExportInstance{Type: ExternTypeFunc, Function: f}); err != nil {
		return err
	}

	m.Functions = append(m.Functions, f)
	return nil
}

func (s *Store) ExportHostGlobals(m *ModuleInstance, nameToValue map[string]uint64, nameToValueType map[string]ValueType) error {
	for name, v := range nameToValue {
		g := &GlobalInstance{
			Val:        v,
			GlobalType: &GlobalType{ValType: nameToValueType[name]},
		}

		m.Globals = append(m.Globals, g)
		s.addGlobalInstances(g)

		if err := m.addExport(name, &ExportInstance{Type: ExternTypeGlobal, Global: g}); err != nil {
			return err
		}
	}
	return nil
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
