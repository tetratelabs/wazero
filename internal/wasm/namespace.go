package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero/api"
)

// nameListNode is a node in a doubly linked list of names.
type nameListNode struct {
	name       string
	next, prev *nameListNode
}

// Namespace is a collection of instantiated modules which cannot conflict on name.
type Namespace struct {
	// moduleNamesList ensures modules are closed in reverse initialization order.
	moduleNamesList *nameListNode // guarded by mux

	// moduleNamesSet ensures no race conditions instantiating two modules of the same name
	moduleNamesSet map[string]*nameListNode // guarded by mux

	// modules holds the instantiated Wasm modules by module name from Instantiate.
	modules map[string]*ModuleInstance // guarded by mux

	// mux is used to guard the fields from concurrent access.
	mux sync.RWMutex
}

// newNamespace returns an empty namespace.
func newNamespace() *Namespace {
	return &Namespace{
		moduleNamesList: nil,
		moduleNamesSet:  map[string]*nameListNode{},
		modules:         map[string]*ModuleInstance{},
	}
}

// addModule makes the module visible for import.
func (ns *Namespace) addModule(m *ModuleInstance) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	ns.modules[m.Name] = m
}

// deleteModule makes the moduleName available for instantiation again.
func (ns *Namespace) deleteModule(moduleName string) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	node, ok := ns.moduleNamesSet[moduleName]
	if !ok {
		return
	}

	// remove this module name
	if node.prev == nil {
		ns.moduleNamesList = node.next
	} else {
		node.prev.next = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}

	delete(ns.modules, moduleName)
	delete(ns.moduleNamesSet, moduleName)
}

// module returns the module of the given name or nil if not in this namespace
func (ns *Namespace) module(moduleName string) *ModuleInstance {
	ns.mux.RLock()
	defer ns.mux.RUnlock()
	return ns.modules[moduleName]
}

// requireModules returns all instantiated modules whose names equal the keys in the input, or errs if any are missing.
func (ns *Namespace) requireModules(moduleNames map[string]struct{}) (map[string]*ModuleInstance, error) {
	ret := make(map[string]*ModuleInstance, len(moduleNames))

	ns.mux.RLock()
	defer ns.mux.RUnlock()

	for n := range moduleNames {
		m, ok := ns.modules[n]
		if !ok {
			return nil, fmt.Errorf("module[%s] not instantiated", n)
		}
		ret[n] = m
	}
	return ret, nil
}

// requireModuleName is a pre-flight check to reserve a module.
// This must be reverted on error with deleteModule if initialization fails.
func (ns *Namespace) requireModuleName(moduleName string) error {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	if _, ok := ns.moduleNamesSet[moduleName]; ok {
		return fmt.Errorf("module[%s] has already been instantiated", moduleName)
	}

	// add the newest node to the moduleNamesList as the head.
	node := &nameListNode{
		name: moduleName,
		next: ns.moduleNamesList,
	}
	if node.next != nil {
		node.next.prev = node
	}
	ns.moduleNamesList = node
	ns.moduleNamesSet[moduleName] = node
	return nil
}

// AliasModule aliases the instantiated module named `src` as `dst`.
//
// Note: This is only used for spectests.
func (ns *Namespace) AliasModule(src, dst string) {
	ns.modules[dst] = ns.modules[src]
}

// CloseWithExitCode implements the same method as documented on wazero.Namespace.
func (ns *Namespace) CloseWithExitCode(ctx context.Context, exitCode uint32) (err error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	// Close modules in reverse initialization order.
	for node := ns.moduleNamesList; node != nil; node = node.next {
		// If closing this module errs, proceed anyway to close the others.
		if m, ok := ns.modules[node.name]; ok {
			if _, e := m.CallCtx.close(ctx, exitCode); e != nil && err == nil {
				err = e // first error
			}
		}
	}
	ns.moduleNamesList = nil
	ns.moduleNamesSet = map[string]*nameListNode{}
	ns.modules = map[string]*ModuleInstance{}
	return
}

// Module implements wazero.Namespace Module
func (ns *Namespace) Module(moduleName string) api.Module {
	if m := ns.module(moduleName); m != nil {
		return m.CallCtx
	} else {
		return nil
	}
}
