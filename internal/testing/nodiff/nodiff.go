package nodiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// We haven't had public APIs for referencing all the imported entries from wazero.CompiledModule,
// so we use the unsafe.Pointer and the internal memory layout to get the internal *wasm.Module
// from wazero.CompiledFunction.  This must be synced with the struct definition of wazero.compiledModule (internal one).
func extractInternalWasmModuleFromCompiledModule(c wazero.CompiledModule) (*wasm.Module, error) {
	// This is the internal representation of interface in Go.
	// https://research.swtch.com/interfaces
	type iface struct {
		tp   *byte
		data unsafe.Pointer
	}

	// This corresponds to the unexported wazero.compiledModule to get *wasm.Module from wazero.CompiledModule interface.
	type compiledModule struct {
		module *wasm.Module
	}

	ciface := (*iface)(unsafe.Pointer(&c))
	if ciface == nil {
		return nil, errors.New("invalid pointer")
	}
	cm := (*compiledModule)(ciface.data)
	return cm.module, nil
}

// RequireNoDiffT is a wrapper of RequireNoDiff for testing.T.
func RequireNoDiffT(t *testing.T, wasmBin []byte, checkMemory, loggingCheck bool) {
	RequireNoDiff(wasmBin, checkMemory, loggingCheck, func(err error) { require.NoError(t, err) })
}

// RequireNoDiff ensures that the behavior is the same between the compiler and the interpreter for any given binary.
func RequireNoDiff(wasmBin []byte, checkMemory, loggingCheck bool, requireNoError func(err error)) {
	const features = api.CoreFeaturesV2 | experimental.CoreFeaturesThreads
	compiler := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigCompiler().WithCoreFeatures(features))
	interpreter := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(features))
	defer compiler.Close(context.Background())
	defer interpreter.Close(context.Background())

	interpreterCtx, compilerCtx := context.Background(), context.Background()
	var interPreterLoggingBuf, compilerLoggingBuf bytes.Buffer
	var errorDuringInvocation bool
	if loggingCheck {
		interpreterCtx = experimental.WithFunctionListenerFactory(interpreterCtx, logging.NewLoggingListenerFactory(&interPreterLoggingBuf))
		compilerCtx = experimental.WithFunctionListenerFactory(compilerCtx, logging.NewLoggingListenerFactory(&compilerLoggingBuf))
		defer func() {
			if !errorDuringInvocation {
				if !bytes.Equal(compilerLoggingBuf.Bytes(), interPreterLoggingBuf.Bytes()) {
					requireNoError(fmt.Errorf("logging mismatch\ncompiler: %s\ninterpreter: %s",
						compilerLoggingBuf.String(), interPreterLoggingBuf.String()))
				}
			}
		}()
	}

	compilerCompiled, err := compiler.CompileModule(compilerCtx, wasmBin)
	if err != nil && strings.Contains(err.Error(), "has an empty module name") {
		// This is the limitation wazero imposes to allow special-casing of anonymous modules.
		return
	}
	requireNoError(err)

	interpreterCompiled, err := interpreter.CompileModule(interpreterCtx, wasmBin)
	requireNoError(err)

	internalMod, err := extractInternalWasmModuleFromCompiledModule(compilerCompiled)
	requireNoError(err)

	if skip := ensureDummyImports(compiler, internalMod, requireNoError); skip {
		return
	}
	ensureDummyImports(interpreter, internalMod, requireNoError)

	// Instantiate module.
	compilerMod, compilerInstErr := compiler.InstantiateModule(compilerCtx, compilerCompiled,
		wazero.NewModuleConfig().WithName(string(internalMod.ID[:])))
	interpreterMod, interpreterInstErr := interpreter.InstantiateModule(interpreterCtx, interpreterCompiled,
		wazero.NewModuleConfig().WithName(string(internalMod.ID[:])))

	okToInvoke, err := ensureInstantiationError(compilerInstErr, interpreterInstErr)
	requireNoError(err)

	if okToInvoke {
		err, errorDuringInvocation = ensureInvocationResultMatch(
			compilerCtx, interpreterCtx,
			compilerMod, interpreterMod, interpreterCompiled.ExportedFunctions())
		requireNoError(err)

		compilerMem, _ := compilerMod.Memory().(*wasm.MemoryInstance)
		interpreterMem, _ := interpreterMod.Memory().(*wasm.MemoryInstance)
		if checkMemory && compilerMem != nil && interpreterMem != nil {
			if !bytes.Equal(compilerMem.Buffer, interpreterMem.Buffer) {
				requireNoError(errors.New("memory state mimsmatch"))
			}
		}
		ensureMutableGlobalsMatch(compilerMod, interpreterMod, requireNoError)
	}
}

func ensureMutableGlobalsMatch(compilerMod, interpreterMod api.Module, requireNoError func(err error)) {
	ci, ii := compilerMod.(*wasm.ModuleInstance), interpreterMod.(*wasm.ModuleInstance)
	if len(ci.Globals) == 0 {
		return
	}
	var es []string
	for i := range ci.Globals[:len(ci.Globals)-1] { // The last global is the fuel, so we can ignore it.
		cg := ci.Globals[i]
		ig := ii.Globals[i]
		if !cg.Type.Mutable {
			continue
		}

		cVal, cValHi := cg.Value()
		iVal, iValHi := ig.Value()

		var ok bool
		switch ig.Type.ValType {
		case wasm.ValueTypeI32, wasm.ValueTypeF32:
			ok = uint32(cVal) == uint32(iVal)
		case wasm.ValueTypeI64, wasm.ValueTypeF64:
			ok = cVal == iVal
		case wasm.ValueTypeV128:
			ok = cVal == iVal && cValHi == iValHi
		default:
			ok = true // Ignore other types.
		}

		if !ok {
			if typ := ig.Type.ValType; typ == wasm.ValueTypeV128 {
				es = append(es, fmt.Sprintf("\t[%d] %s: (%v,%v) != (%v,%v)",
					i, wasm.ValueTypeName(wasm.ValueTypeV128), cVal, cValHi, iVal, iValHi))
			} else {
				es = append(es, fmt.Sprintf("\t[%d] %s: %v != %v",
					i, wasm.ValueTypeName(typ), cVal, iVal))
			}
		}
	}
	if len(es) > 0 {
		requireNoError(fmt.Errorf("mutable globals mismatch:\n%s", strings.Join(es, "\n")))
	}
}

// ensureDummyImports instantiates the modules which are required imports by `origin` *wasm.Module.
func ensureDummyImports(r wazero.Runtime, origin *wasm.Module, requireNoError func(err error)) (skip bool) {
	impMods := make(map[string][]wasm.Import)
	for _, imp := range origin.ImportSection {
		if imp.Module == "" {
			// Importing empty modules are forbidden as future work will allow multiple anonymous modules.
			skip = true
			return
		}
		impMods[imp.Module] = append(impMods[imp.Module], imp)
	}

	for mName, impMod := range impMods {
		usedName := make(map[string]struct{}, len(impMod))
		m := &wasm.Module{NameSection: &wasm.NameSection{ModuleName: mName}}

		for _, imp := range impMod {
			_, ok := usedName[imp.Name]
			if ok {
				// Import segment can have duplicated "{module_name}.{name}" pair while it is prohibited for exports.
				// Decision on allowing modules with these "ill" imports or not is up to embedder, and wazero chooses
				// not to allow. Hence, we skip the entire case.
				// See "Note" at https://www.w3.org/TR/wasm-core-2/syntax/modules.html#imports
				return true
			} else {
				usedName[imp.Name] = struct{}{}
			}

			var index uint32
			switch imp.Type {
			case wasm.ExternTypeFunc:
				tp := origin.TypeSection[imp.DescFunc]
				typeIdx := uint32(len(m.TypeSection))
				index = uint32(len(m.FunctionSection))
				m.FunctionSection = append(m.FunctionSection, typeIdx)
				m.TypeSection = append(m.TypeSection, tp)
				body := bytes.NewBuffer(nil)
				for _, vt := range tp.Results {
					switch vt {
					case wasm.ValueTypeI32:
						body.WriteByte(wasm.OpcodeI32Const)
						body.WriteByte(0)
					case wasm.ValueTypeI64:
						body.WriteByte(wasm.OpcodeI64Const)
						body.WriteByte(0)
					case wasm.ValueTypeF32:
						body.Write([]byte{wasm.OpcodeF32Const, 0, 0, 0, 0})
					case wasm.ValueTypeF64:
						body.Write([]byte{wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0, 0})
					case wasm.ValueTypeV128:
						body.Write([]byte{
							wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const,
							0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
						})
					case wasm.ValueTypeExternref:
						body.Write([]byte{wasm.OpcodeRefNull, wasm.RefTypeExternref})
					case wasm.ValueTypeFuncref:
						body.Write([]byte{wasm.OpcodeRefNull, wasm.RefTypeFuncref})
					}
				}
				body.WriteByte(wasm.OpcodeEnd)
				m.CodeSection = append(m.CodeSection, wasm.Code{Body: body.Bytes()})
			case wasm.ExternTypeGlobal:
				index = uint32(len(m.GlobalSection))
				var data []byte
				var opcode byte
				switch imp.DescGlobal.ValType {
				case wasm.ValueTypeI32:
					opcode = wasm.OpcodeI32Const
					data = []byte{0}
				case wasm.ValueTypeI64:
					opcode = wasm.OpcodeI64Const
					data = []byte{0}
				case wasm.ValueTypeF32:
					opcode = wasm.OpcodeF32Const
					data = []byte{0, 0, 0, 0}
				case wasm.ValueTypeF64:
					opcode = wasm.OpcodeF64Const
					data = []byte{0, 0, 0, 0, 0, 0, 0, 0}
				case wasm.ValueTypeV128:
					opcode = wasm.OpcodeVecPrefix
					data = []byte{wasm.OpcodeVecV128Const, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
				case wasm.ValueTypeExternref:
					opcode = wasm.OpcodeRefNull
					data = []byte{wasm.RefTypeExternref}
				case wasm.ValueTypeFuncref:
					opcode = wasm.OpcodeRefNull
					data = []byte{wasm.RefTypeFuncref}
				}
				m.GlobalSection = append(m.GlobalSection, wasm.Global{
					Type: imp.DescGlobal, Init: wasm.ConstantExpression{Opcode: opcode, Data: data},
				})
			case wasm.ExternTypeMemory:
				m.MemorySection = imp.DescMem
				index = 0
			case wasm.ExternTypeTable:
				index = uint32(len(m.TableSection))
				m.TableSection = append(m.TableSection, imp.DescTable)
			}
			m.ExportSection = append(m.ExportSection, wasm.Export{Type: imp.Type, Name: imp.Name, Index: index})
		}
		_, err := r.Instantiate(context.Background(), binaryencoding.EncodeModule(m))
		requireNoError(err)
	}
	return
}

const valueTypeVector = 0x7b

// ensureInvocationResultMatch invokes all the exported functions from the module, and compare all the results between compiler vs interpreter.
func ensureInvocationResultMatch(
	compilerCtx, interpreterCtx context.Context, compiledMod, interpreterMod api.Module,
	exportedFunctions map[string]api.FunctionDefinition,
) (err error, errorDuringInvocation bool) {
	// In order to do the deterministic execution, we need to sort the exported functions.
	var names []string
	for f := range exportedFunctions {
		names = append(names, f)
	}
	sort.Strings(names)

outer:
	for _, name := range names {
		def := exportedFunctions[name]
		resultTypes := def.ResultTypes()
		for _, rt := range resultTypes {
			switch rt {
			case api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeF32, api.ValueTypeF64, valueTypeVector:
			default:
				// For the sake of simplicity in the assertion, we only invoke the function with the basic types.
				continue outer
			}
		}

		cmpF := compiledMod.ExportedFunction(name)
		intF := interpreterMod.ExportedFunction(name)

		params := getDummyValues(def.ParamTypes())
		cmpRes, cmpErr := cmpF.Call(compilerCtx, params...)
		intRes, intErr := intF.Call(interpreterCtx, params...)
		errorDuringInvocation = errorDuringInvocation || cmpErr != nil || intErr != nil
		if errMismatch := ensureInvocationError(cmpErr, intErr); errMismatch != nil {
			err = errors.Join(err, fmt.Errorf("error mismatch on invoking %s: %v", name, errMismatch))
			continue
		}

		matched := true
		var typesIndex int
		for i := 0; i < len(cmpRes); i++ {
			switch resultTypes[typesIndex] {
			case api.ValueTypeI32, api.ValueTypeF32:
				matched = matched && uint32(cmpRes[i]) == uint32(intRes[i])
			case api.ValueTypeI64, api.ValueTypeF64:
				matched = matched && cmpRes[i] == intRes[i]
			case valueTypeVector:
				matched = matched && cmpRes[i] == intRes[i] && cmpRes[i+1] == intRes[i+1]
				i++ // We need to advance twice (lower and higher 64bits)
			}
			typesIndex++
		}

		if !matched {
			err = errors.Join(err, fmt.Errorf("result mismatch on invoking '%s':\n\tinterpreter got: %v\n\tcompiler got: %v", name, intRes, cmpRes))
		}
	}
	return
}

// getDummyValues returns a dummy input values for function invocations.
func getDummyValues(valueTypes []api.ValueType) (ret []uint64) {
	for _, vt := range valueTypes {
		if vt != 0x7b { // v128
			ret = append(ret, 0)
		} else {
			ret = append(ret, 0, 0)
		}
	}
	return
}

// ensureInvocationError ensures that function invocation errors returned by interpreter and compiler match each other's.
func ensureInvocationError(compilerErr, interpErr error) error {
	if compilerErr == nil && interpErr == nil {
		return nil
	} else if compilerErr == nil && interpErr != nil {
		return fmt.Errorf("compiler returned no error, but interpreter got: %w", interpErr)
	} else if compilerErr != nil && interpErr == nil {
		return fmt.Errorf("interpreter returned no error, but compiler got: %w", compilerErr)
	}

	compilerErrMsg, interpErrMsg := compilerErr.Error(), interpErr.Error()
	if idx := strings.Index(compilerErrMsg, "\n"); idx >= 0 {
		compilerErrMsg = compilerErrMsg[:strings.Index(compilerErrMsg, "\n")]
	}
	if idx := strings.Index(interpErrMsg, "\n"); idx >= 0 {
		interpErrMsg = interpErrMsg[:strings.Index(interpErrMsg, "\n")]
	}

	if compiledStackOverFlow := strings.Contains(compilerErrMsg, "stack overflow"); compiledStackOverFlow && strings.Contains(interpErrMsg, "unreachable") {
		// Compiler is more likely to reach stack overflow than interpreter, so we allow this case. This case is most likely
		// that interpreter reached the unreachable out of "fuel".
		return nil
	} else if interpreterStackOverFlow := strings.Contains(interpErrMsg, "stack overflow"); compiledStackOverFlow && interpreterStackOverFlow {
		// Both compiler and interpreter reached stack overflow, so we ignore diff in the content of the traces.
		return nil
	}

	if compilerErrMsg != interpErrMsg {
		return fmt.Errorf("error mismatch:\n\tinterpreter: %v\n\tcompiler: %v", interpErr, compilerErr)
	}
	return nil
}

// ensureInstantiationError ensures that instantiation errors returned by interpreter and compiler match each other's.
func ensureInstantiationError(compilerErr, interpErr error) (okToInvoke bool, err error) {
	if compilerErr == nil && interpErr == nil {
		return true, nil
	} else if compilerErr == nil && interpErr != nil {
		return false, fmt.Errorf("compiler returned no error, but interpreter got: %w", interpErr)
	} else if compilerErr != nil && interpErr == nil {
		return false, fmt.Errorf("interpreter returned no error, but compiler got: %w", compilerErr)
	}

	compilerErrMsg, interpErrMsg := compilerErr.Error(), interpErr.Error()
	if idx := strings.Index(compilerErrMsg, "\n"); idx >= 0 {
		compilerErrMsg = compilerErrMsg[:strings.Index(compilerErrMsg, "\n")]
	}
	if idx := strings.Index(interpErrMsg, "\n"); idx >= 0 {
		interpErrMsg = interpErrMsg[:strings.Index(interpErrMsg, "\n")]
	}

	if strings.Contains(compilerErrMsg, "stack overflow") && strings.Contains(interpErrMsg, "unreachable") {
		// This is the case where the compiler reached stack overflow, but the interpreter reached the unreachable out of "fuel" during
		// start function invocation. This is fine.
		return false, nil
	}

	if !allowedErrorDuringInstantiation(compilerErrMsg) {
		return false, fmt.Errorf("invalid error occur with compiler: %v vs interpreter: %v", compilerErr, interpErr)
	} else if !allowedErrorDuringInstantiation(interpErrMsg) {
		return false, fmt.Errorf("invalid error occur with interpreter: %v vs compiler: %v", interpErr, compilerErr)
	}

	if compilerErrMsg != interpErrMsg {
		return false, fmt.Errorf("error mismatch:\n\tinterpreter: %v\n\tcompiler: %v", interpErr, compilerErr)
	}
	return false, nil
}

// allowedErrorDuringInstantiation checks if the error message is considered sane.
func allowedErrorDuringInstantiation(errMsg string) bool {
	// This happens when data segment causes out of bound, but it is considered as runtime-error in WebAssembly 2.0
	// which is fine.
	if strings.HasPrefix(errMsg, "data[") && strings.HasSuffix(errMsg, "]: out of bounds memory access") {
		return true
	}

	// Start function failure is neither instantiation nor compilation error, but rather a runtime error, so that is fine.
	if strings.HasPrefix(errMsg, "start function[") && strings.Contains(errMsg, "failed: wasm error:") {
		return true
	}
	return false
}
