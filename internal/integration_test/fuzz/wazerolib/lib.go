package main

import "C"
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func main() {}

// run_wazero ensures that the behavior is the same between the compiler and the interpreter for any given binary.
//
//export run_wazero
func run_wazero(binaryPtr uintptr, binarySize int, watPtr uintptr, watSize int) {
	wasmBin := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: binaryPtr,
		Len:  binarySize,
		Cap:  binarySize,
	}))

	wat := *(*string)(unsafe.Pointer(&reflect.SliceHeader{
		Data: watPtr,
		Len:  watSize,
		Cap:  watSize,
	}))

	// Create two runtimes.

	failed := true
	defer func() {
		if failed {
			// If the test fails, we save the binary and wat into testdata directory.
			saveFailedBinary(wasmBin, wat)
		}
	}()

	requireNoDiff(wasmBin, func(err error) {
		if err != nil {
			panic(err)
		}
	})

	failed = false
	return
}

func requireNoDiff(wasmBin []byte, requireNoError func(err error)) {
	// Choose the context to use for function calls.
	ctx := context.Background()

	compiler := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	interpreter := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())

	defer compiler.Close(ctx)
	defer interpreter.Close(ctx)

	compiledCompiled, err := compiler.CompileModule(ctx, wasmBin)
	requireNoError(err)

	interpreterCompiled, err := interpreter.CompileModule(ctx, wasmBin)
	requireNoError(err)

	// Instantiate module.
	compilerMod, compilerInstErr := compiler.InstantiateModule(ctx, compiledCompiled, wazero.NewModuleConfig())
	interpreterMod, interpreterInstErr := interpreter.InstantiateModule(ctx, interpreterCompiled, wazero.NewModuleConfig())

	okToInvoke, err := ensureInstantiationError(compilerInstErr, interpreterInstErr)
	requireNoError(err)

	if okToInvoke {
		err = ensureInvocationResultMatch(compilerMod, interpreterMod, interpreterCompiled.ExportedFunctions())
		requireNoError(err)
	}
}

const valueTypeVector = 0x7b

func ensureInvocationResultMatch(compiledMod, interpreterMod api.Module, exportedFunctions map[string]api.FunctionDefinition) (err error) {
	ctx := context.Background()

outer:
	for name, def := range exportedFunctions {
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
		cmpRes, cmpErr := cmpF.Call(ctx, params...)
		intRes, intErr := intF.Call(ctx, params...)
		if errMismatch := ensureInvocationError(cmpErr, intErr); errMismatch != nil {
			panic(fmt.Sprintf("error mismatch on invoking %s: %v", name, errMismatch))
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
			err = fmt.Errorf("result mismatch on invoking '%s':\n\tinterpreter got: %v\n\tcompiler got: %v", name, intRes, cmpRes)
		}
	}
	return
}

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

	if compilerErrMsg != interpErrMsg {
		return fmt.Errorf("error mismatch:\n\tinterpreter: %v\n\tcompiler: %v", interpErr, compilerErr)
	}
	return nil
}

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

	if !allowedErrorDuringInstantiation(compilerErrMsg) {
		return false, fmt.Errorf("invalid error occur with compiler: %v", compilerErr)
	} else if !allowedErrorDuringInstantiation(interpErrMsg) {
		return false, fmt.Errorf("invalid error occur with interpreter: %v", interpErrMsg)
	}

	if compilerErrMsg != interpErrMsg {
		return false, fmt.Errorf("error mismatch:\n\tinterpreter: %v\n\tcompiler: %v", interpErr, compilerErr)
	}
	return false, nil
}

func allowedErrorDuringInstantiation(errMsg string) bool {
	if strings.HasPrefix(errMsg, "data[") && strings.HasSuffix(errMsg, "]: out of bounds memory access") {
		return true
	}

	if strings.HasPrefix(errMsg, "start function[") && strings.Contains(errMsg, "failed: wasm error:") {
		return true
	}
	return false
}

const failedCasesDir = "wazerolib/testdata"

func saveFailedBinary(bin []byte, wat string) {
	checksum := sha256.Sum256(bin)
	checkSumStr := hex.EncodeToString(checksum[:])

	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	testDataDir := path.Join(dir, failedCasesDir)
	binaryPath := path.Join(testDataDir, fmt.Sprintf("%s.wasm", checkSumStr))
	f, err := os.Create(binaryPath)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	_, err = f.Write(bin)
	if err != nil {
		panic(err)
	}

	watPath := path.Join(testDataDir, fmt.Sprintf("%s.wat", checkSumStr))
	watF, err := os.Create(watPath)
	if err != nil {
		panic(err)
	}

	defer watF.Close()

	_, err = watF.Write([]byte(wat))
	if err != nil {
		panic(err)
	}

	fmt.Printf(`
Failed WebAssembly Text:
%s

Failed Wasm binary has been written to %s
Failed Wasm Text has been written to %s
To reproduce the failure, execute: WASM_BINARY_PATH=%s go test ./wazerolib/...


`, wat, binaryPath, watPath, binaryPath)
}
