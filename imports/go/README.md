# Overview

Wazero's "github.com/tetratelabs/wazero/imports/go" package allows you to run
a `%.wasm` file compiled by Go. See https://wazero.io/languages/go/ for more.

## Usage

When `GOOS=js` and `GOARCH=wasm`, Go's compiler targets WebAssembly 1.0
Binary format (%.wasm).

Ex.
```bash
GOOS=js GOARCH=wasm go build -o cat.wasm .
```

After compiling `cat.wasm` with wazero.Runtime's `CompileModule`, Run it.

Under the scenes, the compiled Wasm calls host functions that support the
runtime.GOOS. This is similar to what is implemented in [wasm_exec.js][1].

## Experimental

Go defines js "EXPERIMENTAL... exempt from the Go compatibility promise."
Accordingly, wazero cannot guarantee this will work from release to release,
or that usage will be relatively free of bugs. Due to this and the
relatively high implementation overhead, most will choose TinyGo instead.

[1]: https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js
