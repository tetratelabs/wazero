package cranelift

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type (
	compilationContextFunctionIndexKey         struct{}
	compilationContextImportedFunctionCountKey struct{}
	compilationContextModuleKey                struct{}
	compilationContextVmContextOffsetsKey      struct{}
)

func newCompilationContext(m *wasm.Module, funcIndex wasm.Index, offsets *opaqueVmContextOffsets) context.Context {
	cmpCtx := context.WithValue(context.Background(), compilationContextModuleKey{}, m)
	cmpCtx = context.WithValue(cmpCtx, compilationContextImportedFunctionCountKey{}, m.ImportFuncCount())
	cmpCtx = context.WithValue(cmpCtx, compilationContextFunctionIndexKey{}, funcIndex)
	cmpCtx = context.WithValue(cmpCtx, compilationContextVmContextOffsetsKey{}, offsets)
	return cmpCtx
}

func (e *engine) addWazeroModule(ctx context.Context) {
	const wazeroModuleName = "wazero"
	funcs := map[string]interface{}{
		"compile_done":                        e.exportCompileDone,
		"func_index":                          e.exportFuncIndex,
		"current_func_type_index":             e.exportCurrentFuncTypeIndex,
		"func_type_index":                     e.exportFuncTypeIndex,
		"type_counts":                         e.exportTypeCounts,
		"type_lens":                           e.exportTypeLens,
		"type_param_at":                       e.exportTypeParamAt,
		"type_result_at":                      e.exportTypeResultAt,
		"is_locally_defined_function":         e.exportIsLocallyDefinedFunction,
		"memory_min_max":                      e.exportMemoryMinMax,
		"is_memory_imported":                  e.exportIsMemoryImported,
		"memory_instance_base_offset":         e.exportMemoryInstanceBaseOffset,
		"vm_context_local_memory_offset":      e.exportVmContextLocalMemoryOffset,
		"vm_context_imported_memory_offset":   e.exportVmContextImportedMemoryOffset,
		"vm_context_imported_function_offset": e.exportVmContextImportedFunctionOffset,
	}
	names := make(map[string]*wasm.HostFuncNames, len(funcs))
	for n := range funcs {
		names[n] = &wasm.HostFuncNames{}
	}
	wazero, err := wasm.NewHostModule(wazeroModuleName, funcs, names, craneliftFeature)
	if err != nil {
		panic(err)
	}
	e.instantiateHostModule(ctx, wazeroModuleName, wazero)
}

func (e *engine) exportCompileDone(ctx context.Context, mod api.Module, codePtr, codeSize, relocsPtr, relocCounts uint32) {
	m := mustModulePtrFromContext(ctx)

	compiled, ok := mod.Memory().Read(codePtr, codeSize)
	if !ok {
		panic(fmt.Sprintf("invalid memory position for compiled body: (ptr=%#x,size=%#x)", codePtr, codeSize))
	}

	// We have to copy the result into Go-allocated slice.
	body := make([]byte, codeSize)
	copy(body, compiled)

	var relocs []functionRelocationEntry
	if relocCounts > 0 {
		relocsEnd := relocCounts * uint32(unsafe.Sizeof(functionRelocationEntry{})) // TODO: make this as const and add assertion test.
		relocsBytes, ok := mod.Memory().Read(relocsPtr, relocsEnd)
		if !ok {
			panic(fmt.Sprintf("invalid memory position for relocs: (ptr=%#x,size=%#x)", relocsPtr, relocsEnd))
		}

		relocInfos := make([]byte, relocsEnd)
		copy(relocInfos, relocsBytes)

		relocs = *(*[]functionRelocationEntry)(unsafe.Pointer(&reflect.SliceHeader{ //nolint
			Data: uintptr(unsafe.Pointer(&relocInfos[0])),
			Len:  int(relocCounts),
			Cap:  int(relocCounts),
		}))
		runtime.KeepAlive(relocInfos)
	}

	// TODO: take mutex lock.
	e.pendingCompiledFunctions[m.ID] = append(e.pendingCompiledFunctions[m.ID], pendingCompiledBody{
		machineCode: body,
		relocs:      relocs,
	})
}

func (e *engine) exportFuncIndex(ctx context.Context, _ api.Module) uint32 {
	return mustFuncIndexFromContext(ctx)
}

func (e *engine) exportCurrentFuncTypeIndex(ctx context.Context, _ api.Module) uint32 {
	imported := mustImportedFunctionCountFromContext(ctx)
	m := mustModulePtrFromContext(ctx)
	fidx := mustFuncIndexFromContext(ctx)
	return m.FunctionSection[fidx-imported]
}

func (e *engine) exportFuncTypeIndex(ctx context.Context, _ api.Module, fidx uint32) uint32 {
	imported := mustImportedFunctionCountFromContext(ctx)
	m := mustModulePtrFromContext(ctx)
	if fidx < imported {
		return m.ImportSection[fidx].DescFunc
	} else {
		return m.FunctionSection[fidx]
	}
}

func (e *engine) exportTypeCounts(ctx context.Context, _ api.Module) uint32 {
	m := mustModulePtrFromContext(ctx)
	return uint32(len(m.TypeSection))
}

func (e *engine) exportTypeLens(ctx context.Context, craneliftMod api.Module, typeIndex, paramLenPtr, resultLenPtr uint32) {
	m := mustModulePtrFromContext(ctx)
	typ := m.TypeSection[typeIndex]

	mem := craneliftMod.Memory()
	mem.WriteUint32Le(paramLenPtr, uint32(len(typ.Params)))
	mem.WriteUint32Le(resultLenPtr, uint32(len(typ.Results)))
}

func (e *engine) exportTypeParamAt(ctx context.Context, _ api.Module, tpIndex, at uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	return valueTypeToCraneliftEnum(m.TypeSection[tpIndex].Params[at])
}

func (e *engine) exportTypeResultAt(ctx context.Context, _ api.Module, tpIndex, at uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	return valueTypeToCraneliftEnum(m.TypeSection[tpIndex].Results[at])
}

func (e *engine) exportIsLocallyDefinedFunction(ctx context.Context, _ api.Module, funcIndex uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	cnt := m.ImportFuncCount()
	if cnt <= funcIndex {
		return 1
	} else {
		return 0
	}
}

func (e *engine) exportMemoryMinMax(ctx context.Context, craneliftMod api.Module, minPtr, maxPtr uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	var min, max uint32
	if m.MemorySection == nil {
		imported := m.ImportedMemories()
		if len(imported) == 0 {
			return 0
		}
		memDef := imported[0]
		min, max = memDef.Min(), memDef.Min()
	} else {
		min, max = m.MemorySection.Min, m.MemorySection.Max
	}

	mem := craneliftMod.Memory()
	mem.WriteUint32Le(minPtr, min)
	mem.WriteUint32Le(maxPtr, max)
	return 1
}

func (e *engine) exportIsMemoryImported(ctx context.Context, _ api.Module) uint32 {
	m := mustModulePtrFromContext(ctx)
	if m.MemorySection == nil {
		return 1
	} else {
		return 0
	}
}

func (e *engine) exportVmContextLocalMemoryOffset(ctx context.Context, _ api.Module) uint32 {
	offsets := mustVmContextOffsetsFromContext(ctx)
	return uint32(offsets.localMemoryBegin)
}

func (e *engine) exportMemoryInstanceBaseOffset() uint32 {
	return uint32(unsafe.Offsetof(wasm.MemoryInstance{}.Buffer))
}

func (e *engine) exportVmContextImportedMemoryOffset(ctx context.Context, _ api.Module) uint32 {
	offsets := mustVmContextOffsetsFromContext(ctx)
	return uint32(offsets.importedMemoryBegin)
}

func (e *engine) exportVmContextImportedFunctionOffset(ctx context.Context, _ api.Module, index uint32) uint32 {
	offsets := mustVmContextOffsetsFromContext(ctx)
	return uint32(offsets.importedFunctionsBegin + int(index)*16)
}

func valueTypeToCraneliftEnum(v wasm.ValueType) uint32 {
	switch v {
	case wasm.ValueTypeI32:
		return 0
	case wasm.ValueTypeI64:
		return 1
	case wasm.ValueTypeF32:
		return 2
	case wasm.ValueTypeF64:
		return 3
	case wasm.ValueTypeV128:
		return 4
	case wasm.ValueTypeFuncref:
		return 5
	case wasm.ValueTypeExternref:
		return 6
	default:
		panic("BUG")
	}
}

func mustModulePtrFromContext(ctx context.Context) *wasm.Module {
	m, ok := ctx.Value(compilationContextModuleKey{}).(*wasm.Module)
	if !ok {
		panic("BUG: invalid compilation context without *wasm.Module")
	}
	return m
}

func mustFuncIndexFromContext(ctx context.Context) uint32 {
	ret, ok := ctx.Value(compilationContextFunctionIndexKey{}).(wasm.Index)
	if !ok {
		panic("BUG: invalid compilation context without func id")
	}
	return ret
}

func mustVmContextOffsetsFromContext(ctx context.Context) *opaqueVmContextOffsets {
	offsets, ok := ctx.Value(compilationContextVmContextOffsetsKey{}).(*opaqueVmContextOffsets)
	if !ok {
		panic("BUG: invalid compilation context without *opaqueVmContextOffsets")
	}
	return offsets
}

func mustImportedFunctionCountFromContext(ctx context.Context) uint32 {
	counts, ok := ctx.Value(compilationContextImportedFunctionCountKey{}).(uint32)
	if !ok {
		panic("BUG: invalid compilation context without imported function counts")
	}
	return counts
}
