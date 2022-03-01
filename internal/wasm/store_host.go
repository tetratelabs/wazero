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

// ExportHostFunctions is defined internally for use in WASI tests and to keep the code size in the root directory small.
func (s *Store) ExportHostModule(config *publicwasm.HostModuleConfig) (publicwasm.HostExports, error) {
	moduleName := config.Name

	if err := s.requireModuleUnused(moduleName); err != nil {
		return nil, err
	}

	m := &ModuleInstance{Name: moduleName, Exports: make(map[string]*ExportInstance)}
	s.moduleInstances[moduleName] = m

	if err := s.exportHostFunctions(m, config.Functions); err != nil {
		return nil, s.ReleaseModuleInstance(m)
	}

	if err := s.exportHostGlobals(m, config.Globals); err != nil {
		return nil, s.ReleaseModuleInstance(m)
	}

	if err := s.exportHostTableInstance(m, config.Table); err != nil {
		return nil, s.ReleaseModuleInstance(m)
	}

	if err := s.exportHostMemoryInstance(m, config.Memory); err != nil {
		return nil, s.ReleaseModuleInstance(m)
	}

	ret := &HostExports{NameToFunctionInstance: make(map[string]*FunctionInstance, len(config.Functions))}
	for name := range config.Functions {
		ret.NameToFunctionInstance[name] = m.Exports[name].Function
	}

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

func (s *Store) exportHostGlobals(m *ModuleInstance, globals map[string]*publicwasm.HostModuleConfigGlobal) error {
	for name, config := range globals {
		g := &GlobalInstance{
			Val:  config.Value,
			Type: &GlobalType{ValType: config.Type},
		}

		m.Globals = append(m.Globals, g)
		s.addGlobalInstances(g)

		if err := m.addExport(name, &ExportInstance{Type: ExternTypeGlobal, Global: g}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) exportHostTableInstance(m *ModuleInstance, config *publicwasm.HostModuleConfigTable) error {
	if config == nil {
		return nil
	}

	t := newTableInstance(config.Min, config.Max)

	// TODO: check if the module already has memory, and if so, returns error.
	m.TableInstance = t
	s.addTableInstance(t)

	return m.addExport(config.Name, &ExportInstance{Type: ExternTypeTable, Table: t})
}

func (s *Store) exportHostMemoryInstance(m *ModuleInstance, config *publicwasm.HostModuleConfigMemory) error {
	if config == nil {
		return nil
	}

	memory := &MemoryInstance{
		Buffer: make([]byte, MemoryPagesToBytesNum(config.Min)),
		Min:    config.Min,
		Max:    config.Max,
	}

	// TODO: check if the module already has memory, and if so, returns error.
	m.MemoryInstance = memory
	s.addMemoryInstance(memory)

	return m.addExport(config.Name, &ExportInstance{Type: ExternTypeMemory, Memory: memory})
}
