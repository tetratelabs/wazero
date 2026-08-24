;; On amd64, a float select lowers to `movsd y, tmp` + `xmmCMov cond, x, tmp`.
;; When the condition does not hold, tmp := y. If the tmp is declared to the
;; register allocator as a def only, it may be evicted. See #2534.
(module
  (func $g (param f64 f64 f64) (result f64)
    (f64.add (f64.add (local.get 0) (local.get 1)) (local.get 2)))

  (func (export "f") (param i32) (result f64)
    (local f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)
    (local.set 1 (f64.add (f64.mul (f64.convert_i32_s (local.get 0)) (f64.const 1.5)) (f64.const 1.25)))
    (local.set 2 (f64.add (f64.mul (f64.convert_i32_s (local.get 0)) (f64.const 3.5)) (f64.const 15.25)))
    (local.set 11 (select (local.get 1) (local.get 8) (i32.eq (i32.rem_s (local.get 0) (i32.const 3)) (i32.const 0))))
    (local.set 9  (call $g (local.get 3) (local.get 4) (local.get 8)))
    (local.set 7  (select (local.get 2) (local.get 11) (i32.and (local.get 0) (i32.const 2))))
    (local.set 6  (call $g (local.get 5) (local.get 2) (local.get 10)))
    (f64.add (local.get 1) (local.get 7))))
