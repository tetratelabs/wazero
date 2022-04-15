## Import go func example

This example shows how to define, import and call a Go-defined function from a WebAssembly-defined function.

If the current year is 2022, and we give the argument 2000, [age-calculator.go](age-calculator.go) should output 22.
```bash
$ go run age-calculator.go 2000
println >> 21
log_i32 >> 21
```

### Background

WebAssembly has neither a mechanism to get the current year, nor one to print to the console, so we define these in Go.
Similar to Go, WebAssembly functions are namespaced, into modules instead of packages. Just like Go, only exported
functions can be imported into another module. What you'll learn in [age-calculator.go](age-calculator.go), is how to
export functions using [ModuleBuilder](https://pkg.go.dev/github.com/tetratelabs/wazero#ModuleBuilder) and how a
WebAssembly module defined in its [text format](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#text-format%E2%91%A0)
imports it. This only uses the text format for demonstration purposes, to show you what's going on. It is likely, you
will use another language to compile a Wasm (WebAssembly Module) binary, such as TinyGo. Regardless of how wasm is
produced, the export/import mechanics are the same!

### Where next?

The following examples continue the path of learning about importing and exporting functions with wazero:

#### [WebAssembly System Interface (WASI)](../wasi)

This uses an ad-hoc Go-defined function to print to the console. There is an emerging specification to standardize
system calls (similar to Go's [x/sys](https://pkg.go.dev/golang.org/x/sys/unix)) called WebAssembly System Interface
[(WASI)](https://github.com/WebAssembly/WASI). While this is not yet a W3C standard, wazero includes a
[wasi package](https://pkg.go.dev/github.com/tetratelabs/wazero/wasi).

#### [Replace Import](../replace-import)

You may use WebAssembly modules that have imports that don't match your ideal packaging structure. wazero allows you to
replace imports with different module names as needed, on a function granularity using
[ModuleConfig.WithImport](https://pkg.go.dev/github.com/tetratelabs/wazero#ModuleConfig.WithImport).
