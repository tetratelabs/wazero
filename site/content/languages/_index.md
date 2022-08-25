+++
title = "Languages"
layout = "single"
+++

WebAssembly has a virtual machine architecture where the host is the embedding
process and the guest is a program compiled into the WebAssembly Binary Format,
also known as Wasm. The first step is to take a source file and compile it into
the Wasm bytecode.

Ex. If your source is in Go, you might compile it with TinyGo.
```goat
    .-----------.    .----------------------.      .-----------.
   /  main.go  /---->|  tinygo -target=wasi +---->/ main.wasm /
  '-----+-----'      '----------------------'    '-----------'
```

Below are notes wazero contributed so far, in alphabetical order by language.

* [TinyGo](tinygo) Ex. `tinygo build -o X.wasm -target=wasi X.go`
* [Rust](rust) Ex. `rustc -o X.wasm --target wasm32-wasi X.rs`

wazero is a runtime that embeds in Go applications, not a web browser. As
such, these notes bias towards backend use of WebAssembly, not browser use.

Disclaimer: These are not official documentation, nor represent the teams who
maintain language compilers. If you see any errors, please help [maintain][1]
these and [star our GitHub repository][2] if they are helpful. Together, we can
make WebAssembly easier on the next person.

## Concurrency

WebAssembly does not yet support true parallelism; it lacks support for
multiple threads, atomics, and memory barriers. (It may someday; See
the [threads proposal][5].)

For example, a compiler targeting [WASI][3], generates a `_start` function
corresponding to `main` in the original source code. When the WebAssembly
runtime calls `_start`, it remains on the same thread of execution until that
function completes.

Concretely, if using wazero, a Wasm function call remains on the calling
goroutine until it completes.

In summary, while true that host functions can do anything, including launch
processes, Wasm binaries compliant with [WebAssembly Core 2.0][4] cannot do
anything in parallel, unless they use non-standard instructions or conventions
not yet defined by the specification.

### Compiling Parallel Code to Serial Wasm

Until this [changes][5], language compilers cannot generate Wasm that can
control scheduling within a function or safely modify memory in parallel.
In other words, one function cannot do anything in parallel.

This impacts how programming language primitives translate to Wasm:

* Garbage collection invokes on the runtime host's calling thread instead of
  in the background.
* Language-defined threads or co-routines fail compilation or are limited to
  sequential processing.
* Locks and barriers fail compilation or are implemented unsafely.
* Async functions including I/O execute sequentially.

Language compilers often used shared infrastructure, such as [LLVM][6] and
[Binaryen][7]. One tool that helps in translation is Binaryen's [Asyncify][8],
which lets a language support synchronous operations in an async manner.

### Concurrency via Orchestration

To work around lack of concurrency at the WebAssembly Core abstraction, tools
often orchestrate pools of workers, and ensure a module in that pool is only
used sequentially.

For example, [waPC][9] provides a WASM module pool, so host callbacks can be
invoked in parallel, despite not being able to share memory.

[1]: https://github.com/tetratelabs/wazero/tree/main/site/content/languages
[2]: https://github.com/tetratelabs/wazero/stargazers
[3]: https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
[4]: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/
[5]: https://github.com/WebAssembly/threads
[6]: https://llvm.org
[7]: https://github.com/WebAssembly/binaryen
[8]: https://github.com/WebAssembly/binaryen/blob/main/src/passes/Asyncify.cpp
[9]: https://github.com/wapc/wapc-go
