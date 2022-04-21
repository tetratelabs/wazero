## TinyGo allocation example

This example shows how to pass strings in and out of a Wasm function defined
in TinyGo, built with `tinygo build -o greet.wasm -scheduler=none -target=wasi greet.go`

Ex.
```bash
$ go run greet.go wazero
wasm >> Hello, wazero!
go >> Hello, wazero!
```

Under the covers, [greet.go](testdata/greet.go) does a few things of interest:
* Uses `unsafe.Pointer` to change a Go pointer to a numeric type.
* Uses `reflect.StringHeader` to build back a string from a pointer, len pair.
* Relies on TinyGo not eagerly freeing pointers returned.

Go does not export allocation functions, but when TinyGo generates WebAssembly,
it exports "malloc" and "free", which we use for that purpose. These are not
documented, so not necessarily a best practice. See the following issues for
updates:
* WebAssembly exports for allocation: https://github.com/tinygo-org/tinygo/issues/2788
* Memory ownership of TinyGo allocated pointers: https://github.com/tinygo-org/tinygo/issues/2787

Note: While folks here are familiar with TinyGo, wazero isn't a TinyGo project.
We hope this gets you started. For next steps, consider reading the
[TinyGo Using WebAssembly Guide](https://tinygo.org/docs/guides/webassembly/)
or joining the [#TinyGo channel on the Gophers Slack](https://github.com/tinygo-org/tinygo#getting-help).
