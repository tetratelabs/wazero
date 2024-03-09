+++
title = "Docs"
layout = "single"
+++

## Overview

WebAssembly is a way to safely run code compiled in other languages.
Runtimes execute WebAssembly Modules (Wasm), which are most often binaries with a `.wasm` extension.
Most WebAssembly modules import functions from the host, to perform tasks that are otherwise disallowed by their sandbox.
The most commonly imported functions are called WASI, which allow access to system resources such as the console or files.

wazero is a WebAssembly runtime, written completely in Go. It has no platform dependencies, so can be used in any environment supported by Go.

## API

Being a Go library, which we document wazero's API via [godoc][godoc].

## Terminology

Wazero has consistent terminology used inside the codebase which may be new to you, or different than another WebAssembly runtime.
This section covers the most commonly used vocabulary. Terms rarely used may also be defined inline in individual sections.

* Host - Host is a WebAssembly concept that refers to the process that embeds a WebAssembly runtime. In wazero, the host is a program written in Go.
* Binary - Binary is a WebAssembly module, compiled from source such as C, Rust or Tinygo. This is also called Wasm or a guest, and usually is a file with a `.wasm` extension. This is the code wazero runs.
* Sandbox - Sandbox is a term that describes isolation. For example, a WebAssembly module, defined below, is isolated from the host memory and memory of other modules. This means it cannot corrupt the calling process or cause it to crash.
* [Module][Module] - Module an instance of a Binary, which usually exports functions that can be invoked by the embedder. It can also import functions from the host to perform tasks not defined in the WebAssembly Core Specification, such as I/O.
* Host Module - Host Module is a wazero concept that represents a collection of exported functions that give functionality not provided in the WebAssembly Core Specification, such as I/O. These exported functions are defined as normal Go functions (including closures). For example, WASI is often used to describe a host module named "wasi_snapshot_preview1".
* Exported Function - An Exported Function is a function addressable by name. Guests can import functions from a host module, and export them so that Go applications can call them.
* [Runtime][Runtime] - Runtime is the top-level component in wazero that compiles binaries, configures host functions, and runs guests in sandboxes. How it behaves is determined by its engine: interpreter or compiler.
* Compile - In wazero, compile means prepares a binary, or a host module to be instantiated. This is implemented differently based on whether a runtime is a compiler or an interpreter.
* [Compiled Module][CompiledModule] - a prepared and ready to be instantiated object created vi Compilation phrase. This can be used in instantiation multiple times to create multiple and isolated sandbox from a single Wasm binary.
* Instantiate - In wazero, instantiate means allocating a [Compiled Module][CompiledModule] and associating it with a unique name, resulting in a [Module][Module]. This includes running any start functions. The result of instantiation is a module whose exported functions can be called.

## Architecture

This section covers the library architecture wazero uses to implements the WebAssembly Core specification and WASI.
Features unique to Go or wazero are discussed where architecture affecting.

### Components

At a high level, wazero exposes a [Runtime][Runtime], which can compile the binary into [Compiled Module][CompiledModule],
and instantiate it as a sandboxed [Module][Module].
These sandboxed modules are isolated from each other (modulo imports) and the embedding Go program. In a sandbox,
there are 4 types of objects: memory, global, table, and function. Functions might be exported by name, and they can be executed by
the embedding Go programs. During the execution of a function, the objects in the sandbox will be accessed, for example,
a Wasm function can read and write from the memory object in the sandbox. The same goes for globals and tables.

Here's a diagram showing the relationship between these components.

```goat
                                             |           Access during execution
                                             |        +--------+-------+-----------+
                                             |        |        |       |       +---|
                                             |        |        |       |       |   |
                                             |        v        v       v       v   |
                                             |     (Memory, Globals, Table, Functions)
           Wasm Binary                       |                      |              ^
               |                             |                      |              |
+----------+   v   +--------------------+    |    1 : N    +------------------+    |
| Runtime  | ----> |  Compiled  Module  |----|-----------> |      Module      |    |
+----------+       +--------------------+    | Instantiate +------------------+    |
                                             |                      |              |
                                             |                      | 1 : N        |
                                             |                      v              |
                                             |             +-------------------+   |
                                             |             | Exported Function |---+
                                             |             +-------------------+
                                             |
                                             |
                           compile time      |      runtime
                                             |
```


### Host access

First, a Wasm module can require the import of functions at instantiation phrase.
Such import requirements are included in the original Wasm binary. For example,

```wat
(module (import "env" "foo" (func)))
```

this WebAssembly module requires importing the exported function named `foo` from the instantiated module named `env`.
An imported functions can be called by the importing modules, and this is how a Wasm module interacts with the outside of
its own sandbox.

In wazero, the imported modules can be Host Modules which consist of Go functions. Therefore,
the importing modules can invoke Go functions defined by the embedding Go programs.
The notable example of this imported host module is wazero's [`wasi_snapshot_preview1`][wasi] module which provides
the system calls to wasm modules because the Wasm specification itself doesn't define system calls. This way, Wasm modules
are granted the ability to do, for example, file system access, etc.

Here's the diagram of how a Wasm module accesses Go functions:

```goat
                                                        func add(foo, bar int32) int32 {
                                                            return foo + bar
                                                        }         |
                                                                  |
                                                                  | implements
                                host module                       v
+---------+                +------------------+          +-----------------+
| Runtime | -------------> | (module: myhost) | -------> | (function: add) |
+---------+  ^             +------------------+  export  +-----------------+
    \       /                                                       /
     \instantiate                                                  /
      \   /                                                       /
       \ v                                                       /
        \                                                       /
         \                                                     / imported
          \ (import "myhost" "add" (func))                    /
           \                                                 /
            \                                   +-----------/------+
             \                                  |          v       |
              \                                 |   (myhost.add)   |
               v                                |        ^         |
                +--------------------+          |        | call    |
                | (module: need_add) |--------->| (export:use_add) <----- Exported
                +--------------------+          |                  |
                                                +------------------+
                                            functions in need_add's sandbox
```

In this example diagram,  we instantiated a host module named `myhost` which consists of a Go function `add`, and it exports
the Go function under the name `add`. Then, we instantiate the Wasm module which requires importing function whose module is `mymodule`
and name is `add`. This case, the import target module instance and function already exists, and therefore the resulting sandbox contains
the imported function in its sandbox. Finally, the importing module exports the function named `use_add` which in turns calls the imported function,
therefore, we can freely access the imported Go function from the importing Wasm module.

Here's [the working example in wazero repository][age-calculator], so please check it out for more details.


### Engine

There's a concept called "engine" in wazero's codebase. It is in charge of how wazero compiles the raw Wasm binary, transforms it into
intermediate data structure, caches the compiled information, and performs function calls of Wasm functions.
Notably, the interpreter and compiler in wazero's [Runtime configuration][RuntimeConfig] refer to the type of engine tied to [Runtime][Runtime].

#### Compiler

In wazero, a compiler is a runtime configured to compile modules to platform-specific machine code ahead of time (AOT)
during the creation of [CompiledModule][CompiledModule]. This means your WebAssembly functions execute
natively at runtime of the embedding Go program. Compiler is faster than Interpreter, often by order of
magnitude (10x) or more, and therefore enabled by default whenever available. You can read more about wazero's
[optimizing compiler in the detailed documentation]({{< relref "/how_the_optimizing_compiler_works" >}}).

#### Interpreter

Interpreter is a naive interpreter-based implementation of Wasm virtual machine.
Its implementation doesn't have any platform (GOARCH, GOOS) specific code,
therefore interpreter can be used for any compilation target available for Go (such as riscv64).

## How do function calls work?

WebAssembly runtimes let you call functions defined in wasm. How this works in
wazero is different depending on your `RuntimeConfig`.

* `RuntimeConfigCompiler` compiles machine code from your wasm, and jumps to
  that when invoking a function.
* `RuntimeConfigInterpreter` does not generate code. It interprets wasm and
  executes go statements that correspond to WebAssembly instructions.

How the compiler works precisely is a large topic. If you are interested in
digging deeper, please look at [the dedicated documentation]({{< relref "/how_do_compiler_functions_work.md" >}})
on this topic!

## Rationales behind wazero

Please refer to [RATIONALE][rationale] for the notable rationales behind wazero's implementations.

[Module]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Module
[Runtime]: https://pkg.go.dev/github.com/tetratelabs/wazero#Runtime
[RuntimeConfig]: https://pkg.go.dev/github.com/tetratelabs/wazero#RuntimeConfig
[CompiledModule]: https://pkg.go.dev/github.com/tetratelabs/wazero#CompiledModule
[godoc]: https://pkg.go.dev/github.com/tetratelabs/wazero
[rationale]: https://github.com/tetratelabs/wazero/blob/main/RATIONALE.md
[wasi]: https://github.com/tetratelabs/wazero/tree/main/imports/wasi_snapshot_preview1/example
[age-calculator]: https://github.com/tetratelabs/wazero/blob/v1.0.0-rc.1/examples/import-go/age-calculator.go
