## gojs example

This shows how to use Wasm built by go using `GOOS=js GOARCH=wasm`. Notably,
this uses filesystem support.

```bash
$ go run cat.go /test.txt
greet filesystem
```

Internally, this uses [gojs](../README.md), which implements the custom host
functions required by Go.

Notes:
* `GOOS=js GOARCH=wasm` wazero be removed after Go 1.22 is released. Please
  switch to `GOOS=wasip1 GOARCH=wasm` released in Go 1.21.
* `GOWASM=satconv,signext` enables features in WebAssembly Core Specification
  2.0.
