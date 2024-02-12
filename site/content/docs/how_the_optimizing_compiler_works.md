What is a JIT compiler?
=======================

In general, when we talk about a Just-In-Time (JIT) compiler, we mean a compilation technique that spares cycles at build-time, trading it for run-time. In other words, when a language is JIT-compiled, we usually mean that compilation will happen during run-time. Furthermore, when we use the term JIT-compilation, we also often mean is that, because compilation happens _during run-time_, we can use information that we have collected during execution to direct the compilation process: these types of JIT-compilers are often referred to as **tracing-JITs**.

Thus, if we wanted to be pedantic, **wazero** provides an **ahead-of-time**, **load-time** compiler. That is, a compiler that, indeed, performs compilation at run-time, but only when a WebAssembly module is loaded; it currently does not collect or leverage any information during the execution of the Wasm binary itself.

It is important to make such a distinction, because a Just-In-Time compiler may not be an optimizing compiler, and an optimizing compiler may not be a tracing JIT. In fact, the compiler that wazero shipped before the introduction of the new compiler architecture performed code generation at load-time, but did not perform any optimization.

# What is an Optimizing Compiler?

Wazero supports an _optimizing_ compiler in the style of other optimizing compilers out there, such as LLVM's or V8's. Traditionally an optimizing compiler performs compilation in a number of steps.

Compare this to the **old compiler**, where compilation happens in one step or two, depending on how you count:


```goat
            Input         +---------------+     +---------------+
         Wasm Binary ---->| DecodeModule  |---->| CompileModule |----> wazero IR
                          +---------------+     +---------------+
```

That is, the module is (1) validated then (2) translated to an Intermediate Representation (IR).
The wazero IR can then be executed directly (in the case of the interpreter) or it can be further processed and translated into native code by the compiler. This compiler performs a straightforward translation from the IR to native code, without any further passes. The wazero IR is not intended for further processing beyond immediate execution or straightforward translation.

```goat
                        +----   wazero IR    ----+
                        |                        |
                        v                        v
                +--------------+         +--------------+
                |   Compiler   |         | Interpreter  |- - -  executable
                +--------------+         +--------------+
                        |
             +----------+---------+
             |                    |
             v                    v
        +---------+          +---------+
        |  ARM64  |          |  AMD64  |
        | Backend |          | Backend |    - - - - - - - - -   executable
        +---------+          +---------+
```


Validation and translation to an IR in a compiler are usually called the **front-end** part of a compiler, while code-generation occurs in what we call the **back-end** of a compiler. The front-end is the part of a compiler that is closer to the input, and it generally indicates machine-independent processing, such as parsing and static validation. The back-end is the part of a compiler that is closer to the output, and it generally includes machine-specific procedures, such as code-generation.

In the **optimizing** compiler, we still decode and translate Wasm binaries to an intermediate representation in the front-end, but we use a textbook representation called an **SSA** or "Static Single-Assignment Form", that is intended for further transformation.

The benefit of choosing an IR that is meant for transformation is that a lot of optimization passes can apply directly to the IR, and thus be machine-independent. Then the back-end can be relatively simpler, in that it will only have to deal with machine-specific concerns.

The wazero optimizing compiler implements the following compilation passes:

* Front-End:
  - Translation to SSA
  - Optimization

* Back-End:
  - Instruction Selection
  - Registry Allocation
  - Finalization and Encoding

```goat
              Input          +-------------------+      +-------------------+
           Wasm Binary   --->|   DecodeModule    |----->|   CompileModule   |--+
                             +-------------------+      +-------------------+  |
                    +----------------------------------------------------------+
                    |
                    |  +---------------+            +---------------+
                    +->|   Front-End   |----------->|   Back-End    |
                       +---------------+            +---------------+
                               |                            |
                               v                            v
                              SSA                 Instruction Selection
                               |                            |
                               v                            v
                         Optimization              Registry Allocation
                                                            |
                                                            v
                                                  Finalization/Encoding
```

## Front-End: Translation to SSA

We mentioned earlier that wazero uses an internal representation called an "SSA" form or "Static Single-Assignment" form,
but we never explained what that is.

In short terms, every program, or, in our case, every Wasm function, can be translated in a control-flow graph.
The control-flow graph is a directed graph where each node is a sequence of statements that do not contain a control flow instruction,
called a **basic block**. Instead, control-flow instructions are translated into edges.

For instance, take the following implementation of the `abs` function:

```wasm
(module
  (func (;0;) (param i32) (result i32)
     (if (result i32) (i32.lt_s (local.get 0) (i32.const 0))
        (then
            (i32.sub (i32.const 0) (local.get 0)))
        (else
            (local.get 0))
     )
  )
  (export "f" (func 0))
)
```

This is translated to the following block diagram:

```goat
               +---------------------------------------------+
               |blk0: (exec_ctx:i64, module_ctx:i64, v2:i32) |
               |    v3:i32 = Iconst_32 0x0                   |
               |    v4:i32 = Icmp lt_s, v2, v3               |
               |    Brz v4, blk2                             |
               |    Jump blk1                                |
               +---------------------------------------------+
                                      |
                                      |
                      +---(v4 != 0)---+--(v4 == 0)----+
                      |                               |
                      v                               v
        +---------------------------+   +---------------------------+
        |blk1: () <-- (blk0)        |   |blk2: () <-- (blk0)        |
        |    v6:i32 = Iconst_32 0x0 |   |    Jump blk3, v2          |
        |    v7:i32 = Isub v6, v2   |   |                           |
        |    Jump blk3, v7          |   |                           |
        +---------------------------+   +---------------------------+
                      |                               |
                      |                               |
                      +-{v5 := v7}----+---{v5 := v2}--+
                                      |
                                      v
                      +------------------------------+
                      |blk3: (v5:i32) <-- (blk1,blk2)|
                      |    Jump blk_ret, v5          |
                      +------------------------------+
                                      |
                                 {return v5}
                                      |
                                      v
```

We use the ["block argument" variant of SSA][ssa-blocks], which is also the same representation [used in LLVM's MLIR][llvm-mlir]. In this variant, each block takes a list of arguments. Each block ends with a jump instruction with an optional list of arguments; these arguments, are assigned to the target block's arguments like a function.

Consider the first block `blk0`.

```
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
    v3:i32 = Iconst_32 0x0
    v4:i32 = Icmp lt_s, v2, v3
    Brz v4, blk2
    Jump blk1
```

You will notice that, compared to the original function, it takes two extra parameters (`exec_ctx` and `module_ctx`). It then takes one parameter `v2`, corresponding to the function parameter, and it defines two variables `v3`, `v4`. `v3` is the constant 0, `v4` is the result of comparing `v2` to `v3` using the `i32.lt_s` instruction. Then, it branches to `blk2` if `v4` is zero, otherwise it jumps to `blk1`.

You might also have noticed that the instructions do not correspond strictly to  the original Wasm opcodes. This is because, similarly to the wazero IR used by the old compiler, this is a custom IR. You will also notice that, _on the right-hand side of the assignments_ of any statement, no name occurs _twice_: this is why this form is called **single-assignment**.

Finally, notice how `blk1` and `blk2` end with a jump to the last block `blk3`.

```
blk1: ()
    ...
	Jump blk3, v7

blk2: ()
	Jump blk3, v2

blk3: (v5:i32)
    ...
```

`blk3` takes an argument `v5`: `blk1` jumps to `bl3` with `v7` and `blk2` jumps to `blk3` with `v2`, meaning `v5` is effectively a rename of `v5` or `v7`, depending on the originating block. If you are familiar with the traditional representation of an SSA form, you will recognize that the role of block arguments is equivalent to the role of the *Phi (Φ) function*, a special function that returns a different value depending on the incoming edge; e.g., in this case: `v5 := Φ(v7, v2)`.


## Front-End: Optimization

The SSA form makes it easier to perform a number of optimizations. For instance, we can perform constant propagation, dead code elimination, and common subexpression elimination. These optimizations either act upon the instructions within a basic block, or they act upon the control-flow graph as a whole.

On a high, level, consider the following basic block, derived from the previous example:

```
blk0: (exec_ctx:i64, module_ctx:i64)
    v2:i32 = Iconst_32 -5
    v3:i32 = Iconst_32  0
    v4:i32 = Icmp lt_s, v2, v3
    Brz v4, blk2
    Jump blk1
```

It is pretty easy to see that the comparison in `v4` can be replaced by a constant `1`, because the comparison is between two constant values (-5, 0). Therefore, the block can be rewritten as such:

```
blk0: (exec_ctx:i64, module_ctx:i64)
    v4:i32 = Iconst_32 1
    Brz v4, blk2
    Jump blk1
```

However, we can now also see that the branch is always taken, and that the block `blk2` is never executed, so even the branch instruction and the constant definition `v4` can be removed:

```
blk0: (exec_ctx:i64, module_ctx:i64)
    Jump blk1
```

This is a simple example of constant propagation and dead code elimination occurring within a basic block. However, now  `blk2` is unreachable, because there is no other edge in the edge that points to it; thus it can be removed from the control-flow graph. This is an example of dead-code elimination that occurs at the control-flow graph level.

In practice, because WebAssembly is a compilation target, these simple optimizations are often unnecessary. The optimization passes implemented in wazero are also work-in-progress and, at the time of writing, further work is expected to implement more advanced optimizations.

<!-- say more about block layout etc... -->

## Back-End

...

[ssa-blocks]: https://en.wikipedia.org/wiki/Static_single-assignment_form#Block_arguments
[llvm-mlir]: https://mlir.llvm.org/docs/Rationale/Rationale/#block-arguments-vs-phi-nodes
