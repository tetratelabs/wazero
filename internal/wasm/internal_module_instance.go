package wasm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// InternalModuleInstance wraps ModuleInstance to implement
// experimental.InternalModule.
type InternalModuleInstance struct {
	M *ModuleInstance

	// Reusable memory for Globals().
	globals []api.Global
}

// Globals implements experimental.InternalModule.
func (m InternalModuleInstance) Globals() []api.Global {
	if cap(m.globals) < len(m.M.Globals) {
		m.globals = make([]api.Global, len(m.M.Globals))
	} else {
		m.globals = m.globals[:len(m.M.Globals)]
	}
	for i, g := range m.M.Globals {
		m.globals[i] = internalGlobal{g}
	}
	return m.globals
}

// Close implements api.Module.
func (m InternalModuleInstance) Close(ctx context.Context) error {
	return m.M.Close(ctx)
}

// CloseWithExitCode implements api.Module.
func (m InternalModuleInstance) CloseWithExitCode(ctx context.Context, exitCode uint32) error {
	return m.M.CloseWithExitCode(ctx, exitCode)
}

// String implements api.Module.
func (m InternalModuleInstance) String() string {
	return m.M.String()
}

// Name implements api.Module.
func (m InternalModuleInstance) Name() string {
	return m.M.Name()
}

// Memory implements api.Module.
func (m InternalModuleInstance) Memory() api.Memory {
	return m.M.Memory()
}

// ExportedFunction implements api.Module.
func (m InternalModuleInstance) ExportedFunction(name string) api.Function {
	return m.M.ExportedFunction(name)
}

// ExportedFunctionDefinitions implements api.Module.
func (m InternalModuleInstance) ExportedFunctionDefinitions() map[string]api.FunctionDefinition {
	return m.M.ExportedFunctionDefinitions()
}

// ExportedMemory implements api.Module.
func (m InternalModuleInstance) ExportedMemory(name string) api.Memory {
	return m.M.ExportedMemory(name)
}

// ExportedMemoryDefinitions implements api.Module.
func (m InternalModuleInstance) ExportedMemoryDefinitions() map[string]api.MemoryDefinition {
	return m.M.ExportedMemoryDefinitions()
}

// ExportedGlobal implements api.Module.
func (m InternalModuleInstance) ExportedGlobal(name string) api.Global {
	return m.M.ExportedGlobal(name)
}

// internalGlobal wraps GlobalInstance to implement api.Global.
type internalGlobal struct {
	g *GlobalInstance
}

// Type implements api.Global.
func (g internalGlobal) Type() api.ValueType {
	return g.g.Type.ValType
}

// Get implements api.Global.
func (g internalGlobal) Get() uint64 {
	return g.g.Val
}

// String implements api.Global.
func (g internalGlobal) String() string {
	return fmt.Sprintf("global(%d)", g.g.Val)
}
