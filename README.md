# wazero

wazero lets you run WebAssembly modules with zero platform dependencies. Since wazero doesn’t rely on CGO, you keep
portability features like cross compilation. Import wazero and extend your Go application with code written in any
language!

## Example

Here's an example of using wazero to invoke a factorial included in a Wasm binary.

While our [source for this](tests/engine/testdata/fac.wat) is the WebAssembly 1.0 (20191205) Text Format,
it could have been written in another language that targets Wasm, such as AssemblyScript/C/C++/Rust/TinyGo/Zig.

```golang
func main() {
	// Read WebAssembly binary containing an exported "fac" function.
	// * Ex. (func (export "fac") (param i64) (result i64) ...
	source, _ := os.ReadFile("./tests/engine/testdata/fac.wasm")

	// Instantiate the module and return its exported functions
	module, _ := wazero.NewRuntime().InstantiateModuleFromSource(source)
	defer module.Close()

	// Discover 7! is 5040
	fmt.Println(module.ExportedFunction("fac").Call(nil, 7))
}
```

## Status

There are two runtime configurations supported in wazero, where _JIT_ is default:

1. _Interpreter_: a naive interpreter-based implementation of Wasm virtual machine. Its implementation doesn't have any platform (GOARCH, GOOS) specific code, therefore _interpreter_ can be used for any compilation target available for Go (such as `riscv64`).
2. _JIT_: compiles WebAssembly modules, generates the machine code, and executing it all at runtime. Currently wazero implements the JIT compiler for `amd64` and `arm64` target. Generally speaking, _JIT_ is faster than _Interpreter_ by order of magnitude. However, the implementation is immature and has a bunch of aspects that could be improved (for example, it just does a singlepass compilation and doesn't do any optimizations, etc.). Please refer to [internal/wasm/jit/RATIONALE.md](internal/wasm/jit/RATIONALE.md) for the design choices and considerations in our JIT compiler.

Both configurations pass 100% of [WebAssembly spec test suites]((https://github.com/WebAssembly/spec/tree/wg-1.0/test/core)) (on supported platforms).

| Runtime     | Usage| amd64 | arm64 | others |
|:---:|:---:|:---:|:---:|:---:|
| Interpreter|`wazero.NewRuntimeConfigInterpreter()`|✅ |✅|✅|
| JIT |`wazero.NewRuntimeConfigJIT()`|✅|✅ |❌|

If you don't choose, ex `wazero.NewRuntime()`, JIT is used if supported. You can also force the interpreter like so:
```go
r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
```

## Support Policy

The below support policy focuses on compatability concerns of those embedding wazero into their Go applications.

### wazero

wazero is an early project, so APIs are subject to change until version 1.0.

We expect 1.0 to be at or before Q3 2022, so please practice the current APIs to ensure they work for you!

### Go

wazero has no dependencies except Go, so the only source of conflict in your project's use of Wazero is the Go version.

To simplify our support policy, we adopt Go's [Release Policy](https://go.dev/doc/devel/release) (two versions).

This means wazero will remain compilable and tested on the the version prior to the latest release of Go.

For example, once Go 1.29 is released, wazero may choose to use a Go 1.28 feature.

### Platform

wazero has two runtime modes: Interpreter and JIT. The only supported operating
systems are ones we test, but that doesn't necessarily mean other operating
system versions won't work.

We currently test ubuntu-20.04, macos-11 and windows-2022 as packaged by
[GitHub Actions](https://github.com/actions/virtual-environments).

* Interpreter
  * Ubuntu is tested on amd64 (native) as well arm64 and riscv64 via emulation.
  * MacOS and Windows are only tested on amd64.
* JIT
  * Ubuntu is tested on amd64 (native) as well arm64 via emulation.
  * MacOS and Windows are only tested on amd64.

wazero has no dependencies and doesn't require CGO. This means it can also be
embedded in an application that doesn't use an operating system. For example,
via Docker `FROM scratch`. While we don't currently have tests for this, it is
an intended use case and a major differentiator between wazero and alternatives.

## Background

If you want to provide Wasm host environments in your Go programs, currently there's no other choice than using CGO and leveraging the state-of-the-art runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and there's no pure Go Wasm runtime out there. (There's only one exception named [wagon](https://github.com/go-interpreter/wagon), but it was archived with the mention to this project.)

First, why do you want to write host environments in Go? You might want to have plugin systems in your Go project and want these plugin systems to be safe/fast/flexible, and enable users to
write plugins in their favorite languages. That's where Wasm comes into play. You write your own Wasm host environments and embed Wasm runtime in your projects, and now users are able to write plugins in their own favorite lanugages (AssembyScript, C, C++, Rust, Zig, etc.). As a specific example, you maybe write proxy severs in Go and want to allow users to extend the proxy via [Proxy-Wasm ABI](https://github.com/proxy-wasm/spec). Maybe you are writing server-side rendering applications via Wasm, or [OpenPolicyAgent](https://www.openpolicyagent.org/docs/latest/wasm/) is using Wasm for plugin system.

However, experienced Golang developers often avoid using CGO because it introduces complexity. For example, CGO projects are larger and complicated to consume due to their libc + shared library dependency. Debugging is more difficult for Go developers when most of a library is written in Rustlang. [_CGO is not Go_](https://dave.cheney.net/2016/01/18/cgo-is-not-go)[ -- _Rob_ _Pike_](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s) dives in deeper. In short, the primary motivation to start wazero was to avoid CGO.

wazero compiles WebAssembly modules into native assembly (JIT) by default. You may be surprised to find equal or better performance vs mature JIT-style runtimes because [CGO is slow](https://github.com/golang/go/issues/19574). More specifically, if you make large amount of CGO calls which cross the boundary between Go and C (stack) space, then the usage of CGO could be a bottleneck.

