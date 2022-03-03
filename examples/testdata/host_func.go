package main

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"unsafe"
)

func main() {}

//export allocate_buffer
func allocateBuffer(size uint32) *byte {
	// Allocate the in-Wasm memory region and returns its pointer to hosts.
	// The region is supposed to store random bytes generated in hosts.
	buf := make([]byte, size)
	return &buf[0]
}

// Note: Export on a function without an implementation is a Wasm import that defaults to the module "env".
//export get_random_bytes
func get_random_bytes(retBufPtr **byte, retBufSize *int)

// Get random bytes from the host.
func getRandomBytes() []byte {
	var bufPtr *byte
	var bufSize int
	get_random_bytes(&bufPtr, &bufSize)
	//nolint
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(bufPtr)),
		Len:  uintptr(bufSize),
		Cap:  uintptr(bufSize),
	}))
}

//export base64
func base64OnString() {
	// Get random bytes from the host and
	// do base64 encoding them for given times.
	buf := getRandomBytes()
	encoded := base64.StdEncoding.EncodeToString(buf)
	fmt.Println(encoded)
}
