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

For the impatient, here's a peek of a general flow with wazero:

First, you need to compile your code into the WebAssembly Binary Format (Wasm).

Here's source in [TinyGo]({{< ref "/languages/tinygo" >}}), which exports an
"add" function:
```go
package main

//export add
func add(x, y uint32) uint32 {
	return x + y
}
```

Here's the minimal command to build a `%.wasm` binary.
```bash
tinygo build -o add.wasm -target=wasi add.go
```

Finally, you can run that inside your Go application.
```go
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Read a WebAssembly binary containing an exported "add" function.
	wasm, err := os.ReadFile("./path/to/add.wasm")
	if err != nil {
		log.Panicln(err)
	}

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate the module and return its exported functions
	module, err := r.InstantiateModuleFromBinary(ctx, wasm)
	if err != nil {
		log.Panicln(err)
	}

	// Discover 1+2=3
	fmt.Println(module.ExportedFunction("add").Call(ctx, 1, 2))
}
```

Notes:

* The Wasm binary is often called the "guest" in WebAssembly.
* The embedding application is often called the "host" in WebAssembly.
* Many languages compile to (target) Wasm including AssemblyScript, C, C++,
  Rust, TinyGo and Zig!

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

wazero will release its first beta at the end of August 2022, and finalize
1.0 once Go 1.20 is released in Feb 2023. Meanwhile, please practice the
current APIs to ensure they work for you, and give us a [star][2] if you are
enjoying it so far!

[1]: https://github.com/tetratelabs/wazero/blob/main/examples
[2]: https://github.com/tetratelabs/wazero/stargazers
