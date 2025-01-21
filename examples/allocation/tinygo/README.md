## TinyGo allocation example

This example shows how to pass strings in and out of a Wasm function defined
in TinyGo, built with 

```bash
(cd testdata; tinygo build -scheduler=none -target=wasip1 -buildmode=c-shared -o greet.wasm greet.go)
```

```bash
$ go run greet.go wazero
wasm >> Hello, wazero!
go >> Hello, wazero!
```

Under the covers, [greet.go](testdata/greet.go) does a few things of interest:
* Uses `unsafe.Pointer` to change a Go pointer to a numeric type.
* Uses `reflect.StringHeader` to build back a string from a pointer, len pair.
* Relies on CGO to allocate memory used to pass data from TinyGo to host.

See https://wazero.io/languages/tinygo/ for more tips.
