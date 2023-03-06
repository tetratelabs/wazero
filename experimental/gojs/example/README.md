## gojs example

This shows how to use Wasm built by go using `GOARCH=wasm GOOS=js`. Notably,
this shows an interesting feature this supports, HTTP client requests.

```bash
$ cd stars
$ GOARCH=wasm GOOS=js GOWASM=satconv,signext go build -o main.wasm .
$ cd ..
$ go run stars.go
wazero has 9999999 stars. Does that include you?
```

Internally, this uses [gojs](../gojs.go), which implements the custom host
functions required by Go.

Notes:
* `GOARCH=wasm GOOS=js` is experimental as is wazero's support of it. For
  details, see https://wazero.io/languages/go/.
* `GOWASM=satconv,signext` enables features in WebAssembly Core Specification
  2.0.
