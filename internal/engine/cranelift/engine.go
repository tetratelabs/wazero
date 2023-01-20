package cranelift

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"runtime"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/wasm"
	binaryformat "github.com/tetratelabs/wazero/internal/wasm/binary"
)

//go:embed compiler/cranelift_backend.wasm
var craneliftBin []byte

const craneliftFeature = api.CoreFeaturesV2

// Compile-time objects.
type (
	// engine implements wasm.Engine.
	engine struct {
		modules          map[wasm.ModuleID]*compiledModule
		paramsSetupCodes map[string][]byte
		craneliftStore   *wasm.Store
		craneliftEngine  wasm.Engine
		craneliftModule  *wasm.Module
		// TODO: make this pool and does the concurrent compilation.
		craneLiftInst            craneliftModuleInstance
		pendingCompiledFunctions map[wasm.ModuleID][]pendingCompiledBody
	}

	// compiledModule holds the memory-mapped executable and the offsets inside it which maps
	// each function index (without imported ones) to the beginning of the function in executable.
	compiledModule struct {
		executable             []byte
		executableOffsets      []int
		opaqueVmContextOffsets opaqueVmContextOffsets
		engine                 *engine
	}

	opaqueVmContextOffsets struct {
		totalSize              int
		localMemoryBegin       int
		importedFunctionsBegin int
	}

	pendingCompiledBody struct {
		machineCode []byte
		relocs      []functionRelocationEntry
	}

	// functionRelocationEntry must be aligned with functionRelocationEntry in lib.rs.
	functionRelocationEntry struct{ index, offset uint32 }

	craneliftModuleInstance struct {
		m                                     *wasm.CallContext
		allocate, deallocate, compileFunction api.Function
		stdout, stderr                        *bytes.Buffer
	}
)

// Instantiation-time / per-exported function objects.
type (
	// vmContext implements wasm.ModuleEngine and is created per wasm.ModuleInstance.
	// Note: intentionally use the word "vm context" instead of "module engine"
	// to be aligned with the cranelift terminology.
	vmContext struct {
		// opaqueVmContextPtr is used by machine code. See the comment on opaqueVmContext.
		opaqueVmContextPtr *byte

		// The followings are not directly accessed by machine codes.
		// These are defined only
		parent            *compiledModule
		importedFunctions []vmContextImportedFunction
		module            *wasm.ModuleInstance
		// opaqueVmContext is the opaque byte slice of Wasm-compile-time-known Module instance specific contents whose size
		// is only Wasm-compile-time known, hence dynamic. Its contents are basically the pointers to the module instance,
		// specific objects as well as functions. This follows how Wasmtime defines its own VMContext.
		// See https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/crates/runtime/src/vmcontext.rs#L827-L836
		opaqueVmContext []byte
	}

	// vmContextImportedFunction corresponds to each imported wasm.FunctionInstance.
	vmContextImportedFunction struct {
		executable *byte
		vmctx      *vmContext
	}

	// callEngine implements wasm.CallEngine.
	// This is created per exported function on demand.
	callEngine struct {
		entry      entryPointFn
		executable *byte

		setParamsExecutable *byte

		resultsHolderPtr *byte
		resultsHolder    []byte

		stack           []byte
		alignedStackTop uintptr

		vmCtx *vmContext
		typ   *wasm.FunctionType
	}
)

// Comp-time checks on the implementation.
var (
	_ wasm.Engine       = &engine{}
	_ wasm.ModuleEngine = &vmContext{}
	_ wasm.CallEngine   = &callEngine{}
)

func NewEngine(ctx context.Context, _ api.CoreFeatures, _ filecache.Cache) wasm.Engine {
	e := &engine{
		pendingCompiledFunctions: map[wasm.ModuleID][]pendingCompiledBody{},
		modules:                  map[wasm.ModuleID]*compiledModule{},
		paramsSetupCodes:         map[string][]byte{},
	}
	e.craneliftEngine = compiler.NewEngine(ctx, craneliftFeature, nil)
	e.craneliftStore = wasm.NewStore(craneliftFeature, e.craneliftEngine)
	var err error

	// Cranelift requires wasi, so we define here to avoid cycle dep on the public wasi package.
	e.addWASI(ctx)

	// Also, we need to register wazero module to interact cranelift.
	e.addWazeroModule(ctx)

	// Now ready to instantiate cranelift module.
	e.craneliftModule, err = binaryformat.DecodeModule(craneliftBin, craneliftFeature, wasm.MemoryLimitPages, false, true, false)
	if err != nil {
		panic(err)
	}

	if err = e.craneliftModule.Validate(craneliftFeature); err != nil {
		panic(err)
	}

	e.craneliftModule.BuildFunctionDefinitions()
	e.craneliftModule.BuildMemoryDefinitions()

	err = e.craneliftEngine.CompileModule(ctx, e.craneliftModule, nil)
	if err != nil {
		panic(err)
	}

	if err = e.instantiateCraneLiftModule(ctx); err != nil {
		panic(err)
	}
	return e
}

// Cranelift requires wasi, so we define here to avoid cycle dep on the public wasi package.
func (e *engine) addWASI(ctx context.Context) {
	const wasiName = "wasi_snapshot_preview1"

	// NOTE: below is copy-pasted from imports/wasi package.
	wasi, err := wasm.NewHostModule(wasiName,
		map[string]interface{}{
			"clock_time_get": func(uint32, uint64, uint32) uint32 { return 0 },
			"fd_write": func(_ context.Context, mod api.Module, fd uint32, iovs uint32, iovsCount uint32, resultNwritten uint32) uint32 {
				mem := mod.Memory()
				fsc := mod.(*wasm.CallContext).Sys.FS()

				writer := sys.WriterForFile(fsc, fd)
				if writer == nil {
					return 8 // ErrnoBadf
				}

				var err error
				var nwritten uint32
				iovsStop := iovsCount << 3 // iovsCount * 8
				iovsBuf, ok := mem.Read(iovs, iovsStop)
				if !ok {
					return 21 // ErrnoFault
				}

				for iovsPos := uint32(0); iovsPos < iovsStop; iovsPos += 8 {
					offset := binary.LittleEndian.Uint32(iovsBuf[iovsPos:])
					l := binary.LittleEndian.Uint32(iovsBuf[iovsPos+4:])

					var n int
					if writer == io.Discard { // special-case default
						n = int(l)
					} else {
						b, ok := mem.Read(offset, l)
						if !ok {
							return 21 // ErrnoFault
						}
						n, err = writer.Write(b)
						if err != nil {
							return 29 // ErrnoIo
						}
					}
					nwritten += uint32(n)
				}

				if !mod.Memory().WriteUint32Le(resultNwritten, nwritten) {
					return 21 // ErrnoFault
				}
				return 0
			},
			"random_get":        func(uint32, uint32) uint32 { return 0 },
			"environ_get":       func(uint32, uint32) uint32 { return 0 },
			"environ_sizes_get": func(uint32, uint32) uint32 { return 0 },
			"proc_exit":         func(i uint32) { panic(i) },
		}, map[string]*wasm.HostFuncNames{
			"clock_time_get":    {},
			"fd_write":          {},
			"random_get":        {},
			"environ_get":       {},
			"environ_sizes_get": {},
			"proc_exit":         {},
		}, craneliftFeature)
	if err != nil {
		panic(err)
	}

	e.instantiateHostModule(ctx, wasiName, wasi)
}

func (e *engine) instantiateHostModule(ctx context.Context, name string, m *wasm.Module) {
	var err error
	if err = m.Validate(craneliftFeature); err != nil {
		panic(err)
	}
	m.BuildFunctionDefinitions()

	err = e.craneliftEngine.CompileModule(ctx, m, nil)
	if err != nil {
		panic(err)
	}

	_, err = e.craneliftStore.Instantiate(ctx, m, name, sys.DefaultContext(nil))
	if err != nil {
		panic(err)
	}
}

func (e *engine) instantiateCraneLiftModule(ctx context.Context) (err error) {
	e.craneLiftInst.stdout, e.craneLiftInst.stderr = bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	var sysCtx *sys.Context
	sysCtx, err = sys.NewContext(0, nil, nil, nil, e.craneLiftInst.stdout, e.craneLiftInst.stderr, nil, nil, 0, nil, 0, nil, nil)
	if err != nil {
		return
	}
	e.craneLiftInst.m, err = e.craneliftStore.Instantiate(ctx, e.craneliftModule, "", sysCtx)
	if err != nil {
		return err
	}
	m := e.craneLiftInst.m
	e.craneLiftInst.allocate = m.ExportedFunction("_allocate")
	e.craneLiftInst.deallocate = m.ExportedFunction("_deallocate")
	e.craneLiftInst.compileFunction = m.ExportedFunction("compile_function")

	// This selection logic should be lined with WazeroTarget in targets.rs.
	var kind uint64
	if runtime.GOARCH == "amd64" {
		kind = 2
	}
	if runtime.GOOS == "linux" {
		kind++
	}

	if _, err = m.ExportedFunction("initialize_target").Call(ctx, kind); err != nil {
		return fmt.Errorf("failed to initialize cranelift: %v\n%s", err, e.craneLiftInst.stderr.String())
	}
	return
}

// Close implements wasm.Engine Close.
func (e *engine) Close() (err error) {
	for _, c := range e.paramsSetupCodes {
		if err = platform.MunmapCodeSegment(c); err != nil {
			return
		}
	}
	return
}

// CompileModule implements wasm.Engine CompileModule.
func (e *engine) CompileModule(_ context.Context, module *wasm.Module, _ []experimental.FunctionListener) error {
	if module.IsHostModule {
		panic("TODO")
	}

	vmOffsets := getOpaqueVmContextOffsets(module)

	importedFns := module.ImportFuncCount()
	for i, code := range module.CodeSection {
		funcId := uint32(i) + importedFns
		cmpCtx := newCompilationContext(module, funcId, &vmOffsets)
		err := e.compileFunction(cmpCtx, code)
		if err != nil {
			return err
		}
	}

	// TODO: take lock.
	compiledFns, ok := e.pendingCompiledFunctions[module.ID]
	if !ok {
		panic("BUG")
	}

	var totalSize int
	executableOffsets := make([]int, len(compiledFns)) // Function Index (without imports) -> offset
	readers := make([]io.Reader, len(compiledFns))
	for i := range compiledFns {
		compiled := &compiledFns[i]
		// TODO: take alignment into account when necessary
		executableOffsets[i] = totalSize
		readers[i] = bytes.NewReader(compiled.machineCode)
		totalSize += len(compiled.machineCode)
	}

	// Now that we finalized the machine code layout, we are ready to resolve the direct function call relocations.
	applyFunctionRelocations(importedFns, executableOffsets, compiledFns)

	executable, err := platform.MmapCodeSegment(io.MultiReader(readers...), totalSize)
	if err != nil {
		return err
	}

	compiledMod := &compiledModule{
		executable:             executable,
		executableOffsets:      executableOffsets,
		engine:                 e,
		opaqueVmContextOffsets: vmOffsets,
	}
	e.modules[module.ID] = compiledMod

	runtime.SetFinalizer(compiledMod, func(c *compiledModule) {
		executable := c.executable
		if executable == nil {
			return // already released
		}

		// TODO: Add test.
		c.executable = nil
		if err := platform.MunmapCodeSegment(executable); err != nil {
			panic("compiler: failed to munmap executable")
		}
	})

	// TODO: take lock.
	delete(e.pendingCompiledFunctions, module.ID)
	return nil
}

func (e *engine) compileFunction(ctx context.Context, code *wasm.Code) (err error) {
	m := e.craneLiftInst.m.Memory()

	// Allocate the function body inside the cranelift module.
	locals := len(code.LocalTypes)
	localNumLeb128 := leb128.EncodeUint32(uint32(locals))

	// TODO: export wasm.encodeCode and reuse it here.
	bodySize := uint64(len(code.Body) + len(localNumLeb128) + locals*2)
	_raw, err := e.craneLiftInst.allocate.Call(ctx, bodySize)
	if err != nil {
		return err
	}

	offset := uint32(_raw[0])
	offset64 := uint64(offset)
	m.Write(offset, localNumLeb128)
	offset += uint32(len(localNumLeb128))
	for _, lt := range code.LocalTypes {
		m.WriteByte(offset, 1)
		offset++
		m.WriteByte(offset, lt)
		offset++
	}
	m.Write(offset, code.Body)

	// Now ready to call compile_function with the allocated body.
	_, err = e.craneLiftInst.compileFunction.Call(ctx, offset64, bodySize)
	if err != nil {
		return err
	}
	return
}

// CompiledModuleCount implements wasm.Engine CompiledModuleCount.
func (e *engine) CompiledModuleCount() uint32 { return uint32(len(e.modules)) }

// DeleteCompiledModule implements wasm.Engine DeleteCompiledModule.
func (e *engine) DeleteCompiledModule(m *wasm.Module) { delete(e.modules, m.ID) }

// NewModuleEngine implements wasm.Engine NewModuleEngine.
func (e *engine) NewModuleEngine(name string, m *wasm.Module, functions []wasm.FunctionInstance) (wasm.ModuleEngine, error) {
	imported := int(m.ImportFuncCount())
	vmctx := &vmContext{importedFunctions: make([]vmContextImportedFunction, imported)}

	for i, f := range functions[:imported] {
		importedVmCtx := f.Module.Engine.(*vmContext)
		executable := importedVmCtx.resolveFunctionExecutable(f.Idx)
		storage := &vmctx.importedFunctions[i]
		storage.vmctx = importedVmCtx
		storage.executable = executable
	}

	compiled, ok := e.modules[m.ID]
	if !ok {
		return nil, fmt.Errorf("source module for %s must be compiled before instantiation", name)
	}

	vmctx.parent = compiled
	if len(compiled.executableOffsets) > 0 {
		// TODO: change the signature of NewModuleEngine, and make it accept *wasm.ModuleInstance.
		// Then, this entire if block won't be necessary and we can set it ^^ directly.
		mi := functions[imported].Module // Retrieves the wasm.ModuleInstance from first local function instance.
		vmctx.module = mi
		vmctx.buildOpaqueVMContext()
	}
	return vmctx, nil
}

func (vm *vmContext) resolveFunctionExecutable(functionIndex wasm.Index) (executable *byte) {
	if localIndex := int(functionIndex) - len(vm.importedFunctions); localIndex >= 0 {
		compiled := vm.parent
		return &compiled.executable[compiled.executableOffsets[localIndex]]
	} else {
		panic("BUG: resolveFunction must be called only on locally defined functions")
	}
}

func getOpaqueVmContextOffsets(m *wasm.Module) opaqueVmContextOffsets {
	// opaqueVmContext has the following memory representation:
	//
	// type opaqueVmContext struct {
	//     localMemoryBufferPtr *byte   (optional)
	//     localMemoryLength    uint64  (optional)
	//     importedFunctions [len(vm.importedFunctions)] struct { the total size depends on # of imported functions.
	//         executable  *bytes
	//         opaqueVmCtx *byte
	//     }
	//     TODO: add more fields
	// }

	ret := opaqueVmContextOffsets{}
	var offset int
	if m.MemorySection != nil {
		ret.localMemoryBegin = offset
		offset += 16 // buffer base + memory size.
		ret.totalSize += 16
	} else {
		// Indicates that there's no local memory
		ret.localMemoryBegin = -1
	}

	ret.importedFunctionsBegin = offset
	ret.totalSize += int(m.ImportFuncCount()) * 16
	return ret
}

func (vm *vmContext) buildOpaqueVMContext() {
	vmOffsets := vm.parent.opaqueVmContextOffsets
	if vmOffsets.totalSize == 0 {
		return
	}

	vm.opaqueVmContext = make([]byte, vmOffsets.totalSize)
	vm.opaqueVmContextPtr = &vm.opaqueVmContext[0]

	if vmOffsets.localMemoryBegin >= 0 {
		mem := vm.module.Memory
		binary.LittleEndian.PutUint64(vm.opaqueVmContext[vmOffsets.localMemoryBegin:], uint64(uintptr(unsafe.Pointer(&mem.Buffer[0]))))
		binary.LittleEndian.PutUint64(vm.opaqueVmContext[vmOffsets.localMemoryBegin+8:], uint64(len(mem.Buffer)))
	}

	offset := vmOffsets.importedFunctionsBegin
	for i := range vm.importedFunctions {
		imported := &vm.importedFunctions[i]
		binary.LittleEndian.PutUint64(vm.opaqueVmContext[offset:offset+8], uint64(uintptr(unsafe.Pointer(imported.executable))))
		offset += 8
		binary.LittleEndian.PutUint64(vm.opaqueVmContext[offset:offset+8], uint64(uintptr(unsafe.Pointer(imported.vmctx.opaqueVmContextPtr))))
		offset += 8
	}
}

// Name implements wasm.ModuleEngine Name.
func (vm *vmContext) Name() string { return vm.module.Name }

var initialStackSizeInBytes = 1 << 12

// NewCallEngine implements wasm.ModuleEngine NewCallEngine.
func (vm *vmContext) NewCallEngine(callCtx *wasm.CallContext, f *wasm.FunctionInstance) (wasm.CallEngine, error) {
	typ := f.Type

	if _vm, _ := f.Module.Engine.(*vmContext); _vm != vm {
		// This case f is an imported function.
		panic("TODO: add test case to cover this after added support for imported functions")
		// return vm.NewCallEngine(callCtx, f)
	}

	s := make([]byte, initialStackSizeInBytes)
	aligned := alignedStackTop(s)
	entry := getEntryPoint(typ)
	ce := &callEngine{
		entry:           entry,
		stack:           s,
		alignedStackTop: aligned,
		vmCtx:           vm,
		typ:             typ,
		executable:      vm.resolveFunctionExecutable(f.Idx),
	}

	if len(typ.Results) > 0 {
		resultsHolder := make([]byte, typ.ResultNumInUint64*8 /* in bytes */)
		ce.resultsHolder = resultsHolder
		ce.resultsHolderPtr = &resultsHolder[0]
	}
	if len(typ.Params) > 0 {
		executable, err := vm.parent.engine.paramSetupFn(typ)
		if err != nil {
			return nil, err
		}
		ce.setParamsExecutable = &executable[0]
	}
	return ce, nil
}

// LookupFunction implements wasm.ModuleEngine LookupFunction.
func (vm *vmContext) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (idx wasm.Index, err error) {
	panic("TODO")
}

// CreateFuncElementInstance implements wasm.ModuleEngine CreateFuncElementInstance.
func (vm *vmContext) CreateFuncElementInstance(indexes []*wasm.Index) *wasm.ElementInstance {
	panic("TODO")
}

// FunctionInstanceReference implements wasm.ModuleEngine FunctionInstanceReference.
func (vm *vmContext) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference { panic("TODO") }

// String implements fmt.Stringer.
func (f functionRelocationEntry) String() string {
	return fmt.Sprintf("functino_index=%d,offset=%#x", f.index, f.offset)
}

// Call implements wasm.CallEngine Call.
func (ce *callEngine) Call(ctx context.Context, m *wasm.CallContext, params []uint64) (results []uint64, err error) {
	if len(params) > 0 {
		ce.entry(ce.vmCtx.opaqueVmContextPtr, ce.executable, ce.alignedStackTop, ce.resultsHolderPtr, ce.setParamsExecutable, &params[0])
	} else {
		ce.entry(ce.vmCtx.opaqueVmContextPtr, ce.executable, ce.alignedStackTop, ce.resultsHolderPtr, nil, nil)
	}

	if len(ce.resultsHolder) > 0 {
		results = ce.getResults()
	}
	return
}

// alignedStackTop returns 16-bytes aligned stack top of given stack.
// 16 bytes should be good for all platform (arm64/amd64).
func alignedStackTop(s []byte) uintptr {
	stackAddr := uintptr(unsafe.Pointer(&s[len(s)-1]))
	return stackAddr - (stackAddr & (16 - 1))
}

// getResults retrieves u64 represented results from the byte-represented callEngine.resultsHolder.
func (ce *callEngine) getResults() (ret []uint64) {
	resultTypes := ce.typ.Results
	ret = make([]uint64, len(resultTypes))
	offset := 0
	for i, vt := range resultTypes {
		switch vt {
		case wasm.ValueTypeI32, wasm.ValueTypeF32:
			ret[i] = uint64(binary.LittleEndian.Uint32(ce.resultsHolder[offset : offset+4]))
			offset += 4
		case wasm.ValueTypeI64, wasm.ValueTypeF64:
			ret[i] = binary.LittleEndian.Uint64(ce.resultsHolder[offset : offset+8])
			offset += 8
		default:
			panic("TODO")
		}
	}
	return
}
