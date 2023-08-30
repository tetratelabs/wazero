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
		// builtinFunctions is hods compiled builtin function trampolines.
		builtinFunctions *builtinFunctions
	}

	builtinFunctions struct {
		// memoryGrowExecutable is a compiled executable for memory.grow builtin function.
		memoryGrowExecutable []byte
		// stackGrowExecutable is a compiled executable for growing stack builtin function.
		stackGrowExecutable []byte
	}

	// compiledModule is a compiled variant of a wasm.Module and ready to be used for instantiation.
	compiledModule struct {
		executable      []byte
		functionOffsets []compiledFunctionOffset

		// The followings are only available for non host modules.

		offsets          wazevoapi.ModuleContextOffsetData
		builtinFunctions *builtinFunctions
	}

	// compiledFunctionOffset tells us that where in the executable a function begins.
	compiledFunctionOffset struct {
		// offset is the beginning of the function.
		offset int
		// goPreambleSize is the size of Go preamble of the function.
		// This is only needed for non host modules.
		goPreambleSize int
	}
)

// nativeBegin returns the offset of the beginning of the function in the executable after the Go preamble if any.
func (c compiledFunctionOffset) nativeBegin() int {
	return c.offset + c.goPreambleSize
}

var _ wasm.Engine = (*engine)(nil)

// NewEngine returns the implementation of wasm.Engine.
func NewEngine(_ context.Context, _ api.CoreFeatures, _ filecache.Cache) wasm.Engine {
	e := &engine{compiledModules: make(map[wasm.ModuleID]*compiledModule), refToBinaryOffset: make(map[ssa.FuncRef]int)}
	e.compileBuiltinFunctions()
	return e
}

// CompileModule implements wasm.Engine.
func (e *engine) CompileModule(ctx context.Context, module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) error {
	if wazevoapi.DeterministicCompilationVerifierEnabled {
		ctx = wazevoapi.NewDeterministicCompilationVerifierContext(ctx, len(module.CodeSection))
	}
	cm, err := e.compileModule(ctx, module, listeners, ensureTermination)
	if err != nil {
		return err
	}
	e.addCompiledModule(module, cm)

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		for i := 0; i < wazevoapi.DeterministicCompilationVerifyingIter; i++ {
			_, err = e.compileModule(ctx, module, listeners, ensureTermination)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *engine) compileModule(ctx context.Context, module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) (*compiledModule, error) {
	e.rels = e.rels[:0]
	cm := &compiledModule{offsets: wazevoapi.NewModuleContextOffsetData(module)}

	if module.IsHostModule {
		return e.compileHostModule(module)
	}

	importedFns, localFns := int(module.ImportFunctionCount), len(module.FunctionSection)
	if localFns == 0 {
		return cm, nil
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		// The compilation must be deterministic regardless of the order of functions being compiled.
		wazevoapi.DeterministicCompilationVerifierRandomizeIndexes(ctx)
	}

	// TODO: reuse the map to avoid allocation.
	exportedFnIndex := make(map[wasm.Index]string, len(module.ExportSection))
	for i := range module.ExportSection {
		exp := &module.ExportSection[i]
		if exp.Type == wasm.ExternTypeFunc {
			exportedFnIndex[module.ExportSection[i].Index] = exp.Name
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
		if wazevoapi.DeterministicCompilationVerifierEnabled {
			i = wazevoapi.DeterministicCompilationVerifierGetRandomizedLocalFunctionIndex(ctx, i)
		}

		fidx := wasm.Index(i + importedFns)

		_, needGoEntryPreamble := exportedFnIndex[fidx]
		if sf := module.StartSection; sf != nil && *sf == fidx {
			needGoEntryPreamble = true
		}

		body, rels, goPreambleSize, err := e.compileLocalWasmFunction(ctx, module, wasm.Index(i), fidx, needGoEntryPreamble, fe, ssaBuilder, be, listeners, ensureTermination)
		if err != nil {
			return nil, fmt.Errorf("compile function %d/%d: %v", i, len(module.CodeSection)-1, err)
		}

		// Align 16-bytes boundary.
		totalSize = (totalSize + 15) &^ 15
		compiledFuncOffset := &cm.functionOffsets[i]
		compiledFuncOffset.offset = totalSize

		fref := frontend.FunctionIndexToFuncRef(fidx)
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

		bodies[i] = body
		totalSize += len(body)
		if wazevoapi.PrintMachineCodeHexPerFunction {
			fmt.Printf("[[[machine code SSA for %d/%d %s]]]\n%s\n",
				i, len(module.CodeSection)-1, exportedFnIndex[fidx], hex.EncodeToString(body))
		}
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

	if runtime.GOARCH == "arm64" {
		// On arm64, we cannot give all of rwx at the same time, so we change it to exec.
		if err = platform.MprotectRX(executable); err != nil {
			return nil, err
		}
	}
	e.compiledModules[module.ID] = cm
	cm.builtinFunctions = e.builtinFunctions

	// TODO: finalizer.

	return cm, nil
}

func (e *engine) compileLocalWasmFunction(
	ctx context.Context,
	module *wasm.Module,
	localFunctionIndex,
	functionIndex wasm.Index,
	needGoEntryPreamble bool,
	fe *frontend.Compiler,
	ssaBuilder ssa.Builder,
	be backend.Compiler,
	_ []experimental.FunctionListener, _ bool,
) (body []byte, rels []backend.RelocationInfo, goPreambleSize int, err error) {
	typ := &module.TypeSection[module.FunctionSection[localFunctionIndex]]
	codeSeg := &module.CodeSection[localFunctionIndex]

	// Initializes both frontend and backend compilers.
	fe.Init(localFunctionIndex, typ, codeSeg.LocalTypes, codeSeg.Body)
	be.Init(needGoEntryPreamble)

	// Lower Wasm to SSA.
	err = fe.LowerToSSA()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("wasm->ssa: %v", err)
	}

	if wazevoapi.PrintSSA {
		def := module.FunctionDefinition(functionIndex)
		fmt.Printf("[[[SSA for %d/%d %s]]]%s\n", localFunctionIndex, len(module.CodeSection)-1, def.Debugname, ssaBuilder.Format())
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		def := module.FunctionDefinition(functionIndex)
		name := def.DebugName()
		if len(def.ExportNames()) > 0 {
			name = def.ExportNames()[0]
		}
		ctx = wazevoapi.DeterministicCompilationVerifierSetCurrentFunctionName(ctx, fmt.Sprintf("[%d/%d]:\"%s\"", localFunctionIndex, len(module.CodeSection)-1, name))
		wazevoapi.VerifyOrSetDeterministicCompilationContextValue(ctx, "SSA", ssaBuilder.Format())
	}

	// Run SSA-level optimization passes.
	ssaBuilder.RunPasses()

	if wazevoapi.PrintOptimizedSSA {
		def := module.FunctionDefinition(functionIndex)
		fmt.Printf("[[[Optimized SSA for %d/%d %s]]]%s\n", localFunctionIndex, len(module.CodeSection)-1, def.Debugname, ssaBuilder.Format())
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		wazevoapi.VerifyOrSetDeterministicCompilationContextValue(ctx, "Optimized SSA", ssaBuilder.Format())
	}

	// Finalize the layout of SSA blocks which might use the optimization results.
	ssaBuilder.LayoutBlocks()

	if wazevoapi.PrintBlockLaidOutSSA {
		def := module.FunctionDefinition(functionIndex)
		fmt.Printf("[[[Laidout SSA for %d/%d %s]]]%s\n", localFunctionIndex, len(module.CodeSection)-1, def.Debugname, ssaBuilder.Format())
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		wazevoapi.VerifyOrSetDeterministicCompilationContextValue(ctx, "Block laid out SSA", ssaBuilder.Format())
	}

	// Now our ssaBuilder contains the necessary information to further lower them to
	// machine code.
	original, rels, goPreambleSize, err := be.Compile(ctx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("ssa->machine code: %v", err)
	}

	// TODO: optimize as zero copy.
	copied := make([]byte, len(original))
	copy(copied, original)
	return copied, rels, goPreambleSize, nil
}

func (e *engine) compileHostModule(module *wasm.Module) (*compiledModule, error) {
	machine := newMachine()
	be := backend.NewCompiler(machine, ssa.NewBuilder())

	num := len(module.CodeSection)
	cm := &compiledModule{}
	cm.functionOffsets = make([]compiledFunctionOffset, num)
	totalSize := 0 // Total binary size of the executable.
	bodies := make([][]byte, num)
	var sig ssa.Signature
	for i := range module.CodeSection {
		totalSize = (totalSize + 15) &^ 15
		cm.functionOffsets[i].offset = totalSize

		typIndex := module.FunctionSection[i]
		typ := &module.TypeSection[typIndex]
		if typ.ParamNumInUint64 >= goFunctionCallStackSize || typ.ResultNumInUint64 >= goFunctionCallStackSize {
			return nil, fmt.Errorf("too many params or results for a host function (maximum %d): %v",
				goFunctionCallStackSize, typ)
		}

		// We can relax until the index fits together in ExitCode as we do in wazevoapi.ExitCodeCallGoModuleFunctionWithIndex.
		// However, 1 << 16 should be large enough for a real use case.
		const hostFunctionNumMaximum = 1 << 16
		if i >= hostFunctionNumMaximum {
			return nil, fmt.Errorf("too many host functions (maximum %d)", hostFunctionNumMaximum)
		}

		sig.ID = ssa.SignatureID(typIndex) // This is important since we reuse the `machine` which caches the ABI based on the SignatureID.
		sig.Params = append(sig.Params[:0],
			ssa.TypeI64, // First argument must be exec context.
			ssa.TypeI64, // The second argument is the moduleContextOpaque of this host module.
		)
		for _, t := range typ.Params {
			sig.Params = append(sig.Params, frontend.WasmTypeToSSAType(t))
		}

		sig.Results = sig.Results[:0]
		for _, t := range typ.Results {
			sig.Results = append(sig.Results, frontend.WasmTypeToSSAType(t))
		}

		c := &module.CodeSection[i]
		if c.GoFunc == nil {
			panic("BUG: GoFunc must be set for host module")
		}

		var exitCode wazevoapi.ExitCode
		fn := c.GoFunc
		switch fn.(type) {
		case api.GoModuleFunction:
			exitCode = wazevoapi.ExitCodeCallGoModuleFunctionWithIndex(i)
		case api.GoFunction:
			exitCode = wazevoapi.ExitCodeCallGoFunctionWithIndex(i)
		}

		be.Init(false)
		machine.CompileGoFunctionTrampoline(exitCode, &sig, true)
		be.Encode()
		body := be.Buf()

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

	if runtime.GOARCH == "arm64" {
		// On arm64, we cannot give all of rwx at the same time, so we change it to exec.
		if err = platform.MprotectRX(executable); err != nil {
			return nil, err
		}
	}
	return cm, nil
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
	e.builtinFunctions = nil
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
	me.importedFunctions = make([]importedFunction, m.ImportFunctionCount)

	compiled, ok := e.compiledModules[m.ID]
	if !ok {
		return nil, fmt.Errorf("binary of module %q is not compiled", mi.ModuleName)
	}
	me.parent = compiled
	me.module = mi

	if m.IsHostModule {
		me.opaque = buildHostModuleOpaque(m)
		me.opaquePtr = &me.opaque[0]
	} else {
		if size := compiled.offsets.TotalSize; size != 0 {
			opaque := make([]byte, size)
			me.opaque = opaque
			me.opaquePtr = &opaque[0]
		}
	}
	return me, nil
}

func (e *engine) compileBuiltinFunctions() {
	machine := newMachine()
	be := backend.NewCompiler(machine, ssa.NewBuilder())
	e.builtinFunctions = &builtinFunctions{}

	{
		src := machine.CompileGoFunctionTrampoline(wazevoapi.ExitCodeGrowMemory, &ssa.Signature{
			Params:  []ssa.Type{ssa.TypeI32 /* exec context */, ssa.TypeI32},
			Results: []ssa.Type{ssa.TypeI32},
		}, false)
		e.builtinFunctions.memoryGrowExecutable = mmapExecutable(src)
	}

	// TODO: table grow, etc.

	be.Init(false)
	{
		src := machine.CompileStackGrowCallSequence()
		e.builtinFunctions.stackGrowExecutable = mmapExecutable(src)
	}

	// TODO: finalizer.
}

func mmapExecutable(src []byte) []byte {
	executable, err := platform.MmapCodeSegment(len(src))
	if err != nil {
		panic(err)
	}

	copy(executable, src)

	if runtime.GOARCH == "arm64" {
		// On arm64, we cannot give all of rwx at the same time, so we change it to exec.
		if err = platform.MprotectRX(executable); err != nil {
			panic(err)
		}
	}
	return executable
}
