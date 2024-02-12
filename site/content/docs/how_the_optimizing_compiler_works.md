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
