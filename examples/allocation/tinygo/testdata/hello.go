package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

// main is required for TinyGo to compile to Wasm.
func main() {}

// sayHello prints a greeting to the console using log.
func sayHello(name string) {
	log(fmt.Sprint("Hello, ", name, "!"))
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

// _sayHello is a WebAssembly export that accepts a string pointer (linear
// memory offset) and calls [`say_hello`].
//export say_hello
func _sayHello(ptr, size uint32) {
	// Here, we want to get a string represented by the ptr and size. If we
	// wanted a []byte, we'd use reflect.SliceHeader instead.
	var name string
	strHdr := (*reflect.StringHeader)(unsafe.Pointer(&name))
	strHdr.Data = uintptr(ptr)
	strHdr.Len = uintptr(size)
	sayHello(name)
}
