+++
title = "History of wazero"
+++

wazero was originally developed by [Takeshi Yoneda][1] as a hobby project in
mid 2020. In late 2021, it was sponsored by Tetrate as a top-level project.
That said, Takeshi's original motivation is as relevant today as when he
started the project, and worthwhile reading:

If you want to provide Wasm host environments in your Go programs, currently
there's no other choice than using CGO leveraging the state-of-the-art
runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and
there's no pure Go Wasm runtime out there. (There's only one exception named
[wagon][2], but it was archived with the mention to this project.)

First, why do you want to write host environments in Go? You might want to have
plugin systems in your Go project and want these plugin systems to be
safe/fast/flexible, and enable users to write plugins in their favorite
languages. That's where Wasm comes into play. You write your own Wasm host
environments and embed Wasm runtime in your projects, and now users are able to
write plugins in their own favorite languages (AssemblyScript, C, C++, Rust,
Zig, etc.). As a specific example, you maybe write proxy severs in Go and want
to allow users to extend the proxy via [Proxy-Wasm ABI][3]. Maybe you are
writing server-side rendering applications via Wasm, or [OpenPolicyAgent][4]
is using Wasm for plugin system.

However, experienced Go developers often avoid using CGO because it
introduces complexity. For example, CGO projects are larger and complicated to
consume due to their libc + shared library dependency. Debugging is more
difficult for Go developers when most of a library is written in Rustlang.
[_CGO is not Go_][5] [ -- _Rob_ _Pike_][6] dives in deeper. In short, the
primary motivation to start wazero was to avoid CGO.

wazero compiles WebAssembly modules into native assembly (Compiler) by default. You
may be surprised to find equal or better performance vs mature Compiler-style
runtimes because [CGO is slow][7]. More specifically, if you make large amount
of CGO calls which cross the boundary between Go and C (stack) space, then the
usage of CGO could be a bottleneck.

[1]: https://github.com/mathetake
[2]: https://github.com/go-interpreter/wagon
[3]: https://github.com/proxy-wasm/spec
[4]: https://www.openpolicyagent.org/docs/latest/wasm/
[5]: https://dave.cheney.net/2016/01/18/cgo-is-not-go
[6]: https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s
[7]: https://github.com/golang/go/issues/19574
