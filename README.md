# wazero

wazero lets you run WebAssembly modules with zero platform dependencies. Since wazero doesn’t rely on CGO, you keep
portability features like cross compilation. Import wazero and extend your Go application with code written in any
language!

## Example
Here's an example of using wazero to invoke a Fibonacci function included in a Wasm binary.

While our [source for this](examples/wasm/fibonacci.go) is [TinyGo](https://tinygo.org/), it could have been written in
another language that targets Wasm, such as Rust.

```golang
func Test_fibonacci(t *testing.T) {
    source, _ := os.ReadFile("wasm/fibonacci.wasm")
	mod, _ := binary.DecodeModule(source)
	store := wasm.NewStore(wazeroir.NewEngine())
	store.Instantiate(mod, "test")

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, retTypes, err := store.CallFunction("test", "fibonacci", uint64(c.in))
		require.NoError(t, err)
		require.Len(t, ret, len(retTypes))
		require.Equal(t, wasm.ValueTypeI32, retTypes[0])
		require.Equal(t, c.exp, int32(ret[0]))
	}
}

```

## Status
- wazero is an early project, so APIs are subject to change until version 1.0.

- The interpreter-based wazero runtime passes all the [Wasm Spec test suites](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core) and
is fully compatible with the WebAssembly v1.0 Specification.
- We are currently working on a JIT compilation engine written in Go to improve the performance issue. You can check the work-in-progress JIT engine codebase in [wasm/jit](./wasm/jit) directory. Please refer to [wasm/jit/RATIONALE.md](wasm/jit/RATIONALE.md) for the design choices and considerations in our JIT engine.

## Background

If you want to provide Wasm host environments in your Go programs, currently there's no other choice than using CGO and leveraging the state-of-the-art runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and there's no pure Go Wasm runtime out there. (There's only one exception named [wagon](https://github.com/go-interpreter/wagon), but it was archived with the mention to this project.)

First of all, why do you want to write host environments in Go? You might want to have plugin systems in your Go project and want these plugin systems to be safe/fast/flexible, and enable users to
write plugins in their favorite lanugages. That's where Wasm comes into play. You write your own Wasm host environments and embed Wasm runtime in your projects, and now users are able to write plugins in their own favorite lanugages (AssembyScript, C, C++, Rust, Zig, etc.). As an specific example, you maybe write proxy severs in Go and want to allow users to extend the proxy via [Proxy-Wasm ABI](https://github.com/proxy-wasm/spec). Maybe you are writing server-side rendering applications via Wasm, or [OpenPolicyAgent](https://www.openpolicyagent.org/docs/latest/wasm/) is using Wasm for plugin system.

However, experienced Golang developers often avoid using CGO because [_CGO is not Go_](https://dave.cheney.net/2016/01/18/cgo-is-not-go)[ -- _Rob_ _Pike_](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s), and it introduces another complexity into your projects. But unfortunately, as I mentioned there's no pure Go Wasm runtime out there, so you have to resort to CGO.

Currently any performance optimization hasn't been done to this runtime yet, and the runtime is just a simple interpreter of Wasm binary. That means in terms of performance, the runtime here is infereior to any aforementioned runtimes (e.g. Wasmtime) for now.

However _theoretically speaking_, this project have the potential to compete with these state-of-the-art JIT-style runtimes. The rationale for that is it is well-know that [CGO is slow](https://github.com/golang/go/issues/19574). More specifically, if you make large amount of CGO calls which cross the boundary between Go and C (stack) space, then the usage of CGO could be a bottleneck.

Luckily with unsafe pointer casts, we can do JIT compilation purely in Go (e.g. https://github.com/bspaans/jit-compiler), so if we develop JIT Wasm compiler in Go without using CGO, this runtime could be the fastest one for some usecases where we have to make large amount of CGO calls (e.g. Proxy-Wasm host environment, or request-based plugin systems).
