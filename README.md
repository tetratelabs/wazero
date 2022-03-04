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
	module, _ := wazero.NewRuntime().NewModuleFromSource(source)

	// Discover 7! is 5040
	fmt.Println(module.Function("fac").Call(context.Background(), 7))
}
```

## Status

wazero is an early project, so APIs are subject to change until version 1.0.

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

## Background

If you want to provide Wasm host environments in your Go programs, currently there's no other choice than using CGO and leveraging the state-of-the-art runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and there's no pure Go Wasm runtime out there. (There's only one exception named [wagon](https://github.com/go-interpreter/wagon), but it was archived with the mention to this project.)

First, why do you want to write host environments in Go? You might want to have plugin systems in your Go project and want these plugin systems to be safe/fast/flexible, and enable users to
write plugins in their favorite languages. That's where Wasm comes into play. You write your own Wasm host environments and embed Wasm runtime in your projects, and now users are able to write plugins in their own favorite lanugages (AssembyScript, C, C++, Rust, Zig, etc.). As a specific example, you maybe write proxy severs in Go and want to allow users to extend the proxy via [Proxy-Wasm ABI](https://github.com/proxy-wasm/spec). Maybe you are writing server-side rendering applications via Wasm, or [OpenPolicyAgent](https://www.openpolicyagent.org/docs/latest/wasm/) is using Wasm for plugin system.

However, experienced Golang developers often avoid using CGO because it introduces complexity. For example, CGO projects are larger and complicated to consume due to their libc + shared library dependency. Debugging is more difficult for Go developers when most of a library is written in Rustlang. [_CGO is not Go_](https://dave.cheney.net/2016/01/18/cgo-is-not-go)[ -- _Rob_ _Pike_](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s) dives in deeper. In short, the primary motivation to start wazero was to avoid CGO.

wazero compiles WebAssembly modules into native assembly (JIT) by default. You may be surprised to find equal or better performance vs mature JIT-style runtimes because [CGO is slow](https://github.com/golang/go/issues/19574). More specifically, if you make large amount of CGO calls which cross the boundary between Go and C (stack) space, then the usage of CGO could be a bottleneck.

