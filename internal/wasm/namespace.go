package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero/api"
)

// Namespace is a collection of instantiated modules which cannot conflict on name.
type Namespace struct {
	// moduleNamesList ensures modules are closed in reverse initialization order.
	moduleNamesList []string // guarded by mux

	// moduleNamesSet ensures no race conditions instantiating two modules of the same name
	moduleNamesSet map[string]struct{} // guarded by mux

	// modules holds the instantiated Wasm modules by module name from Instantiate.
	modules map[string]*ModuleInstance // guarded by mux

	// mux is used to guard the fields from concurrent access.
	mux sync.RWMutex
}

// newNamespace returns an empty namespace.
func newNamespace() *Namespace {
	return &Namespace{
		moduleNamesList: nil,
		moduleNamesSet:  map[string]struct{}{},
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
	delete(ns.modules, moduleName)
	delete(ns.moduleNamesSet, moduleName)
	// remove this module name
	for i, n := range ns.moduleNamesList {
		if n == moduleName {
			ns.moduleNamesList = append(ns.moduleNamesList[:i], ns.moduleNamesList[i+1:]...)
			break
		}
	}
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
	ns.moduleNamesSet[moduleName] = struct{}{}
	ns.moduleNamesList = append(ns.moduleNamesList, moduleName)
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
	for i := len(ns.moduleNamesList) - 1; i >= 0; i-- {
		// If closing this module errs, proceed anyway to close the others.
		if m, ok := ns.modules[ns.moduleNamesList[i]]; ok {
			if _, e := m.CallCtx.close(ctx, exitCode); e != nil && err == nil {
				err = e // first error
			}
		}
	}
	ns.moduleNamesList = nil
	ns.moduleNamesSet = map[string]struct{}{}
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
