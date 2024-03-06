+++
title = "How the Optimizing Compiler Works"
layout = "single"
+++

wazero supports two modes of execution: interpreter mode and compilation mode.
The interpreter mode is a fallback mode for platforms where compilation is not
supported. Compilation mode is otherwise the default mode of execution: it
translates Wasm modules to native code to get the best run-time performance.

Translating Wasm bytecode into machine code can take multiple forms.  wazero
1.0 performs a straightforward translation from a given instruction to a native
instruction. wazero 2.0 introduces an optimizing compiler that is able to
perform nontrivial optimizing transformations, such as constant folding or
dead-code elimination, and it makes better use of the underlying hardware, such
as CPU registers. This document digs deeper into what we mean when we say
"optimizing compiler", and explains how it is implemented in wazero.

This document is intended for maintainers, researchers, developers and in
general anyone interested in understanding the internals of wazero.

What is an Optimizing Compiler?
-------------------------------

Wazero supports an _optimizing_ compiler in the style of other optimizing
compilers such as LLVM's or V8's. Traditionally an optimizing
compiler performs compilation in a number of steps.

Compare this to the **old compiler**, where compilation happens in one step or
two, depending on how you count:


```goat
    Input         +---------------+     +---------------+
 Wasm Binary ---->| DecodeModule  |---->| CompileModule |----> wazero IR
                  +---------------+     +---------------+
```

That is, the module is (1) validated then (2) translated to an Intermediate
Representation (IR). The wazero IR can then be executed directly (in the case
of the interpreter) or it can be further processed and translated into native
code by the compiler. This compiler performs a straightforward translation from
the IR to native code, without any further passes. The wazero IR is not intended
for further processing beyond immediate execution or straightforward
translation.

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


Validation and translation to an IR in a compiler are usually called the
**front-end** part of a compiler, while code-generation occurs in what we call
the **back-end** of a compiler. The front-end is the part of a compiler that is
closer to the input, and it generally indicates machine-independent processing,
such as parsing and static validation. The back-end is the part of a compiler
that is closer to the output, and it generally includes machine-specific
procedures, such as code-generation.

In the **optimizing** compiler, we still decode and translate Wasm binaries to
an intermediate representation in the front-end, but we use a textbook
representation called an **SSA** or "Static Single-Assignment Form", that is
intended for further transformation.

The benefit of choosing an IR that is meant for transformation is that a lot of
optimization passes can apply directly to the IR, and thus be
machine-independent. Then the back-end can be relatively simpler, in that it
will only have to deal with machine-specific concerns.

The wazero optimizing compiler implements the following compilation passes:

* Front-End:
  - Translation to SSA
  - Optimization
  - Block Layout
  - Control Flow Analysis

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
                      |                            |
                      v                            v
                Block Layout             Finalization/Encoding
```

Like the other engines, the implementation can be found under `engine`, specifically
in the `wazevo` sub-package. The entry-point is found under `internal/engine/wazevo/engine.go`,
where the implementation of the interface `wasm.Engine` is found.

All the passes can be dumped to the console for debugging, by enabling, the build-time
flags under `internal/engine/wazevo/wazevoapi/debug_options.go`. The flags are disabled
by default and should only be enabled during debugging. These may also change in the future.

In the following we will assume all paths to be relative to the `internal/engine/wazevo`,
so we will omit the prefix.

## Index

- [Front-End](frontend/)
- [Back-End](backend/)
- [Appendix](appendix/)
