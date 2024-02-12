What is a JIT compiler?
=======================

In general, when we talk about a Just-In-Time (JIT) compiler, we mean a compilation technique that spares cycles at build-time, trading it for run-time. In other words, when a language is JIT-compiled, we usually mean that compilation will happen during run-time. Furthermore, when we use the term JIT-compilation, we also often mean is that, because compilation happens _during run-time_, we can use information that we have collected during execution to direct the compilation process: these types of JIT-compilers are often referred to as **tracing-JITs**.

Thus, if we wanted to be pedantic, **wazero** provides an **ahead-of-time**, **load-time** compiler. That is, a compiler that, indeed, performs compilation at run-time, but only when a WebAssembly module is loaded; it currently does not collect or leverage any information during the execution of the Wasm binary itself.

It is important to make such a distinction, because a Just-In-Time compiler may not be an optimizing compiler, and an optimizing compiler may not be a tracing JIT. In fact, the compiler that wazero shipped before the introduction of the new compiler architecture performed code generation at load-time, but did not perform any optimization.

# What is an Optimizing Compiler?

Wazero supports an _optimizing_ compiler in the style of other optimizing compilers out there, such as LLVM's or V8's. Traditionally an optimizing compiler performs compilation in a number of steps. Compare this to the old compiler, where compilation happens in one step or two, depending on how you count:


```goat

                    +-------------------+        +-------------------+ 
     Input          |                   |        |                   | 
  Wasm Binary  ---> |   DecodeModule    | -----> |   CompileModule   | ---->  wazero IR
                    |                   |        |                   | 
                    +-------------------+        +-------------------+ 
```
