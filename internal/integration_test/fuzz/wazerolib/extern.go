package main

import "C"
import (
	"reflect"
	"unsafe"
)

func main() {}

// require_no_diff ensures that the behavior is the same between the compiler and the interpreter for any given binary.
// And if there's diff, this also saves the problematic binary and wat into testdata directory.
//
//export require_no_diff
func require_no_diff(binaryPtr uintptr, binarySize int, checkMemory bool) {
	// TODO: use unsafe.Slice after flooring Go 1.20.
	var wasmBin []byte
	wasmHdr := (*reflect.SliceHeader)(unsafe.Pointer(&wasmBin))
	wasmHdr.Data = binaryPtr
	wasmHdr.Len = binarySize
	wasmHdr.Cap = binarySize

	failed := true
	defer func() {
		if failed {
			// If the test fails, we save the binary and wat into testdata directory.
			saveFailedBinary(wasmBin, "TestReRunFailedRequireNoDiffCase")
		}
	}()

	requireNoDiff(wasmBin, checkMemory, func(err error) {
		if err != nil {
			panic(err)
		}
	})

	failed = false
}

// validate accepts maybe-invalid Wasm module bytes and ensures that our validation phase works correctly
// as well as the compiler doesn't panic during compilation!
//
//export validate
func validate(binaryPtr uintptr, binarySize int) {
	// TODO: use unsafe.Slice after flooring Go 1.20.
	var wasmBin []byte
	wasmHdr := (*reflect.SliceHeader)(unsafe.Pointer(&wasmBin))
	wasmHdr.Data = binaryPtr
	wasmHdr.Len = binarySize
	wasmHdr.Cap = binarySize

	failed := true
	defer func() {
		if failed {
			// If the test fails, we save the binary and wat into testdata directory.
			saveFailedBinary(wasmBin, "TestReRunFailedValidateCase")
		}
	}()

	tryCompile(wasmBin)
	failed = false
}
