(module
  (type (;0;) (func (param i32 i32 i32 i32) (result i32)))
  (type (;1;) (func (param i32)))
  (type (;2;) (func (param i32 i32)))
  (import "repro" "update_nonce" (func (;0;) (type 1)))
  (memory (;0;) 17)
  (global (;0;) (mut i32) i32.const 0)
  (export "__stack_pointer" (global 0))
  (export "fill_blocks" (func 3))
  (func (;1;) (type 2) (param i32 i32)
    unreachable
  )
  (func (;2;) (type 0) (param i32 i32 i32 i32) (result i32)
    i32.const 18
  )
  (func (;3;) (type 0) (param i32 i32 i32 i32) (result i32)
    (local i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64)
    global.get 0
    local.tee 4
    local.set 26
    local.get 4
    global.set 0
    local.get 0
    i32.load offset=16
    local.tee 24
    i32.const 2
    i32.shl
    local.tee 4
    i32.eqz
    i32.eqz
    if ;; label = @1
      nop
      i32.const 6
      local.set 25
      block ;; label = @2
        block ;; label = @3
          block ;; label = @4
            block ;; label = @5
              local.get 0
              i32.load offset=8
              local.tee 19
              local.get 24
              i32.const 3
              i32.shl
              local.tee 7
              local.get 19
              local.get 7
              i32.gt_u
              select
              local.get 4
              i32.div_u
              local.tee 8
              local.get 4
              i32.mul
              local.tee 21
              local.get 2
              i32.gt_u
              local.tee 6
              br_if 0 (;@5;)
              local.get 8
              i32.const 2
              i32.shl
              local.tee 20
              i32.eqz
              br_if 3 (;@2;)
              local.get 0
              i32.load offset=12
              local.set 9
              local.get 20
              local.get 21
              local.get 21
              local.get 20
              i32.rem_u
              i32.sub
              local.tee 23
              i32.gt_u
              i32.eqz
              if ;; label = @6
                nop
                local.get 20
                i32.const 10
                i32.shl
                local.set 10
                i32.const 0
                local.set 11
                local.get 1
                local.set 5
                loop ;; label = @7
                  local.get 11
                  local.tee 14
                  i32.const 1
                  i32.add
                  local.set 11
                  local.get 23
                  local.get 20
                  i32.sub
                  local.set 23
                  local.get 5
                  local.get 10
                  i32.add
                  local.set 5
                  i32.const 0
                  local.set 22
                  i32.const 0
                  local.set 4
                  loop ;; label = @8
                    local.get 17
                    i32.const 4
                    i32.store offset=1108
                    local.get 17
                    i32.const 4
                    i32.store offset=1100
                    local.get 17
                    i32.const 64
                    i32.store offset=1092
                    local.get 17
                    local.get 3
                    i32.store offset=1088
                    local.get 17
                    local.get 22
                    i32.store offset=60
                    local.get 17
                    local.get 14
                    i32.store offset=64
                    local.get 17
                    local.get 17
                    i32.const 64
                    i32.add
                    i32.store offset=1104
                    local.get 17
                    local.get 17
                    i32.const 60
                    i32.add
                    i32.store offset=1096
                    local.get 17
                    i32.const 2112
                    i32.add
                    i32.const 0
                    i32.const 1
                    memory.fill
                    local.get 17
                    i32.const 1088
                    i32.add
                    i32.const 3
                    local.get 17
                    i32.const 2112
                    i32.add
                    i32.const 1024
                    call 2
                    local.tee 25
                    i32.const 255
                    i32.and
                    i32.const 18
                    i32.ne
                    br_if 3 (;@5;)
                    local.get 4
                    i32.const 1024
                    i32.add
                    local.set 12
                    local.get 22
                    i32.const 1
                    i32.add
                    local.set 22
                    local.get 17
                    i32.const 2112
                    i32.add
                    local.set 2
                    i32.const 1024
                    local.set 4
                    i32.const 0
                    local.set 19
                    i32.const 129
                    local.set 7
                    loop ;; label = @9
                      local.get 4
                      i32.const 7
                      i32.le_u
                      br_if 6 (;@3;)
                      local.get 7
                      i32.const -1
                      i32.add
                      local.tee 7
                      i32.eqz
                      br_if 5 (;@4;)
                      local.get 19
                      local.get 2
                      i64.load align=1
                      i64.store
                      local.get 19
                      i32.const 1
                      i32.add
                      local.set 19
                      local.get 2
                      local.get 4
                      i32.const 8
                      local.get 4
                      i32.const 8
                      i32.lt_u
                      select
                      local.tee 18
                      i32.add
                      local.set 2
                      local.get 4
                      local.get 18
                      i32.sub
                      local.tee 4
                      br_if 0 (;@9;)
                    end
                    local.get 12
                    local.tee 4
                    local.get 4
                    i32.ne
                    br_if 0 (;@8;)
                  end
                  local.get 20
                  local.get 23
                  i32.le_u
                  br_if 0 (;@7;)
                end
              end
              i32.const 18
              local.set 25
              local.get 9
              i32.eqz
              br_if 0 (;@5;)
              i32.const 0
              local.get 1
              local.get 6
              select
              local.set 15
              local.get 8
              i32.const 3
              i32.mul
              local.tee 23
              i32.const -1
              i32.add
              local.set 11
              local.get 0
              i32.load8_u offset=80
              local.tee 4
              i64.extend_i32_u
              i64.const 3
              i64.and
              local.set 38
              i64.const 1
              local.set 39
              local.get 9
              i64.extend_i32_u
              local.set 36
              local.get 21
              i64.extend_i32_u
              local.set 40
              i64.const 0
              local.set 31
              local.get 0
              i32.load offset=68
              i32.const 16
              i32.eq
              local.set 27
              local.get 4
              local.set 16
              loop ;; label = @6
                local.get 31
                local.tee 32
                i64.const 1
                i64.add
                local.set 31
                local.get 27
                local.get 32
                i64.eqz
                local.tee 12
                i32.or
                local.set 14
                local.get 16
                local.get 12
                i32.and
                local.set 28
                i64.const 0
                local.set 33
                loop ;; label = @7
                  local.get 33
                  local.set 30
                  local.get 17
                  i32.const 1
                  i32.eq
                  if (result i32) ;; label = @8
                    i32.const 1
                  else
                    nop
                    local.get 28
                  end
                  local.set 22
                  local.get 30
                  i64.const 1
                  i64.add
                  local.set 33
                  local.get 24
                  i32.eqz
                  i32.eqz
                  if ;; label = @8
                    nop
                    local.get 30
                    i64.eqz
                    local.set 29
                    local.get 8
                    local.set 3
                    local.get 8
                    local.get 30
                    i32.wrap_i64
                    i32.mul
                    local.set 10
                    local.get 30
                    local.get 32
                    i64.or
                    i64.const 4294967295
                    i64.and
                    local.set 37
                    i64.const 0
                    local.set 34
                    loop ;; label = @9
                      local.get 17
                      i32.const 64
                      i32.add
                      i32.const 0
                      i32.const 1024
                      memory.fill
                      local.get 17
                      i32.const 1088
                      i32.add
                      i32.const 0
                      i32.const 1024
                      memory.fill
                      local.get 17
                      i32.const 2112
                      i32.add
                      i32.const 0
                      i32.const 1024
                      memory.fill
                      block (result i32) ;; label = @10
                        block ;; label = @11
                          block ;; label = @12
                            local.get 22
                            i32.eqz
                            i32.eqz
                            if ;; label = @13
                              nop
                              local.get 17
                              local.get 38
                              i64.store offset=1128
                              local.get 17
                              local.get 36
                              i64.store offset=1120
                              local.get 17
                              local.get 40
                              i64.store offset=1112
                              local.get 17
                              local.get 30
                              i64.store offset=1104
                              local.get 17
                              local.get 34
                              i64.store offset=1096
                              local.get 17
                              local.get 32
                              i64.store offset=1088
                              local.get 37
                              i64.eqz
                              i32.eqz
                              br_if 1 (;@12;)
                              br 2 (;@11;)
                            end
                            local.get 37
                            i64.eqz
                            br_if 1 (;@11;)
                          end
                          local.get 20
                          local.get 34
                          i32.wrap_i64
                          i32.mul
                          local.get 10
                          i32.add
                          local.tee 7
                          local.get 29
                          i32.add
                          local.set 4
                          i32.const 0
                          local.set 18
                          local.get 10
                          local.set 6
                          local.get 17
                          br 1 (;@10;)
                        end
                        i32.const 2
                        local.set 18
                        local.get 20
                        local.get 34
                        i32.wrap_i64
                        i32.mul
                        i32.const 2
                        i32.or
                        local.tee 7
                        local.set 4
                        i32.const 1
                      end
                      local.set 0
                      local.get 18
                      local.get 8
                      i32.ge_u
                      i32.eqz
                      if ;; label = @10
                        nop
                        local.get 6
                        local.set 9
                        local.get 4
                        i32.const -1
                        i32.add
                        local.set 4
                        local.get 1
                        local.get 7
                        i32.const 10
                        i32.shl
                        i32.add
                        local.set 19
                        local.get 34
                        i32.wrap_i64
                        local.set 5
                        loop ;; label = @11
                          block ;; label = @12
                            block ;; label = @13
                              local.get 22
                              i32.eqz
                              if ;; label = @14
                                nop
                                local.get 4
                                local.get 21
                                i32.ge_u
                                br_if 1 (;@13;)
                                local.get 15
                                local.get 4
                                i32.const 10
                                i32.shl
                                i32.add
                                local.set 2
                                br 2 (;@12;)
                              end
                              block ;; label = @14
                                local.get 18
                                i32.const 127
                                i32.and
                                local.tee 2
                                br_if 0 (;@14;)
                              end
                              local.get 17
                              i32.const 64
                              i32.add
                              local.get 2
                              i32.const 3
                              i32.shl
                              i32.add
                              local.set 2
                              br 1 (;@12;)
                            end
                            unreachable
                          end
                          local.get 2
                          i64.load
                          local.set 35
                          block (result i32) ;; label = @12
                            local.get 12
                            i32.eqz
                            i32.eqz
                            if ;; label = @13
                              nop
                              local.get 0
                              i32.eqz
                              i32.eqz
                              if ;; label = @14
                                nop
                                local.get 5
                                local.set 13
                                local.get 18
                                i32.const -1
                                i32.add
                                br 2 (;@12;)
                              end
                              local.get 34
                              local.get 35
                              i64.const 32
                              i64.shr_u
                              i32.wrap_i64
                              local.get 24
                              i32.rem_u
                              local.tee 13
                              i64.extend_i32_u
                              i64.eq
                              i32.eqz
                              if ;; label = @14
                                nop
                                local.get 6
                                local.get 18
                                i32.eqz
                                i32.sub
                                br 2 (;@12;)
                              end
                              local.get 9
                              local.get 18
                              i32.add
                              br 1 (;@12;)
                            end
                            local.get 34
                            local.get 35
                            i64.const 32
                            i64.shr_u
                            i32.wrap_i64
                            local.get 24
                            i32.rem_u
                            local.tee 13
                            i64.extend_i32_u
                            i64.eq
                            i32.eqz
                            if ;; label = @13
                              nop
                              local.get 23
                              local.get 18
                              i32.eqz
                              i32.sub
                              br 1 (;@12;)
                            end
                            local.get 11
                            local.get 18
                            i32.add
                          end
                          local.tee 2
                          local.get 3
                          i32.add
                          local.get 35
                          i64.const 4294967295
                          i64.and
                          local.tee 35
                          local.get 35
                          i64.mul
                          i64.const 32
                          i64.shr_u
                          local.get 2
                          i64.extend_i32_u
                          i64.mul
                          i64.const 32
                          i64.shr_u
                          i32.wrap_i64
                          i32.const -1
                          i32.xor
                          i32.add
                          local.get 20
                          i32.rem_u
                          local.set 2
                          block ;; label = @12
                            block ;; label = @13
                              block ;; label = @14
                                block ;; label = @15
                                  local.get 4
                                  local.get 21
                                  i32.ge_u
                                  i32.eqz
                                  if ;; label = @16
                                    nop
                                    local.get 2
                                    local.get 13
                                    local.get 20
                                    i32.mul
                                    i32.add
                                    local.get 21
                                    i32.ge_u
                                    br_if 1 (;@15;)
                                    local.get 14
                                    i32.eqz
                                    if ;; label = @17
                                      nop
                                      local.get 7
                                      local.get 21
                                      i32.ge_u
                                      br_if 3 (;@14;)
                                      i32.const 0
                                      local.set 4
                                      loop ;; label = @18
                                        local.get 19
                                        local.get 4
                                        i32.add
                                        local.tee 2
                                        local.get 2
                                        i64.load
                                        local.get 17
                                        i32.const 3136
                                        i32.add
                                        local.get 4
                                        i32.add
                                        i64.load
                                        i64.xor
                                        i64.store
                                        local.get 4
                                        i32.const 8
                                        i32.add
                                        local.tee 4
                                        i32.const 1024
                                        i32.ne
                                        br_if 0 (;@18;)
                                      end
                                      br 5 (;@12;)
                                    end
                                    local.get 7
                                    local.get 21
                                    i32.lt_u
                                    br_if 3 (;@13;)
                                    unreachable
                                  end
                                  unreachable
                                end
                                unreachable
                              end
                              unreachable
                            end
                            local.get 1
                            local.get 7
                            i32.const 10
                            i32.shl
                            i32.add
                            local.get 17
                            i32.const 3136
                            i32.add
                            i32.const 1024
                            memory.copy
                          end
                          local.get 19
                          i32.const 0
                          i32.add
                          local.set 19
                          local.get 7
                          local.tee 4
                          i32.const 1
                          i32.add
                          local.set 7
                          local.get 18
                          i32.const 1
                          i32.add
                          local.tee 18
                          local.get 8
                          i32.lt_u
                          br_if 0 (;@11;)
                        end
                      end
                      local.get 34
                      i64.const 1
                      i64.add
                      local.tee 34
                      local.get 39
                      i64.ne
                      br_if 0 (;@9;)
                    end
                  end
                  local.get 33
                  i64.const 4
                  i64.ne
                  br_if 0 (;@7;)
                end
                local.get 31
                local.get 36
                i64.ne
                br_if 0 (;@6;)
              end
            end
            local.get 26
            global.set 0
            local.get 25
            return
          end
          unreachable
        end
        unreachable
      end
      unreachable
    end
    unreachable
  )
)
