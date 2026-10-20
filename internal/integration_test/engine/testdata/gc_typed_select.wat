;; Minimal repro: typed select with a GC concrete ref type result.
;; The inline type annotation (ref null $point) is multi-byte in the
;; binary encoding (0x63 + LEB128 type index), which caused the
;; interpreter's compiler to desynchronize its PC.
(module
  (type $point (struct (field i32) (field i32)))

  (func (export "select_ref") (param $a (ref $point)) (param $b (ref null $point)) (result i32)
    (struct.get $point 0
      (select (result (ref null $point))
        (local.get $a)
        (local.get $b)
        (ref.is_null (local.get $b))))
  )
)
