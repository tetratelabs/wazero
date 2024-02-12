What is a JIT compiler?
=======================

In general, when we talk about a Just-In-Time (JIT) compiler, we mean a compilation technique that spares cycles at build-time, trading it for run-time. In other words, when a language is JIT-compiled, we usually mean that compilation will happen during run-time. Furthermore, when we use the term JIT-compilation, we also often mean is that, because compilation happens _during run-time_, we can use information that we have collected during execution to direct the compilation process: these types of JIT-compilers are often referred to as **tracing-JITs**.

Thus, if we wanted to be pedantic, **wazero** provides an **ahead-of-time**, **load-time** compiler. That is, a compiler that, indeed, performs compilation at run-time, but only when a WebAssembly module is loaded; it currently does not collect or leverage any information during the execution of the Wasm binary itself.

It is important to make such a distinction, because a Just-In-Time compiler may not be an optimizing compiler, and an optimizing compiler may not be a tracing JIT. In fact, the compiler that wazero shipped before the introduction of the new compiler architecture performed code generation at load-time, but did not perform any optimization.

# What is an Optimizing Compiler?

Wazero supports an _optimizing_ compiler in the style of other optimizing compilers out there, such as LLVM's or V8's. Traditionally an optimizing compiler performs compilation in a number of steps.

Compare this to the **old compiler**, where compilation happens in one step or two, depending on how you count:


```goat

                   +-------------------+      +-------------------+
     Input         |                   |      |                   |
  Wasm Binary  --->|   DecodeModule    |----->|   CompileModule   |---->  wazero IR
                   |                   |      |                   |
                   +-------------------+      +-------------------+
```

That is, the module is (1) validated then (2) translated to an Intermediate Representation (IR).
The wazero IR can then be executed directly (in the case of the interpreter) or it can be further processed and translated into native code by the compiler. This compiler performs a straightforward translation from the IR to native code, without any further passes. The wazero IR is not intended for further processing beyond immediate execution or straightforward translation.

```goat

                   +----   wazero IR    ----+
                   |                        |
                   v                        v
           +--------------+         +--------------+
           |              |         |              |
           |   Compiler   |         | Interpreter  |---------* executable
           |              |         |              |
           +--------------+         +--------------+
                  |
       +----------+---------+
       |                    |
       v                    v
+-------------+      +-------------+
|             |      |             |
|    ARM64    |      |    AMD64    |
|   Backend   |      |   Backend   |    ---------------------* executable
|             |      |             |
+-------------+      +-------------+

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

                      +-------------------+      +-------------------+
       Input          |                   |      |                   |
    Wasm Binary   --->|   DecodeModule    |----->|   CompileModule   |--+
                      |                   |      |                   |  |
                      +-------------------+      +-------------------+  |
                                                                        |
                                                                        |
                                                                        |
+-----------------------------------------------------------------------+
|
|
|
|  +---------------------------+                    +---------------------------+
|  |                           |                    |                           |
+->|         Front-End         |------------------->|         Back-End          |
   |                           |                    |                           |
   +---------------------------+                    +---------------------------+
                 |                                                |
                 |                                                |
                 v                                                v
                SSA                                     Instruction Selection
                 |                                                |
                 |                                                |
                 v                                                v
           Optimization                                  Registry Allocation
                                                                  |
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



<!--
We use the "block argument" variant of SSA: https://en.wikipedia.org/wiki/Static_single-assignment_form#Block_arguments
which is equivalent to the traditional PHI function based one, but more convenient during optimizations.
However, in this package's source code comment, we might use PHI whenever it seems necessary in order to be aligned with
existing literatures, e.g. SSA level optimization algorithms are often described using PHI nodes.

The rationale doc for the choice of "block argument" by MLIR of LLVM is worth a read:
https://mlir.llvm.org/docs/Rationale/Rationale/#block-arguments-vs-phi-nodes

The algorithm to resolve variable definitions used here is based on the paper
"Simple and Efficient Construction of Static Single Assignment Form": https://link.springer.com/content/pdf/10.1007/978-3-642-37051-9_6.pdf.
-->
