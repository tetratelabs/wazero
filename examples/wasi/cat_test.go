package wasi_example

import "os"

// Example_main ensures the following will work:
//
//	go run cat.go ./test.txt
func Example_main() {

	// Save the old os.Args and replace with our example input.
	oldArgs := os.Args
	os.Args = []string{"cat", "./test.txt"}
	defer func() { os.Args = oldArgs }()

	main()

	// Output:
	// hello filesystem
}
