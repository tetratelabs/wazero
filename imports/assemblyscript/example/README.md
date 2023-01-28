## AssemblyScript example

This example runs a WebAssembly program compiled using AssemblyScript, built
with `npm install && npm run build`.

AssemblyScript program exports two functions, `hello_world` which executes
simple math, and `goodbye_world`, which throws an error that is logged using
AssemblyScript `abort` built-in function.

This demo configures AssemblyScript imports for errors and trace messages.

```bash
$ go run assemblyscript.go 7
hello_world returned: 10
sad sad world at index.ts:7:3
```

Note: [index.ts](testdata/index.ts) avoids use of JavaScript functions that use
I/O, such as [console.log][1]. If your code uses these, compile your code with
the [wasi-shim][2] and configure in wazero using
`wasi_snapshot_preview1.Instantiate`.

[1]: https://github.com/AssemblyScript/assemblyscript/blob/v0.26.7/std/assembly/bindings/dom.ts#L143
[2]: https://github.com/AssemblyScript/wasi-shim#usage
