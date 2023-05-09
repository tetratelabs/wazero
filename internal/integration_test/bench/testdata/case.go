package main

import (
	"encoding/base64"
	"math/rand"
	"strings"
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

// Get random string from the hosts.
func getRandomString() string {
	var bufPtr *byte
	var bufSize int
	getRandomStringRaw(&bufPtr, &bufSize)
	return unsafe.String(bufPtr, bufSize)
}

//export base64
func base64OnString(num uint32) {
	// Get random strings from the host and
	// do base64 encoding them for given times.
	for i := uint32(0); i < num; i++ {
		msg := getRandomString()
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

//export string_manipulation
func stringManipulation(initialSize uint32) {
	var str string
	for i := 0; i < int(initialSize); i++ {
		str += "a"
		str += "b"
	}
	str2 := strings.Replace(str, "a", "b", -1)
	str3 := str2[:3]
	str4 := str2[3:]
	lastStr := str4 + str3
	lastStr = ""
	print(lastStr)
}

//export reverse_array
func reverseArray(size uint32) {
	array := make([]int, size)
	for i := 0; i < (len(array) >> 1); i++ {
		j := len(array) - i - 1
		array[j], array[i] = array[i], array[j]
	}
}

//export random_mat_mul
func randomMatMul(n uint32) {
	var a, b, result [][]int
	for i := uint32(0); i < n; i++ {
		arow := make([]int, n)
		brow := make([]int, n)
		for j := uint32(0); j < n; j++ {
			arow[j] = rand.Int()
			brow[j] = rand.Int()
		}
		a = append(a, arow)
		b = append(b, brow)
		result = append(result, make([]int, n))
	}

	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			for k := uint32(0); k < n; k++ {
				result[i][j] += a[i][k] * b[k][j]
			}
		}
	}
}
