package age_calculator

import "os"

// Example_main ensures the following will work:
//
//	go run age-calculator.go 2000
func Example_main() {

	// Save the old os.Args and replace with our example input.
	oldArgs := os.Args
	_ = os.Setenv("CURRENT_YEAR", "2021")
	os.Args = []string{"age-calculator", "2000"}
	defer func() {
		os.Args = oldArgs
		_ = os.Unsetenv("CURRENT_YEAR")
	}()

	main()

	// Output:
	// println >> 21
	// log_i32 >> 21
}
