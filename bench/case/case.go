package main

import (
	"encoding/base64"
	"reflect"
	"unsafe"
)

func main() {}

//export allocate_buffer
func allocateBuffer(size uint32) *byte {
	// Allocate the in-Wasm memory region and returns its pointer to hosts.
	// The region is supposed to store random strings generated in hosts,
	// meaning that this is called "inside" of get_random_string.
	buf := make([]byte, size)
	return &buf[0]
}

//export get_random_string
func getRandomStringRaw(retBufPtr **byte, retBufSize *int)

// Get randm string from the hosts.
func getRandmString() string {
	var bufPtr *byte
	var bufSize int
	getRandomStringRaw(&bufPtr, &bufSize)
	//nolint
	return *(*string)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(bufPtr)),
		Len:  uintptr(bufSize),
		Cap:  uintptr(bufSize),
	}))
}

//export base64
func base64OnString(num uint32) {
	// Get random strings from the host and
	// do base64 encoding them for given times.
	for i := uint32(0); i < num; i++ {
		msg := getRandmString()
		_ = base64.StdEncoding.EncodeToString([]byte(msg))
	}
}

//export fibonacci
func fibonacci(in uint32) uint32 {
	if in <= 1 {
		return in
	}
	return fibonacci(in-1) + fibonacci(in-2)
}
