package main

import (
	"os"

	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

// Example_main ensures the following will work:
//
//	go build cat.go
//	./cat ./test.txt
func Example_main() {
	b, err := binary.DecodeModule(catWasm, wasm.FeaturesFinished, wasm.MemoryMaxPages)
	if err != nil {
		panic(err)
	}
	catWasm = binary.EncodeModule(b)

	// Save the old os.Args and replace with our example input.
	oldArgs := os.Args
	os.Args = []string{"cat", "./test.txt"}
	defer func() { os.Args = oldArgs }()

	main()

	// Output:
	// hello filesystem
}
