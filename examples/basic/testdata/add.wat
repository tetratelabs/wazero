(module
  ;; Define the optional module name. '$' prefixing is a part of the text format.
  $wasm/math

  ;; add returns $x+$y.
  ;;
  ;; Notes:
  ;; * The stack begins empty and anything left must match the result type.
  ;; * export allows api.Module to return this via ExportedFunction("add")
  (func (export "add") (param $x i32) (param $y i32) (result i32)
    local.get $x ;; stack: [$x]
    local.get $y ;; stack: [$x, $y]
    i32.add      ;; stack: [$x+$y]
  )
)
