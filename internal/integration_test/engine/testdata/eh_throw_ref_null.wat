;; Test that throw_ref on a null exnref traps as "null reference" not "unreachable".
(module
  (func (export "throw_ref_null") (param exnref)
    local.get 0
    throw_ref
  )
)
