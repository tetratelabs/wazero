## AssemblyScript example

This example runs a WebAssembly program compiled using AssemblyScript, built with `npm install && npm run build`.
The program exports two functions, `hello_world` which executes simple integer math, and `goodbye_world`, which
throws an error that is logged using the AssemblyScript `abort` built-in function. Wazero is configured to export
functions used by WebAssembly for reporting errors and trace messages.

Ex.
```bash
$ go run assemblyscript.go 7
hello_world returned: 10
sad sad world at assemblyscript.ts:7:3
```
