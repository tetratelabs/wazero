+++
title = "wazero: the zero dependency WebAssembly runtime for Go developers"
layout = "single"
+++

WebAssembly is a way to safely run code compiled in other languages. Runtimes
execute WebAssembly Modules (Wasm), which are most often binaries with a
`.wasm` extension.

wazero is the only zero dependency WebAssembly runtime written in Go.

## Example

The best way to learn wazero is by trying one of our [examples][1]

For the impatient, here's how invoking a factorial function looks in wazero:

```go
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Read a WebAssembly binary containing an exported "fac" function.
	// * Ex. (func (export "fac") (param i64) (result i64) ...
	wasm, err := os.ReadFile("./path/to/fac.wasm")
	if err != nil {
		log.Panicln(err)
	}

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate the module and return its exported functions
	module, err := r.InstantiateModuleFromBinary(ctx, wasm)
	if err != nil {
		log.Panicln(err)
	}

	// Discover 7! is 5040
	fmt.Println(module.ExportedFunction("fac").Call(ctx, 7))
}
```

Note: `fac.wasm` was compiled from [fac.wat][2], in the [WebAssembly 1.0][3]
Text Format, it could have been written in another language that compiles to
(targets) WebAssembly, such as AssemblyScript, C, C++, Rust, TinyGo or Zig.

## Why zero?

By avoiding CGO, wazero avoids prerequisites such as shared libraries or libc,
and lets you keep features like cross compilation. Being pure Go, wazero adds
only a small amount of size to your binary. Meanwhile, wazero’s API gives
features you expect in Go, such as safe concurrency and context propagation.

### When can I use this?

wazero is an early project, so APIs are subject to change until version 1.0.
To use wazero meanwhile, you need to add its main branch to your project like
this:

```bash
go get github.com/tetratelabs/wazero@main
```

We expect [wazero 1.0][4] to be at or before Q3 2022, so please practice the
current APIs to ensure they work for you, and give us a [star][5] if you are
enjoying it so far!

[1]: https://github.com/tetratelabs/wazero/blob/main/examples
[2]: https://github.com/tetratelabs/wazero/blob/main/internal/integration_test/post1_0/multi-value/testdata/fac.wat
[3]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
[4]: https://github.com/tetratelabs/wazero/issues/506
[5]: https://github.com/tetratelabs/wazero/stargazers
