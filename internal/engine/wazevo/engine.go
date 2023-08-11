package wazevo

import (
	"context"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/frontend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type (
	// engine implements wasm.Engine.
	engine struct {
		compiledModules   map[wasm.ModuleID]*compiledModule
		mux               sync.RWMutex
		rels              []backend.RelocationInfo
		refToBinaryOffset map[ssa.FuncRef]int
	}

	// compiledModule is a compiled variant of a wasm.Module and ready to be used for instantiation.
	compiledModule struct {
		executable      []byte
		functionOffsets []compiledFunctionOffset
		offsets         wazevoapi.ModuleContextOffsetData
	}

	// compiledFunctionOffset tells us that where in the executable a function begins.
	compiledFunctionOffset struct {
		// offset is the beggining of the function.
		offset int
		// goPreambleSize is the size of Go preamble of the function.
		goPreambleSize int
	}
)

var _ wasm.Engine = (*engine)(nil)

// NewEngine returns the implementation of wasm.Engine.
func NewEngine(_ context.Context, _ api.CoreFeatures, _ filecache.Cache) wasm.Engine {
	return &engine{compiledModules: make(map[wasm.ModuleID]*compiledModule), refToBinaryOffset: make(map[ssa.FuncRef]int)}
}

// CompileModule implements wasm.Engine.
func (e *engine) CompileModule(_ context.Context, module *wasm.Module, _ []experimental.FunctionListener, ensureTermination bool) error {
	e.rels = e.rels[:0]
	cm := &compiledModule{offsets: wazevoapi.NewModuleContextOffsetData(module)}

	importedFns, localFns := int(module.ImportFunctionCount), len(module.FunctionSection)
	if importedFns+localFns == 0 {
		e.addCompiledModule(module, cm)
		return nil
	}

	// TODO: reuse the map to avoid allocation.
	exportedFnIndex := make(map[wasm.Index]struct{}, len(module.ExportSection))
	for i := range module.ExportSection {
		if module.ExportSection[i].Type == wasm.ExternTypeFunc {
			exportedFnIndex[module.ExportSection[i].Index] = struct{}{}
		}
	}

	// Creates new compiler instances which are reused for each function.
	ssaBuilder := ssa.NewBuilder()
	fe := frontend.NewFrontendCompiler(module, ssaBuilder, &cm.offsets)
	machine := newMachine()
	be := backend.NewCompiler(machine, ssaBuilder)

	totalSize := 0 // Total binary size of the executable.
	cm.functionOffsets = make([]compiledFunctionOffset, localFns)
	bodies := make([][]byte, localFns)
	for i := range module.CodeSection {
		fidx := wasm.Index(i + importedFns)
		fref := frontend.FunctionIndexToFuncRef(fidx)

		_, needGoEntryPreamble := exportedFnIndex[fidx]

		// Align 16-bytes boundary.
		totalSize = (totalSize + 15) &^ 15
		compiledFuncOffset := &cm.functionOffsets[i]
		compiledFuncOffset.offset = totalSize

		typ := &module.TypeSection[module.FunctionSection[i]]

		codeSeg := &module.CodeSection[i]
		if codeSeg.GoFunc != nil {
			panic("TODO: host module")
		}

		// Initializes both frontend and backend compilers.
		fe.Init(wasm.Index(i), typ, codeSeg.LocalTypes, codeSeg.Body)
		be.Init(needGoEntryPreamble)

		// Lower Wasm to SSA.
		err := fe.LowerToSSA()
		if err != nil {
			return fmt.Errorf("wasm->ssa: %v", err)
		}

		// Run SSA-level optimization passes.
		ssaBuilder.RunPasses()

		// Finalize the layout of SSA blocks which might use the optimization results.
		ssaBuilder.LayoutBlocks()

		// Now our ssaBuilder contains the necessary information to further lower them to
		// machine code.
		body, rels, goPreambleSize, err := be.Compile()
		if err != nil {
			return fmt.Errorf("ssa->machine code: %v", err)
		}

		e.refToBinaryOffset[fref] = totalSize +
			// During the relocation, call target needs to be the beginning of function after Go entry preamble.
			goPreambleSize
		if needGoEntryPreamble {
			compiledFuncOffset.goPreambleSize = goPreambleSize
		}

		// At this point, relocation offsets are relative to the start of the function body,
		// so we adjust it to the start of the executable.
		for _, r := range rels {
			r.Offset += int64(totalSize)
			e.rels = append(e.rels, r)
		}

		// TODO: optimize as zero copy.
		copied := make([]byte, len(body))
		copy(copied, body)
		bodies[i] = copied
		totalSize += len(body)
	}

	// Allocate executable memory and then copy the generated machine code.
	executable, err := platform.MmapCodeSegment(totalSize)
	if err != nil {
		panic(err)
	}
	cm.executable = executable

	for i, b := range bodies {
		offset := cm.functionOffsets[i]
		copy(executable[offset.offset:], b)
	}

	// Resolve relocations for local function calls.
	machine.ResolveRelocations(e.refToBinaryOffset, executable, e.rels)

	fmt.Println(hex.EncodeToString(executable))

	if runtime.GOARCH == "arm64" {
		// On arm64, we cannot give all of rwx at the same time, so we change it to exec.
		if err = platform.MprotectRX(executable); err != nil {
			return err
		}
	}
	e.compiledModules[module.ID] = cm
	return nil
}

// Close implements wasm.Engine.
func (e *engine) Close() (err error) {
	e.mux.Lock()
	defer e.mux.Unlock()

	for _, cm := range e.compiledModules {
		cm.executable = nil
		cm.functionOffsets = nil
	}
	e.compiledModules = nil
	return nil
}

// CompiledModuleCount implements wasm.Engine.
func (e *engine) CompiledModuleCount() uint32 {
	e.mux.RLock()
	defer e.mux.RUnlock()
	return uint32(len(e.compiledModules))
}

// DeleteCompiledModule implements wasm.Engine.
func (e *engine) DeleteCompiledModule(m *wasm.Module) {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.compiledModules, m.ID)
}

func (e *engine) addCompiledModule(m *wasm.Module, cm *compiledModule) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.compiledModules[m.ID] = cm
}

// NewModuleEngine implements wasm.Engine.
func (e *engine) NewModuleEngine(m *wasm.Module, mi *wasm.ModuleInstance) (wasm.ModuleEngine, error) {
	me := &moduleEngine{}

	// Note: imported functions are resolved in moduleEngine.ResolveImportedFunction.

	compiled, ok := e.compiledModules[m.ID]
	if !ok {
		return nil, fmt.Errorf("binary of module %q is not compiled", mi.ModuleName)
	}
	me.parent = compiled
	me.module = mi

	if size := compiled.offsets.TotalSize; size != 0 {
		opaque := make([]byte, size)
		me.opaque = opaque
		me.opaquePtr = &opaque[0]
	}
	return me, nil
}
