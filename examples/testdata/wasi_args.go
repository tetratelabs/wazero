package main

import (
	"fmt"
	"os"
	"syscall"
)

//go:wasm-module wasi_snapshot_preview1
//export args_get
func args_get(uint32, uint32) uint16

//go:wasm-module wasi_snapshot_preview1
//export args_sizes_get
func args_sizes_get(uint32, uint32) uint16
func main() {
	testGetWASIArgs()
	testInvalidArguments()
}

func testGetWASIArgs() {
	var expectedArgs = []string{
		"foo",
		"bar",
		"foobar",
		"",
		"baz",
	}

	if len(os.Args) != len(expectedArgs) {
		panic(fmt.Sprintf("The length of the args is not expected: %v", len(os.Args)))
	}

	for i, expectedArg := range expectedArgs {
		if os.Args[i] != expectedArg {
			panic(fmt.Sprintf("os.Args are not what are expected. expected: %v, actual: %v", expectedArgs, os.Args))
		}
	}
}

func testInvalidArguments() {
	outOfBounds := uint32(0x1000_000) // some big value which exceeds the memory size

	errno := args_get(outOfBounds, outOfBounds)
	if errno != uint16(syscall.EINVAL) {
		panic(fmt.Sprintf("args_get didn't return EINVAL. errno: %v", errno))
	}

	errno = args_sizes_get(outOfBounds, outOfBounds)
	if errno != uint16(syscall.EINVAL) {
		panic(fmt.Sprintf("args_get didn't return EINVAL. errno: %v", errno))
	}
}
