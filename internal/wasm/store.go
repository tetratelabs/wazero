package internalwasm

import (
	"bytes"
	"context"
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/leb128"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

type (
	// Store is the runtime representation of "instantiated" Wasm module and objects.
	// Multiple modules can be instantiated within a single store, and each instance,
	// (e.g. function instance) can be referenced by other module instances in a Store via Module.ImportSection.
	//
	// Every type whose name ends with "Instance" suffix belongs to exactly one store.
	//
	// Note that store is not thread (concurrency) safe, meaning that using single Store
	// via multiple goroutines might result in race conditions. In that case, the invocation
	// and access to any methods and field of Store must be guarded by mutex.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#store%E2%91%A0
	Store struct {
		// The following fields are wazero-specific fields of Store.

		// ctx is the default context used for function calls
		ctx context.Context

		// engine is a global context for a Store which is in responsible for compilation and execution of Wasm modules.
		engine Engine

		// ModuleInstances holds the instantiated Wasm modules by module name from Instantiate.
		moduleInstances map[string]*ModuleInstance

		// hostExports holds host functions by module name from ExportHostFunctions.
		hostExports map[string]*HostExports

		// ModuleContexts holds default host function call contexts keyed by module name.
		moduleContexts map[string]*ModuleContext

		// TypeIDs maps each FunctionType.String() to a unique FunctionTypeID. This is used at runtime to
		// do type-checks on indirect function calls.
		typeIDs map[string]FunctionTypeID

		// maximumFunctionIndex represents the limit on the number of function addresses (= function instances) in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionIndex FunctionIndex
		//  maximumFunctionTypes represents the limit on the number of function types in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionTypes int

		// releasedFunctionIndex holds reusable FunctionIndexes. An index is added when
		// an function instance is released in releaseFunctionInstances, and is popped when
		// an new instance is added in store.addFunctionInstances.
		releasedFunctionIndex []FunctionIndex

		// releasedMemoryIndex holds reusable memoryIndexes. An index is added when
		// an memory instance is released in releaseMemoryInstance, and is popped when
		// an new instance is added in store.addMemoryInstance.
		releasedMemoryIndex []memoryIndex

		// releasedTableIndex holds reusable tableIndexes. An index is added when
		// an table instance is released in releaseTableInstance, and is popped when
		// an new instance is added in store.addTableInstance.
		releasedTableIndex []tableIndex

		// releasedGlobalIndex holds reusable globalIndexes. An index is added when
		// an global instance is released in releaseGlobalInstances, and is popped when
		// an new instance is added in store.addGlobalInstances.
		releasedGlobalIndex []globalIndex

		// The followings fields match the definition of Store in the specification.

		// Functions holds function instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as funcaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr).
		functions []*FunctionInstance
		// Globals holds global instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as globaladdr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-globaladdr).
		globals []*GlobalInstance
		// Memories holds memory instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as memaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-memaddr).
		memories []*MemoryInstance
		// Tables holds table instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as tableaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-tableaddr).
		tables []*TableInstance
	}
)

// The wazero specific limitations described at RATIONALE.md.
const (
	maximumFunctionIndex = 1 << 27
	maximumFunctionTypes = 1 << 27
)

func NewStore(ctx context.Context, engine Engine) *Store {
	return &Store{
		ctx:                  ctx,
		moduleInstances:      map[string]*ModuleInstance{},
		moduleContexts:       map[string]*ModuleContext{},
		typeIDs:              map[string]FunctionTypeID{},
		engine:               engine,
		maximumFunctionIndex: maximumFunctionIndex,
		maximumFunctionTypes: maximumFunctionTypes,
	}
}

// checkFunctionIndexOverflow checks if there would be too many function instances in a store.
func (s *Store) checkFunctionIndexOverflow(newInstanceNum int) error {
	if len(s.functions) > int(s.maximumFunctionIndex)-newInstanceNum {
		return fmt.Errorf("too many functions in a store")
	}
	return nil
}

func (s *Store) Instantiate(module *Module, name string) (*ModuleContext, error) {
	if err := s.requireModuleUnused(name); err != nil {
		return nil, err
	}

	if err := s.checkFunctionIndexOverflow(len(module.FunctionSection)); err != nil {
		return nil, err
	}

	importedFunctions, importedGlobals, importedTable, importedMemory, moduleImports, err := s.resolveImports(module)
	if err != nil {
		return nil, err
	}

	functions, globals, table, memory :=
		module.buildFunctionInstances(), module.buildGlobalInstances(importedGlobals), module.buildTableInstance(), module.buildMemoryInstance()

	types, err := s.getTypeInstances(module.TypeSection)
	if err != nil {
		return nil, err
	}

	// Now we have all instances from imports and local ones, so ready to create a new ModuleInstance.
	instance := newModuleInstance(name, module, importedFunctions, functions, importedGlobals,
		globals, importedTable, table, importedMemory, memory, types, moduleImports)

	if err = instance.validateElements(module.ElementSection); err != nil {
		return nil, err
	}

	if err := instance.validateData(module.DataSection); err != nil {
		return nil, err
	}

	// Now we are ready to compile functions.
	s.addFunctionInstances(functions...) // Need to assign funcaddr to each instance before compilation.
	for i, f := range functions {
		if err := s.engine.Compile(f); err != nil {
			// On the failure, release the assigned funcaddr and already compiled functions.
			if err := s.releaseFunctionInstances(functions...); err != nil {
				return nil, err
			}
			idx := module.SectionElementCount(SectionIDFunction) - 1
			return nil, fmt.Errorf("compilation failed at index %d/%d: %w", i, idx, err)
		}
	}

	// Now all the validation passes, we are safe to mutate memory/table instances (possibly imported ones).
	instance.applyElements(module.ElementSection)
	instance.applyData(module.DataSection)

	// Persist the instances other than functions (which we already persisted before compilation).
	s.addGlobalInstances(globals...)
	s.addTableInstance(table)
	s.addMemoryInstance(instance.MemoryInstance)

	// Increase the reference count of imported modules.
	for imported := range instance.moduleImports {
		imported.refCount++
	}

	// Build the default context for calls to this module.
	modCtx := NewModuleContext(s.ctx, s.engine, instance)
	s.moduleContexts[instance.Name] = modCtx
	s.moduleInstances[instance.Name] = instance

	// Execute the start function.
	if module.StartSection != nil {
		funcIdx := *module.StartSection
		if _, err := s.engine.Call(modCtx, instance.Functions[funcIdx]); err != nil {
			return nil, fmt.Errorf("module[%s] start function failed: %w", name, err)
		}
	}
	return modCtx, nil
}

func (s *Store) ReleaseModuleInstance(instance *ModuleInstance) error {
	instance.refCount--
	if instance.refCount > 0 {
		// This case other modules are importing this module instance and still alive.
		return nil
	}

	// TODO: check outstanding calls and wait until they exit.

	// Recursively release the imported instances.
	for mod := range instance.moduleImports {
		if err := s.ReleaseModuleInstance(mod); err != nil {
			return fmt.Errorf("unable to release imported module [%s]: %w", mod.Name, err)
		}
	}

	if err := s.releaseFunctionInstances(instance.Functions...); err != nil {
		return fmt.Errorf("unable to release function instance: %w", err)
	}
	s.releaseMemoryInstance(instance.MemoryInstance)
	s.releaseTableInstance(instance.TableInstance)
	s.releaseGlobalInstances(instance.Globals...)

	// Explicitly assign nil so that we ensure this moduleInstance no longer holds reference to instances.
	instance.Exports = nil
	instance.Globals = nil
	instance.Functions = nil
	instance.TableInstance = nil
	instance.MemoryInstance = nil
	instance.Types = nil

	s.moduleContexts[instance.Name] = nil
	s.moduleInstances[instance.Name] = nil
	return nil
}

func (s *Store) releaseFunctionInstances(fs ...*FunctionInstance) error {
	for _, f := range fs {
		if err := s.engine.Release(f); err != nil {
			return err
		}

		// Release refernce to the function instance.
		s.functions[f.Index] = nil

		// Append the address so that we can reuse it in order to avoid index space explosion.
		s.releasedFunctionIndex = append(s.releasedFunctionIndex, f.Index)
	}
	return nil
}

func (s *Store) addFunctionInstances(fs ...*FunctionInstance) {
	for _, f := range fs {
		var addr FunctionIndex
		if len(s.releasedFunctionIndex) > 0 {
			id := len(s.releasedFunctionIndex) - 1
			// Pop one address from releasedFunctionIndex slice.
			addr, s.releasedFunctionIndex = s.releasedFunctionIndex[id], s.releasedFunctionIndex[:id]
			s.functions[f.Index] = f
		} else {
			addr = FunctionIndex(len(s.functions))
			s.functions = append(s.functions, f)
		}
		f.Index = addr
	}
}

func (s *Store) releaseGlobalInstances(gs ...*GlobalInstance) {
	for _, g := range gs {
		// Release refernce to the global instance.
		s.globals[g.index] = nil

		// Append the address so that we can reuse it in order to avoid index space explosion.
		s.releasedGlobalIndex = append(s.releasedGlobalIndex, g.index)
	}
}

func (s *Store) addGlobalInstances(gs ...*GlobalInstance) {
	for _, g := range gs {
		var addr globalIndex
		if len(s.releasedGlobalIndex) > 0 {
			id := len(s.releasedGlobalIndex) - 1
			// Pop one address from releasedGlobalIndex slice.
			addr, s.releasedGlobalIndex = s.releasedGlobalIndex[id], s.releasedGlobalIndex[:id]
			s.globals[g.index] = g
		} else {
			addr = globalIndex(len(s.globals))
			s.globals = append(s.globals, g)
		}
		g.index = addr
	}
}

func (s *Store) releaseTableInstance(t *TableInstance) {
	// Release refernce to the table instance.
	s.tables[t.index] = nil

	// Append the index so that we can reuse it in order to avoid index space explosion.
	s.releasedTableIndex = append(s.releasedTableIndex, t.index)
}

func (s *Store) addTableInstance(t *TableInstance) {
	if t == nil {
		return
	}

	var addr tableIndex
	if len(s.releasedTableIndex) > 0 {
		id := len(s.releasedTableIndex) - 1
		// Pop one index from releasedTableIndex slice.
		addr, s.releasedTableIndex = s.releasedTableIndex[id], s.releasedTableIndex[:id]
		s.tables[addr] = t
	} else {
		addr = tableIndex(len(s.tables))
		s.tables = append(s.tables, t)
	}
	t.index = addr
}

func (s *Store) releaseMemoryInstance(m *MemoryInstance) {
	// Release refernce to the memory instance.
	s.memories[m.index] = nil

	// Append the index so that we can reuse it in order to avoid index space explosion.
	s.releasedMemoryIndex = append(s.releasedMemoryIndex, m.index)
}

func (s *Store) addMemoryInstance(m *MemoryInstance) {
	if m == nil {
		return
	}

	var addr memoryIndex
	if len(s.releasedMemoryIndex) > 0 {
		id := len(s.releasedMemoryIndex) - 1
		// Pop one index from releasedMemoryIndex slice.
		addr, s.releasedMemoryIndex = s.releasedMemoryIndex[id], s.releasedMemoryIndex[:id]
		s.memories[addr] = m
	} else {
		addr = memoryIndex(len(s.memories))
		s.memories = append(s.memories, m)
	}
	m.index = addr
}

func (s *Store) AliasModuleInstance(src, dst string) {
	s.moduleInstances[dst] = s.moduleInstances[src]
}

// ModuleExports implements wasm.Store ModuleExports
func (s *Store) ModuleExports(moduleName string) publicwasm.ModuleExports {
	if m, ok := s.moduleContexts[moduleName]; !ok {
		return nil
	} else {
		return m
	}
}

func (s *Store) requireModuleUnused(moduleName string) error {
	if _, ok := s.hostExports[moduleName]; ok {
		return fmt.Errorf("module %s has already been exported by this host", moduleName)
	}
	if _, ok := s.moduleContexts[moduleName]; ok {
		return fmt.Errorf("module %s has already been instantiated", moduleName)
	}
	return nil
}

// HostExports implements wasm.Store HostExports
func (s *Store) HostExports(moduleName string) publicwasm.HostExports {
	return s.hostExports[moduleName]
}

func (s *Store) getExport(moduleName string, name string, et ExternType) (exp *ExportInstance, err error) {
	if m, ok := s.moduleInstances[moduleName]; !ok {
		return nil, fmt.Errorf("module %s not instantiated", moduleName)
	} else if exp, err = m.GetExport(name, et); err != nil {
		return
	}
	return
}

func (s *Store) resolveImports(module *Module) (
	functions []*FunctionInstance, globals []*GlobalInstance,
	table *TableInstance, memory *MemoryInstance,
	moduleImports map[*ModuleInstance]struct{},
	err error,
) {
	moduleImports = map[*ModuleInstance]struct{}{}
	for _, is := range module.ImportSection {
		m, ok := s.moduleInstances[is.Module]
		if !ok {
			err = fmt.Errorf("module \"%s\" not instantiated", is.Module)
			return
		}

		// Note: at this point we don't increase the ref count.
		moduleImports[m] = struct{}{}

		var exp *ExportInstance
		exp, err = m.GetExport(is.Name, is.Type)
		if err != nil {
			return
		}

		switch is.Type {
		case ExternTypeFunc:
			typeIndex := is.DescFunc
			if int(typeIndex) >= len(module.TypeSection) {
				err = fmt.Errorf("unknown type for function import")
				return
			}
			expectedType := module.TypeSection[typeIndex]
			f := exp.Function
			if !bytes.Equal(expectedType.Results, f.FunctionType.Type.Results) || !bytes.Equal(expectedType.Params, f.FunctionType.Type.Params) {
				err = fmt.Errorf("signature mimatch: %s != %s", expectedType, f.FunctionType.Type)
				return
			}
			functions = append(functions, f)
		case ExternTypeTable:
			tableType := is.DescTable
			table = exp.Table
			if table.ElemType != tableType.ElemType {
				err = fmt.Errorf("incompatible table improt: element type mismatch")
				return
			}
			if table.Min < tableType.Limit.Min {
				err = fmt.Errorf("incompatible table import: minimum size mismatch")
				return
			}

			if tableType.Limit.Max != nil {
				if table.Max == nil {
					err = fmt.Errorf("incompatible table import: maximum size mismatch")
					return
				} else if *table.Max > *tableType.Limit.Max {
					err = fmt.Errorf("incompatible table import: maximum size mismatch")
					return
				}
			}
		case ExternTypeMemory:
			memoryType := is.DescMem
			memory = exp.Memory
			if memory.Min < memoryType.Min {
				err = fmt.Errorf("incompatible memory import: minimum size mismatch")
				return
			}
			if memoryType.Max != nil {
				if memory.Max == nil {
					err = fmt.Errorf("incompatible memory import: maximum size mismatch")
					return
				} else if *memory.Max > *memoryType.Max {
					err = fmt.Errorf("incompatible memory import: maximum size mismatch")
					return
				}
			}
		case ExternTypeGlobal:
			globalType := is.DescGlobal
			g := exp.Global
			if globalType.Mutable != g.Type.Mutable {
				err = fmt.Errorf("incompatible global import: mutability mismatch")
				return
			} else if globalType.ValType != g.Type.ValType {
				err = fmt.Errorf("incompatible global import: value type mismatch")
				return
			}
			globals = append(globals, g)
		}
	}
	return
}

func executeConstExpression(globals []*GlobalInstance, expr *ConstantExpression) (v interface{}) {
	r := bytes.NewBuffer(expr.Data)
	switch expr.Opcode {
	case OpcodeI32Const:
		v, _, _ = leb128.DecodeInt32(r)
	case OpcodeI64Const:
		v, _, _ = leb128.DecodeInt64(r)
	case OpcodeF32Const:
		v, _ = ieee754.DecodeFloat32(r)
	case OpcodeF64Const:
		v, _ = ieee754.DecodeFloat64(r)
	case OpcodeGlobalGet:
		id, _, _ := leb128.DecodeUint32(r)
		g := globals[id]
		switch g.Type.ValType {
		case ValueTypeI32:
			v = int32(g.Val)
		case ValueTypeI64:
			v = int64(g.Val)
		case ValueTypeF32:
			v = publicwasm.DecodeF32(g.Val)
		case ValueTypeF64:
			v = publicwasm.DecodeF64(g.Val)
		}
	}
	return
}

func (s *Store) getTypeInstances(ts []*FunctionType) ([]*TypeInstance, error) {
	ret := make([]*TypeInstance, len(ts))
	for i, t := range ts {
		inst, err := s.getTypeInstance(t)
		if err != nil {
			return nil, err
		}
		ret[i] = inst
	}
	return ret, nil
}

func (s *Store) getTypeInstance(t *FunctionType) (*TypeInstance, error) {
	key := t.String()
	id, ok := s.typeIDs[key]
	if !ok {
		l := len(s.typeIDs)
		if l >= s.maximumFunctionTypes {
			return nil, fmt.Errorf("too many function types in a store")
		}
		id = FunctionTypeID(len(s.typeIDs))
		s.typeIDs[key] = id
	}
	return &TypeInstance{Type: t, TypeID: id}, nil
}

func newTableInstance(min uint32, max *uint32) *TableInstance {
	tableInst := &TableInstance{
		Table:    make([]TableElement, min),
		Min:      min,
		Max:      max,
		ElemType: 0x70, // funcref
	}
	for i := range tableInst.Table {
		tableInst.Table[i] = TableElement{
			FunctionTypeID: UninitializedTableElementTypeID,
		}
	}
	return tableInst
}

// UninitializedTableElementTypeID math.MaxUint32 to represent the uninitialized elements.
const UninitializedTableElementTypeID FunctionTypeID = math.MaxUint32
