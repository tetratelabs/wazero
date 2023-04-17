package wasm

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// deleteModule makes the moduleName available for instantiation again.
func (s *Store) deleteModule(m *ModuleInstance) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	// Remove this module name.
	if m.prev != nil {
		m.prev.next = m.next
	}
	if m.next != nil {
		m.next.prev = m.prev
	}
	if s.moduleList == m {
		s.moduleList = m.next
	}
	// Clear the m state so it does not enter any other branch
	// on subsequent calls to deleteModule.
	m.prev = nil
	m.next = nil

	if m.ModuleName != "" {
		delete(s.nameToModule, m.ModuleName)

		// Under normal circumstances, m.aliases will be nil this loop will not
		// be entered unless aliases have been created. See `*store.AliasModule`
		for _, alias := range m.aliases {
			delete(s.nameToModule, alias)
		}

		// Shrink the map if it's allocated more than twice the size of the list
		newCap := len(s.nameToModule)
		if newCap < nameToModuleShrinkThreshold {
			newCap = nameToModuleShrinkThreshold
		}
		if newCap*2 <= s.nameToModuleCap {
			nameToModule := make(map[string]*ModuleInstance, newCap)
			for k, v := range s.nameToModule {
				nameToModule[k] = v
			}
			s.nameToModule = nameToModule
			s.nameToModuleCap = newCap
		}
	}
	return nil
}

// module returns the module of the given name or error if not in this store
func (s *Store) module(moduleName string) (*ModuleInstance, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()
	m, ok := s.nameToModule[moduleName]
	if !ok {
		return nil, fmt.Errorf("module[%s] not instantiated", moduleName)
	}
	return m, nil
}

// registerModule registers a ModuleInstance into the store.
// This makes the ModuleInstance visible for import if it's not anonymous, and ensures it is closed when the store is.
func (s *Store) registerModule(m *ModuleInstance) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.nameToModule == nil {
		return errors.New("already closed")
	}

	if m.ModuleName != "" {
		if _, ok := s.nameToModule[m.ModuleName]; ok {
			return fmt.Errorf("module[%s] has already been instantiated", m.ModuleName)
		}
		s.nameToModule[m.ModuleName] = m
		if len(s.nameToModule) > s.nameToModuleCap {
			s.nameToModuleCap = len(s.nameToModule)
		}
	}

	// Add the newest node to the moduleNamesList as the head.
	m.next = s.moduleList
	if m.next != nil {
		m.next.prev = m
	}
	s.moduleList = m
	return nil
}

// AliasModule aliases the instantiated module named `src` as `dst`.
//
// Note: This is only used for spectests.
func (s *Store) AliasModule(src, dst string) error {
	s.mux.Lock()
	defer s.mux.Unlock()
	if _, ok := s.nameToModule[dst]; ok {
		return nil
	}
	m, ok := s.nameToModule[src]
	if !ok {
		return nil
	}
	m.aliases = append(m.aliases, dst)
	s.nameToModule[dst] = m
	if len(s.nameToModule) > s.nameToModuleCap {
		s.nameToModuleCap = len(s.nameToModule)
	}
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
