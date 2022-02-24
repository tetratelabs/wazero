# wazero

wazero lets you run WebAssembly modules with zero platform dependencies. Since wazero doesn’t rely on CGO, you keep
portability features like cross compilation. Import wazero and extend your Go application with code written in any
language!

## Example

Here's an example of using wazero to invoke a factorial included in a Wasm binary.

While our [source for this](tests/engine/testdata/fac.wat) is the WebAssembly 1.0 (MVP) Text Format,
it could have been written in another language that targets Wasm, such as AssemblyScript/C/C++/Rust/TinyGo/Zig.

```golang
func main() {
	// Read WebAssembly binary containing an exported "fac" function.
	// * Ex. (func (export "fac") (param i64) (result i64) ...
	source, _ := os.ReadFile("./tests/engine/testdata/fac.wasm")

	// Instantiate the module with a Wasm Interpreter, to return its exported functions
	exports, _ := wazero.InstantiateModule(wazero.NewStore(), &wazero.ModuleConfig{Source: source})

	// Discover 7! is 5040
	fmt.Println(exports.Function("fac").Call(context.Background(), 7))
}
```

## Status

wazero is an early project, so APIs are subject to change until version 1.0.

There's the concept called "engine" in wazero (which is a word commonly used in Wasm runtimes). Engines are responsible for compiling and executing WebAssembly modules.
There are two types of engines are available for wazero:

1. _Interpreter_: a naive interpreter-based implementation of Wasm virtual machine. Its implementation doesn't have any platform (GOARCH, GOOS) specific code, therefore _interpreter_ engine can be used for any compilation target available for Go (such as `riscv64`).
2. _JIT engine_: compiles WebAssembly modules, generates the machine code, and executing it all at runtime. Currently wazero implements the JIT compiler for `amd64` and `arm64` target. Generally speaking, _JIT engine_ is faster than _Interpreter_ by order of magnitude. However, the implementation is immature and has a bunch of aspects that could be improved (for example, it just does a singlepass compilation and doesn't do any optimizations, etc.). Please refer to [internal/wasm/jit/RATIONALE.md](internal/wasm/jit/RATIONALE.md) for the design choices and considerations in our JIT engine.

Both of engines passes 100% of [WebAssembly spec test suites]((https://github.com/WebAssembly/spec/tree/wg-1.0/test/core)) (on supported platforms).

| Engine     | Usage| amd64 | arm64 | others |
|:---:|:---:|:---:|:---:|:---:|
| Interpreter|`wazero.NewEngineInterpreter()`|✅ |✅|✅|
| JIT engine |`wazero.NewEngineJIT()`|✅|✅ |❌|

*Note:* JIT does not yet work on Windows. Please use the interpreter and track [this issue](https://github.com/tetratelabs/wazero/issues/270) if interested.

If you choose no configuration, ex `wazero.NewStore()`, the interpreter is used. You can also choose explicitly like so:
```go
store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{Engine: wazero.NewEngineJIT()})
```

## Background

If you want to provide Wasm host environments in your Go programs, currently there's no other choice than using CGO and leveraging the state-of-the-art runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and there's no pure Go Wasm runtime out there. (There's only one exception named [wagon](https://github.com/go-interpreter/wagon), but it was archived with the mention to this project.)

First, why do you want to write host environments in Go? You might want to have plugin systems in your Go project and want these plugin systems to be safe/fast/flexible, and enable users to
write plugins in their favorite languages. That's where Wasm comes into play. You write your own Wasm host environments and embed Wasm runtime in your projects, and now users are able to write plugins in their own favorite lanugages (AssembyScript, C, C++, Rust, Zig, etc.). As a specific example, you maybe write proxy severs in Go and want to allow users to extend the proxy via [Proxy-Wasm ABI](https://github.com/proxy-wasm/spec). Maybe you are writing server-side rendering applications via Wasm, or [OpenPolicyAgent](https://www.openpolicyagent.org/docs/latest/wasm/) is using Wasm for plugin system.

However, experienced Golang developers often avoid using CGO because [_CGO is not Go_](https://dave.cheney.net/2016/01/18/cgo-is-not-go)[ -- _Rob_ _Pike_](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s), and it introduces another complexity into your projects. But unfortunately, as I mentioned there's no pure Go Wasm runtime out there, so you have to resort to CGO.

Currently, any performance optimization hasn't been done to this runtime yet, and the runtime is just a simple interpreter of Wasm binary. That means in terms of performance, the runtime here is inferior to any aforementioned runtimes (e.g. Wasmtime) for now.

However, _theoretically speaking_, this project has the potential to compete with these state-of-the-art JIT-style runtimes. The rationale for that is it is well-know that [CGO is slow](https://github.com/golang/go/issues/19574). More specifically, if you make large amount of CGO calls which cross the boundary between Go and C (stack) space, then the usage of CGO could be a bottleneck.

Since we can do JIT compilation purely in Go, this runtime could be the fastest one for some use cases where we have to make large amount of CGO calls (e.g. Proxy-Wasm host environment, or request-based plugin systems).
