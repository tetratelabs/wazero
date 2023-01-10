## Multiple runtimes

Sometimes, a Wasm module might want a stateful host module. In that case, we have to create
multiple `wazero.Runtime` if we want to run it multiple times.
This example shows how to use multiple Runtimes while sharing
the same compilation caches so that we could reduce the compilation time of Wasm modules.

In this example, we create two `wazero.Runtime` which shares the underlying cache, and
instantiate a Wasm module which requires the stateful "env" module on each runtime.

```bash
$ go run counter.go
m1 count=0
m2 count=0
m1 count=1
m2 count=1
```
