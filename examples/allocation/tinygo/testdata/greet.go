package main

// #include <stdlib.h>
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// main is required for TinyGo to compile to Wasm.
func main() {}

// greet prints a greeting to the console.
func greet(name string) {
	log(fmt.Sprint("wasm >> ", greeting(name)))
}

// log a message to the console using _log.
func log(message string) {
	ptr, size := stringToPtr(message)
	_log(ptr, size)

	// as the stringToPtr returns a pointer to the underlying bytes
	// of the message it must be ensured that the string is not
	// freed
	runtime.KeepAlive(message)
}

// _log is a WebAssembly import which prints a string (linear memory offset,
// byteCount) to the console.
//
// Note: In TinyGo "//export" on a func is actually an import!
//
//go:wasm-module env
//export log
func _log(ptr uint32, size uint32)

// greeting gets a greeting for the name.
func greeting(name string) string {
	return fmt.Sprint("Hello, ", name, "!")
}

// _greet is a WebAssembly export that accepts a string pointer (linear memory
// offset) and calls greet.
//
//export greet
func _greet(ptr, size uint32) {
	name := ptrToString(ptr, size)
	greet(name)
}

// _greeting is a WebAssembly export that accepts a string pointer (linear memory
// offset) and returns a pointer/size pair packed into a uint64.
//
// Note: This uses a uint64 instead of two result values for compatibility with
// WebAssembly 1.0.
//
//export greeting
func _greeting(ptr, size uint32) (ptrSize uint64) {
	name := ptrToString(ptr, size)
	g := greeting(name)
	ptr, size = stringToLeakedPtr(g)
	return (uint64(ptr) << uint64(32)) | uint64(size)
}

// ptrToString returns a string from WebAssembly compatible numeric types
// representing its pointer and length.
func ptrToString(ptr uint32, size uint32) string {
	return unsafe.String((*byte)(unsafe.Pointer(uintptr(ptr))), size)
}

// stringToPtr returns a pointer and size pair to the underlying bytes of the given
// string in a way compatible with WebAssembly numeric types.
//
// Notes:
//   - since Go strings are  immutable, the returned pointer must not be modified.
//   - this method does not keep the given string alive, hence it is the responsibility
//     of the caller to ensure the string is kept alive
func stringToPtr(s string) (uint32, uint32) {
	ptr := unsafe.StringData(s)
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return uint32(unsafePtr), uint32(len(s))
}

// stringToLeakedPtr returns a pointer and size pair for the given string in a way
// compatible with WebAssembly numeric types. The pointer is not automatically
// managed by TinyGo hence it must be freed by the host.
func stringToLeakedPtr(s string) (uint32, uint32) {
	size := C.ulong(len(s))
	ptr := unsafe.Pointer(C.malloc(size))

	copy(unsafe.Slice((*byte)(ptr), size), []byte(s))

	return uint32(uintptr(ptr)), uint32(len(s))
}
