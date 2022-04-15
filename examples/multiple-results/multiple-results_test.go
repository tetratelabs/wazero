package multiple_results

// Example_main ensures the following will work:
//
//	go run multiple-results.go
func Example_main() {

	main()

	// Output:
	// result-offset/wasm: age=37
	// result-offset/host: age=37
	// multi-value/wasm: age=37
	// multi-value/host: age=37
}
