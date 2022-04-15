package age_calculator

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/tetratelabs/wazero"
)

// main shows how to define, import and call a Go-defined function from a
// WebAssembly-defined function.
//
// See README.md for a full description.
func main() {
	r := wazero.NewRuntime()

	// Instantiate a module named "env" that exports functions to get the
	// current year and log to the console.
	//
	// Note: As noted on ExportFunction documentation, function signatures are
	// constrained to a subset of numeric types.
	// Note: "env" is a module name conventionally used for arbitrary
	// host-defined functions, but any name would do.
	env, err := r.NewModuleBuilder("env").
		ExportFunction("log_i32", func(v uint32) {
			fmt.Println("log_i32 >>", v)
		}).
		ExportFunction("current_year", func() uint32 {
			if envYear, err := strconv.ParseUint(os.Getenv("CURRENT_YEAR"), 10, 64); err == nil {
				return uint32(envYear) // Allow env-override to prevent annual test maintenance!
			}
			return uint32(time.Now().Year())
		}).
		Instantiate()
	if err != nil {
		log.Fatal(err)
	}
	defer env.Close()

	// Instantiate a module named "age-calculator" that imports functions
	// defined in "env".
	//
	// Note: The import syntax in both Text and Binary format is the same
	// regardless of if the function was defined in Go or WebAssembly.
	ageCalculator, err := r.InstantiateModuleFromCode([]byte(`
;; Define the optional module name. '$' prefixing is a part of the text format.
(module $age-calculator

  ;; In WebAssembly, you don't import an entire module, rather each function.
  ;; This imports the functions and gives them names which are easier to read
  ;; than the alternative (zero-based index).
  ;;
  ;; Note: Importing unused functions is not an error in WebAssembly.
  (import "env" "log_i32" (func $log (param i32)))
  (import "env" "current_year" (func $year (result i32)))

  ;; get_age looks up the current year and subtracts the input from it.
  ;; Note: The stack begins empty and anything left must match the result type.
  (func $get_age (param $year_born i32) (result i32)
                 ;; stack: []
    call $year   ;; stack: [$year.result]
    local.get 0  ;; stack: [$year.result, $year_born]
    i32.sub      ;; stack: [$year.result-$year_born]
  )
  ;; export allows api.Module to return this via ExportedFunction("get_age")
  (export "get_age" (func $get_age))

  ;; log_age
  (func $log_age (param $year_born i32)
	              ;; stack: []
    local.get 0   ;; stack: [$year_born]
    call $get_age ;; stack: [$get_age.result]
    call $log     ;; stack: []
  )
  (export "log_age" (func $log_age))
)`))
	// ^^ Note: wazero's text compiler is incomplete #59. We are using it anyway to keep this example dependency free.
	if err != nil {
		log.Fatal(err)
	}
	defer ageCalculator.Close()

	// Read the birthYear from the arguments to main
	birthYear, err := strconv.ParseUint(os.Args[1], 10, 64)
	if err != nil {
		log.Fatalf("invalid arg %v: %v", os.Args[1], err)
	}

	// First, try calling the "get_age" function and printing to the console externally.
	results, err := ageCalculator.ExportedFunction("get_age").Call(nil, birthYear)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("println >>", results[0])

	// First, try calling the "log_age" function and printing to the console externally.
	_, err = ageCalculator.ExportedFunction("log_age").Call(nil, birthYear)
	if err != nil {
		log.Fatal(err)
	}
}
