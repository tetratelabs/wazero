package main

import (
	"os"
)

// ExampleMain ensures the following will work:
//
//	go build add.go
//	./add 7 9
func ExampleMain() {

	// Save the old os.Args and replace with our example input.
	oldArgs := os.Args
	os.Args = []string{"add", "7", "9"}
	defer func() { os.Args = oldArgs }()

	main()

	// Output:
	// wasm/math: 7 + 9 = 16
	// host/math: 7 + 9 = 16
}
