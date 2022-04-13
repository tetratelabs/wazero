package main

import "os"

// ExampleMain ensures the following will work:
//
//	go build cat.go
//	./cat ./test.txt
func ExampleMain() {

	// Save the old os.Args and replace with our example input.
	oldArgs := os.Args
	os.Args = []string{"cat", "./test.txt"}
	defer func() { os.Args = oldArgs }()

	main()

	// Output:
	// hello filesystem
}
