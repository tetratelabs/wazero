package wasm

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// deleteModule makes the moduleName available for instantiation again.
func (s *Store) deleteModule(m *ModuleInstance) error {
	if m == nil {
		return nil
	}

	s.mux.Lock()
	defer s.mux.Unlock()

	// remove this module name
	if m.prev != nil {
		m.prev.next = m.next
	}
	if m.next != nil {
		m.next.prev = m.prev
	}
	if s.moduleList == m {
		s.moduleList = m.next
	}
	// clear the m state so it does not enter any other branch
	// on subsequent calls to deleteModule
	m.prev = nil
	m.next = nil

	if m.ModuleName != "" {
		delete(s.nameToNode, m.ModuleName)
	}
	return nil
}

// module returns the module of the given name or error if not in this store
func (s *Store) module(moduleName string) (*ModuleInstance, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()
	m, ok := s.nameToNode[moduleName]
	if !ok {
		return nil, fmt.Errorf("module[%s] not in store", moduleName)
	}

	if m == nil {
		return nil, fmt.Errorf("module[%s] not set in store", moduleName)
	}
	return m, nil
}

// requireModules returns all instantiated modules whose names equal the keys in the input, or errs if any are missing.
func (s *Store) requireModules(moduleNames map[string]struct{}) (map[string]*ModuleInstance, error) {
	ret := make(map[string]*ModuleInstance, len(moduleNames))

	s.mux.RLock()
	defer s.mux.RUnlock()

	for n := range moduleNames {
		module, ok := s.nameToNode[n]
		if !ok {
			return nil, fmt.Errorf("module[%s] not instantiated", n)
		}
		ret[n] = module
	}
	return ret, nil
}

// registerModule registers
// This makes the module visible for import, and ensures it is closed when the store is.
func (s *Store) registerModule(m *ModuleInstance) {
	s.mux.Lock()
	defer s.mux.Unlock()

	// add the newest node to the moduleNamesList as the head.
	m.next = s.moduleList
	if m.next != nil {
		m.next.prev = m
	}
	s.moduleList = m

	if m.ModuleName != "" {
		s.nameToNode[m.ModuleName] = m
	}
}

// AliasModule aliases the instantiated module named `src` as `dst`.
//
// Note: This is only used for spectests.
func (s *Store) AliasModule(src, dst string) error {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.nameToNode[dst] = s.nameToNode[src]
	return nil
}

// Module implements wazero.Runtime Module
func (s *Store) Module(moduleName string) api.Module {
	m, err := s.module(moduleName)
	if err != nil {
		return nil
	}
	return m
}
