(module
  (type (;0;) (func (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)))
  (type (;1;) (func (result f64 f64 f32 f64 f64 f32 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)))
  (type (;2;) (func (param i32)))
  (func (;0;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    (local f64 f64)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;1;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    (local i32)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;2;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    (local f64 i32)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    local.get 1
    unreachable
  )
  (func (;3;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    (local f64)
;;    global.get 3
;;    i32.eqz
;;    if  ;; label = @1
;;      unreachable
;;    end
;;    global.get 3
;;    i32.const 1
;;    i32.sub
;;    global.set 3
    v128.const i32x4 0xa3a3a3a3 0xa3a3a3a3 0xffff25ff 0xffffffff
    v128.const i32x4 0xffffff7f 0xffffffff 0xffffffff 0xffffffff
    v128.const i32x4 0xffff01ff 0xffffffff 0xffffffff 0xffffffff
    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
    v128.const i32x4 0xffffffff 0x00000035 0xffffffff 0xffffffff
    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
    v128.const i32x4 0xffffffff 0xffffffff 0x0000d30e 0xf7010000
    v128.const i32x4 0x0000ffff 0x0000db00 0xffffffd0 0x0000d0d0
    v128.const i32x4 0x00000001 0x000002ff 0x00000000 0xffffff00
    v128.const i32x4 0xffffffff 0xffffffff 0xf3f3ff2e 0x000000f3
    v128.const i32x4 0x00000000 0xf3f3f300 0xf3ffffff 0xff01f3f3
    v128.const i32x4 0x00ffffff 0xffff0000 0xffffffff 0xccffffff
  )
  (func (;4;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;5;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    data.drop 0
    call 0
  )
  (func (;6;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;7;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    (local f64)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;8;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    (local i32)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;9;)
    (local f64 f64 i32)
;;    global.get 3
;;    i32.eqz
;;    if  ;; label = @1
;;      unreachable
;;    end
;;    global.get 3
;;    i32.const 1
;;    i32.sub
;;    global.set 3
;;    local.get 0
    ;;call 3

;;        v128.const i32x4 0xa3a3a3a3 0xa3a3a3a3 0xffff25ff 0xffffffff
;;        v128.const i32x4 0xffffff7f 0xffffffff 0xffffffff 0xffffffff
        v128.const i32x4 0xffff01ff 0xffffffff 0xffffffff 0xffffffff
        v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
        v128.const i32x4 0xffffffff 0x00000035 0xffffffff 0xffffffff
        v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
        v128.const i32x4 0xffffffff 0xffffffff 0x0000d30e 0xf7010000
        v128.const i32x4 0x0000ffff 0x0000db00 0xffffffd0 0x0000d0d0
;;        v128.const i32x4 0x00000001 0x000002ff 0x00000000 0xffffff00
;;        v128.const i32x4 0xffffffff 0xffffffff 0xf3f3ff2e 0x000000f3
;;    v128.const i32x4 0x11111111 0x11111111 0x00000000 0x00000000
;;    i64x2.extmul_low_i32x4_u
;;    i64x2.extmul_low_i32x4_u
;;    i64x2.extmul_low_i32x4_u
    i64x2.extmul_low_i32x4_u
    i32x4.lt_s
    i64x2.extmul_low_i32x4_u
    i64x2.extmul_low_i32x4_u
    i64x2.gt_s
    i16x8.all_true
    drop

    i32.const 0

    if (result f64 f64 f32 f64 f64 f32 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)  ;; label = @1
      f64.const -nan:0xfffffffffffff (;=NaN;)
       call 3

;;          v128.const i32x4 0xa3a3a3a3 0xa3a3a3a3 0xffff25ff 0xffffffff
;;          v128.const i32x4 0xffffff7f 0xffffffff 0xffffffff 0xffffffff
;;          v128.const i32x4 0xffff01ff 0xffffffff 0xffffffff 0xffffffff
;;          v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
;;          v128.const i32x4 0xffffffff 0x00000035 0xffffffff 0xffffffff
;;          v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
;;          v128.const i32x4 0xffffffff 0xffffffff 0x0000d30e 0xf7010000
;;          v128.const i32x4 0x0000ffff 0x0000db00 0xffffffd0 0x0000d0d0
;;          v128.const i32x4 0x00000001 0x000002ff 0x00000000 0xffffff00
;;          v128.const i32x4 0xffffffff 0xffffffff 0xf3f3ff2e 0x000000f3
;;          v128.const i32x4 0x00000000 0xf3f3f300 0xf3ffffff 0xff01f3f3
;;          v128.const i32x4 0x00ffffff 0xffff0000 0xffffffff 0xccffffff


      ;;

      f64x2.lt
      i64x2.extmul_low_i32x4_u
      i64x2.extmul_low_i32x4_u
      i64x2.extmul_low_i32x4_u
      i64x2.extmul_low_i32x4_u
      i64x2.extmul_low_i32x4_u
      i8x16.ge_u
      i8x16.ge_u
      i8x16.ge_u
      drop
      drop
      drop
      f64.const -nan:0xfffffd00000db (;=NaN;)
      f32.const -nan:0x7fffff (;=NaN;)
      f64.const -0x1.cccffffffffffp+205 (;=-92562142190011980000000000000000000000000000000000000000000000;)
      f64.const 0x1.4ffffe666p-1037 (;=0.000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000891238167443;)
      f32.const -0x1.0012p+127 (;=-170187910000000000000000000000000000000;)
      f64.const 0x1.0d0ffff0000dbp-962 (;=0.000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000026962327372680765;)
      f64.const -0x1p+1009 (;=-5486124068793689000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.fffffffff2effp+832 (;=-57277807836609680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3f3f3f3f3f3p+832 (;=-35714397827745240000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3f3f3f3f3f3p+832 (;=-35714397827745240000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3f3f3f3f3f3p+832 (;=-35714397827745240000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -nan:0xfffffffffffff (;=NaN;)
      f64.const -nan:0xf0e0e0e0e0eff (;=NaN;)
      f64.const -nan:0xfffffffffffff (;=NaN;)
      f64.const -nan:0xfccccccccccff (;=NaN;)
      f64.const -0x1.0090000000029p+1009 (;=-5498178540624584000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const 0x1.0d0ffff0000dbp-962 (;=0.000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000026962327372680765;)
      f64.const -0x1p+1009 (;=-5486124068793689000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.fffffffff2effp+832 (;=-57277807836609680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
    else
      f64.const -0x1.3f3f3f3f3f3f3p+832 (;=-35714397827745240000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3f3f3f3f3f3p+832 (;=-35714397827745240000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f32.const -0x1.e7e7e6p+104 (;=-38655884000000000000000000000000;)
      f64.const 0x1.a61dffe7e6p-1035 (;=0.000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004478654460604;)
      f64.const 0x1.ffffee02p-1035 (;=0.00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000543230631202;)
      f32.const -nan:0x7701db (;=NaN;)
      f64.const -0x1.000db000000ffp+257 (;=-231632545913043800000000000000000000000000000000000000000000000000000000000000;)
      f64.const 0x1.000d0d0ffffffp-1007 (;=0.0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000007292573994327566;)
      f64.const 0x1.2b0fffp-1026 (;=0.00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000162459989467255;)
      f64.const -nan:0xfffffffffff00 (;=NaN;)
      f64.const -0x1.effffffffffffp+1011 (;=-42517461533151080000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3f3ffffffffp+832 (;=-35714399113024630000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3f3f3f3f3f3p+832 (;=-35714397827745240000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -0x1.3f3fffffff3f3p+832 (;=-35714726859250427000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const -nan:0xfffffffffff01 (;=NaN;)
      f64.const -nan:0xfffffffffffff (;=NaN;)
      f64.const 0x1.000dbccccccffp-783 (;=0.00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001966094649608921;)
      f64.const -nan:0xfff0000012b00 (;=NaN;)
      f64.const -nan:0xfffffffffffff (;=NaN;)
      f64.const -nan:0xfffffff2effff (;=NaN;)
    end
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i32.reinterpret_f32
    global.get 1
    i32.xor
    global.set 1
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0

    i32.reinterpret_f32
    global.get 1
    i32.xor
    global.set 1
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0




    i64.reinterpret_f64
;;    global.get 0
;;    i64.xor
    global.set 0 ;; last failure

;;    unreachable

;;drop drop drop

;;
;;    global.get 2
;;    v128.xor
;;    global.set 2
;;    global.get 2
;;    v128.xor
;;    global.set 2
;;    i64.reinterpret_f64
;;    global.get 0
;;    i64.xor
;;    global.set 0
;;    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xcccccccc
;;    v128.const i32x4 0x29ffffcc 0x00000000 0xdbff0009 0xffff0000
;;    v128.const i32x4 0x0003d0d0 0x00000000 0xffff0000 0xffffff2e
;;    v128.const i32x4 0xf3f3ffff 0xf3f3f3f3 0xf3f3f3f3 0xf3f3f3f3
;;    v128.const i32x4 0xf3f3f3f3 0xf3f3f3f3 0xf3f3f3f3 0xf3f3f3f3
;;    v128.const i32x4 0xf3f3f3f3 0xf3f3f3f3 0xf3ffffff 0xff01f3f3
;;    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xccffffff
;;    v128.const i32x4 0xcccccccc 0x0029ffff 0x09000000 0x00dbff00
;;    v128.const i32x4 0xd0ffff00 0x000003d0 0x00000000 0x2effff00
;;    v128.const i32x4 0xffffffff 0xf3f3f3ff 0xf3f3f3f3 0xf3f3f3f3
;;    v128.const i32x4 0xf3f3f3f3 0xf3f3f3f3 0xf3f3f3f3 0x29fff3f3
;;    v128.const i32x4 0x00000000 0xdbff0009 0xffff0000 0x0003d0d0
  )
  (func (;10;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (func (;11;) (type 0) (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    global.get 3
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 3
    i32.const 1
    i32.sub
    global.set 3
    unreachable
  )
  (table (;0;) 1000 1000 externref)
  (table (;1;) 827 828 funcref)
  (memory (;0;) 0 3)
  (global (;0;) (mut i64) i64.const 0)
  (global (;1;) (mut i32) i32.const 0)
  (global (;2;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;3;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (func 1))
  (export "2" (func 2))
  (export "3" (func 3))
  (export "4" (func 4))
  (export "5" (func 5))
  (export "6" (func 6))
  (export "7" (func 7))
  (export "8" (func 8))
  (export "9" (func 9))
  (export "10" (func 10))
  (export "11" (func 11))
  (export "12" (table 0))
  (export "13" (table 1))
  (export "14" (memory 0))
  (export "15" (global 0))
  (export "16" (global 1))
  (export "17" (global 2))
  (elem (;0;) (i32.const 0) externref)
  (data (;0;) "\ff")
)
