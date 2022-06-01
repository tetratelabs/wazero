## Stateful import example

This example shows how WebAssembly modules can import their own stateful host
module, such as "env", in the same runtime.

```bash
$ go run counter.go
ns1 count=0
ns2 count=0
ns1 count=1
ns2 count=1
```

Specifically, each WebAssembly-defined module is instantiated alongside its own
Go-defined "env" module in a separate `wazero.Namespace`. This is more
efficient than separate runtimes as instantiation re-uses the same compilation
cache.
