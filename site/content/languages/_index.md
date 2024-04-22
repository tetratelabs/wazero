+++
title = "Languages"
layout = "single"
+++

WebAssembly has a virtual machine architecture where the host is the embedding
process and the guest is a program compiled into the WebAssembly Binary Format,
also known as Wasm. The first step is to take a source file and compile it into
the Wasm bytecode.

e.g. If your source is in Go, you might compile it with TinyGo.
```goat
    .-----------.    .----------------------.      .-----------.
   /  main.go  /---->|  tinygo -target=wasi +---->/ main.wasm /
  '-----+-----'      '----------------------'    '-----------'
```

Below are notes wazero contributed so far, in alphabetical order by language.

* [TinyGo]({{< relref "/tinygo.md" >}}) e.g. `tinygo build -o X.wasm -target=wasi X.go`
* [Rust]({{< relref "/rust.md" >}}) e.g. `rustc -o X.wasm --target wasm32-wasi X.rs`
* [Zig]({{< relref "/zig.md" >}}) e.g. `zig build-exe X.zig -target wasm32-wasi`

wazero is a runtime that embeds in Go applications, not a web browser. As
such, these notes bias towards backend use of WebAssembly, not browser use.

Disclaimer: These are not official documentation, nor represent the teams who
maintain language compilers. If you see any errors, please help [maintain][1]
these and [star our GitHub repository][2] if they are helpful. Together, we can
make WebAssembly easier on the next person.

## Constraints

The [WebAssembly Core specification]({{< ref "/specs#core" >}}) defines a
stack-based virtual machine. The only features that work by default are
computational in nature, and the only way to communicate is via functions,
memory or global variables.

WebAssembly has no standard library or system call interface to implement
features the operating system would otherwise provide. Certain capabilities,
such as forking a process, will not work. Support of common I/O features, such
as writing to the console, vary. See [System Calls](#system-calls) for more.

Software is more than technical constraints. WebAssembly remains a relatively
niche target, with limited maintenance and development. This means that certain
features may not work, yet, even if they could technically.

In general, developing with WebAssembly is difficult, and fewer problems can
be discovered at compilation time vs more supported targets. This results in
more runtime errors, or even panics. Where error messages exist, they may be
misleading. Finally, the languages maintainers may be less familiar with how to
solve the problems, and/or rely on less available key maintainers.

### Mitigating Constraints

The above constraints affect the library design and dependency choices in your
source, and by extension the choices of library dependencies you can use. In
extreme cases, constraints or support concerns may lead developers to choose
newer languages like [Zig][10].

Regardless of the programming language used, the best advice is to unit test
your code, and run tests with your intended WebAssembly runtime, like wazero.

These tests should cover the critical paths of your code, including errors.
Doing so protects your time. You'll have higher confidence, and more efficient
means to communicate problems vs ad-hoc reports.

## System Calls

WebAssembly is a stack-based virtual machine specification, so operates at a
lower level than an operating system. For functionality the operating system
would otherwise provide, system interfaces are needed.

Programming languages usually include a standard library, with features that
require I/O, such as writing to the console. Portability is helped along with
[POSIX][3] conforming implementations of system calls, such as `fd_read`.

There is a [WebAssembly System Interface]({{< ref "/specs#wasi" >}}), a.k.a.
WASI, which defines host functions loosely based on POSIX. There's also a
de facto implementation [wasi-libc][4]. However, WASI is not a standard and
language compilers don't always support it.

For example, AssemblyScript once supported WASI, but no longer does. Even
compilers that target WASI using [wasi-libc][4] have gaps. For example,
[TinyGo]({{< relref "/tinygo.md" >}}) does not yet support `fd_readdir`. Some toolchains have a
hybrid approach. For example, Emscripten uses WASI for console output, but its
own virtual filesystem functions. Finally, the team behind WASI are
developing an incompatible, modular replacement to the current version.

It is important to note that even when system interfaces are supported, some
users prefer a freestanding compilation target that restricts them. This helps
them control binary size and performance.

In summary, system interfaces in WebAssembly are not standard and are immature.
Developers need to understand and test the system interfaces they rely on.
Testing ensures not only the present capabilities, but also they continue to
operate as the ecosystem matures.

## Concurrency

WebAssembly does not yet support true parallelism; it lacks support for
multiple threads, atomics, and memory barriers. (It may someday; See
the [threads proposal][5].)

For example, a compiler targeting [WASI]({{< ref "/specs#wasi" >}}), generates
a `_start` function corresponding to `main` in the original source code. When
the WebAssembly runtime calls `_start`, it remains on the same thread of
execution until that function completes.

Concretely, if using wazero, a Wasm function call remains on the calling
goroutine until it completes.

In summary, while true that host functions can do anything, including launch
processes, Wasm binaries compliant with the [WebAssembly Core Specification]
({{< ref "/specs#core" >}}) cannot do anything in parallel, unless they use
non-standard instructions or conventions not yet defined by the specification.

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
[3]: https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/contents.html
[4]: https://github.com/WebAssembly/wasi-libc
[5]: https://github.com/WebAssembly/threads
[6]: https://llvm.org
[7]: https://github.com/WebAssembly/binaryen
[8]: https://github.com/WebAssembly/binaryen/blob/main/src/passes/Asyncify.cpp
[9]: https://github.com/wapc/wapc-go
[10]: https://ziglang.org/
