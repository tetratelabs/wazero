## gojs example

This shows how to use Wasm built by go using `GOARCH=wasm GOOS=js`. Notably,
this shows an interesting feature this supports, HTTP client requests.

```bash
$ go run stars.go
wazero has 9999999 stars. Does that include you?
```

Internally, this uses [gojs](../../experimental/gojs/gojs.go), which implements
the custom host functions required by Go.

Note: `GOARCH=wasm GOOS=js` is experimental as is wazero's support of it. For
details, see https://wazero.io/languages/go/.
