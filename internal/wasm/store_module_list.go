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

		// shrink the map if it's allocated more than twice the size of the list
		nameToNodeLen := len(s.nameToNode)
		if s.nameToNodeCap > initialNameToNodeListSize && s.nameToNodeCap > nameToNodeLen*2 {
			newCap := initialNameToNodeListSize
			if newCap < nameToNodeLen {
				newCap = nameToNodeLen
			}

			nameToNode := make(map[string]*moduleListNode, newCap)
			for k, v := range s.nameToNode {
				nameToNode[k] = v
			}
			s.nameToNode = nameToNode
			s.nameToNodeCap = nameToNodeLen
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
	s.nameToModule[dst] = s.nameToModule[src]
	s.nameToNodeCap++
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
