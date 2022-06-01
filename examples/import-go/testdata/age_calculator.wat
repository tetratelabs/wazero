(module $age-calculator

  ;; In WebAssembly, you don't import an entire module, rather each function.
  ;; This imports the functions and gives them names which are easier to read
  ;; than the alternative (zero-based index).
  ;;
  ;; Note: Importing unused functions is not an error in WebAssembly.
  (import "env" "log_i32" (func $log (param i32)))
  (import "env" "current_year" (func $year (result i32)))

  ;; get_age looks up the current year and subtracts the input from it.
  (func $get_age (export "get_age") (param $year_born i32) (result i32)
    call $year            ;; stack: [$year.result]
    local.get $year_born  ;; stack: [$year.result, $year_born]
    i32.sub               ;; stack: [$year.result-$year_born]
  )

  ;; log_age calls $log with the result of $get_age
  (func (export "log_age") (param $year_born i32)
	                       ;; stack: []
    local.get $year_born   ;; stack: [$year_born]
    call $get_age          ;; stack: [$get_age.result]
    call $log              ;; stack: []
  )
)
