(module
  (func (export "main")
    (param f32 v128 i32 f64 i64)
    (result
      ;; 180 results (in terms of uint64 representations) which use up all all the possible unreserved registers
      ;; of both general purpose and vector types on the function return.
      v128 f64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 20
      v128 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 40
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 60
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 80
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 f64 v128 ;; 100
      v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 ;; 120
      v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 ;; 140
      v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 ;; 160
      v128 v128 v128 v128 v128 v128 v128 v128 v128 f64 f64 ;; 180
    )
    (v128.const i64x2 1 2)
    (f64.const 1.5e-323) ;; math.Float64frombits(3)
    (i64.const 4)(i64.const 5)(i64.const 6)(i64.const 7)(i64.const 8)(i64.const 9)(i64.const 10)
    (i64.const 11)(i64.const 12)(i64.const 13)(i64.const 14)(i64.const 15)(i64.const 16)(i64.const 17)(i64.const 18)(i64.const 19)(i64.const 20)
    (v128.const i64x2 21 22) (i64.const 23)(i64.const 24)(i64.const 25)(i64.const 26)(i64.const 27)(i64.const 28)(i64.const 29)(i64.const 30)
    (i64.const 31)(i64.const 32)(i64.const 33)(i64.const 34)(i64.const 35)(i64.const 36)(i64.const 37)(i64.const 38)(i64.const 39)(i64.const 40)
    (i64.const 41)(i64.const 42)(i64.const 43)(i64.const 44)(i64.const 45)(i64.const 46)(i64.const 47)(i64.const 48)(i64.const 49)(i64.const 50)
    (i64.const 51)(i64.const 52)(i64.const 53)(i64.const 54)(i64.const 55)(i64.const 56)(i64.const 57)(i64.const 58)(i64.const 59)(i64.const 60)
    (i64.const 61)(i64.const 62)(i64.const 63)(i64.const 64)(i64.const 65)(i64.const 66)(i64.const 67)(i64.const 68)(i64.const 69)(i64.const 70)
    (i64.const 71)(i64.const 72)(i64.const 73)(i64.const 74)(i64.const 75)(i64.const 76)(i64.const 77)(i64.const 78)(i64.const 79)(i64.const 80)
    (i64.const 81)(i64.const 82)(i64.const 83)(i64.const 84)(i64.const 85)(i64.const 86)(i64.const 87)(i64.const 88)(i64.const 89)(i64.const 90)
    (i64.const 91)(i64.const 92)(i64.const 93)(i64.const 94)(i64.const 95)(i64.const 96)(i64.const 97)
    (f64.const 4.84e-322) ;; math.Float64frombits(98)
    (v128.const i64x2 99 100)
    (v128.const i64x2 101 102) (v128.const i64x2 103 104) (v128.const i64x2 105 106) (v128.const i64x2 107 108) (v128.const i64x2 109 110)
    (v128.const i64x2 111 112) (v128.const i64x2 113 114) (v128.const i64x2 115 116) (v128.const i64x2 117 118) (v128.const i64x2 119 120)
    (v128.const i64x2 121 122) (v128.const i64x2 123 124) (v128.const i64x2 125 126) (v128.const i64x2 127 128) (v128.const i64x2 129 130)
    (v128.const i64x2 131 132) (v128.const i64x2 133 134) (v128.const i64x2 135 136) (v128.const i64x2 137 138) (v128.const i64x2 139 140)
    (v128.const i64x2 141 142) (v128.const i64x2 143 144) (v128.const i64x2 145 146) (v128.const i64x2 147 148) (v128.const i64x2 149 150)
    (v128.const i64x2 151 152) (v128.const i64x2 153 154) (v128.const i64x2 155 156) (v128.const i64x2 157 158) (v128.const i64x2 159 160)
    (v128.const i64x2 161 162) (v128.const i64x2 163 164) (v128.const i64x2 165 166) (v128.const i64x2 167 168) (v128.const i64x2 169 170)
    (v128.const i64x2 171 172) (v128.const i64x2 173 174) (v128.const i64x2 175 176) (v128.const i64x2 177 178)
    (f64.const 8.84e-322) ;; math.Float64frombits(179)
    (f64.const 8.9e-322) ;; math.Float64frombits(180)
  )
  (func (export "memory_fill_after_main") (param i32 i32 i32)
    (result
      ;; 180 results (in terms of uint64 representations) which use up all all the possible unreserved registers
      ;; of both general purpose and vector types on the function return.
      v128 f64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 20
      v128 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 40
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 60
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 80
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 f64 v128 ;; 100
      v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 ;; 120
      v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 ;; 140
      v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 ;; 160
      v128 v128 v128 v128 v128 v128 v128 v128 v128 f64 f64 ;; 180
    )
    f32.const 0
    v128.const i64x2 0 0
    i32.const 0
    f64.const 0
    i64.const 0
    call 0
    ;; Call memory.fill with params. memory.fill/copy internally calls the Go runtime's runtime.memmove,
    ;; which has a slightly tricky calling convention. This ensures that across the call to memory.fill
    ;; the registers are preserved. https://github.com/tetratelabs/wazero/pull/2202
    local.get 0
    local.get 1
    local.get 2
    memory.fill
  )
  (memory 1)
)
