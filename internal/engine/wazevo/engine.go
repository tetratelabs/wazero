package wazevo

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"unsafe"

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
		compiledModules map[wasm.ModuleID]*compiledModule
		// sortedCompiledModules is a list of compiled modules sorted by the initial address of the executable.
		sortedCompiledModules []*compiledModule
		mux                   sync.RWMutex
		// rels is a list of relocations to be resolved. This is reused for each compilation to avoid allocation.
		rels []backend.RelocationInfo
		// refToBinaryOffset is reused for each compilation to avoid allocation.
		refToBinaryOffset map[ssa.FuncRef]int
		// sharedFunctions is compiled functions shared by all modules.
		sharedFunctions *sharedFunctions
		// setFinalizer defaults to runtime.SetFinalizer, but overridable for tests.
		setFinalizer func(obj interface{}, finalizer interface{})

		// The followings are reused for compiling shared functions.
		machine backend.Machine
		be      backend.Compiler
	}

	sharedFunctions struct {
		// memoryGrowExecutable is a compiled executable for memory.grow builtin function.
		memoryGrowExecutable []byte
		// checkModuleExitCode is a compiled executable for checking module instance exit code. This
		// is used when ensureTermination is true.
		checkModuleExitCode []byte
		// stackGrowExecutable is a compiled executable for growing stack builtin function.
		stackGrowExecutable       []byte
		entryPreambles            map[*wasm.FunctionType][]byte
		listenerBeforeTrampolines map[*wasm.FunctionType][]byte
		listenerAfterTrampolines  map[*wasm.FunctionType][]byte
	}

	// compiledModule is a compiled variant of a wasm.Module and ready to be used for instantiation.
	compiledModule struct {
		executable []byte
		// functionOffsets maps a local function index to the offset in the executable.
		functionOffsets           []int
		parent                    *engine
		module                    *wasm.Module
		entryPreambles            []*byte // indexed-correlated with the type index.
		ensureTermination         bool
		listeners                 []experimental.FunctionListener
		listenerBeforeTrampolines []*byte
		listenerAfterTrampolines  []*byte

		// The followings are only available for non host modules.

		offsets         wazevoapi.ModuleContextOffsetData
		sharedFunctions *sharedFunctions
	}
)

var _ wasm.Engine = (*engine)(nil)

// NewEngine returns the implementation of wasm.Engine.
func NewEngine(ctx context.Context, _ api.CoreFeatures, _ filecache.Cache) wasm.Engine {
	machine := newMachine()
	be := backend.NewCompiler(ctx, machine, ssa.NewBuilder())
	e := &engine{
		compiledModules: make(map[wasm.ModuleID]*compiledModule), refToBinaryOffset: make(map[ssa.FuncRef]int),
		setFinalizer: runtime.SetFinalizer,
		machine:      machine,
		be:           be,
	}
	e.compileSharedFunctions()
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

	if len(listeners) > 0 {
		cm.listeners = listeners
		cm.listenerBeforeTrampolines = make([]*byte, len(module.TypeSection))
		cm.listenerAfterTrampolines = make([]*byte, len(module.TypeSection))
		for i := range module.TypeSection {
			typ := &module.TypeSection[i]
			before, after := e.getListenerTrampolineForType(typ)
			cm.listenerBeforeTrampolines[i] = before
			cm.listenerAfterTrampolines[i] = after
		}
	}
	return nil
}

func (e *engine) compileModule(ctx context.Context, module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) (*compiledModule, error) {
	e.rels = e.rels[:0]
	cm := &compiledModule{
		offsets: wazevoapi.NewModuleContextOffsetData(module), parent: e, module: module,
		ensureTermination: ensureTermination,
	}

	if module.IsHostModule {
		return e.compileHostModule(ctx, module, listeners)
	}

	cm.entryPreambles = make([]*byte, len(module.TypeSection))
	for i := range cm.entryPreambles {
		cm.entryPreambles[i] = e.getEntryPreambleForType(&module.TypeSection[i])
	}

	importedFns, localFns := int(module.ImportFunctionCount), len(module.FunctionSection)
	if localFns == 0 {
		return cm, nil
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		// The compilation must be deterministic regardless of the order of functions being compiled.
		wazevoapi.DeterministicCompilationVerifierRandomizeIndexes(ctx)
	}

	// Creates new compiler instances which are reused for each function.
	ssaBuilder := ssa.NewBuilder()
	fe := frontend.NewFrontendCompiler(module, ssaBuilder, &cm.offsets, ensureTermination, len(listeners) > 0)
	machine := newMachine()
	be := backend.NewCompiler(ctx, machine, ssaBuilder)

	totalSize := 0 // Total binary size of the executable.
	cm.functionOffsets = make([]int, localFns)
	bodies := make([][]byte, localFns)
	for i := range module.CodeSection {
		if wazevoapi.DeterministicCompilationVerifierEnabled {
			i = wazevoapi.DeterministicCompilationVerifierGetRandomizedLocalFunctionIndex(ctx, i)
		}

		fidx := wasm.Index(i + importedFns)

		if wazevoapi.NeedFunctionNameInContext {
			def := module.FunctionDefinition(fidx)
			name := def.DebugName()
			if len(def.ExportNames()) > 0 {
				name = def.ExportNames()[0]
			}
			ctx = wazevoapi.SetCurrentFunctionName(ctx, fmt.Sprintf("[%d/%d] \"%s\"", i, len(module.CodeSection)-1, name))
		}

		needListener := len(listeners) > 0 && listeners[i] != nil
		body, rels, err := e.compileLocalWasmFunction(ctx, module, wasm.Index(i), fe, ssaBuilder, be, needListener)
		if err != nil {
			return nil, fmt.Errorf("compile function %d/%d: %v", i, len(module.CodeSection)-1, err)
		}

		// Align 16-bytes boundary.
		totalSize = (totalSize + 15) &^ 15
		cm.functionOffsets[i] = totalSize

		fref := frontend.FunctionIndexToFuncRef(fidx)
		e.refToBinaryOffset[fref] = totalSize

		// At this point, relocation offsets are relative to the start of the function body,
		// so we adjust it to the start of the executable.
		for _, r := range rels {
			r.Offset += int64(totalSize)
			e.rels = append(e.rels, r)
		}

		bodies[i] = body
		totalSize += len(body)
		if wazevoapi.PrintMachineCodeHexPerFunction {
			fmt.Printf("[[[machine code for %s]]]\n%s\n\n", wazevoapi.GetCurrentFunctionName(ctx), hex.EncodeToString(body))
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
		copy(executable[offset:], b)
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
	cm.sharedFunctions = e.sharedFunctions
	e.setFinalizer(cm, compiledModuleFinalizer)
	return cm, nil
}

func (e *engine) compileLocalWasmFunction(
	ctx context.Context,
	module *wasm.Module,
	localFunctionIndex wasm.Index,
	fe *frontend.Compiler,
	ssaBuilder ssa.Builder,
	be backend.Compiler,
	needListener bool,
) (body []byte, rels []backend.RelocationInfo, err error) {
	typIndex := module.FunctionSection[localFunctionIndex]
	typ := &module.TypeSection[typIndex]
	codeSeg := &module.CodeSection[localFunctionIndex]

	// Initializes both frontend and backend compilers.
	fe.Init(localFunctionIndex, typIndex, typ, codeSeg.LocalTypes, codeSeg.Body, needListener)
	be.Init()

	// Lower Wasm to SSA.
	fe.LowerToSSA()
	if wazevoapi.PrintSSA {
		fmt.Printf("[[[SSA for %s]]]%s\n", wazevoapi.GetCurrentFunctionName(ctx), ssaBuilder.Format())
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		wazevoapi.VerifyOrSetDeterministicCompilationContextValue(ctx, "SSA", ssaBuilder.Format())
	}

	// Run SSA-level optimization passes.
	ssaBuilder.RunPasses()

	if wazevoapi.PrintOptimizedSSA {
		fmt.Printf("[[[Optimized SSA for %s]]]%s\n", wazevoapi.GetCurrentFunctionName(ctx), ssaBuilder.Format())
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		wazevoapi.VerifyOrSetDeterministicCompilationContextValue(ctx, "Optimized SSA", ssaBuilder.Format())
	}

	// Finalize the layout of SSA blocks which might use the optimization results.
	ssaBuilder.LayoutBlocks()

	if wazevoapi.PrintBlockLaidOutSSA {
		fmt.Printf("[[[Laidout SSA for %s]]]%s\n", wazevoapi.GetCurrentFunctionName(ctx), ssaBuilder.Format())
	}

	if wazevoapi.DeterministicCompilationVerifierEnabled {
		wazevoapi.VerifyOrSetDeterministicCompilationContextValue(ctx, "Block laid out SSA", ssaBuilder.Format())
	}

	// Now our ssaBuilder contains the necessary information to further lower them to
	// machine code.
	original, rels, err := be.Compile(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("ssa->machine code: %v", err)
	}

	// TODO: optimize as zero copy.
	copied := make([]byte, len(original))
	copy(copied, original)
	return copied, rels, nil
}

func (e *engine) compileHostModule(ctx context.Context, module *wasm.Module, listeners []experimental.FunctionListener) (*compiledModule, error) {
	machine := newMachine()
	be := backend.NewCompiler(ctx, machine, ssa.NewBuilder())

	num := len(module.CodeSection)
	cm := &compiledModule{module: module, listeners: listeners}
	cm.functionOffsets = make([]int, num)
	totalSize := 0 // Total binary size of the executable.
	bodies := make([][]byte, num)
	var sig ssa.Signature
	for i := range module.CodeSection {
		totalSize = (totalSize + 15) &^ 15
		cm.functionOffsets[i] = totalSize

		typIndex := module.FunctionSection[i]
		typ := &module.TypeSection[typIndex]

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

		withListener := len(listeners) > 0 && listeners[i] != nil
		var exitCode wazevoapi.ExitCode
		fn := c.GoFunc
		switch fn.(type) {
		case api.GoModuleFunction:
			exitCode = wazevoapi.ExitCodeCallGoModuleFunctionWithIndex(i, withListener)
		case api.GoFunction:
			exitCode = wazevoapi.ExitCodeCallGoFunctionWithIndex(i, withListener)
		}

		be.Init()
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
		copy(executable[offset:], b)
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
		cm.module = nil
		cm.parent = nil
	}
	e.sortedCompiledModules = nil
	e.compiledModules = nil
	e.sharedFunctions = nil
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
	cm, ok := e.compiledModules[m.ID]
	if ok {
		cm.parent = nil
		cm.module = nil
		cm.functionOffsets = nil
		if len(cm.executable) > 0 {
			e.deleteCompiledModuleFromSortedList(cm)
		}
		delete(e.compiledModules, m.ID)
	}
}

func (e *engine) addCompiledModule(m *wasm.Module, cm *compiledModule) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.compiledModules[m.ID] = cm
	if len(cm.executable) > 0 {
		e.addCompiledModuleToSortedList(cm)
	}
}

func (e *engine) addCompiledModuleToSortedList(cm *compiledModule) {
	ptr := uintptr(unsafe.Pointer(&cm.executable[0]))

	index := sort.Search(len(e.sortedCompiledModules), func(i int) bool {
		return uintptr(unsafe.Pointer(&e.sortedCompiledModules[i].executable[0])) >= ptr
	})
	e.sortedCompiledModules = append(e.sortedCompiledModules, nil)
	copy(e.sortedCompiledModules[index+1:], e.sortedCompiledModules[index:])
	e.sortedCompiledModules[index] = cm
}

func (e *engine) deleteCompiledModuleFromSortedList(cm *compiledModule) {
	ptr := uintptr(unsafe.Pointer(&cm.executable[0]))

	index := sort.Search(len(e.sortedCompiledModules), func(i int) bool {
		return uintptr(unsafe.Pointer(&e.sortedCompiledModules[i].executable[0])) >= ptr
	})
	copy(e.sortedCompiledModules[index:], e.sortedCompiledModules[index+1:])
	e.sortedCompiledModules = e.sortedCompiledModules[:len(e.sortedCompiledModules)-1]
}

func (e *engine) compiledModuleOfAddr(addr uintptr) *compiledModule {
	e.mux.RLock()
	defer e.mux.RUnlock()

	index := sort.Search(len(e.sortedCompiledModules), func(i int) bool {
		return uintptr(unsafe.Pointer(&e.sortedCompiledModules[i].executable[0])) > addr
	})
	index -= 1
	if index < 0 {
		return nil
	}
	return e.sortedCompiledModules[index]
}

// NewModuleEngine implements wasm.Engine.
func (e *engine) NewModuleEngine(m *wasm.Module, mi *wasm.ModuleInstance) (wasm.ModuleEngine, error) {
	me := &moduleEngine{}

	// Note: imported functions are resolved in moduleEngine.ResolveImportedFunction.
	me.importedFunctions = make([]importedFunction, m.ImportFunctionCount)

	compiled, ok := e.compiledModules[m.ID]
	if !ok {
		return nil, errors.New("source module must be compiled before instantiation")
	}
	me.parent = compiled
	me.module = mi
	me.listeners = compiled.listeners

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

func (e *engine) compileSharedFunctions() {
	e.sharedFunctions = &sharedFunctions{
		entryPreambles:            make(map[*wasm.FunctionType][]byte),
		listenerBeforeTrampolines: make(map[*wasm.FunctionType][]byte),
		listenerAfterTrampolines:  make(map[*wasm.FunctionType][]byte),
	}

	e.be.Init()
	{
		src := e.machine.CompileGoFunctionTrampoline(wazevoapi.ExitCodeGrowMemory, &ssa.Signature{
			Params:  []ssa.Type{ssa.TypeI32 /* exec context */, ssa.TypeI32},
			Results: []ssa.Type{ssa.TypeI32},
		}, false)
		e.sharedFunctions.memoryGrowExecutable = mmapExecutable(src)
	}

	e.be.Init()
	{
		src := e.machine.CompileGoFunctionTrampoline(wazevoapi.ExitCodeCheckModuleExitCode, &ssa.Signature{
			Params:  []ssa.Type{ssa.TypeI32 /* exec context */},
			Results: []ssa.Type{ssa.TypeI32},
		}, false)
		e.sharedFunctions.checkModuleExitCode = mmapExecutable(src)
	}

	// TODO: table grow, etc.

	e.be.Init()
	{
		src := e.machine.CompileStackGrowCallSequence()
		e.sharedFunctions.stackGrowExecutable = mmapExecutable(src)
	}

	e.setFinalizer(e.sharedFunctions, sharedFunctionsFinalizer)
}

func sharedFunctionsFinalizer(sf *sharedFunctions) {
	if err := platform.MunmapCodeSegment(sf.memoryGrowExecutable); err != nil {
		panic(err)
	}
	if err := platform.MunmapCodeSegment(sf.checkModuleExitCode); err != nil {
		panic(err)
	}
	if err := platform.MunmapCodeSegment(sf.stackGrowExecutable); err != nil {
		panic(err)
	}
	for _, f := range sf.entryPreambles {
		if err := platform.MunmapCodeSegment(f); err != nil {
			panic(err)
		}
	}
	for _, f := range sf.listenerBeforeTrampolines {
		if err := platform.MunmapCodeSegment(f); err != nil {
			panic(err)
		}
	}
	for _, f := range sf.listenerAfterTrampolines {
		if err := platform.MunmapCodeSegment(f); err != nil {
			panic(err)
		}
	}

	sf.memoryGrowExecutable = nil
	sf.checkModuleExitCode = nil
	sf.stackGrowExecutable = nil
	sf.entryPreambles = nil
	sf.listenerBeforeTrampolines = nil
	sf.listenerAfterTrampolines = nil
}

func compiledModuleFinalizer(cm *compiledModule) {
	if len(cm.executable) > 0 {
		if err := platform.MunmapCodeSegment(cm.executable); err != nil {
			panic(err)
		}
	}
	cm.executable = nil
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

func (cm *compiledModule) functionIndexOf(addr uintptr) wasm.Index {
	addr -= uintptr(unsafe.Pointer(&cm.executable[0]))
	offset := cm.functionOffsets
	index := sort.Search(len(offset), func(i int) bool {
		return offset[i] > int(addr)
	})
	index -= 1
	if index < 0 {
		panic("BUG")
	}
	return wasm.Index(index)
}

func (e *engine) getEntryPreambleForType(functionType *wasm.FunctionType) *byte {
	e.mux.RLock()
	executable, ok := e.sharedFunctions.entryPreambles[functionType]
	e.mux.RUnlock()
	if ok {
		return &executable[0]
	}

	sig := frontend.SignatureForWasmFunctionType(functionType)
	e.be.Init()
	buf := e.machine.CompileEntryPreamble(&sig)
	executable = mmapExecutable(buf)
	e.mux.Lock()
	defer e.mux.Unlock()
	e.sharedFunctions.entryPreambles[functionType] = executable
	return &executable[0]
}

func (e *engine) getListenerTrampolineForType(functionType *wasm.FunctionType) (before, after *byte) {
	e.mux.RLock()
	beforeBuf, ok := e.sharedFunctions.listenerBeforeTrampolines[functionType]
	afterBuf := e.sharedFunctions.listenerAfterTrampolines[functionType]
	e.mux.RUnlock()
	if ok {
		return &beforeBuf[0], &afterBuf[0]
	}

	beforeSig, afterSig := frontend.SignatureForListener(functionType)

	e.be.Init()
	buf := e.machine.CompileGoFunctionTrampoline(wazevoapi.ExitCodeCallListenerBefore, beforeSig, false)
	beforeBuf = mmapExecutable(buf)

	e.be.Init()
	buf = e.machine.CompileGoFunctionTrampoline(wazevoapi.ExitCodeCallListenerAfter, afterSig, false)
	afterBuf = mmapExecutable(buf)

	e.mux.Lock()
	defer e.mux.Unlock()
	e.sharedFunctions.listenerBeforeTrampolines[functionType] = beforeBuf
	e.sharedFunctions.listenerAfterTrampolines[functionType] = afterBuf
	return &beforeBuf[0], &afterBuf[0]
}
