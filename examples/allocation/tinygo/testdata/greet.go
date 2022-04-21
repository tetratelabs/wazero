package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

// main is required for TinyGo to compile to Wasm.
func main() {}

/// greet prints a greeting to the console.
func greet(name string) {
	log(fmt.Sprint("wasi >> ", greeting(name)))
}

/// Logs a message to the console using _log.
func log(message string) {
	buf := []byte(message)
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	_log(uint32(unsafePtr), uint32(len(buf)))
}

// _log is a WebAssembly import which prints a string (linear memory offset,
// byteCount) to the console.
//
// Note: In TinyGo "//export" on a func is actually an import!
//go:wasm-module env
//export log
func _log(ptr uint32, size uint32)

// greeting gets a greeting for the name.
func greeting(name string) string {
	return fmt.Sprint("Hello, ", name, "!")
}

// _greet is a WebAssembly export that accepts a string pointer (linear
// memory offset) and calls [`greet`].
//export greet
func _greet(ptr, size uint32) {
	name := pointerAndSizeToString(ptr, size)
	greet(name)
}

// _greet is a WebAssembly export that accepts a string pointer (linear
// memory offset) and returns a pointer/size pair packed into a uint64.
//
// Note: This approach is compatible with WebAssembly 1.0, which doesn't
// support multiple return values.
//export greeting
func _greeting(ptr, size uint32) (ptrSize uint64) {
	name := pointerAndSizeToString(ptr, size)
	g := greeting(name)
	ptr, size = stringToPointerAndSize(g)
	return (uint64(ptr) << uint64(32)) | uint64(size)
}

// pointerAndSizeToString returns a string from WebAssembly compatible numeric
// types representing its pointer and length.
func pointerAndSizeToString(ptr uint32, size uint32) (ret string) {
	// Here, we want to get a string represented by the ptr and size. If we
	// wanted a []byte, we'd use reflect.SliceHeader instead.
	strHdr := (*reflect.StringHeader)(unsafe.Pointer(&ret))
	strHdr.Data = uintptr(ptr)
	strHdr.Len = uintptr(size)
	return
}

// stringToPointerAndSize returns a pointer and size pair for the given string
// in a way that is compatible with WebAssembly numeric types.
func stringToPointerAndSize(s string) (uint32, uint32) {
	buf := []byte(s)
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return uint32(unsafePtr), uint32(len(buf))
}
