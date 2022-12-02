package main

import "C"
import (
	"context"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/tetratelabs/wazero"
)

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

func tryCompile(wasmBin []byte) {
	ctx := context.Background()
	compiler := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	_, err := compiler.CompileModule(ctx, wasmBin)
	if err != nil {
		fmt.Println(err)
	}
}
