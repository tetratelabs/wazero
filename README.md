# Gasm

A minimal implementation of Wasm Virtual machine purely written in Go.
The VM can be embedded in your Go program without any dependency like cgo, and enables Gophers to write Wasm host environments easily.

The implementation is quite straightforward and I hope this code would be a
 good starting point for novices to learn Wasm spec.

## Example

```golang
func Test_fibonacci(t *testing.T) {
	binary, _ := os.ReadFile("wasm/fibonacci.wasm")
	mod, _ := wasm.DecodeModule(binary)
	vm, _ := wasm.NewVM()
	wasi.NewEnvironment().RegisterToVirtualMachine(vm)
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
- https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md
