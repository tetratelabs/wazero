package main

import "C"
import (
	"context"
	"reflect"
	"unsafe"

	"github.com/tetratelabs/wazero"
)

// validate accepts maybe-invalid Wasm module bytes and ensures that our validation phase works correctly
// as well as the compiler doesn't panic during compilation!
//
//export validate
func validate(binaryPtr uintptr, binarySize int) {
	wasmBin := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: binaryPtr,
		Len:  binarySize,
		Cap:  binarySize,
	}))

	failed := true
	defer func() {
		if failed {
			// If the test fails, we save the binary and wat into testdata directory.
			saveFailedBinary(wasmBin, "", "TestReRunFailedValidateCase")
		}
	}()

	tryCompile(wasmBin)
	failed = false
}

// Ensure that validation and compilation do not panic!
func tryCompile(wasmBin []byte) {
	ctx := context.Background()
	compiler := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	_, _ = compiler.CompileModule(ctx, wasmBin)
}
