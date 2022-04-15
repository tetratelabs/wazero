package add

import (
	"os"
)

// Example_main ensures the following will work:
//
//	go run add.go 7 9
func Example_main() {

	// Save the old os.Args and replace with our example input.
	oldArgs := os.Args
	os.Args = []string{"add", "7", "9"}
	defer func() { os.Args = oldArgs }()

	main()

	// Output:
	// wasm/math: 7 + 9 = 16
	// host/math: 7 + 9 = 16
}
