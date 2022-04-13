package main

// ExampleMain ensures the following will work:
//
//	go build multiple-results.go
//	./multiple-results
func ExampleMain() {

	main()

	// Output:
	// result-offset/wasm: age=37
	// result-offset/host: age=37
	// multi-value/wasm: age=37
	// multi-value/host: age=37
}
