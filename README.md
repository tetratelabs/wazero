# Gasm

A minimal implementation of Wasm Virtual Machine purely written in Go. The VM passes all the [Wasm Spec test suites](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core) and is fully compatible with the WebAssembly v1.0 Specification.

The VM can be embedded in your Go program without any dependency like cgo, and enables Gophers to write Wasm host environments easily.

## Example

```golang
func Test_fibonacci(t *testing.T) {
	binary, _ := os.ReadFile("wasm/fibonacci.wasm")
	mod, _ := wasm.DecodeModule(binary)
	vm, _ := wasm.NewVM()
	vm.InstantiateModule(mod, "test")

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, _, _ := vm.ExecExportedFunction("test", "fibonacci", uint64(c.in))
		require.Equal(t, c.exp, int32(ret[0]))
	}
}
```

## References

- https://webassembly.github.io/spec/core/index.html
